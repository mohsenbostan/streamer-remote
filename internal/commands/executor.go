package commands

import (
	"context"
	"log/slog"
	"time"

	"streamer-remote/internal/input"
)

// job is a combo queued for execution.
type job struct {
	actions []Action
	holdMs  int
}

// Executor runs combos one at a time on a dedicated goroutine. Serializing
// is what keeps latency predictable: concurrent SendInput calls would race
// on key ordering, and a single worker with a small bounded queue means a
// burst of chat commands degrades by dropping the oldest overflow rather
// than piling up growing delay.
type Executor struct {
	jobs   chan job
	logger *slog.Logger
}

func NewExecutor(logger *slog.Logger, queueSize int) *Executor {
	return &Executor{
		jobs:   make(chan job, queueSize),
		logger: logger,
	}
}

// Submit enqueues a combo for execution. It never blocks: if the queue is
// full, the combo is dropped and logged rather than adding latency to
// everything behind it.
func (e *Executor) Submit(actions []Action, holdMs int) {
	select {
	case e.jobs <- job{actions: actions, holdMs: holdMs}:
	default:
		e.logger.Warn("executor queue full, dropping combo")
	}
}

// QueueLen reports how many jobs are currently queued. Mainly useful for
// tests and diagnostics.
func (e *Executor) QueueLen() int { return len(e.jobs) }

// Run processes queued jobs until ctx is cancelled. Call it in its own
// goroutine.
func (e *Executor) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-e.jobs:
			e.runJob(j)
		}
	}
}

func (e *Executor) runJob(j job) {
	defer func() {
		if r := recover(); r != nil {
			e.logger.Error("recovered panic executing combo", "panic", r)
		}
	}()

	hold := time.Duration(j.holdMs) * time.Millisecond

	for _, a := range j.actions {
		switch a.Kind {
		case KindKey:
			if err := input.KeyDown(a.Name); err != nil {
				e.logger.Error("key down failed", "key", a.Name, "error", err)
			}
		case KindClick:
			if err := input.MouseDown(a.Name); err != nil {
				e.logger.Error("mouse down failed", "button", a.Name, "error", err)
			}
		case KindMove:
			dx, dy := directionDelta(a.Name, a.Amount)
			if err := input.MoveMouseRelative(dx, dy); err != nil {
				e.logger.Error("mouse move failed", "direction", a.Name, "error", err)
			}
		case KindScroll:
			delta := a.Amount * 120
			if a.Name == "down" {
				delta = -delta
			}
			if err := input.ScrollMouse(delta); err != nil {
				e.logger.Error("scroll failed", "direction", a.Name, "error", err)
			}
		}
	}

	time.Sleep(hold)

	for i := len(j.actions) - 1; i >= 0; i-- {
		a := j.actions[i]
		switch a.Kind {
		case KindKey:
			if err := input.KeyUp(a.Name); err != nil {
				e.logger.Error("key up failed", "key", a.Name, "error", err)
			}
		case KindClick:
			if err := input.MouseUp(a.Name); err != nil {
				e.logger.Error("mouse up failed", "button", a.Name, "error", err)
			}
		}
	}
}

func directionDelta(direction string, amount int32) (dx, dy int32) {
	switch direction {
	case "up":
		return 0, -amount
	case "down":
		return 0, amount
	case "left":
		return -amount, 0
	case "right":
		return amount, 0
	default:
		return 0, 0
	}
}

// EffectiveHoldMs picks the hold duration for a combo: the longest explicit
// hold: override present, or the configured default tap duration.
func EffectiveHoldMs(actions []Action, defaultMs int) int {
	hold := defaultMs
	for _, a := range actions {
		if a.HoldMs > hold {
			hold = a.HoldMs
		}
	}
	return hold
}
