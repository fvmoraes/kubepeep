package logging

import (
	"sync"
	"time"
)

// Sampler rate-limits repeated log events (O-03). The first burst events are
// allowed immediately; after that, at most one event passes per window. It is
// safe for concurrent use and never blocks: Allow only answers yes/no.
type Sampler struct {
	mu     sync.Mutex
	burst  int
	window time.Duration
	count  int
	last   time.Time
	now    func() time.Time
}

// NewSampler returns a sampler that lets the first burst events through and
// then at most one event per window. burst must be >= 1; window must be > 0.
func NewSampler(burst int, window time.Duration) *Sampler {
	if burst < 1 {
		burst = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &Sampler{burst: burst, window: window, now: time.Now}
}

// Allow reports whether the event should be logged now. The first burst
// events pass immediately; afterwards at most one event passes per window
// regardless of how much time elapses between probes.
func (sampler *Sampler) Allow() bool {
	sampler.mu.Lock()
	defer sampler.mu.Unlock()
	now := sampler.now()
	if sampler.count < sampler.burst {
		sampler.count++
		sampler.last = now
		return true
	}
	if now.Sub(sampler.last) >= sampler.window {
		sampler.last = now
		return true
	}
	return false
}
