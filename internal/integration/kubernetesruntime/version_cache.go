package kubernetesruntime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourceService "github.com/fvmoraes/kubepeep/internal/services/resources"
)

const defaultVersionCacheTTL = 10 * time.Minute

type versionCacheEntry struct {
	result    kubernetes.ConnectivityResult
	expiresAt time.Time
}

// versionCache keeps only successful Kubernetes /version responses. Failures
// remain retryable and every selection generation has an isolated identity.
type versionCache struct {
	mu        sync.RWMutex
	entries   map[discoveryCacheKey]versionCacheEntry
	coalescer *resourceService.RequestCoalescer
	ttl       time.Duration
	now       func() time.Time
	epoch     uint64
}

func newVersionCache(ttl time.Duration, now func() time.Time) *versionCache {
	if ttl <= 0 {
		ttl = defaultVersionCacheTTL
	}
	if now == nil {
		now = time.Now
	}
	return &versionCache{entries: make(map[discoveryCacheKey]versionCacheEntry), coalescer: resourceService.NewRequestCoalescer(), ttl: ttl, now: now}
}

func (cache *versionCache) Check(ctx context.Context, binding namespaces.SelectionBinding, load func(context.Context) kubernetes.ConnectivityResult) (kubernetes.ConnectivityResult, error) {
	if cache == nil {
		return load(ctx), nil
	}
	key := discoveryKey(binding)
	if result, ok := cache.lookup(key); ok {
		return result, nil
	}
	cache.mu.RLock()
	epoch := cache.epoch
	cache.mu.RUnlock()
	value, err := cache.coalescer.Do(ctx, key.requestID()+":version", func(shared context.Context) (any, error) {
		if result, ok := cache.lookup(key); ok {
			return result, nil
		}
		result := load(shared)
		if result.Status == kubernetes.ConnectivityHealthy && result.Version != "" {
			cache.mu.Lock()
			if cache.epoch == epoch {
				cache.entries[key] = versionCacheEntry{result: result, expiresAt: cache.now().Add(cache.ttl)}
			}
			cache.mu.Unlock()
		}
		return result, nil
	})
	if err != nil {
		return kubernetes.ConnectivityResult{}, err
	}
	result, ok := value.(kubernetes.ConnectivityResult)
	if !ok {
		return kubernetes.ConnectivityResult{}, fmt.Errorf("version cache returned an invalid result")
	}
	return result, nil
}

func (cache *versionCache) lookup(key discoveryCacheKey) (kubernetes.ConnectivityResult, bool) {
	now := cache.now()
	cache.mu.RLock()
	entry, ok := cache.entries[key]
	cache.mu.RUnlock()
	if !ok || !now.Before(entry.expiresAt) {
		return kubernetes.ConnectivityResult{}, false
	}
	return entry.result, true
}

func (cache *versionCache) InvalidateAll() {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	cache.epoch++
	clear(cache.entries)
	cache.mu.Unlock()
}

func (cache *versionCache) Close() {
	if cache != nil && cache.coalescer != nil {
		cache.coalescer.Close()
	}
}
