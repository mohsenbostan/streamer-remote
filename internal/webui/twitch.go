package webui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"streamer-remote/internal/app"
	"streamer-remote/internal/config"
)

// twitchAuthState tracks the in-progress device-code login so the
// dashboard can poll for the code/link to show the streamer, and later
// whether it succeeded.
type twitchAuthState struct {
	mu              sync.Mutex
	state           string // idle | pending | connected | error
	verificationURI string
	userCode        string
	err             string
}

type twitchAuthStateDTO struct {
	State           string `json:"state"`
	VerificationURI string `json:"verificationUri,omitempty"`
	UserCode        string `json:"userCode,omitempty"`
	Error           string `json:"error,omitempty"`
}

func (t *twitchAuthState) setPending(uri, code string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state, t.verificationURI, t.userCode, t.err = "pending", uri, code, ""
}

func (t *twitchAuthState) setConnected() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state, t.err = "connected", ""
}

func (t *twitchAuthState) setError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state, t.err = "error", err.Error()
}

func (t *twitchAuthState) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state, t.verificationURI, t.userCode, t.err = "", "", "", ""
}

func (t *twitchAuthState) snapshot() twitchAuthStateDTO {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == "" {
		return twitchAuthStateDTO{State: "idle"}
	}
	return twitchAuthStateDTO{
		State:           t.state,
		VerificationURI: t.verificationURI,
		UserCode:        t.userCode,
		Error:           t.err,
	}
}

type twitchSetupRequest struct {
	Channel  string `json:"channel"`
	ClientID string `json:"clientId"`
}

func (s *Server) handleTwitchSetup(w http.ResponseWriter, r *http.Request) {
	var req twitchSetupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Channel == "" || req.ClientID == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("channel and clientId are both required"))
		return
	}

	if err := config.UpdateTwitchFields(s.configPath, req.Channel, req.ClientID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reloaded, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dispatcher.UpdateConfig(reloaded)
	w.WriteHeader(http.StatusNoContent)
}

// handleTwitchConnect kicks off the OAuth device code flow in the
// background and returns immediately; the dashboard polls
// GET /api/twitch/auth for the code to show and for completion.
func (s *Server) handleTwitchConnect(w http.ResponseWriter, _ *http.Request) {
	cfg := s.dispatcher.Config()
	if err := cfg.Twitch.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	auth := s.newAuthenticator(cfg)
	auth.OnCode = func(uri, code string) { s.authState.setPending(uri, code) }

	go func() {
		ctx, cancel := context.WithTimeout(s.rootCtx, 10*time.Minute)
		defer cancel()
		if _, err := auth.EnsureToken(ctx); err != nil {
			s.authState.setError(err)
			s.logger.Error("twitch authorization failed", "error", err)
			return
		}
		s.authState.setConnected()
		s.startTwitchLocked(cfg, auth)
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleTwitchAuthState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.authState.snapshot())
}

// handleTwitchLogout tears down the running connection, forgets the
// cached OAuth token, and clears the saved channel/Client ID — a full
// reset so the Overview tab shows the first-time setup form again, for
// switching to a different Twitch account or channel.
func (s *Server) handleTwitchLogout(w http.ResponseWriter, _ *http.Request) {
	s.twitchMu.Lock()
	if s.twitchSession != nil {
		s.twitchSession.Stop()
		s.twitchSession = nil
	}
	s.twitchMu.Unlock()

	if err := os.Remove(app.TokenCachePath); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("could not remove cached twitch token", "error", err)
	}

	if err := config.UpdateTwitchFields(s.configPath, "", ""); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reloaded, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dispatcher.UpdateConfig(reloaded)
	s.authState.reset()
	s.logger.Info("disconnected from twitch", "by", "dashboard")
	w.WriteHeader(http.StatusNoContent)
}
