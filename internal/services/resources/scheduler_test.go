package resources

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRequestSchedulerReservesVisibleCapacityAndQPS(t *testing.T) {
	t.Parallel()
	scheduler := NewRequestScheduler(4, nil)
	first, err := scheduler.Acquire(t.Context(), PriorityLikelyNext)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := scheduler.Acquire(t.Context(), PriorityLikelyNext)
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	if _, err := scheduler.Acquire(t.Context(), PriorityLikelyNext); !errors.Is(err, ErrPrefetchDeferred) {
		t.Fatalf("speculative work consumed reserved capacity: %v", err)
	}
	visible, err := scheduler.Acquire(t.Context(), PriorityVisible)
	if err != nil {
		t.Fatalf("visible work lost its reserved slot: %v", err)
	}
	visible()
	if stats := scheduler.Stats(); stats.Active != 2 || stats.Deferred != 1 {
		t.Fatalf("scheduler stats = %#v", stats)
	}
}

func TestRequestSchedulerWaitCancellationAndCongestion(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	scheduler := NewRequestScheduler(1, func() time.Time { return now })
	active, err := scheduler.Acquire(t.Context(), PriorityVisible)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, waitErr := scheduler.Acquire(ctx, PriorityVisible); done <- waitErr }()
	deadline := time.Now().Add(time.Second)
	for scheduler.Stats().VisibleWaiting != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if scheduler.Stats().VisibleWaiting != 1 {
		t.Fatal("visible waiter never registered")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait = %v", err)
	}
	active()
	if stats := scheduler.Stats(); stats.Active != 0 || stats.VisibleWaiting != 0 {
		t.Fatalf("slot leaked: %#v", stats)
	}
	scheduler.Observe(time.Second, true, false)
	if _, err := scheduler.Acquire(t.Context(), PriorityLikelyNext); !errors.Is(err, ErrPrefetchDeferred) {
		t.Fatalf("429 did not defer prefetch: %v", err)
	}
	now = now.Add(16 * time.Second)
	// A single-slot scheduler still reserves its only slot for visible work.
	if _, err := scheduler.Acquire(t.Context(), PriorityLikelyNext); !errors.Is(err, ErrPrefetchDeferred) {
		t.Fatalf("prefetch stole single visible slot: %v", err)
	}
	if scheduler.Stats().Congestion != 1 {
		t.Fatal("congestion was not observed")
	}
}

func TestRequestSchedulerAIMDReducesAndRecoversWithinBounds(t *testing.T) {
	t.Parallel()
	scheduler := NewRequestSchedulerWithConfig(SchedulerConfig{
		Minimum: 2, Initial: 4, Maximum: 8, Adaptive: true, HealthyThreshold: 2,
	}, nil)

	scheduler.Observe(100*time.Millisecond, true, false)
	stats := scheduler.Stats()
	if stats.Limit != 2 || stats.Reductions != 1 || !stats.Adaptive {
		t.Fatalf("429 reduction = %#v", stats)
	}
	scheduler.Observe(100*time.Millisecond, true, false)
	if stats = scheduler.Stats(); stats.Limit != 2 || stats.Reductions != 1 {
		t.Fatalf("minimum bound = %#v", stats)
	}

	for expected := 3; expected <= 8; expected++ {
		scheduler.Observe(10*time.Millisecond, false, false)
		scheduler.Observe(10*time.Millisecond, false, false)
		if stats = scheduler.Stats(); stats.Limit != expected {
			t.Fatalf("recovery to %d = %#v", expected, stats)
		}
	}
	scheduler.Observe(10*time.Millisecond, false, false)
	scheduler.Observe(10*time.Millisecond, false, false)
	if stats = scheduler.Stats(); stats.Limit != 8 || stats.Increases != 6 {
		t.Fatalf("maximum bound = %#v", stats)
	}
}
