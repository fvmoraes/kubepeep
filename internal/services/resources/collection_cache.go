package resources

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/observability"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

const (
	DefaultCollectionCacheMaxBytes   = 64 << 20
	DefaultCollectionCacheMaxEntries = 512
	DefaultCollectionCacheFreshFor   = 30 * time.Second
)

// CollectionCache keeps only bounded, already-authorized list DTO pages. It
// never stores Secret metadata or details; a hit is reauthorized per origin.
type CollectionCache struct {
	mu         sync.Mutex
	entries    map[string]*collectionCacheEntry
	generation string
	revision   uint64
	bytes      int
	maxBytes   int
	maxEntries int
	freshFor   time.Duration
	now        func() time.Time
	metrics    *observability.Registry
}

type collectionCacheEntry struct {
	data       []byte
	generation string
	collection Collection
	origins    []Origin
	expiresAt  time.Time
	lastUsed   time.Time
}

type CollectionCacheToken struct {
	generation string
	revision   uint64
	key        string
}

type CollectionCacheStats struct {
	Entries int
	Bytes   int
}

func NewCollectionCache(maxBytes, maxEntries int, freshFor time.Duration, now func() time.Time) *CollectionCache {
	return NewCollectionCacheWithMetrics(maxBytes, maxEntries, freshFor, now, nil)
}

func NewCollectionCacheWithMetrics(maxBytes, maxEntries int, freshFor time.Duration, now func() time.Time, metrics *observability.Registry) *CollectionCache {
	if maxBytes <= 0 {
		maxBytes = DefaultCollectionCacheMaxBytes
	}
	if maxEntries <= 0 {
		maxEntries = DefaultCollectionCacheMaxEntries
	}
	if freshFor <= 0 {
		freshFor = DefaultCollectionCacheFreshFor
	}
	if now == nil {
		now = time.Now
	}
	return &CollectionCache{
		entries:  make(map[string]*collectionCacheEntry),
		maxBytes: maxBytes, maxEntries: maxEntries, freshFor: freshFor, now: now, metrics: metrics,
	}
}

func (cache *CollectionCache) Begin(generation, key string) (CollectionCacheToken, bool) {
	if cache == nil || generation == "" || key == "" {
		return CollectionCacheToken{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.generation == "" {
		cache.generation = generation
	}
	if cache.generation != generation {
		return CollectionCacheToken{}, false
	}
	return CollectionCacheToken{generation: generation, revision: cache.revision, key: key}, true
}

// SwitchGeneration fences every in-flight writer and removes all prior data.
func (cache *CollectionCache) SwitchGeneration(next string) {
	if cache == nil || next == "" {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.revision++
	for _, entry := range cache.entries {
		cache.metrics.IncCounter(observability.CollectionCacheInvalidationsTotalName, collectionMetricLabels(entry.collection))
	}
	cache.generation = next
	clear(cache.entries)
	cache.bytes = 0
	cache.recordGaugesLocked()
}

func (cache *CollectionCache) InvalidateCollection(generation string, collection Collection) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.generation != generation {
		return
	}
	cache.revision++
	for key, entry := range cache.entries {
		if entry.generation == generation && entry.collection == collection {
			cache.metrics.IncCounter(observability.CollectionCacheInvalidationsTotalName, collectionMetricLabels(collection))
			cache.removeLocked(key)
		}
	}
	cache.recordGaugesLocked()
}

func (cache *CollectionCache) Stats() CollectionCacheStats {
	if cache == nil {
		return CollectionCacheStats{}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return CollectionCacheStats{Entries: len(cache.entries), Bytes: cache.bytes}
}

// LoadCollectionPage checks current authorization before returning decoded
// data. An unknown decision is never treated as a positive cache hit.
func LoadCollectionPage[T ListItem](ctx context.Context, cache *CollectionCache, key, generation string, checker AuthorizationChecker) (ListResult[T], bool) {
	if cache == nil || checker == nil {
		return ListResult[T]{}, false
	}
	cache.mu.Lock()
	entry := cache.entries[key]
	if entry == nil || cache.generation != generation || entry.generation != generation || !cache.now().Before(entry.expiresAt) {
		cache.mu.Unlock()
		return ListResult[T]{}, false
	}
	data := append([]byte(nil), entry.data...)
	origins := append([]Origin(nil), entry.origins...)
	revision := cache.revision
	cache.mu.Unlock()
	for _, origin := range origins {
		capability := checker.Check(ctx, authorization.Key{Generation: generation, Namespace: origin.Namespace, APIGroup: origin.APIGroup, Resource: origin.Resource, Verb: "list"})
		if capability.Decision != authorization.DecisionAllowed {
			cache.InvalidateCollection(generation, entry.collection)
			return ListResult[T]{}, false
		}
	}
	var result ListResult[T]
	if err := json.Unmarshal(data, &result); err != nil {
		return ListResult[T]{}, false
	}
	cache.mu.Lock()
	current := cache.entries[key]
	valid := current != nil && current.generation == generation && cache.revision == revision && cache.now().Before(current.expiresAt)
	if valid {
		current.lastUsed = cache.now()
	}
	cache.mu.Unlock()
	return result, valid
}

// StoreCollectionPage is limited to the seven watch-safe DTO families. The
// caller must begin before its network read so invalidation fences late data.
func StoreCollectionPage[T ListItem](cache *CollectionCache, token CollectionCacheToken, collection Collection, result ListResult[T]) bool {
	topic, supported := collectionCacheTopic(collection)
	if cache == nil || !supported || len(result.Coverage.Failed) != 0 || token.key == "" || result.Cursor == nil {
		return false
	}
	for _, item := range result.Items {
		if !validCollectionObject(item, topic) {
			return false
		}
	}
	for _, state := range result.Cursor.Origins {
		for _, item := range state.Buffered {
			if !validCollectionObject(item, topic) {
				return false
			}
		}
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) > cache.maxBytes {
		return false
	}
	origins := make([]Origin, 0, len(result.Cursor.Origins))
	for _, state := range result.Cursor.Origins {
		origins = append(origins, state.Origin)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.generation != token.generation || cache.revision != token.revision {
		return false
	}
	if previous := cache.entries[token.key]; previous != nil {
		cache.removeLocked(token.key)
	}
	for len(cache.entries) >= cache.maxEntries || cache.bytes+len(data) > cache.maxBytes {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range cache.entries {
			if oldestKey == "" || entry.lastUsed.Before(oldest) {
				oldestKey, oldest = key, entry.lastUsed
			}
		}
		if oldestKey == "" {
			return false
		}
		cache.metrics.IncCounter(observability.CollectionCacheEvictionsTotalName, collectionMetricLabels(cache.entries[oldestKey].collection))
		cache.removeLocked(oldestKey)
	}
	now := cache.now()
	cache.entries[token.key] = &collectionCacheEntry{data: data, generation: token.generation, collection: collection, origins: origins, expiresAt: now.Add(cache.freshFor), lastUsed: now}
	cache.bytes += len(data)
	cache.recordGaugesLocked()
	return true
}

func (cache *CollectionCache) recordGaugesLocked() {
	cache.metrics.SetGauge(observability.CollectionCacheEntriesName, nil, int64(len(cache.entries)))
	cache.metrics.SetGauge(observability.CollectionCacheBytesName, nil, int64(cache.bytes))
}

func collectionMetricLabels(collection Collection) map[string]string {
	if _, ok := collectionCacheTopic(collection); !ok {
		return nil
	}
	return map[string]string{"resource": string(collection)}
}

func validCollectionObject[T ListItem](item T, topic Topic) bool {
	object, ok := any(item).(TopicObject)
	if !ok || object == nil {
		return false
	}
	value := reflect.ValueOf(object)
	return !(value.Kind() == reflect.Ptr && value.IsNil()) && object.resourceTopic() == topic
}

func (cache *CollectionCache) removeLocked(key string) {
	if entry := cache.entries[key]; entry != nil {
		cache.bytes -= len(entry.data)
		delete(cache.entries, key)
	}
}

func collectionCacheTopic(collection Collection) (Topic, bool) {
	switch collection {
	case CollectionPods:
		return TopicPods, true
	case CollectionEvents:
		return TopicEvents, true
	case CollectionWorkloads:
		return TopicWorkloads, true
	case CollectionServices:
		return TopicServices, true
	case CollectionIngresses:
		return TopicIngresses, true
	case CollectionEndpointSlices:
		return TopicEndpointSlices, true
	case CollectionConfigMaps:
		return TopicConfigMaps, true
	default:
		return "", false
	}
}
