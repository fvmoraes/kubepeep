package resources

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestLocalIndexStaysInsideCacheBudgetAcrossTwoHundredNamespaces(t *testing.T) {
	t.Parallel()
	const budget = 4 << 20
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: budget, MaxEntries: 256})
	subscriptions := make([]*CacheSubscription, 0, 200)
	expectedNamespaces := make([]string, 0, 200)
	defer func() {
		for _, subscription := range subscriptions {
			subscription.Close()
		}
	}()
	for namespaceIndex := 0; namespaceIndex < 200; namespaceIndex++ {
		namespace := fmt.Sprintf("team-%03d", namespaceIndex)
		expectedNamespaces = append(expectedNamespaces, namespace)
		key := resourceCacheTestKey("gen-200", namespace)
		key.Selector = ""
		subscription, err := cache.Subscribe(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}
		subscriptions = append(subscriptions, subscription)
		token, err := subscription.BeginRefresh(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		items := make([]TopicObject, 0, 10)
		for podIndex := 0; podIndex < 10; podIndex++ {
			items = append(items, PodDTO{Namespace: namespace, Name: fmt.Sprintf("portal-%02d", podIndex), Status: "Running", Labels: map[string]string{"app": "portal"}})
		}
		if err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: items}, false); err != nil {
			t.Fatal(err)
		}
	}
	stats := cache.Stats()
	if stats.Bytes > budget || stats.Entries != 200 {
		t.Fatalf("cache stats exceeded the configured budget: %#v", stats)
	}
	index := cache.LocalIndex("gen-200", expectedNamespaces...)
	if len(index.Resources) != 2_000 || len(index.NamespaceCount) != 200 {
		t.Fatalf("index size = resources %d namespaces %d", len(index.Resources), len(index.NamespaceCount))
	}
	if got := index.Search("portal", 200); len(got) != 200 {
		t.Fatalf("bounded search returned %d resources", len(got))
	}
}

func TestLocalIndexIsGenerationScopedAndReportsMissingCoverage(t *testing.T) {
	t.Parallel()
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 8})
	key := resourceCacheTestKey("gen-current", "portal")
	key.Selector = ""
	subscription, err := cache.Subscribe(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	token, err := subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{PodDTO{Namespace: "portal", Name: "portal-api", Status: "Running", Labels: map[string]string{"app": "portal"}}}}, false); err != nil {
		t.Fatal(err)
	}

	otherKey := resourceCacheTestKey("gen-old", "portal")
	otherKey.Selector = ""
	other, err := cache.Subscribe(t.Context(), otherKey)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	otherToken, err := other.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = other.Commit(t.Context(), otherToken, WatchSnapshot{Items: []TopicObject{PodDTO{Namespace: "portal", Name: "must-not-leak"}}}, false); err != nil {
		t.Fatal(err)
	}
	selectorKey := resourceCacheTestKey("gen-current", "portal")
	selectorKey.Selector = "app=hidden"
	selector, err := cache.Subscribe(t.Context(), selectorKey)
	if err != nil {
		t.Fatal(err)
	}
	defer selector.Close()
	selectorToken, err := selector.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = selector.Commit(t.Context(), selectorToken, WatchSnapshot{Items: []TopicObject{PodDTO{Namespace: "portal", Name: "must-not-index"}}}, false); err != nil {
		t.Fatal(err)
	}

	index := cache.LocalIndex("gen-current", "portal")
	if len(index.Resources) != 1 || index.Resources[0].Name != "portal-api" {
		t.Fatalf("generation-scoped resources = %#v", index.Resources)
	}
	if got := index.Search("portal", 10); len(got) != 1 || got[0].Name != "portal-api" {
		t.Fatalf("search = %#v", got)
	}
	coverage := make(map[Topic]IndexCoverage, len(index.Coverage))
	for _, item := range index.Coverage {
		coverage[item.Topic] = item
	}
	if !coverage[TopicPods].Complete || coverage[TopicPods].State != CacheStateFresh {
		t.Fatalf("pod coverage = %#v", coverage[TopicPods])
	}
	if coverage[TopicEvents].Complete || coverage[TopicEvents].State != CacheStateExpired {
		t.Fatalf("missing event coverage was presented as complete: %#v", coverage[TopicEvents])
	}
}

func TestIndexCoverageRequiresEveryWorkloadGVRForEveryExpectedNamespace(t *testing.T) {
	t.Parallel()
	origins := map[string]map[schema.GroupVersionResource]struct{}{
		"portal": {},
	}
	for _, gvr := range TopicGVRs(TopicWorkloads)[:len(TopicGVRs(TopicWorkloads))-1] {
		origins["portal"][gvr] = struct{}{}
	}
	if indexOriginsComplete(TopicWorkloads, origins, []string{"portal"}) {
		t.Fatal("partial workload GVR coverage was marked complete")
	}
	for _, gvr := range TopicGVRs(TopicWorkloads) {
		origins["portal"][gvr] = struct{}{}
	}
	if !indexOriginsComplete(TopicWorkloads, origins, []string{"portal"}) {
		t.Fatal("complete workload GVR coverage was not recognized")
	}
	if indexOriginsComplete(TopicWorkloads, origins, []string{"portal", "restricted"}) {
		t.Fatal("missing expected namespace coverage was marked complete")
	}
}

func TestInvestigationResolvesOwnerServiceStorageConfigAndEventsLocally(t *testing.T) {
	t.Parallel()
	index := LocalIndexSnapshot{
		Generation: "gen",
		Coverage:   []IndexCoverage{{Topic: TopicPods, State: CacheStateFresh, Complete: true}},
		Pods:       []PodDTO{{Namespace: "portal", Name: "portal-api-abc", Status: "Running", Labels: map[string]string{"app": "portal"}, Owner: &OwnerDTO{Kind: "ReplicaSet", Name: "portal-api-rs"}, ConfigMaps: []string{"portal-config"}, PVCs: []string{"portal-data"}}},
		Workloads: []WorkloadDTO{
			{Namespace: "portal", Kind: "ReplicaSet", Name: "portal-api-rs", Owner: &OwnerDTO{Kind: "Deployment", Name: "portal-api"}},
			{Namespace: "portal", Kind: "Deployment", Name: "portal-api"},
		},
		Services:       []ServiceDTO{{Namespace: "portal", Name: "portal", Selector: map[string]string{"app": "portal"}}},
		EndpointSlices: []EndpointSliceDTO{{Namespace: "portal", Name: "portal-xyz", ServiceName: "portal"}},
		ConfigMaps:     []ConfigMapListDTO{{Namespace: "portal", Name: "portal-config"}},
		PVCs:           []PersistentVolumeClaimDTO{{Namespace: "portal", Name: "portal-data", Status: "Bound"}},
		Events:         []EventDTO{{Namespace: "portal", Name: "portal-warning", ObjectKind: "Pod", ObjectName: "portal-api-abc", Reason: "Unhealthy", Type: "Warning"}},
	}
	index.Resources = []IndexedResource{{Kind: "Pod", Namespace: "portal", Name: "portal-api-abc", Status: "Running"}}

	got := index.Investigate("Pod", "portal", "portal-api-abc")
	if len(got.OwnerChain) != 2 || got.OwnerChain[0].Kind != "ReplicaSet" || got.OwnerChain[1].Kind != "Deployment" {
		t.Fatalf("owner chain = %#v", got.OwnerChain)
	}
	if len(got.Services) != 1 || len(got.EndpointSlices) != 1 || len(got.ConfigMaps) != 1 || len(got.PVCs) != 1 || len(got.Events) != 1 {
		t.Fatalf("investigation relationships = services:%#v slices:%#v configmaps:%#v pvcs:%#v events:%#v", got.Services, got.EndpointSlices, got.ConfigMaps, got.PVCs, got.Events)
	}
}

func TestLocalIndexAcceptsPVCWatchTopic(t *testing.T) {
	t.Parallel()
	cache := NewResourceCache(ResourceCacheConfig{MaxBytes: 1 << 20, MaxEntries: 2})
	key := resourceCacheTestKey("gen", "portal")
	key.Selector = ""
	key.Topic = TopicPVCs
	key.GVR = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}
	subscription, err := cache.Subscribe(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	token, err := subscription.BeginRefresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = subscription.Commit(t.Context(), token, WatchSnapshot{Items: []TopicObject{PersistentVolumeClaimDTO{Namespace: "portal", Name: "data", Status: "Pending"}}}, false); err != nil {
		t.Fatal(err)
	}
	index := cache.LocalIndex("gen")
	if len(index.PVCs) != 1 || index.PVCs[0].Name != "data" {
		t.Fatalf("PVC index = %#v", index.PVCs)
	}
}
