package webui

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// LiveEvent is one entry in the dashboard's live monitor feed. It mirrors
// a slog record; the dashboard shows raw log output rather than a
// separate hand-maintained event vocabulary, so every log call anywhere
// in the app automatically shows up live with zero extra plumbing.
type LiveEvent struct {
	Time  time.Time         `json:"time"`
	Level string            `json:"level"`
	Msg   string            `json:"msg"`
	Attrs map[string]string `json:"attrs"`
}

// Hub fans published events out to every connected SSE client.
type Hub struct {
	mu   sync.Mutex
	subs map[chan LiveEvent]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[chan LiveEvent]struct{})}
}

func (h *Hub) subscribe() (<-chan LiveEvent, func()) {
	ch := make(chan LiveEvent, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe
}

func (h *Hub) publish(e LiveEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
			// Slow/gone client: drop rather than block log calls elsewhere
			// in the app on a stuck browser tab.
		}
	}
}

// sseHandler is a slog.Handler that publishes every record to an
// Hub, so the live monitor is just "tail the logs" over SSE rather
// than a second event system threaded through the whole app.
type sseHandler struct {
	hub   *Hub
	attrs []slog.Attr
}

func newSSEHandler(hub *Hub) *sseHandler {
	return &sseHandler{hub: hub}
}

func (h *sseHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *sseHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]string, r.NumAttrs()+len(h.attrs))
	for _, a := range h.attrs {
		attrs[a.Key] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	h.hub.publish(LiveEvent{Time: r.Time, Level: r.Level.String(), Msg: r.Message, Attrs: attrs})
	return nil
}

func (h *sseHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &sseHandler{hub: h.hub, attrs: merged}
}

func (h *sseHandler) WithGroup(_ string) slog.Handler {
	return h // groups aren't used anywhere in this app
}
