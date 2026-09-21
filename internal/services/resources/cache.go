package resources

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fvmoraes/kubepeep/internal/observability"
)

const (
	DefaultResourceCacheMaxBytes   = 192 << 20
	DefaultResourceCacheMaxEntries = 2048
	DefaultResourceCacheFreshFor   = 30 * time.Second
	DefaultResourceCacheMaxStale   = 5 * time.Minute
)

var (
	ErrCacheGenerationInvalidated = errors.New("resource cache generation invalidated")
	ErrCacheWriteFenced           = errors.New("resource cache write fenced")
)

type CacheState string

const (
	CacheStateFresh      CacheState = "FRESH"
	CacheStateStale      CacheState = "STALE"
	CacheStateRefreshing CacheState = "REFRESHING"
	CacheStatePartial    CacheState = "PARTIAL"
	CacheStateExpired    CacheState = "EXPIRED"
)

// ResourceCacheQuery extends the authorization-sensitive WatchKey with every
// query dimension that can change the visible result. Empty Query identifies a
// complete topic snapshot rather than a derived page.
type ResourceCacheQuery struct {
	Filters  []string
	Sort     string
	Order    string
	PageSize int
	Cursor   string
}

// ResourceCacheKey binds cached data to its generation, selection, effective
// origins and query. The identity is process-local and is never exported as a
// metric or trace attribute.
type ResourceCacheKey struct {
	WatchKey
	Query ResourceCacheQuery
}

type ResourceCacheConfig struct {
	MaxBytes   int
	MaxEntries int
	FreshFor   time.Duration
	MaxStale   time.Duration
	Now        func() time.Time
	Metrics    *observability.Registry
}

type CachedSnapshot struct {
	Snapshot WatchSnapshot
	State    CacheState
	StoredAt time.Time
	LastUsed time.Time
	Bytes    int
	RefCount int
	Complete bool
}

type ResourceCacheStats struct {
	Entries       int
	Bytes         int
	Subscriptions int
	Evictions     uint64
	Invalidations uint64
}

type CacheRefreshToken struct {
	identity        string
	generation      string
	generationEpoch uint64
	keyEpoch        uint64
}

type resourceCacheEntry struct {
	key        ResourceCacheKey
	snapshot   WatchSnapshot
	storedAt   time.Time
	lastUsed   time.Time
	freshUntil time.Time
	expiresAt  time.Time
	bytes      int
	partial    bool
	refreshing bool
}

type ResourceCache struct {
	mu                    sync.Mutex
	entries               map[string]*resourceCacheEntry
	refs                  map[string]int
	keys                  map[string]ResourceCacheKey
	keyEpoch              map[string]uint64
	generationEpoch       map[string]uint64
	invalidatedGeneration map[string]struct{}
	bytes                 int
	evictions             uint64
	invalidations         uint64
	maxBytes              int
	maxEntries            int
	freshFor              time.Duration
	maxStale              time.Duration
	now                   func() time.Time
	metrics               *observability.Registry
}

type CacheSubscription struct {
	cache           *ResourceCache
	key             ResourceCacheKey
	identity        string
	generationEpoch uint64
	closed          atomic.Bool
}

func NewResourceCache(config ResourceCacheConfig) *ResourceCache {
	if config.MaxBytes <= 0 {
		config.MaxBytes = DefaultResourceCacheMaxBytes
	}
	if config.MaxEntries <= 0 {
		config.MaxEntries = DefaultResourceCacheMaxEntries
	}
	if config.FreshFor <= 0 {
		config.FreshFor = DefaultResourceCacheFreshFor
	}
	if config.MaxStale < config.FreshFor {
		config.MaxStale = DefaultResourceCacheMaxStale
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &ResourceCache{
		entries:               make(map[string]*resourceCacheEntry),
		refs:                  make(map[string]int),
		keys:                  make(map[string]ResourceCacheKey),
		keyEpoch:              make(map[string]uint64),
		generationEpoch:       make(map[string]uint64),
		invalidatedGeneration: make(map[string]struct{}),
		maxBytes:              config.MaxBytes,
		maxEntries:            config.MaxEntries,
		freshFor:              config.FreshFor,
		maxStale:              config.MaxStale,
		now:                   config.Now,
		metrics:               config.Metrics,
	}
}

func (cache *ResourceCache) Subscribe(ctx context.Context, key ResourceCacheKey) (*CacheSubscription, error) {
	if cache == nil {
		return nil, domainError(CodeFeatureUnavailable, "Resource cache is unavailable.", nil)
	}
	canonical, identity, err := canonicalResourceCacheKey(key)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	_, end := observability.StartSpan(ctx, "cache.snapshot")
	defer end(nil)

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if _, invalid := cache.invalidatedGeneration[canonical.Generation]; invalid {
		return nil, domainError(CodeGenerationChanged, "The cache generation is no longer active.", ErrCacheGenerationInvalidated)
	}
	cache.refs[identity]++
	cache.keys[identity] = canonical
	return &CacheSubscription{
		cache: cache, key: canonical, identity: identity,
		generationEpoch: cache.generationEpoch[canonical.Generation],
	}, nil
}

func (subscription *CacheSubscription) Load(ctx context.Context) (CachedSnapshot, bool) {
	if subscription == nil || subscription.cache == nil || subscription.closed.Load() {
		return CachedSnapshot{}, false
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	cache := subscription.cache
	cache.mu.Lock()
	entry := cache.entries[subscription.identity]
	if entry == nil || !subscription.currentLocked() {
		cache.mu.Unlock()
		cache.metrics.IncCounter(observability.ResourceCacheMissesTotalName, cacheLabels(subscription.key.Topic))
		observability.CacheOutcome(ctx, "miss")
		return CachedSnapshot{}, false
	}
	now := cache.now().UTC()
	entry.lastUsed = now
	state := entry.state(now)
	snapshot, err := cloneWatchSnapshot(entry.snapshot)
	view := CachedSnapshot{
		Snapshot: snapshot, State: state, StoredAt: entry.storedAt, LastUsed: entry.lastUsed,
		Bytes: entry.bytes, RefCount: cache.refs[subscription.identity], Complete: !entry.partial && state != CacheStateExpired,
	}
	cache.mu.Unlock()
	if err != nil {
		cache.metrics.IncCounter(observability.ResourceCacheMissesTotalName, cacheLabels(subscription.key.Topic))
		observability.CacheOutcome(ctx, "miss")
		return CachedSnapshot{}, false
	}
	cache.metrics.IncCounter(observability.ResourceCacheHitsTotalName, cacheLabels(subscription.key.Topic))
	observability.CacheOutcome(ctx, "hit")
	return view, true
}

func (subscription *CacheSubscription) BeginRefresh(ctx context.Context) (CacheRefreshToken, error) {
	if subscription == nil || subscription.cache == nil || subscription.closed.Load() {
		return CacheRefreshToken{}, ErrCacheWriteFenced
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	observability.CacheOutcome(ctx, "refresh")
	cache := subscription.cache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if subscription.closed.Load() || !subscription.currentLocked() {
		return CacheRefreshToken{}, ErrCacheWriteFenced
	}
	cache.keyEpoch[subscription.identity]++
	if entry := cache.entries[subscription.identity]; entry != nil {
		entry.refreshing = true
		entry.lastUsed = cache.now().UTC()
	}
	return CacheRefreshToken{
		identity: subscription.identity, generation: subscription.key.Generation,
		generationEpoch: subscription.generationEpoch, keyEpoch: cache.keyEpoch[subscription.identity],
	}, nil
}

func (subscription *CacheSubscription) Commit(ctx context.Context, token CacheRefreshToken, snapshot WatchSnapshot, partial bool) error {
	if subscription == nil || subscription.cache == nil || subscription.closed.Load() {
		return ErrCacheWriteFenced
	}
	if ctx == nil {
		ctx = context.TODO()
	}
	_, end := observability.StartSpan(ctx, "cache.snapshot")

	cloned, size, err := validatedCacheSnapshot(subscription.key.Topic, snapshot)
	if err != nil {
		end(err)
		return err
	}
	cache := subscription.cache
	cache.mu.Lock()
	if subscription.closed.Load() || !subscription.currentLocked() || token.identity != subscription.identity || token.generation != subscription.key.Generation ||
		token.generationEpoch != cache.generationEpoch[subscription.key.Generation] || token.keyEpoch != cache.keyEpoch[subscription.identity] {
		cache.mu.Unlock()
		end(ErrCacheWriteFenced)
		return ErrCacheWriteFenced
	}
	previousBytes := 0
	if previous := cache.entries[subscription.identity]; previous != nil {
		previousBytes = previous.bytes
	}
	if size > cache.maxBytes || !cache.evictForLocked(subscription.identity, size-previousBytes, cache.entries[subscription.identity] == nil) {
		if entry := cache.entries[subscription.identity]; entry != nil {
			entry.refreshing = false
		}
		cache.mu.Unlock()
		err = domainError(CodeLimitExceeded, "The resource cache memory budget is exhausted.", nil)
		end(err)
		return err
	}
	now := cache.now().UTC()
	cache.bytes += size - previousBytes
	cache.entries[subscription.identity] = &resourceCacheEntry{
		key: subscription.key, snapshot: cloned, storedAt: now, lastUsed: now,
		freshUntil: now.Add(cache.freshFor), expiresAt: now.Add(cache.maxStale), bytes: size, partial: partial,
	}
	cache.recordGaugesLocked()
	cache.mu.Unlock()
	end(nil)
	return nil
}

func (subscription *CacheSubscription) AbortRefresh(token CacheRefreshToken) {
	if subscription == nil || subscription.cache == nil {
		return
	}
	cache := subscription.cache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if token.identity != subscription.identity || token.generationEpoch != cache.generationEpoch[subscription.key.Generation] || token.keyEpoch != cache.keyEpoch[subscription.identity] {
		return
	}
	if entry := cache.entries[subscription.identity]; entry != nil {
		entry.refreshing = false
	}
}

// MarkStale preserves the last safe snapshot while ensuring the next consumer
// revalidates it. It is used when live deltas advance a worker's in-memory
// state before that state is checkpointed back into the cache.
func (subscription *CacheSubscription) MarkStale() {
	if subscription == nil || subscription.cache == nil || subscription.closed.Load() {
		return
	}
	cache := subscription.cache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if !subscription.currentLocked() {
		return
	}
	if entry := cache.entries[subscription.identity]; entry != nil {
		entry.freshUntil = cache.now().UTC()
		entry.refreshing = false
	}
}

func (subscription *CacheSubscription) Close() {
	if subscription == nil || subscription.cache == nil || !subscription.closed.CompareAndSwap(false, true) {
		return
	}
	cache := subscription.cache
	cache.mu.Lock()
	if cache.refs[subscription.identity] > 1 {
		cache.refs[subscription.identity]--
	} else {
		delete(cache.refs, subscription.identity)
		if cache.entries[subscription.identity] == nil {
			delete(cache.keys, subscription.identity)
			delete(cache.keyEpoch, subscription.identity)
		}
	}
	cache.mu.Unlock()
}

func (subscription *CacheSubscription) currentLocked() bool {
	cache := subscription.cache
	if _, invalid := cache.invalidatedGeneration[subscription.key.Generation]; invalid {
		return false
	}
	return subscription.generationEpoch == cache.generationEpoch[subscription.key.Generation]
}

func (cache *ResourceCache) Invalidate(key ResourceCacheKey) error {
	if cache == nil {
		return nil
	}
	canonical, identity, err := canonicalResourceCacheKey(key)
	if err != nil {
		return err
	}
	cache.mu.Lock()
	cache.keyEpoch[identity]++
	cache.removeEntryLocked(identity, false)
	cache.invalidations++
	cache.recordGaugesLocked()
	cache.mu.Unlock()
	cache.metrics.IncCounter(observability.ResourceCacheInvalidationsTotalName, cacheLabels(canonical.Topic))
	return nil
}

func (cache *ResourceCache) InvalidateGeneration(generation string) {
	if cache == nil || generation == "" {
		return
	}
	cache.mu.Lock()
	cache.generationEpoch[generation]++
	cache.invalidatedGeneration[generation] = struct{}{}
	labels := make([]map[string]string, 0)
	for identity, key := range cache.keys {
		if key.Generation != generation {
			continue
		}
		if cache.entries[identity] != nil {
			labels = append(labels, cacheLabels(key.Topic))
		}
		cache.removeEntryLocked(identity, false)
		cache.keyEpoch[identity]++
	}
	cache.invalidations += uint64(len(labels))
	cache.recordGaugesLocked()
	cache.mu.Unlock()
	for _, label := range labels {
		cache.metrics.IncCounter(observability.ResourceCacheInvalidationsTotalName, label)
	}
}

// EvictInactive removes unreferenced entries that have not been used within
// idleFor. Active subscriptions are never evicted by this lifecycle sweep.
func (cache *ResourceCache) EvictInactive(idleFor time.Duration) int {
	if cache == nil || idleFor < 0 {
		return 0
	}
	cache.mu.Lock()
	cutoff := cache.now().UTC().Add(-idleFor)
	evicted := 0
	for identity, entry := range cache.entries {
		if cache.refs[identity] == 0 && !entry.lastUsed.After(cutoff) {
			cache.removeEntryLocked(identity, true)
			evicted++
		}
	}
	cache.recordGaugesLocked()
	cache.mu.Unlock()
	return evicted
}

func (cache *ResourceCache) Stats() ResourceCacheStats {
	if cache == nil {
		return ResourceCacheStats{}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	subscriptions := 0
	for _, refs := range cache.refs {
		subscriptions += refs
	}
	return ResourceCacheStats{
		Entries: len(cache.entries), Bytes: cache.bytes, Subscriptions: subscriptions,
		Evictions: cache.evictions, Invalidations: cache.invalidations,
	}
}

func (entry *resourceCacheEntry) state(now time.Time) CacheState {
	if !now.Before(entry.expiresAt) {
		return CacheStateExpired
	}
	if entry.refreshing {
		return CacheStateRefreshing
	}
	if entry.partial {
		return CacheStatePartial
	}
	if !now.Before(entry.freshUntil) {
		return CacheStateStale
	}
	return CacheStateFresh
}

func (cache *ResourceCache) evictForLocked(exclude string, additionalBytes int, additionalEntry bool) bool {
	projectedBytes := cache.bytes + additionalBytes
	projectedEntries := len(cache.entries)
	if additionalEntry {
		projectedEntries++
	}
	if projectedBytes <= cache.maxBytes && projectedEntries <= cache.maxEntries {
		return true
	}
	type candidate struct {
		identity string
		expired  bool
		lastUsed time.Time
	}
	now := cache.now().UTC()
	candidates := make([]candidate, 0, len(cache.entries))
	for identity, entry := range cache.entries {
		if identity == exclude || cache.refs[identity] > 0 {
			continue
		}
		candidates = append(candidates, candidate{identity: identity, expired: !now.Before(entry.expiresAt), lastUsed: entry.lastUsed})
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].expired != candidates[right].expired {
			return candidates[left].expired
		}
		return candidates[left].lastUsed.Before(candidates[right].lastUsed)
	})
	availableBytes := projectedBytes
	availableEntries := projectedEntries
	for _, item := range candidates {
		availableBytes -= cache.entries[item.identity].bytes
		availableEntries--
		if availableBytes <= cache.maxBytes && availableEntries <= cache.maxEntries {
			break
		}
	}
	if availableBytes > cache.maxBytes || availableEntries > cache.maxEntries {
		return false
	}
	for _, item := range candidates {
		entry := cache.entries[item.identity]
		projectedBytes -= entry.bytes
		projectedEntries--
		cache.removeEntryLocked(item.identity, true)
		if projectedBytes <= cache.maxBytes && projectedEntries <= cache.maxEntries {
			return true
		}
	}
	return false
}

func (cache *ResourceCache) removeEntryLocked(identity string, eviction bool) {
	entry := cache.entries[identity]
	if entry == nil {
		return
	}
	cache.bytes -= entry.bytes
	delete(cache.entries, identity)
	if cache.refs[identity] == 0 {
		delete(cache.keys, identity)
		delete(cache.keyEpoch, identity)
	}
	if eviction {
		cache.evictions++
		cache.metrics.IncCounter(observability.ResourceCacheEvictionsTotalName, cacheLabels(entry.key.Topic))
	}
}

func (cache *ResourceCache) recordGaugesLocked() {
	cache.metrics.SetGauge(observability.ResourceCacheEntriesName, nil, int64(len(cache.entries)))
	cache.metrics.SetGauge(observability.ResourceCacheBytesName, nil, int64(cache.bytes))
}

func canonicalResourceCacheKey(key ResourceCacheKey) (ResourceCacheKey, string, error) {
	if key.Generation == "" || key.Context == "" || key.Scope == "" || key.GVR.Resource == "" {
		return ResourceCacheKey{}, "", validationError("resource cache binding is incomplete")
	}
	if !slices.Contains(topicGVRs[key.Topic], key.GVR) {
		return ResourceCacheKey{}, "", validationError("resource cache topic and resource do not match")
	}
	if key.Query.PageSize < 0 {
		return ResourceCacheKey{}, "", validationError("resource cache page size is invalid")
	}
	key.Selector = strings.TrimSpace(key.Selector)
	key.EffectiveOrigins = canonicalCacheStrings(key.EffectiveOrigins)
	key.Query.Filters = canonicalCacheStrings(key.Query.Filters)
	key.Query.Sort = strings.TrimSpace(key.Query.Sort)
	key.Query.Order = strings.ToLower(strings.TrimSpace(key.Query.Order))
	parts := []string{
		key.Generation, key.Context, key.Scope, string(key.Topic), key.GVR.String(), key.Namespace, key.Selector,
		strings.Join(key.EffectiveOrigins, "\x1e"), strings.Join(key.Query.Filters, "\x1e"), key.Query.Sort,
		key.Query.Order, strconv.Itoa(key.Query.PageSize), key.Query.Cursor,
	}
	return key, strings.Join(parts, "\x00"), nil
}

func canonicalCacheStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func validatedCacheSnapshot(topic Topic, snapshot WatchSnapshot) (WatchSnapshot, int, error) {
	if len(snapshot.Items) > MaximumSnapshotItems {
		return WatchSnapshot{}, 0, domainError(CodeLimitExceeded, "The resource cache snapshot is too large.", nil)
	}
	for _, item := range snapshot.Items {
		if item == nil || reflect.ValueOf(item).Kind() == reflect.Ptr && reflect.ValueOf(item).IsNil() || item.resourceTopic() != topic {
			return WatchSnapshot{}, 0, validationError("resource cache snapshot contains an object for another topic")
		}
	}
	encoded, err := json.Marshal(snapshot.Items)
	if err != nil {
		return WatchSnapshot{}, 0, validationError("resource cache snapshot cannot be encoded")
	}
	cloned, err := cloneWatchSnapshot(snapshot)
	if err != nil {
		return WatchSnapshot{}, 0, err
	}
	return cloned, len(encoded) + len(snapshot.ResourceVersion), nil
}

func cloneWatchSnapshot(snapshot WatchSnapshot) (WatchSnapshot, error) {
	result := WatchSnapshot{ResourceVersion: snapshot.ResourceVersion, Items: make([]TopicObject, 0, len(snapshot.Items))}
	for _, item := range snapshot.Items {
		cloned, err := cloneTopicObject(item)
		if err != nil {
			return WatchSnapshot{}, err
		}
		result.Items = append(result.Items, cloned)
	}
	return result, nil
}

func cloneTopicObject(item TopicObject) (TopicObject, error) {
	if item == nil {
		return nil, validationError("resource cache snapshot contains a nil object")
	}
	typeOf := reflect.TypeOf(item)
	if typeOf.Kind() == reflect.Ptr && reflect.ValueOf(item).IsNil() {
		return nil, validationError("resource cache snapshot contains a nil object")
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		return nil, validationError("resource cache object cannot be encoded")
	}
	pointer := typeOf.Kind() == reflect.Ptr
	if pointer {
		typeOf = typeOf.Elem()
	}
	target := reflect.New(typeOf)
	if err = json.Unmarshal(encoded, target.Interface()); err != nil {
		return nil, validationError("resource cache object cannot be decoded")
	}
	var value any = target.Elem().Interface()
	if pointer {
		value = target.Interface()
	}
	cloned, ok := value.(TopicObject)
	if !ok {
		return nil, validationError("resource cache object type is unsupported")
	}
	return cloned, nil
}

func cacheLabels(topic Topic) map[string]string {
	if topic == "" {
		return nil
	}
	return map[string]string{"resource": string(topic)}
}
