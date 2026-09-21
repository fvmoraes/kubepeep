package resources

import (
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/observability"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

func TestCollectionCacheReauthorizesAndFencesWrites(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	cache := NewCollectionCache(1<<20, 4, 30*time.Second, func() time.Time { return now })
	checker := &fakeAuthorization{decisions: map[string]authorization.Decision{"ns": authorization.DecisionAllowed}}
	origin := Origin{Namespace: "ns", Version: "v1", Resource: "pods"}
	result := ListResult[PodDTO]{
		Items:  []PodDTO{{Namespace: "ns", Name: "api"}},
		Cursor: &CompositeCursor[PodDTO]{Origins: []OriginCursor[PodDTO]{{Origin: origin}}},
		Page:   PageDTO{Complete: true, FilterScope: FilterScopeCollection},
	}
	token, ok := cache.Begin("gen", "query")
	if !ok || !StoreCollectionPage(cache, token, CollectionPods, result) {
		t.Fatal("failed to store authorized page")
	}
	loaded, ok := LoadCollectionPage[PodDTO](t.Context(), cache, "query", "gen", checker)
	if !ok || len(loaded.Items) != 1 {
		t.Fatalf("cache hit=%v result=%#v", ok, loaded)
	}
	loaded.Items[0].Name = "mutated"
	again, ok := LoadCollectionPage[PodDTO](t.Context(), cache, "query", "gen", checker)
	if !ok || again.Items[0].Name != "api" {
		t.Fatal("cached page was mutable")
	}
	checker.mu.Lock()
	checker.decisions["ns"] = authorization.DecisionDenied
	checker.mu.Unlock()
	if _, ok := LoadCollectionPage[PodDTO](t.Context(), cache, "query", "gen", checker); ok || cache.Stats().Entries != 0 {
		t.Fatal("denied page was served or retained")
	}
	checker.mu.Lock()
	checker.decisions["ns"] = authorization.DecisionAllowed
	checker.mu.Unlock()
	token, _ = cache.Begin("gen", "query")
	cache.InvalidateCollection("gen", CollectionPods)
	if StoreCollectionPage(cache, token, CollectionPods, result) {
		t.Fatal("event invalidation allowed late repopulation")
	}
	token, _ = cache.Begin("gen", "query")
	cache.SwitchGeneration("next")
	if StoreCollectionPage(cache, token, CollectionPods, result) {
		t.Fatal("generation switch allowed late repopulation")
	}
}

func TestCollectionCacheBudgetExpiryAndSecretExclusion(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	cache := NewCollectionCache(1<<20, 1, time.Second, func() time.Time { return now })
	checker := &fakeAuthorization{}
	result := ListResult[PodDTO]{Cursor: &CompositeCursor[PodDTO]{Origins: []OriginCursor[PodDTO]{{Origin: Origin{Namespace: "ns", Version: "v1", Resource: "pods"}}}}}
	first, _ := cache.Begin("gen", "first")
	if !StoreCollectionPage(cache, first, CollectionPods, result) {
		t.Fatal("first page rejected")
	}
	second, _ := cache.Begin("gen", "second")
	if !StoreCollectionPage(cache, second, CollectionPods, result) || cache.Stats().Entries != 1 {
		t.Fatal("LRU budget not enforced")
	}
	if _, ok := LoadCollectionPage[PodDTO](t.Context(), cache, "first", "gen", checker); ok {
		t.Fatal("evicted page returned")
	}
	now = now.Add(2 * time.Second)
	if _, ok := LoadCollectionPage[PodDTO](t.Context(), cache, "second", "gen", checker); ok {
		t.Fatal("expired page returned")
	}
	secret, _ := cache.Begin("gen", "secret")
	if StoreCollectionPage(cache, secret, CollectionSecrets, ListResult[SecretMetadataDTO]{Cursor: &CompositeCursor[SecretMetadataDTO]{}}) {
		t.Fatal("Secret metadata entered collection cache")
	}
	if StoreCollectionPage(cache, secret, CollectionPods, ListResult[SecretMetadataDTO]{Items: []SecretMetadataDTO{{}}, Cursor: &CompositeCursor[SecretMetadataDTO]{}}) {
		t.Fatal("Secret metadata entered cache under a forged collection")
	}
}

func TestCollectionCacheMetricsCountRealEvictionsAndInvalidations(t *testing.T) {
	t.Parallel()
	metrics := observability.NewRegistry()
	cache := NewCollectionCacheWithMetrics(1<<20, 1, time.Minute, nil, metrics)
	result := ListResult[PodDTO]{Cursor: &CompositeCursor[PodDTO]{Origins: []OriginCursor[PodDTO]{{Origin: Origin{Namespace: "ns", Version: "v1", Resource: "pods"}}}}}
	first, _ := cache.Begin("gen", "first")
	if !StoreCollectionPage(cache, first, CollectionPods, result) {
		t.Fatal("first store failed")
	}
	second, _ := cache.Begin("gen", "second")
	if !StoreCollectionPage(cache, second, CollectionPods, result) {
		t.Fatal("second store failed")
	}
	cache.InvalidateCollection("gen", CollectionPods)
	rendered := metrics.Render()
	for _, series := range []string{
		"kubepeep_collection_cache_entries 0",
		"kubepeep_collection_cache_bytes 0",
		"kubepeep_collection_cache_evictions_total{resource=\"pods\"} 1",
		"kubepeep_collection_cache_invalidations_total{resource=\"pods\"} 1",
	} {
		if !strings.Contains(rendered, series) {
			t.Fatalf("missing real metric %q in %s", series, rendered)
		}
	}
}
