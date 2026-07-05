package webui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"streamer-remote/internal/config"
)

type rewardProfilesResponse struct {
	Profiles []config.RewardProfile `json:"profiles"`
	Active   string                 `json:"active"`
}

func (s *Server) handleListProfiles(w http.ResponseWriter, _ *http.Request) {
	cfg := s.dispatcher.Config()
	profiles := cfg.RewardProfiles
	if profiles == nil {
		profiles = []config.RewardProfile{}
	}
	writeJSON(w, rewardProfilesResponse{Profiles: profiles, Active: cfg.ActiveRewardProfile})
}

type saveProfileRequest struct {
	Name string `json:"name"`
}

// handleSaveProfile snapshots the currently live rewardActions under the
// given name, creating the profile or overwriting it if the name already
// exists. Since the snapshot is exactly what's live, saving also marks it
// the active profile.
func (s *Server) handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	var req saveProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("a profile name is required"))
		return
	}

	cfg := s.dispatcher.Config()
	rewards := append([]config.RewardAction{}, cfg.RewardActions...)
	profile := config.RewardProfile{Name: name, Rewards: rewards}

	if err := config.SaveRewardProfile(s.configPath, profile); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := config.SetActiveRewardProfile(s.configPath, name); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reloaded, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dispatcher.UpdateConfig(reloaded)
	s.logger.Info("reward profile saved", "profile", name)
	writeJSON(w, profile)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := config.DeleteRewardProfile(s.configPath, name); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	reloaded, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dispatcher.UpdateConfig(reloaded)
	s.logger.Info("reward profile deleted", "profile", name)
	w.WriteHeader(http.StatusNoContent)
}

// handleActivateProfile swaps the live rewards for the named profile's:
// it tears down every currently live reward on Twitch, then creates one
// per entry in the target profile, replacing rewardActions with the fresh
// set (new RewardIDs, since Twitch assigns a new one on every creation).
func (s *Server) handleActivateProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg := s.dispatcher.Config()

	var target *config.RewardProfile
	for i := range cfg.RewardProfiles {
		if cfg.RewardProfiles[i].Name == name {
			target = &cfg.RewardProfiles[i]
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("no reward profile named %q", name))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	helix, broadcasterID, err := s.helixClientForRewards(ctx, cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	for _, ra := range cfg.RewardActions {
		if err := helix.DeleteCustomReward(ctx, broadcasterID, ra.RewardID); err != nil {
			s.logger.Warn("could not delete reward on twitch, continuing profile switch", "error", err, "reward", ra.RewardTitle)
		}
	}
	if err := config.ClearRewardActions(s.configPath); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	for _, ra := range target.Rewards {
		rewardID, err := helix.CreateCustomReward(ctx, broadcasterID, ra.RewardTitle, ra.Cost)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Errorf("creating %q: %w", ra.RewardTitle, err))
			return
		}
		created := config.RewardAction{Action: ra.Action, RewardTitle: ra.RewardTitle, Cost: ra.Cost, RewardID: rewardID}
		if err := config.AddRewardAction(s.configPath, created); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if err := config.SetActiveRewardProfile(s.configPath, name); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	reloaded, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.dispatcher.UpdateConfig(reloaded)
	s.logger.Info("reward profile activated", "profile", name)
	writeJSON(w, rewardProfilesResponse{Profiles: reloaded.RewardProfiles, Active: reloaded.ActiveRewardProfile})
}
