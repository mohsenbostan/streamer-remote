package commands

import (
	"context"
	"log/slog"
	"time"

	"streamer-remote/internal/input"
)

// job is a sequence of steps queued for execution.
type job struct {
	steps []Step
}

// Executor runs sequences one at a time on a dedicated goroutine.
// Serializing is what keeps latency predictable: concurrent SendInput
// calls would race on key ordering, and a single worker with a small
// bounded queue means a burst of chat commands degrades by dropping the
// oldest overflow rather than piling up growing delay.
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

// Submit enqueues a sequence for execution. It never blocks: if the queue
// is full, the sequence is dropped and logged rather than adding latency
// to everything behind it.
func (e *Executor) Submit(steps []Step) {
	select {
	case e.jobs <- job{steps: steps}:
	default:
		e.logger.Warn("executor queue full, dropping sequence")
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
			e.logger.Error("recovered panic executing sequence", "panic", r)
		}
	}()

	for _, step := range j.steps {
		e.runStep(step)
	}
}

// runStep presses every action in the step down together, holds for
// HoldMs, then releases in reverse order. A step with no actions is a
// pure delay: it just sleeps.
func (e *Executor) runStep(step Step) {
	for _, a := range step.Actions {
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
			dx, dy := moveDelta(a)
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

	time.Sleep(time.Duration(step.HoldMs) * time.Millisecond)

	for i := len(step.Actions) - 1; i >= 0; i-- {
		a := step.Actions[i]
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

// moveDelta resolves a KindMove action to a pixel offset: either an
// explicit dx,dy (Name == "xy") or a named direction + magnitude.
func moveDelta(a Action) (dx, dy int32) {
	if a.Name == "xy" {
		return a.Amount, a.Amount2
	}
	switch a.Name {
	case "up":
		return 0, -a.Amount
	case "down":
		return 0, a.Amount
	case "left":
		return -a.Amount, 0
	case "right":
		return a.Amount, 0
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
