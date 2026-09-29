package resources

import (
	"context"
	"errors"
	"sync"
	"time"

	"k8s.io/client-go/util/flowcontrol"
)

type RequestPriority uint8

const (
	PriorityVisible RequestPriority = iota
	PriorityLikelyNext
	PriorityUnrelated
)

var ErrPrefetchDeferred = errors.New("prefetch deferred to visible work")

type SchedulerStats struct {
	Active         int
	VisibleWaiting int
	Deferred       uint64
	Congestion     uint64
	Limit          int
	Adaptive       bool
	Increases      uint64
	Reductions     uint64
}

type SchedulerConfig struct {
	Minimum          int
	Initial          int
	Maximum          int
	Adaptive         bool
	HealthyThreshold int
}

// RequestScheduler bounds LIST windows across resources. Prefetch uses a
// separate, small QPS allowance and reserved slots so it cannot consume the
// visible request's concurrency or tokens. Full AIMD tuning belongs to F6.
type RequestScheduler struct {
	mu                 sync.Mutex
	changed            chan struct{}
	active             int
	visibleWaiting     int
	deferred           uint64
	congestion         uint64
	prefetchPauseUntil time.Time
	minimum            int
	limit              int
	maximum            int
	adaptive           bool
	healthy            int
	healthyThreshold   int
	increases          uint64
	reductions         uint64
	now                func() time.Time
	visibleRate        flowcontrol.RateLimiter
	prefetchRate       flowcontrol.RateLimiter
}

func NewRequestScheduler(maximum int, now func() time.Time) *RequestScheduler {
	if maximum <= 0 {
		maximum = 8
	}
	return NewRequestSchedulerWithConfig(SchedulerConfig{Minimum: maximum, Initial: maximum, Maximum: maximum}, now)
}

func NewRequestSchedulerWithConfig(config SchedulerConfig, now func() time.Time) *RequestScheduler {
	if config.Minimum <= 0 {
		config.Minimum = 2
	}
	if config.Maximum <= 0 {
		config.Maximum = 8
	}
	if config.Initial <= 0 {
		config.Initial = 4
	}
	config.Minimum = min(config.Minimum, config.Maximum)
	config.Initial = max(config.Minimum, min(config.Initial, config.Maximum))
	if config.HealthyThreshold <= 0 {
		config.HealthyThreshold = 8
	}
	if now == nil {
		now = time.Now
	}
	return &RequestScheduler{
		changed: make(chan struct{}), minimum: config.Minimum, limit: config.Initial, maximum: config.Maximum,
		adaptive: config.Adaptive, healthyThreshold: config.HealthyThreshold, now: now,
		visibleRate:  flowcontrol.NewTokenBucketRateLimiter(18, 36),
		prefetchRate: flowcontrol.NewTokenBucketRateLimiter(2, 4),
	}
}

func (scheduler *RequestScheduler) Acquire(ctx context.Context, priority RequestPriority) (func(), error) {
	if scheduler == nil {
		return func() {}, nil
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for {
		scheduler.mu.Lock()
		if priority != PriorityVisible && scheduler.prefetchDeferredLocked(priority) {
			scheduler.deferred++
			scheduler.mu.Unlock()
			return nil, ErrPrefetchDeferred
		}
		if scheduler.active < scheduler.limit {
			scheduler.active++
			scheduler.mu.Unlock()
			limiter := scheduler.visibleRate
			if priority != PriorityVisible {
				limiter = scheduler.prefetchRate
			}
			if err := limiter.Wait(ctx); err != nil {
				scheduler.release()
				return nil, err
			}
			var once sync.Once
			return func() { once.Do(scheduler.release) }, nil
		}
		if priority != PriorityVisible {
			scheduler.deferred++
			scheduler.mu.Unlock()
			return nil, ErrPrefetchDeferred
		}
		scheduler.visibleWaiting++
		changed := scheduler.changed
		scheduler.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			scheduler.mu.Lock()
			scheduler.visibleWaiting--
			scheduler.mu.Unlock()
			return nil, ctx.Err()
		}
		scheduler.mu.Lock()
		scheduler.visibleWaiting--
		scheduler.mu.Unlock()
	}
}

func (scheduler *RequestScheduler) prefetchDeferredLocked(priority RequestPriority) bool {
	reserve := 2
	if priority == PriorityUnrelated {
		reserve = 4
	}
	if scheduler.limit <= reserve {
		return true
	}
	return scheduler.visibleWaiting > 0 || scheduler.active >= scheduler.limit-reserve || scheduler.now().Before(scheduler.prefetchPauseUntil)
}

func (scheduler *RequestScheduler) release() {
	scheduler.mu.Lock()
	scheduler.active--
	close(scheduler.changed)
	scheduler.changed = make(chan struct{})
	scheduler.mu.Unlock()
}

// Observe delays speculative work after 429, timeout, or high latency. It
// never throttles already-visible work; transport metrics record exact status.
func (scheduler *RequestScheduler) Observe(latency time.Duration, throttled, timedOut bool) {
	if scheduler == nil {
		return
	}
	scheduler.mu.Lock()
	if !throttled && !timedOut && latency < 2*time.Second {
		if scheduler.adaptive && scheduler.limit < scheduler.maximum {
			scheduler.healthy++
			if scheduler.healthy >= scheduler.healthyThreshold {
				scheduler.limit++
				scheduler.healthy = 0
				scheduler.increases++
				close(scheduler.changed)
				scheduler.changed = make(chan struct{})
			}
		}
		scheduler.mu.Unlock()
		return
	}
	scheduler.congestion++
	scheduler.healthy = 0
	if scheduler.adaptive && (throttled || timedOut) {
		next := max(scheduler.minimum, scheduler.limit/2)
		if next < scheduler.limit {
			scheduler.limit = next
			scheduler.reductions++
		}
	}
	pause := 5 * time.Second
	if throttled || timedOut {
		pause = 15 * time.Second
	}
	until := scheduler.now().Add(pause)
	if until.After(scheduler.prefetchPauseUntil) {
		scheduler.prefetchPauseUntil = until
	}
	scheduler.mu.Unlock()
}

func (scheduler *RequestScheduler) Stats() SchedulerStats {
	if scheduler == nil {
		return SchedulerStats{}
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return SchedulerStats{
		Active: scheduler.active, VisibleWaiting: scheduler.visibleWaiting, Deferred: scheduler.deferred,
		Congestion: scheduler.congestion, Limit: scheduler.limit, Adaptive: scheduler.adaptive,
		Increases: scheduler.increases, Reductions: scheduler.reductions,
	}
}
