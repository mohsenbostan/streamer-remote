package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"streamer-remote/internal/commands"
	"streamer-remote/internal/config"
	"streamer-remote/internal/supervisor"
	"streamer-remote/internal/twitchauth"
)

// NewLogHandler returns a slog.Handler that feeds hub, for wiring into
// the process logger before a Server even exists (main constructs the
// logger, then the dispatcher, then the Server, in that order — all three
// need to share one Hub).
func NewLogHandler(hub *Hub) slog.Handler {
	return newSSEHandler(hub)
}

// preferredPort is tried first so the URL is stable across restarts;
// falling back to an OS-assigned port keeps the app working even if
// something else is already listening on it.
const preferredPort = 47829

type Server struct {
	configPath string
	logger     *slog.Logger
	version    string
	dispatcher *commands.Dispatcher
	localOnly  bool
	hub        *Hub
	rootCtx    context.Context

	twitchMu      sync.Mutex
	twitchSession *supervisor.TwitchSession
	authState     twitchAuthState
}

// New builds the dashboard server. rootCtx bounds the lifetime of anything
// the dashboard starts on demand (namely a Twitch connection). hub must be
// the same one passed to NewLogHandler when the logger was built.
func New(rootCtx context.Context, configPath string, dispatcher *commands.Dispatcher, logger *slog.Logger, version string, localOnly bool, hub *Hub) *Server {
	return &Server{
		configPath: configPath,
		logger:     logger,
		version:    version,
		dispatcher: dispatcher,
		localOnly:  localOnly,
		hub:        hub,
		rootCtx:    rootCtx,
	}
}

// StartExistingTwitchSession begins the Twitch connection at boot, if the
// config already has valid credentials and a cached token. Errors are
// logged, not returned: a broken Twitch connection shouldn't stop the
// dashboard itself from starting, since the dashboard is how the streamer
// fixes it.
func (s *Server) StartExistingTwitchSession() {
	cfg := s.dispatcher.Config()
	if err := cfg.Twitch.Validate(); err != nil {
		return
	}
	auth := s.newAuthenticator(cfg)
	// Only proceed if a token is already cached and covers every scope
	// this version needs. Otherwise EnsureToken would silently fall back
	// to a device-code login with no OnCode set — the code/link would
	// only ever reach the log file, never the dashboard — leaving the
	// streamer stuck on "not connected" with no visible way to fix it.
	// The Overview tab's "Connect" button (which does set OnCode) is the
	// right path for that case.
	tok, err := twitchauth.LoadToken(supervisor.TokenCachePath)
	if err != nil || !tok.HasScopes(supervisor.TwitchAuthScopes) {
		return
	}
	s.startTwitchLocked(cfg, auth)
}

func (s *Server) newAuthenticator(cfg *config.Config) *twitchauth.Authenticator {
	return twitchauth.New(cfg.Twitch.ClientID, supervisor.TokenCachePath, supervisor.TwitchAuthScopes, s.logger)
}

func (s *Server) startTwitchLocked(cfg *config.Config, auth *twitchauth.Authenticator) {
	s.twitchMu.Lock()
	defer s.twitchMu.Unlock()
	if s.twitchSession != nil {
		s.twitchSession.Stop()
	}
	s.twitchSession = supervisor.StartTwitch(s.rootCtx, cfg, s.logger, s.dispatcher, auth)
}

func (s *Server) twitchConnected() bool {
	s.twitchMu.Lock()
	defer s.twitchMu.Unlock()
	return s.twitchSession != nil && s.twitchSession.Client.Connected()
}

// Bind claims a local port and returns the dashboard's URL. Separate from
// Serve so callers (main) can know the URL immediately — e.g. to hand to
// the system tray's "Open Dashboard" item — without waiting on the
// (long-running) Serve call.
func (s *Server) Bind() (net.Listener, string, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferredPort))
	if err != nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, "", fmt.Errorf("webui: bind local port: %w", err)
		}
	}
	url := fmt.Sprintf("http://%s", listener.Addr())
	return listener, url, nil
}

// Serve runs the dashboard on listener until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	httpServer := &http.Server{Handler: s.routes()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handlePutSettings)
	mux.HandleFunc("POST /api/pause", s.handlePause)
	mux.HandleFunc("POST /api/resume", s.handleResume)
	mux.HandleFunc("POST /api/test", s.handleTest)
	mux.HandleFunc("GET /api/events", s.handleEvents)

	mux.HandleFunc("POST /api/twitch/setup", s.handleTwitchSetup)
	mux.HandleFunc("POST /api/twitch/connect", s.handleTwitchConnect)
	mux.HandleFunc("GET /api/twitch/auth", s.handleTwitchAuthState)

	mux.HandleFunc("GET /api/rewards", s.handleListRewards)
	mux.HandleFunc("POST /api/rewards", s.handleAddReward)
	mux.HandleFunc("DELETE /api/rewards/{id}", s.handleRemoveReward)

	mux.HandleFunc("GET /api/update", s.handleCheckUpdate)
	mux.HandleFunc("POST /api/update/apply", s.handleApplyUpdate)

	mux.Handle("/", http.FileServerFS(staticFS()))

	return withRecover(s.logger, mux)
}

// withRecover ensures a panic in a handler becomes a 500 response instead
// of taking the whole process down — this server runs for as long as the
// app does, so one bad request must not be fatal.
func withRecover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("recovered panic handling request", "path", r.URL.Path, "panic", rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	http.Error(w, err.Error(), status)
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

type StatusResponse struct {
	Version          string `json:"version"`
	LocalOnly        bool   `json:"localOnly"`
	TwitchConfigured bool   `json:"twitchConfigured"`
	TwitchConnected  bool   `json:"twitchConnected"`
	Paused           bool   `json:"paused"`
	Channel          string `json:"channel"`
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	cfg := s.dispatcher.Config()
	writeJSON(w, StatusResponse{
		Version:          s.version,
		LocalOnly:        s.localOnly,
		TwitchConfigured: cfg.Twitch.Validate() == nil,
		TwitchConnected:  s.twitchConnected(),
		Paused:           !s.dispatcher.Enabled(),
		Channel:          cfg.Twitch.Channel,
	})
}

func (s *Server) handlePause(w http.ResponseWriter, _ *http.Request) {
	s.dispatcher.Pause("dashboard")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResume(w http.ResponseWriter, _ *http.Request) {
	s.dispatcher.Resume("dashboard")
	w.WriteHeader(http.StatusNoContent)
}

type testRequest struct {
	Permission string `json:"permission"`
	Text       string `json:"text"`
}

func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	var req testRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	perm, ok := commands.ParsePermission(req.Permission)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown permission %q", req.Permission))
		return
	}
	s.dispatcher.Handle(commands.ChatMessage{Username: "dashboard", Permission: perm, Text: req.Text})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}

	ch, unsubscribe := s.hub.subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
