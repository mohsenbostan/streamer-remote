package webui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"streamer-remote/internal/commands"
	"streamer-remote/internal/config"
	"streamer-remote/internal/twitch"
)

func (s *Server) handleListRewards(w http.ResponseWriter, _ *http.Request) {
	cfg := s.dispatcher.Config()
	rewards := cfg.RewardActions
	if rewards == nil {
		rewards = []config.RewardAction{}
	}
	writeJSON(w, rewards)
}

// helixClientForRewards builds a Helix client authorized for managing
// Channel Points rewards. It only uses an already-cached token — it will
// not kick off an interactive login, since there's no way to show a
// device code from a plain JSON endpoint. The streamer connects Twitch
// (Overview tab) before this is reachable in practice.
func (s *Server) helixClientForRewards(ctx context.Context, cfg *config.Config) (*twitch.HelixClient, string, error) {
	if err := cfg.Twitch.Validate(); err != nil {
		return nil, "", err
	}
	auth := s.newAuthenticator(cfg)
	tok, err := auth.EnsureToken(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("connect Twitch first (Overview tab): %w", err)
	}
	helix := twitch.NewHelixClient(cfg.Twitch.ClientID, tok.AccessToken)
	broadcasterID, err := helix.GetOwnUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	return helix, broadcasterID, nil
}

type addRewardRequest struct {
	Action      string `json:"action"`
	RewardTitle string `json:"rewardTitle"`
	Cost        int    `json:"cost"`
}

func (s *Server) handleAddReward(w http.ResponseWriter, r *http.Request) {
	var req addRewardRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Action == "" || req.RewardTitle == "" || req.Cost <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("action, rewardTitle, and a positive cost are required"))
		return
	}
	cfg := s.dispatcher.Config()
	if _, err := commands.ParseCombo(strings.ToLower(req.Action), cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	helix, broadcasterID, err := s.helixClientForRewards(ctx, cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rewardID, err := helix.CreateCustomReward(ctx, broadcasterID, req.RewardTitle, req.Cost)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	ra := config.RewardAction{Action: req.Action, RewardTitle: req.RewardTitle, Cost: req.Cost, RewardID: rewardID}
	if err := config.AddRewardAction(s.configPath, ra); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reloaded, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dispatcher.UpdateConfig(reloaded)
	s.logger.Info("reward created", "action", req.Action, "reward", req.RewardTitle)
	writeJSON(w, ra)
}

func (s *Server) handleRemoveReward(w http.ResponseWriter, r *http.Request) {
	rewardID := r.PathValue("id")
	cfg := s.dispatcher.Config()

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if helix, broadcasterID, err := s.helixClientForRewards(ctx, cfg); err == nil {
		if err := helix.DeleteCustomReward(ctx, broadcasterID, rewardID); err != nil {
			s.logger.Warn("could not delete reward on twitch, removing from config anyway", "error", err)
		}
	}

	if err := config.RemoveRewardAction(s.configPath, rewardID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reloaded, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dispatcher.UpdateConfig(reloaded)
	s.logger.Info("reward removed", "rewardId", rewardID)
	w.WriteHeader(http.StatusNoContent)
}
