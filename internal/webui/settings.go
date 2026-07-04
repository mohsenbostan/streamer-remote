package webui

import (
	"net/http"

	"streamer-remote/internal/config"
)

// Settings is the subset of config.Config the dashboard's Settings tab
// edits directly (Twitch fields and reward actions have their own tabs
// and endpoints, since they involve external Twitch API calls).
type Settings struct {
	Prefix            string           `json:"prefix"`
	ModOnlyMode       bool             `json:"modOnlyMode"`
	GlobalCooldownMs  int              `json:"globalCooldownMs"`
	PerUserCooldownMs int              `json:"perUserCooldownMs"`
	MaxComboSize      int              `json:"maxComboSize"`
	TapHoldMs         int              `json:"tapHoldMs"`
	MaxHoldMs         int              `json:"maxHoldMs"`
	MaxMoveStep       int              `json:"maxMoveStep"`
	LogDebug          bool             `json:"logDebug"`
	Blacklist         config.Blacklist `json:"blacklist"`
}

func settingsFromConfig(cfg *config.Config) Settings {
	return Settings{
		Prefix:            cfg.Prefix,
		ModOnlyMode:       cfg.ModOnlyMode,
		GlobalCooldownMs:  cfg.GlobalCooldownMs,
		PerUserCooldownMs: cfg.PerUserCooldownMs,
		MaxComboSize:      cfg.MaxComboSize,
		TapHoldMs:         cfg.TapHoldMs,
		MaxHoldMs:         cfg.MaxHoldMs,
		MaxMoveStep:       cfg.MaxMoveStep,
		LogDebug:          cfg.LogDebug,
		Blacklist:         cfg.Blacklist,
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, settingsFromConfig(s.dispatcher.Config()))
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	current := s.dispatcher.Config()
	next := *current // shallow copy: Twitch/RewardActions fields carry over unchanged
	next.Prefix = in.Prefix
	next.ModOnlyMode = in.ModOnlyMode
	next.GlobalCooldownMs = in.GlobalCooldownMs
	next.PerUserCooldownMs = in.PerUserCooldownMs
	next.MaxComboSize = in.MaxComboSize
	next.TapHoldMs = in.TapHoldMs
	next.MaxHoldMs = in.MaxHoldMs
	next.MaxMoveStep = in.MaxMoveStep
	next.LogDebug = in.LogDebug
	next.Blacklist = in.Blacklist

	if err := next.Save(s.configPath); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.dispatcher.UpdateConfig(&next)
	s.logger.Info("settings updated", "by", "dashboard")
	w.WriteHeader(http.StatusNoContent)
}
