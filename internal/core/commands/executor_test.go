package commands

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// recordingSink captures the order of calls made by the executor, so a
// test can assert a combo presses keys down then releases them in reverse
// without touching the real Windows input API.
type recordingSink struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingSink) record(s string) {
	r.mu.Lock()
	r.calls = append(r.calls, s)
	r.mu.Unlock()
}

func (r *recordingSink) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recordingSink) KeyDown(name string) error            { r.record("down:" + name); return nil }
func (r *recordingSink) KeyUp(name string) error              { r.record("up:" + name); return nil }
func (r *recordingSink) MouseDown(b string) error             { r.record("mdown:" + b); return nil }
func (r *recordingSink) MouseUp(b string) error               { r.record("mup:" + b); return nil }
func (r *recordingSink) MoveMouseRelative(dx, dy int32) error { r.record("move"); return nil }
func (r *recordingSink) ScrollMouse(delta int32) error        { r.record("scroll"); return nil }

func TestExecutorPressesComboDownThenReleasesInReverse(t *testing.T) {
	sink := &recordingSink{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ex := NewExecutor(logger, 4, sink)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ex.Run(ctx)

	// A "shift+w" combo: both go down, then both come up in reverse order.
	ex.Submit([]Step{{
		Actions: []Action{{Kind: KindKey, Name: "shift"}, {Kind: KindKey, Name: "w"}},
		HoldMs:  1,
	}})

	deadline := time.After(2 * time.Second)
	for {
		if len(sink.snapshot()) >= 4 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out; got %v", sink.snapshot())
		case <-time.After(5 * time.Millisecond):
		}
	}

	got := sink.snapshot()
	want := []string{"down:shift", "down:w", "up:w", "up:shift"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d: expected %q, got %q (full: %v)", i, want[i], got[i], got)
		}
	}
}
