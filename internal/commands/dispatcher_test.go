package commands

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"streamer-remote/internal/config"
)

func testDispatcher(cfg *config.Config) (*Dispatcher, *Executor) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	executor := NewExecutor(logger, 10)
	return NewDispatcher(cfg, logger, executor), executor
}

func TestDispatcherIgnoresMessagesWithoutPrefix(t *testing.T) {
	d, ex := testDispatcher(testConfig())
	d.Handle(ChatMessage{Username: "viewer1", Permission: Everyone, Text: "hello chat"})
	if ex.QueueLen() != 0 {
		t.Fatal("expected no job queued for a message without the prefix")
	}
}

func TestDispatcherAcceptsValidCommand(t *testing.T) {
	d, ex := testDispatcher(testConfig())
	d.Handle(ChatMessage{Username: "viewer1", Permission: Everyone, Text: "rc!w"})
	if ex.QueueLen() != 1 {
		t.Fatalf("expected 1 job queued, got %d", ex.QueueLen())
	}
}

func TestDispatcherModOnlyModeBlocksViewers(t *testing.T) {
	cfg := testConfig()
	cfg.ModOnlyMode = true
	d, ex := testDispatcher(cfg)

	d.Handle(ChatMessage{Username: "viewer1", Permission: Everyone, Text: "rc!w"})
	if ex.QueueLen() != 0 {
		t.Fatal("expected viewer command to be blocked in mod-only mode")
	}

	d.Handle(ChatMessage{Username: "mod1", Permission: Moderator, Text: "rc!w"})
	if ex.QueueLen() != 1 {
		t.Fatal("expected moderator command to pass in mod-only mode")
	}
}

func TestDispatcherCooldowns(t *testing.T) {
	cfg := testConfig()
	cfg.GlobalCooldownMs = 0
	cfg.PerUserCooldownMs = 10_000
	d, ex := testDispatcher(cfg)

	d.Handle(ChatMessage{Username: "viewer1", Permission: Everyone, Text: "rc!w"})
	d.Handle(ChatMessage{Username: "viewer1", Permission: Everyone, Text: "rc!a"})
	if ex.QueueLen() != 1 {
		t.Fatalf("expected second command from same user to be cooldown-blocked, queue=%d", ex.QueueLen())
	}

	d.Handle(ChatMessage{Username: "viewer2", Permission: Everyone, Text: "rc!a"})
	if ex.QueueLen() != 2 {
		t.Fatal("expected a different user to bypass the per-user cooldown")
	}
}

func TestDispatcherPauseResumeRequiresModerator(t *testing.T) {
	d, ex := testDispatcher(testConfig())

	d.Handle(ChatMessage{Username: "viewer1", Permission: Everyone, Text: "rc!pause"})
	d.Handle(ChatMessage{Username: "viewer1", Permission: Everyone, Text: "rc!w"})
	if ex.QueueLen() != 1 {
		t.Fatal("expected a viewer's pause attempt to be ignored, remote should still be active")
	}

	d.Handle(ChatMessage{Username: "mod1", Permission: Moderator, Text: "rc!pause"})
	d.Handle(ChatMessage{Username: "mod1", Permission: Moderator, Text: "rc!w"})
	if ex.QueueLen() != 1 {
		t.Fatal("expected commands to be dropped while paused")
	}

	d.Handle(ChatMessage{Username: "mod1", Permission: Moderator, Text: "rc!resume"})
	d.Handle(ChatMessage{Username: "mod1", Permission: Moderator, Text: "rc!w"})
	if ex.QueueLen() != 2 {
		t.Fatal("expected commands to resume after a moderator resumes")
	}
}

func TestDispatcherBlacklistBlocksCombo(t *testing.T) {
	cfg := testConfig()
	cfg.Blacklist.DeniedCombos = [][]string{{"alt", "f4"}}
	d, ex := testDispatcher(cfg)

	d.Handle(ChatMessage{Username: "viewer1", Permission: Broadcaster, Text: "rc!alt+f4"})
	if ex.QueueLen() != 0 {
		t.Fatal("expected denylisted combo to be blocked even for the broadcaster")
	}
}

func TestDispatcherCooldownCleanupRemovesStaleUsers(t *testing.T) {
	cfg := testConfig()
	d, _ := testDispatcher(cfg)
	d.commitCooldown("stale-user", time.Now().Add(-1*time.Hour))

	d.mu.Lock()
	count := len(d.lastPerUser)
	d.mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 tracked user before cleanup, got %d", count)
	}

	cutoff := time.Now().Add(-10 * time.Minute)
	d.mu.Lock()
	for user, last := range d.lastPerUser {
		if last.Before(cutoff) {
			delete(d.lastPerUser, user)
		}
	}
	count = len(d.lastPerUser)
	d.mu.Unlock()
	if count != 0 {
		t.Fatal("expected stale user to be pruned")
	}
}
