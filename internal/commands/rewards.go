package commands

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"streamer-remote/internal/config"
)

// RedemptionResult tells the caller (the Twitch redemption forwarder) what
// to do with a Channel Points redemption after HandleRedemption runs.
type RedemptionResult int

const (
	// RedemptionIgnored means the reward ID isn't one we manage; leave its
	// status alone.
	RedemptionIgnored RedemptionResult = iota
	// RedemptionFulfilled means the action ran; mark it FULFILLED on Twitch.
	RedemptionFulfilled
	// RedemptionRefunded means the action was blocked (paused or
	// blacklisted); mark it CANCELED on Twitch so the viewer gets their
	// points back.
	RedemptionRefunded
)

// gatedAction is a reward-only action, precomputed once from config so
// that neither the chat path nor the redemption path needs to re-parse or
// re-validate it on every message.
type gatedAction struct {
	rewardID    string
	rewardTitle string
	actions     []Action
	signature   string
}

// actionSignature is an order-independent identity for a parsed combo,
// used to recognize "this is the same action" whether it arrives as
// "alt+f4" or "f4+alt" typed in chat. Hold duration and move/scroll
// amount are deliberately excluded: gating is about which action, not how
// long/how far.
func actionSignature(actions []Action) string {
	parts := make([]string, len(actions))
	for i, a := range actions {
		parts[i] = fmt.Sprintf("%d:%s", a.Kind, a.Name)
	}
	sort.Strings(parts)
	return strings.Join(parts, "+")
}

// buildGatedActions parses each configured reward action once at startup.
// Entries that fail to parse (e.g. after a maxComboSize change) or have no
// RewardID yet (created on Twitch but config write raced, or hand-edited)
// are skipped with a warning rather than failing the whole app.
func buildGatedActions(cfg *config.Config, logger *slog.Logger) (bySignature, byRewardID map[string]gatedAction) {
	bySignature = make(map[string]gatedAction, len(cfg.RewardActions))
	byRewardID = make(map[string]gatedAction, len(cfg.RewardActions))

	for _, ra := range cfg.RewardActions {
		if ra.RewardID == "" {
			logger.Warn("skipping rewardActions entry with no rewardId", "action", ra.Action)
			continue
		}
		actions, err := ParseCombo(ra.Action, cfg)
		if err != nil {
			logger.Warn("skipping invalid rewardActions entry", "action", ra.Action, "error", err)
			continue
		}
		ga := gatedAction{
			rewardID:    ra.RewardID,
			rewardTitle: ra.RewardTitle,
			actions:     actions,
			signature:   actionSignature(actions),
		}
		bySignature[ga.signature] = ga
		byRewardID[ga.rewardID] = ga
	}
	return bySignature, byRewardID
}

// HandleRedemption executes the action bound to a Channel Points
// redemption. Unlike Handle, there's no chat permission to check here —
// redeeming the reward at all is the authorization — but the blacklist
// and paused state still apply, since those are hard safety floors rather
// than trust levels.
func (d *Dispatcher) HandleRedemption(rewardID, redeemerUsername string) RedemptionResult {
	snap := d.snap.Load()
	ga, ok := snap.gatedByRewardID[rewardID]
	if !ok {
		return RedemptionIgnored
	}

	if !d.enabled.Load() {
		d.logger.Info("refunding redemption: remote is paused", "user", redeemerUsername, "reward", ga.rewardTitle)
		return RedemptionRefunded
	}
	if reason := snap.blacklist.Check(ga.actions); reason != "" {
		d.logger.Warn("refunding redemption: action is blacklisted", "user", redeemerUsername, "reward", ga.rewardTitle, "reason", reason)
		return RedemptionRefunded
	}

	hold := EffectiveHoldMs(ga.actions, snap.cfg.TapHoldMs)
	d.executor.Submit(ga.actions, hold)
	d.logger.Info("redeemed", "user", redeemerUsername, "reward", ga.rewardTitle)
	return RedemptionFulfilled
}
