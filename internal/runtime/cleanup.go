package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// CleanupFunc releases one acquired resource.
type CleanupFunc func(context.Context) error

type cleanupEntry struct {
	name string
	fn   CleanupFunc
}

// CleanupRegistry runs every registered hook in reverse acquisition order.
// It aggregates failures and never stops at the first error.
type CleanupRegistry struct {
	mutex   sync.Mutex
	entries []cleanupEntry
	run     bool
	done    chan struct{}
	err     error
}

// Add registers a named hook. Register resources immediately after acquiring
// them so partial startup failures are covered.
func (registry *CleanupRegistry) Add(name string, cleanup CleanupFunc) error {
	if cleanup == nil {
		return errors.New("runtime: cleanup hook is nil")
	}
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	if registry.run {
		return errors.New("runtime: cleanup has already started")
	}
	registry.entries = append(registry.entries, cleanupEntry{name: name, fn: cleanup})
	return nil
}

// Run executes the registry exactly once.
func (registry *CleanupRegistry) Run(ctx context.Context) error {
	registry.mutex.Lock()
	if registry.run {
		done := registry.done
		registry.mutex.Unlock()
		<-done
		registry.mutex.Lock()
		result := registry.err
		registry.mutex.Unlock()
		return result
	}
	registry.run = true
	registry.done = make(chan struct{})
	done := registry.done
	entries := append([]cleanupEntry(nil), registry.entries...)
	registry.mutex.Unlock()

	var result error
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		err := runCleanup(entry, ctx)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("runtime: cleanup %s: %w", entry.name, err))
		}
	}
	registry.mutex.Lock()
	registry.err = result
	registry.mutex.Unlock()
	close(done)
	return result
}

func runCleanup(entry cleanupEntry, ctx context.Context) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v", recovered)
		}
	}()
	return entry.fn(ctx)
}
