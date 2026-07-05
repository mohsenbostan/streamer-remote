package webui

import (
	"fmt"
	"net/http"

	"streamer-remote/internal/config"
)

type kickSetupRequest struct {
	Channel string `json:"channel"`
}

// handleKickSetup saves the channel slug and connects immediately — Kick
// needs no OAuth login to read a channel's public chat, so unlike Twitch
// there's no device-code step in between.
func (s *Server) handleKickSetup(w http.ResponseWriter, r *http.Request) {
	var req kickSetupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Channel == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("channel is required"))
		return
	}

	if err := config.UpdateKickChannel(s.configPath, req.Channel); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reloaded, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dispatcher.UpdateConfig(reloaded)
	s.startKickLocked(reloaded)
	w.WriteHeader(http.StatusNoContent)
}

// handleKickConnect (re)starts the Kick session using the already-saved
// channel — used to reconnect after a disconnect without re-entering it.
func (s *Server) handleKickConnect(w http.ResponseWriter, _ *http.Request) {
	cfg := s.dispatcher.Config()
	if cfg.Kick.Channel == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no kick channel configured"))
		return
	}
	s.startKickLocked(cfg)
	w.WriteHeader(http.StatusNoContent)
}

// handleKickLogout tears down the running connection and clears the
// saved channel, so the Overview tab shows the first-time setup form
// again, for switching to a different Kick channel.
func (s *Server) handleKickLogout(w http.ResponseWriter, _ *http.Request) {
	s.kickMu.Lock()
	if s.kickSession != nil {
		s.kickSession.Stop()
		s.kickSession = nil
	}
	s.kickMu.Unlock()

	if err := config.UpdateKickChannel(s.configPath, ""); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reloaded, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dispatcher.UpdateConfig(reloaded)
	s.logger.Info("disconnected from kick", "by", "dashboard")
	w.WriteHeader(http.StatusNoContent)
}
