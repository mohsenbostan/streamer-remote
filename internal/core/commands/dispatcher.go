package commands

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"streamer-remote/internal/config"
)

// snapshot bundles a config with the state derived from it (blacklist,
// reward gating). Bundling them means a config update is one atomic swap
// instead of several fields that could otherwise be read half-updated by
// a concurrent Handle call — this matters now that the dashboard can
// change settings while the dispatcher is actively processing events.
type snapshot struct {
	cfg              *config.Config
	blacklist        *blacklist
	gatedBySignature map[string]gatedAction
	gatedByRewardID  map[string]gatedAction
}

func newSnapshot(cfg *config.Config, logger *slog.Logger) *snapshot {
	bySignature, byRewardID := buildGatedActions(cfg, logger)
	return &snapshot{
		cfg:              cfg,
		blacklist:        buildBlacklist(cfg),
		gatedBySignature: bySignature,
		gatedByRewardID:  byRewardID,
	}
}

// Dispatcher turns incoming chat messages into executed input, enforcing
// permissions, cooldowns, and the blacklist along the way. It is the one
// place chat trust boundaries are decided.
//
// Moderators and the broadcaster are exempt from cooldowns, the
// blacklist, and reward-only gating when they type a command themselves —
// a human mod present in chat is trusted. That exemption does not extend
// to Channel Points redemptions (see HandleRedemption): anyone can redeem
// a reward, so the blacklist still applies there.
type Dispatcher struct {
	snap     atomic.Pointer[snapshot]
	logger   *slog.Logger
	executor *Executor

	enabled atomic.Bool

	mu          sync.Mutex
	lastGlobal  time.Time
	lastPerUser map[string]time.Time
}

func NewDispatcher(cfg *config.Config, logger *slog.Logger, executor *Executor) *Dispatcher {
	d := &Dispatcher{
		logger:      logger,
		executor:    executor,
		lastPerUser: make(map[string]time.Time),
	}
	d.snap.Store(newSnapshot(cfg, logger))
	d.enabled.Store(true)
	return d
}

// UpdateConfig atomically swaps in a new config and rebuilds the
// blacklist/reward-gating state derived from it. Safe to call while the
// dispatcher is actively handling messages — used by the dashboard's
// Settings tab to apply changes without restarting the app.
func (d *Dispatcher) UpdateConfig(cfg *config.Config) {
	d.snap.Store(newSnapshot(cfg, d.logger))
}

// Config returns the currently active config, e.g. for read-only display.
func (d *Dispatcher) Config() *config.Config {
	return d.snap.Load().cfg
}

// Handle processes one chat message. Safe for concurrent use.
func (d *Dispatcher) Handle(msg ChatMessage) {
	snap := d.snap.Load()
	cfg := snap.cfg

	body, ok := TrimPrefix(msg.Text, cfg.Prefix)
	if !ok || body == "" {
		return
	}

	switch strings.ToLower(body) {
	case ControlPause:
		d.handleControl(msg, ControlPause)
		return
	case ControlResume:
		d.handleControl(msg, ControlResume)
		return
	}

	if !d.enabled.Load() {
		d.logger.Debug("dropped: remote is paused", "user", msg.Username)
		return
	}

	isMod := msg.Permission >= Moderator

	if cfg.ModOnlyMode && !isMod {
		d.logger.Debug("dropped: mod-only mode active", "user", msg.Username)
		return
	}
	if !isMod {
		if reason := d.checkCooldown(cfg, msg.Username, time.Now()); reason != "" {
			d.logger.Debug("dropped: "+reason, "user", msg.Username)
			return
		}
	}

	steps, err := ParseSequence(body, cfg)
	if err != nil {
		d.logger.Debug("dropped: invalid command", "user", msg.Username, "text", msg.Text, "error", err)
		return
	}

	if !isMod {
		if ga, gated := snap.gatedBySignature[sequenceSignature(steps)]; gated {
			d.logger.Info("blocked: reward-only action", "user", msg.Username, "reward", ga.rewardTitle)
			return
		}
		if reason := snap.blacklist.Check(flattenActions(steps)); reason != "" {
			d.logger.Info("blocked command", "user", msg.Username, "text", msg.Text, "reason", reason)
			return
		}
		d.commitCooldown(msg.Username, time.Now())
	}

	d.executor.Submit(steps)
	d.logger.Debug("dispatched", "user", msg.Username, "text", msg.Text)
}

func (d *Dispatcher) handleControl(msg ChatMessage, action string) {
	if msg.Permission < Moderator {
		d.logger.Debug("dropped: control command requires moderator", "user", msg.Username, "action", action)
		return
	}
	switch action {
	case ControlPause:
		d.Pause(msg.Username)
	case ControlResume:
		d.Resume(msg.Username)
	}
}

// Pause and Resume are the single source of truth for the remote's
// enabled state, used by both the "!pause"/"!resume" chat commands and
// the dashboard's pause switch. by is a free-text label for the log (a
// chat username, or e.g. "dashboard").
func (d *Dispatcher) Pause(by string) {
	d.enabled.Store(false)
	d.logger.Info("remote paused", "by", by)
}

func (d *Dispatcher) Resume(by string) {
	d.enabled.Store(true)
	d.logger.Info("remote resumed", "by", by)
}

// Enabled reports whether the remote is currently active (not paused).
func (d *Dispatcher) Enabled() bool {
	return d.enabled.Load()
}

// checkCooldown returns a non-empty reason if msg should be dropped.
func (d *Dispatcher) checkCooldown(cfg *config.Config, username string, now time.Time) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	globalCooldown := time.Duration(cfg.GlobalCooldownMs) * time.Millisecond
	if now.Sub(d.lastGlobal) < globalCooldown {
		return "global cooldown"
	}
	perUserCooldown := time.Duration(cfg.PerUserCooldownMs) * time.Millisecond
	if last, ok := d.lastPerUser[username]; ok && now.Sub(last) < perUserCooldown {
		return "per-user cooldown"
	}
	return ""
}

func (d *Dispatcher) commitCooldown(username string, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastGlobal = now
	d.lastPerUser[username] = now
}

// RunCooldownCleanup periodically forgets stale per-user cooldown entries
// so long-running streams with thousands of unique chatters don't leak
// memory. Call it in its own goroutine.
func (d *Dispatcher) RunCooldownCleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-10 * time.Minute)
			d.mu.Lock()
			for user, last := range d.lastPerUser {
				if last.Before(cutoff) {
					delete(d.lastPerUser, user)
				}
			}
			d.mu.Unlock()
		}
	}
}
