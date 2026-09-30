package resources

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/observability"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourceCacheIdentityIncludesAuthorizationAndQueryDimensions(t *testing.T) {
	t.Parallel()
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 16})
	firstKey := resourceCacheTestKey("gen", "payments")
	firstKey.EffectiveOrigins = []string{"team-b", "team-a", "team-a"}
	firstKey.Query.Filters = []string{"status=Running", "name=api"}
	first, err := cache.Subscribe(t.Context(), firstKey)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	token, err := first.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Commit(t.Context(), token, WatchSnapshot{ResourceVersion: "10", Items: []TopicObject{PodDTO{Namespace: "payments", Name: "api"}}}, false); err != nil {
		t.Fatal(err)
	}

	equivalentKey := firstKey
	equivalentKey.EffectiveOrigins = []string{"team-a", "team-b"}
	equivalentKey.Query.Filters = []string{"name=api", "status=Running"}
	equivalent, err := cache.Subscribe(t.Context(), equivalentKey)
	if err != nil {
		t.Fatal(err)
	}
	defer equivalent.Close()
	if view, ok := equivalent.Load(t.Context()); !ok || view.State != CacheStateFresh || view.RefCount != 2 {
		t.Fatalf("equivalent load = %#v, %v", view, ok)
	}

	tests := []struct {
		name   string
		mutate func(*ResourceCacheKey)
	}{
		{name: "generation", mutate: func(key *ResourceCacheKey) { key.Generation = "other" }},
		{name: "context", mutate: func(key *ResourceCacheKey) { key.Context = "other" }},
		{name: "scope", mutate: func(key *ResourceCacheKey) { key.Scope = "other" }},
		{name: "namespace", mutate: func(key *ResourceCacheKey) { key.Namespace = "other" }},
		{name: "selector", mutate: func(key *ResourceCacheKey) { key.Selector = "app=other" }},
		{name: "origins", mutate: func(key *ResourceCacheKey) { key.EffectiveOrigins = []string{"team-c"} }},
		{name: "filters", mutate: func(key *ResourceCacheKey) { key.Query.Filters = []string{"name=worker"} }},
		{name: "sort", mutate: func(key *ResourceCacheKey) { key.Query.Sort = "restarts" }},
		{name: "order", mutate: func(key *ResourceCacheKey) { key.Query.Order = "desc" }},
		{name: "page size", mutate: func(key *ResourceCacheKey) { key.Query.PageSize = 50 }},
		{name: "cursor", mutate: func(key *ResourceCacheKey) { key.Query.Cursor = "opaque" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key := firstKey
			test.mutate(&key)
			subscription, subscribeErr := cache.Subscribe(t.Context(), key)
			if subscribeErr != nil {
				t.Fatal(subscribeErr)
			}
			defer subscription.Close()
			if _, ok := subscription.Load(t.Context()); ok {
				t.Fatal("cache data crossed an identity boundary")
			}
		})
	}
}

func TestResourceCacheRejectsTopicGVRMismatch(t *testing.T) {
	t.Parallel()
	cache := NewResourceCache(ResourceCacheConfig{})
	key := resourceCacheTestKey("gen", "payments")
	key.GVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	if _, err := cache.Subscribe(t.Context(), key); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("secret resource accepted for Pods cache: %v", err)
	}
}

func TestResourceCacheCopiesMutableObjectsAndRejectsTopicMismatch(t *testing.T) {
	t.Parallel()
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 4})
	key := resourceCacheTestKey("gen", "payments")
	key.Topic = TopicServices
	key.GVR = schema.GroupVersionResource{Version: "v1", Resource: "services"}
	subscription, err := cache.Subscribe(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	token, err := subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	original := ServiceDTO{Namespace: "payments", Name: "api", Selector: map[string]string{"app": "api"}, ClusterIPs: []string{"10.0.0.1"}}
	if err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{original}}, false); err != nil {
		t.Fatal(err)
	}
	original.Selector["app"] = "mutated"
	view, ok := subscription.Load(t.Context())
	if !ok {
		t.Fatal("cache miss")
	}
	loaded := view.Snapshot.Items[0].(ServiceDTO)
	loaded.Selector["app"] = "consumer-mutation"
	loaded.ClusterIPs[0] = "127.0.0.1"
	view, ok = subscription.Load(t.Context())
	if !ok {
		t.Fatal("cache miss after consumer mutation")
	}
	reloaded := view.Snapshot.Items[0].(ServiceDTO)
	if reloaded.Selector["app"] != "api" || reloaded.ClusterIPs[0] != "10.0.0.1" {
		t.Fatalf("cached object was aliased: %#v", reloaded)
	}

	token, err = subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "wrong-topic"}}}, false)
	if ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("topic mismatch = %v", err)
	}
}

func TestResourceCacheFreshnessLifecycle(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	cache := NewResourceCache(ResourceCacheConfig{
		MaxBytes: 1 << 20, MaxEntries: 4, FreshFor: 10 * time.Second, MaxStale: time.Minute,
		Now: func() time.Time { return now },
	})
	subscription, err := cache.Subscribe(t.Context(), resourceCacheTestKey("gen", "payments"))
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	token, err := subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "api"}}}, false); err != nil {
		t.Fatal(err)
	}
	assertCacheState(t, subscription, CacheStateFresh, true)
	now = now.Add(11 * time.Second)
	assertCacheState(t, subscription, CacheStateStale, true)
	token, err = subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertCacheState(t, subscription, CacheStateRefreshing, true)
	subscription.AbortRefresh(token)
	assertCacheState(t, subscription, CacheStateStale, true)
	now = now.Add(time.Minute)
	assertCacheState(t, subscription, CacheStateExpired, false)

	token, err = subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "api"}}}, true); err != nil {
		t.Fatal(err)
	}
	assertCacheState(t, subscription, CacheStatePartial, false)
}

func TestResourceCacheInvalidationFencesOldWrites(t *testing.T) {
	t.Parallel()
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 4})
	key := resourceCacheTestKey("gen", "payments")
	subscription, err := cache.Subscribe(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	token, err := subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cache.InvalidateGeneration("gen")
	err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "late"}}}, false)
	if !errors.Is(err, ErrCacheWriteFenced) {
		t.Fatalf("late commit = %v", err)
	}
	subscription.Close()
	_, err = cache.Subscribe(t.Context(), key)
	if !errors.Is(err, ErrCacheGenerationInvalidated) || ErrorCodeOf(err) != CodeGenerationChanged {
		t.Fatalf("invalid generation subscribe = %v", err)
	}

	freshKey := resourceCacheTestKey("fresh-generation", "payments")
	fresh, err := cache.Subscribe(t.Context(), freshKey)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	staleToken, err := fresh.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = cache.Invalidate(freshKey); err != nil {
		t.Fatal(err)
	}
	if err = fresh.Commit(t.Context(), staleToken, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "late"}}}, false); !errors.Is(err, ErrCacheWriteFenced) {
		t.Fatalf("key-fenced commit = %v", err)
	}
	currentToken, err := fresh.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = fresh.Commit(t.Context(), currentToken, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "current"}}}, false); err != nil {
		t.Fatal(err)
	}
}

func TestResourceCacheClosedSubscriptionCannotCommit(t *testing.T) {
	t.Parallel()
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 4})
	subscription, err := cache.Subscribe(t.Context(), resourceCacheTestKey("gen", "payments"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	subscription.Close()
	if err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "late"}}}, false); !errors.Is(err, ErrCacheWriteFenced) {
		t.Fatalf("closed subscription commit = %v", err)
	}
	if stats := cache.Stats(); stats.Entries != 0 || stats.Subscriptions != 0 {
		t.Fatalf("closed subscription retained data: %#v", stats)
	}
}

func TestResourceCacheNewerRefreshFencesOlderResponse(t *testing.T) {
	t.Parallel()
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 4})
	subscription, err := cache.Subscribe(t.Context(), resourceCacheTestKey("gen", "payments"))
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	older, err := subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	newer, err := subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = subscription.Commit(t.Context(), newer, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "newer"}}}, false); err != nil {
		t.Fatal(err)
	}
	if err = subscription.Commit(t.Context(), older, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "older"}}}, false); !errors.Is(err, ErrCacheWriteFenced) {
		t.Fatalf("older response = %v", err)
	}
	view, ok := subscription.Load(t.Context())
	if !ok || view.Snapshot.Items[0].(PodDTO).Name != "newer" {
		t.Fatalf("newer snapshot overwritten: %#v, %v", view, ok)
	}
}

func TestResourceCacheEvictsOnlyInactiveEntriesUnderBudgetPressure(t *testing.T) {
	t.Parallel()
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 2})
	firstKey := resourceCacheTestKey("gen", "first")
	first := storeCacheSnapshot(t, cache, firstKey)
	first.Close()
	second := storeCacheSnapshot(t, cache, resourceCacheTestKey("gen", "second"))
	defer second.Close()
	third := storeCacheSnapshot(t, cache, resourceCacheTestKey("gen", "third"))
	defer third.Close()
	if stats := cache.Stats(); stats.Entries != 2 || stats.Evictions != 1 || stats.Subscriptions != 2 {
		t.Fatalf("stats after eviction = %#v", stats)
	}
	firstAgain, err := cache.Subscribe(t.Context(), firstKey)
	if err != nil {
		t.Fatal(err)
	}
	defer firstAgain.Close()
	if _, ok := firstAgain.Load(t.Context()); ok {
		t.Fatal("least-recent inactive entry was not evicted")
	}

	fourth, err := cache.Subscribe(t.Context(), resourceCacheTestKey("gen", "fourth"))
	if err != nil {
		t.Fatal(err)
	}
	defer fourth.Close()
	token, err := fourth.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	err = fourth.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{PodDTO{Name: "fourth"}}}, false)
	if ErrorCodeOf(err) != CodeLimitExceeded {
		t.Fatalf("active-entry pressure = %v", err)
	}
}

func TestResourceCacheEvictsIdleAndExportsBoundedMetrics(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	metrics := observability.NewRegistry()
	cache := NewResourceCache(ResourceCacheConfig{
		MaxBytes: 1 << 20, MaxEntries: 4, Now: func() time.Time { return now }, Metrics: metrics,
	})
	key := resourceCacheTestKey("sensitive-generation", "sensitive-namespace")
	subscription := storeCacheSnapshot(t, cache, key)
	if _, ok := subscription.Load(t.Context()); !ok {
		t.Fatal("cache miss")
	}
	subscription.Close()
	now = now.Add(time.Minute)
	if evicted := cache.EvictInactive(30 * time.Second); evicted != 1 {
		t.Fatalf("evicted = %d", evicted)
	}
	rendered := metrics.Render()
	for _, metric := range []string{
		observability.ResourceCacheEntriesName,
		observability.ResourceCacheBytesName,
		observability.ResourceCacheHitsTotalName,
		observability.ResourceCacheEvictionsTotalName,
	} {
		if !strings.Contains(rendered, metric) {
			t.Fatalf("metric %q missing from:\n%s", metric, rendered)
		}
	}
	if strings.Contains(rendered, "sensitive-generation") || strings.Contains(rendered, "sensitive-namespace") {
		t.Fatalf("sensitive cache identity leaked into metrics:\n%s", rendered)
	}
}

func assertCacheState(t *testing.T, subscription *CacheSubscription, want CacheState, complete bool) {
	t.Helper()
	view, ok := subscription.Load(t.Context())
	if !ok || view.State != want || view.Complete != complete {
		t.Fatalf("cache state = %#v, %v; want %s complete=%v", view, ok, want, complete)
	}
}

func storeCacheSnapshot(t *testing.T, cache *ResourceCache, key ResourceCacheKey) *CacheSubscription {
	t.Helper()
	subscription, err := cache.Subscribe(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	token, err := subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{PodDTO{Namespace: key.Namespace, Name: "api"}}}, false); err != nil {
		t.Fatal(err)
	}
	return subscription
}

func resourceCacheTestKey(generation, namespace string) ResourceCacheKey {
	return ResourceCacheKey{WatchKey: WatchKey{
		Generation: generation, Context: "ctx", Scope: "scope", Topic: TopicPods,
		GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Namespace: namespace, Selector: "app=api",
	}}
}
