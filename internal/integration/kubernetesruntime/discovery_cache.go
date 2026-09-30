package kubernetesruntime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourceService "github.com/fvmoraes/kubepeep/internal/services/resources"
)

const defaultDiscoveryCacheTTL = 10 * time.Minute

type discoveryCacheKey struct {
	ClusterProfileID int64
	Context          string
	Cluster          string
	ActiveScopeID    int64
	Generation       string
}

func discoveryKey(binding namespaces.SelectionBinding) discoveryCacheKey {
	return discoveryCacheKey{
		ClusterProfileID: binding.ClusterProfileID,
		Context:          binding.Context,
		Cluster:          binding.Cluster,
		ActiveScopeID:    binding.ActiveScopeID,
		Generation:       binding.Generation,
	}
}

func (key discoveryCacheKey) requestID() string {
	material := fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s", key.ClusterProfileID, key.Context, key.Cluster, key.ActiveScopeID, key.Generation)
	digest := sha256.Sum256([]byte(material))
	return fmt.Sprintf("discovery:%x", digest)
}

type discoveryCacheEntry struct {
	available bool
	expiresAt time.Time
}

type metricsDiscoveryCache struct {
	mu        sync.RWMutex
	entries   map[discoveryCacheKey]discoveryCacheEntry
	coalescer *resourceService.RequestCoalescer
	ttl       time.Duration
	now       func() time.Time
	epoch     uint64
}

func newMetricsDiscoveryCache(ttl time.Duration, now func() time.Time) *metricsDiscoveryCache {
	if ttl <= 0 {
		ttl = defaultDiscoveryCacheTTL
	}
	if now == nil {
		now = time.Now
	}
	return &metricsDiscoveryCache{
		entries:   make(map[discoveryCacheKey]discoveryCacheEntry),
		coalescer: resourceService.NewRequestCoalescer(),
		ttl:       ttl,
		now:       now,
	}
}

func (cache *metricsDiscoveryCache) Available(ctx context.Context, binding namespaces.SelectionBinding, load func(context.Context) (bool, error)) (bool, error) {
	if cache == nil {
		return load(ctx)
	}
	key := discoveryKey(binding)
	if available, ok := cache.lookup(key); ok {
		return available, nil
	}

	cache.mu.RLock()
	epoch := cache.epoch
	cache.mu.RUnlock()
	value, err := cache.coalescer.Do(ctx, key.requestID(), func(shared context.Context) (any, error) {
		if available, ok := cache.lookup(key); ok {
			return available, nil
		}
		available, loadErr := load(shared)
		if loadErr != nil {
			return false, loadErr
		}
		cache.mu.Lock()
		if cache.epoch == epoch {
			cache.entries[key] = discoveryCacheEntry{available: available, expiresAt: cache.now().Add(cache.ttl)}
		}
		cache.mu.Unlock()
		return available, nil
	})
	if err != nil {
		return false, err
	}
	available, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("metrics discovery cache returned an invalid result")
	}
	return available, nil
}

func (cache *metricsDiscoveryCache) lookup(key discoveryCacheKey) (bool, bool) {
	now := cache.now()
	cache.mu.RLock()
	entry, ok := cache.entries[key]
	cache.mu.RUnlock()
	if !ok || !now.Before(entry.expiresAt) {
		return false, false
	}
	return entry.available, true
}

func (cache *metricsDiscoveryCache) InvalidateAll() {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	cache.epoch++
	clear(cache.entries)
	cache.mu.Unlock()
}
