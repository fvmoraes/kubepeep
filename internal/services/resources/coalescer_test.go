package resources

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestCoalescerRunsIdenticalOverlapOnce(t *testing.T) {
	coalescer := NewRequestCoalescer()
	t.Cleanup(coalescer.Close)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	operation := func(context.Context) (any, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return "value", nil
	}
	var wait sync.WaitGroup
	results := make(chan any, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := coalescer.Do(t.Context(), "same", operation)
			if err != nil {
				t.Errorf("Do: %v", err)
			}
			results <- result
		}()
	}
	<-started
	deadline := time.Now().Add(time.Second)
	for {
		coalescer.mu.Lock()
		waiters := coalescer.calls["same"].waiters
		coalescer.mu.Unlock()
		if waiters == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second request did not join the in-flight call")
		}
		runtime.Gosched()
	}
	close(release)
	wait.Wait()
	close(results)
	if calls.Load() != 1 {
		t.Fatalf("executions = %d", calls.Load())
	}
	for result := range results {
		if result != "value" {
			t.Fatalf("result = %#v", result)
		}
	}
}

func TestRequestCoalescerCancelsWhenLastWaiterLeaves(t *testing.T) {
	coalescer := NewRequestCoalescer()
	t.Cleanup(coalescer.Close)
	workerDone := make(chan error, 1)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := coalescer.Do(ctx, "cancel", func(shared context.Context) (any, error) {
			<-shared.Done()
			workerDone <- shared.Err()
			return nil, shared.Err()
		})
		done <- err
	}()
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("waiter error = %v", err)
	}
	select {
	case err := <-workerDone:
		if err != context.Canceled {
			t.Fatalf("worker error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("orphaned coalesced work")
	}
}

func TestRequestCoalescerStartsFreshAfterAllWaitersCancel(t *testing.T) {
	coalescer := NewRequestCoalescer()
	t.Cleanup(coalescer.Close)
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := coalescer.Do(ctx, "same", func(shared context.Context) (any, error) {
			close(started)
			<-shared.Done()
			return nil, shared.Err()
		})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("canceled waiter error = %v", err)
	}

	value, err := coalescer.Do(t.Context(), "same", func(context.Context) (any, error) {
		return "fresh", nil
	})
	if err != nil || value != "fresh" {
		t.Fatalf("fresh result = %#v, err = %v", value, err)
	}
}
