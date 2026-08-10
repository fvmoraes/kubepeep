package kubernetes

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
)

var errGenerationChanged = &SafeError{
	Code:      CodeGenerationChanged,
	Message:   "The Kubernetes client generation changed.",
	Retryable: true,
}

type cacheEntry struct {
	fingerprint Fingerprint
	clients     *Clients
}

type buildCall struct {
	done    chan struct{}
	clients *Clients
	err     error
}

// Lease binds cached clients to one cancellable generation.
type Lease struct {
	Clients     *Clients
	Generation  *Generation
	Descriptor  Descriptor
	fingerprint Fingerprint
}

// ClientCache is keyed only by ordered paths plus context. Fingerprints are
// transient versions used to replace entries under the same logical key.
type ClientCache struct {
	mu               sync.Mutex
	ctx              context.Context
	cancel           context.CancelCauseFunc
	builder          ClientBuilder
	unaryTimeout     time.Duration
	entries          map[string]*cacheEntry
	inflight         map[string]*buildCall
	epochs           map[string]uint64
	active           *Lease
	activationSeq    uint64
	activationTarget string
	generationSeq    uint64
	closed           bool
}

func NewClientCache(parent context.Context, builder ClientBuilder, unaryTimeout time.Duration) (*ClientCache, error) {
	if parent == nil || builder == nil {
		return nil, safeError(CodeClientUnavailable, "The Kubernetes client cache dependencies are invalid.", false)
	}
	if unaryTimeout == 0 {
		unaryTimeout = DefaultUnaryTimeout
	}
	if unaryTimeout < 0 {
		return nil, safeError(CodeClientUnavailable, "The unary Kubernetes timeout is invalid.", false)
	}
	ctx, cancel := context.WithCancelCause(parent)
	return &ClientCache{
		ctx:          ctx,
		cancel:       cancel,
		builder:      builder,
		unaryTimeout: unaryTimeout,
		entries:      make(map[string]*cacheEntry),
		inflight:     make(map[string]*buildCall),
		epochs:       make(map[string]uint64),
	}, nil
}

// Activate returns a current-generation lease, deduplicating concurrent builds.
// A stale Resolution is rejected instead of mixing credentials from two file
// generations.
func (cache *ClientCache) Activate(ctx context.Context, resolution *Resolution) (*Lease, error) {
	if ctx == nil || resolution == nil {
		return nil, safeError(CodeClientUnavailable, "A Kubernetes client resolution is required.", false)
	}
	descriptor := resolution.Descriptor()
	key := descriptor.CacheKey()
	currentFingerprint, err := resolution.CurrentFingerprint(ctx)
	if err != nil {
		cache.invalidateKey(key, errGenerationChanged)
		return nil, err
	}
	if currentFingerprint != resolution.Fingerprint() {
		cache.invalidateKey(key, errGenerationChanged)
		return nil, errGenerationChanged
	}

	cache.mu.Lock()
	if cache.closed {
		cache.mu.Unlock()
		return nil, safeError(CodeClientUnavailable, "The Kubernetes client cache is closed.", false)
	}
	if cache.active != nil && cache.active.Descriptor.CacheKey() == key && cache.active.fingerprint == currentFingerprint && cache.active.Generation.Context().Err() == nil {
		lease := cache.active
		cache.mu.Unlock()
		return lease, nil
	}
	epoch := cache.epochs[key]
	buildKey := key + ":" + currentFingerprint.String() + ":" + strconv.FormatUint(epoch, 10)
	if cache.activationTarget != buildKey {
		cache.activationSeq++
		cache.activationTarget = buildKey
	}
	activation := cache.activationSeq
	if cache.active != nil && (cache.active.Descriptor.CacheKey() != key || cache.active.fingerprint != currentFingerprint) {
		cache.active.Generation.cancelWith(errGenerationChanged)
		cache.active = nil
	}
	if entry := cache.entries[key]; entry != nil && entry.fingerprint == currentFingerprint {
		lease := cache.activateLocked(descriptor, currentFingerprint, entry.clients)
		cache.mu.Unlock()
		return lease, nil
	}
	if entry := cache.entries[key]; entry != nil && entry.fingerprint != currentFingerprint {
		delete(cache.entries, key)
		entry.clients.CloseIdleConnections()
	}
	call := cache.inflight[buildKey]
	if call == nil {
		call = &buildCall{done: make(chan struct{})}
		cache.inflight[buildKey] = call
		cache.mu.Unlock()
		clients, buildErr := cache.builder.Build(cache.ctx, resolution)
		cache.mu.Lock()
		call.clients = clients
		call.err = buildErr
		superseded := cache.epochs[key] != epoch || activation != cache.activationSeq || cache.activationTarget != buildKey
		if buildErr == nil && !cache.closed && !superseded {
			cache.entries[key] = &cacheEntry{fingerprint: currentFingerprint, clients: clients}
		} else if clients != nil && superseded {
			clients.CloseIdleConnections()
		}
		delete(cache.inflight, buildKey)
		close(call.done)
		if cache.closed {
			if clients != nil {
				clients.CloseIdleConnections()
			}
			cache.mu.Unlock()
			return nil, safeError(CodeClientUnavailable, "The Kubernetes client cache is closed.", false)
		}
		if superseded {
			cache.mu.Unlock()
			return nil, errGenerationChanged
		}
		if buildErr != nil {
			cache.mu.Unlock()
			return nil, buildErr
		}
		if cache.active != nil && cache.active.Descriptor.CacheKey() == key && cache.active.fingerprint == currentFingerprint && cache.active.Generation.Context().Err() == nil {
			lease := cache.active
			cache.mu.Unlock()
			return lease, nil
		}
		lease := cache.activateLocked(descriptor, currentFingerprint, clients)
		cache.mu.Unlock()
		return lease, nil
	}
	cache.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, SanitizeError(ctx.Err())
	case <-cache.ctx.Done():
		return nil, safeError(CodeClientUnavailable, "The Kubernetes client cache is closed.", false)
	case <-call.done:
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if activation != cache.activationSeq {
		return nil, errGenerationChanged
	}
	if call.err != nil {
		return nil, call.err
	}
	if cache.closed {
		return nil, safeError(CodeClientUnavailable, "The Kubernetes client cache is closed.", false)
	}
	if cache.active != nil && cache.active.Descriptor.CacheKey() == key && cache.active.fingerprint == currentFingerprint && cache.active.Generation.Context().Err() == nil {
		return cache.active, nil
	}
	return cache.activateLocked(descriptor, currentFingerprint, call.clients), nil
}

func (cache *ClientCache) activateLocked(descriptor Descriptor, fingerprint Fingerprint, clients *Clients) *Lease {
	if cache.active != nil {
		cache.active.Generation.cancelWith(errGenerationChanged)
	}
	cache.generationSeq++
	generation := newGeneration(cache.ctx, cache.generationSeq, cache.unaryTimeout)
	lease := &Lease{
		Clients:     clients,
		Generation:  generation,
		Descriptor:  descriptor,
		fingerprint: fingerprint,
	}
	cache.active = lease
	return lease
}

// Invalidate removes one logical entry and cancels it if active.
func (cache *ClientCache) Invalidate(descriptor Descriptor) {
	cache.invalidateKey(descriptor.CacheKey(), errGenerationChanged)
}

// InvalidateOnError rebuilds only for classified authentication failures.
func (cache *ClientCache) InvalidateOnError(descriptor Descriptor, err error) bool {
	if !IsRebuildableAuthenticationError(err) {
		return false
	}
	cache.Invalidate(descriptor)
	return true
}

func (cache *ClientCache) invalidateKey(key string, cause error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.epochs[key]++
	cache.activationSeq++
	cache.activationTarget = ""
	if entry := cache.entries[key]; entry != nil {
		delete(cache.entries, key)
		entry.clients.CloseIdleConnections()
	}
	if cache.active != nil && cache.active.Descriptor.CacheKey() == key {
		cache.active.Generation.cancelWith(cause)
		cache.active = nil
	}
}

func (cache *ClientCache) Close() error {
	cache.mu.Lock()
	if cache.closed {
		cache.mu.Unlock()
		return nil
	}
	cache.closed = true
	cache.cancel(context.Canceled)
	if cache.active != nil {
		cache.active.Generation.cancelWith(context.Canceled)
		cache.active = nil
	}
	entries := cache.entries
	cache.entries = make(map[string]*cacheEntry)
	cache.mu.Unlock()
	for _, entry := range entries {
		entry.clients.CloseIdleConnections()
	}
	return nil
}

// IsGenerationChanged permits coordinators to discard stale results without
// examining error strings.
func IsGenerationChanged(err error) bool {
	var safe *SafeError
	return errors.As(err, &safe) && safe.Code == CodeGenerationChanged
}
