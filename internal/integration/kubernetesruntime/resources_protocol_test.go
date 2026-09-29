package kubernetesruntime

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

func TestRelatedPrefetchWarmsBoundedPodAndEventPages(t *testing.T) {
	t.Parallel()
	client := kubefake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api-1"}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api-started"}, InvolvedObject: corev1.ObjectReference{Kind: "Deployment", Name: "api"}},
	)
	backend := &ResourceBackend{
		clients:             fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}},
		authorizer:          &allowResourceAuthorization{},
		now:                 time.Now,
		listWindowTimeout:   resources.NormalizeListWindowTimeout(0),
		listFanout:          resources.NormalizeFanout(0),
		collectionCache:     resources.NewCollectionCache(1<<20, 10, time.Minute, nil),
		resourceCache:       resources.NewResourceCache(resources.ResourceCacheConfig{MaxBytes: 1 << 20}),
		scheduler:           resources.NewRequestScheduler(4, nil),
		intelligentPrefetch: true,
		prefetchCancels:     make(map[uint64]context.CancelFunc),
	}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}
	backend.scheduleRelatedPrefetch(t.Context(), binding, resolution, resources.WorkloadDTO{Namespace: "default", Kind: "Deployment", Name: "api"})
	backend.prefetchWG.Wait()
	backend.stopPrefetch()

	var podLists, eventLists int
	for _, action := range client.Actions() {
		if action.GetVerb() != "list" {
			continue
		}
		switch action.GetResource().Resource {
		case "pods":
			podLists++
		case "events":
			eventLists++
		}
	}
	if podLists != 1 || eventLists != 1 {
		t.Fatalf("related prefetch actions: pods=%d events=%d actions=%#v", podLists, eventLists, client.Actions())
	}
}
