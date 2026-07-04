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

// Dispatcher turns incoming chat messages into executed input, enforcing
// permissions, cooldowns, and the blacklist along the way. It is the one
// place chat trust boundaries are decided.
type Dispatcher struct {
	cfg       *config.Config
	logger    *slog.Logger
	executor  *Executor
	blacklist *blacklist

	enabled atomic.Bool

	mu          sync.Mutex
	lastGlobal  time.Time
	lastPerUser map[string]time.Time
}

func NewDispatcher(cfg *config.Config, logger *slog.Logger, executor *Executor) *Dispatcher {
	d := &Dispatcher{
		cfg:         cfg,
		logger:      logger,
		executor:    executor,
		blacklist:   buildBlacklist(cfg),
		lastPerUser: make(map[string]time.Time),
	}
	d.enabled.Store(true)
	return d
}

// Handle processes one chat message. Safe for concurrent use.
func (d *Dispatcher) Handle(msg ChatMessage) {
	body, ok := TrimPrefix(msg.Text, d.cfg.Prefix)
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
	if d.cfg.ModOnlyMode && msg.Permission < Moderator {
		d.logger.Debug("dropped: mod-only mode active", "user", msg.Username)
		return
	}
	if reason := d.checkCooldown(msg.Username, time.Now()); reason != "" {
		d.logger.Debug("dropped: "+reason, "user", msg.Username)
		return
	}

	actions, err := ParseCombo(body, d.cfg)
	if err != nil {
		d.logger.Debug("dropped: invalid command", "user", msg.Username, "text", msg.Text, "error", err)
		return
	}
	if reason := d.blacklist.Check(actions); reason != "" {
		d.logger.Info("blocked command", "user", msg.Username, "text", msg.Text, "reason", reason)
		return
	}

	d.commitCooldown(msg.Username, time.Now())
	hold := EffectiveHoldMs(actions, d.cfg.TapHoldMs)
	d.executor.Submit(actions, hold)
	d.logger.Debug("dispatched", "user", msg.Username, "text", msg.Text)
}

func (d *Dispatcher) handleControl(msg ChatMessage, action string) {
	if msg.Permission < Moderator {
		d.logger.Debug("dropped: control command requires moderator", "user", msg.Username, "action", action)
		return
	}
	switch action {
	case ControlPause:
		d.enabled.Store(false)
		d.logger.Info("remote paused", "by", msg.Username)
	case ControlResume:
		d.enabled.Store(true)
		d.logger.Info("remote resumed", "by", msg.Username)
	}
}

// checkCooldown returns a non-empty reason if msg should be dropped.
func (d *Dispatcher) checkCooldown(username string, now time.Time) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	globalCooldown := time.Duration(d.cfg.GlobalCooldownMs) * time.Millisecond
	if now.Sub(d.lastGlobal) < globalCooldown {
		return "global cooldown"
	}
	perUserCooldown := time.Duration(d.cfg.PerUserCooldownMs) * time.Millisecond
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
