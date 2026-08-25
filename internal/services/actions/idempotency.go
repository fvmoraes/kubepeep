package actions

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

const maximumIdempotencyEntries = 4096

type idempotencyIdentity struct {
	Method           string
	Path             string
	ClusterProfileID int64
	Generation       string
	BodyHash         string
}

type idempotencyEntry[T any] struct {
	identity   idempotencyIdentity
	done       chan struct{}
	value      T
	err        error
	terminalAt time.Time
}

type idempotencyRegistry[T any] struct {
	mu      sync.Mutex
	clock   Clock
	ttl     time.Duration
	entries map[string]*idempotencyEntry[T]
}

func newIdempotencyRegistry[T any](clock Clock, ttl time.Duration) *idempotencyRegistry[T] {
	return &idempotencyRegistry[T]{clock: clock, ttl: ttl, entries: make(map[string]*idempotencyEntry[T])}
}

func (r *idempotencyRegistry[T]) Do(ctx context.Context, key string, identity idempotencyIdentity, execute func() (T, error)) (value T, replayed bool, err error) {
	r.mu.Lock()
	r.pruneLocked(r.clock.Now())
	if existing, ok := r.entries[key]; ok {
		if existing.identity != identity {
			r.mu.Unlock()
			return value, false, publicError(CodeIdempotencyConflict, http.StatusConflict, false, nil)
		}
		done := existing.done
		r.mu.Unlock()
		select {
		case <-done:
			return existing.value, true, existing.err
		case <-ctx.Done():
			return value, true, translateError(ctx.Err())
		}
	}
	if len(r.entries) >= maximumIdempotencyEntries {
		r.mu.Unlock()
		return value, false, publicError(CodeLimitExceeded, http.StatusTooManyRequests, true, nil)
	}
	entry := &idempotencyEntry[T]{identity: identity, done: make(chan struct{})}
	r.entries[key] = entry
	r.mu.Unlock()

	defer func() {
		if recovered := recover(); recovered != nil {
			err = publicError(CodeInternal, http.StatusInternalServerError, false, nil)
		}
		r.mu.Lock()
		entry.value = value
		entry.err = err
		entry.terminalAt = r.clock.Now()
		if errors.Is(err, context.Canceled) {
			delete(r.entries, key)
		}
		close(entry.done)
		r.mu.Unlock()
	}()
	value, err = execute()
	return value, false, err
}

func (r *idempotencyRegistry[T]) pruneLocked(now time.Time) {
	for key, entry := range r.entries {
		if entry.terminalAt.IsZero() {
			continue
		}
		if !now.Before(entry.terminalAt.Add(r.ttl)) {
			delete(r.entries, key)
		}
	}
}
