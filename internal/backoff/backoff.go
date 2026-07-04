// Package backoff provides a small exponential-backoff-with-jitter helper
// used anywhere a subsystem (Twitch connection, token refresh) needs to
// retry after a failure without hammering the remote end.
package backoff

import (
	"math/rand"
	"time"
)

type Backoff struct {
	min, max time.Duration
	attempt  int
}

func New(min, max time.Duration) *Backoff {
	return &Backoff{min: min, max: max}
}

// Next returns the delay to wait before the next attempt and advances
// internal state. Call Reset after a successful attempt.
func (b *Backoff) Next() time.Duration {
	d := b.min * (1 << uint(min(b.attempt, 10)))
	if d > b.max || d <= 0 {
		d = b.max
	}
	b.attempt++
	jitter := time.Duration(rand.Int63n(int64(d) / 2))
	return d/2 + jitter
}

func (b *Backoff) Reset() {
	b.attempt = 0
}
