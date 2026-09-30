package kubernetesruntime

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/fvmoraes/kubepeep/internal/observability"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

func TestListMetricsRecordStrategyDurationAndItems(t *testing.T) {
	client := kubefake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "alpha", Name: "api"}})
	registry := observability.NewRegistry()
	backend := &ResourceBackend{
		clients:    fixedResourceClientProvider{set: resourceClientSet{kubernetes: client}},
		authorizer: &allowResourceAuthorization{},
		now:        time.Now,
		metrics:    registry,
	}
	result, err := backend.ListPods(
		context.Background(),
		namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"},
		namespaces.ScopeResolution{ScopeName: "all", Namespaces: []string{"alpha"}, PreferGlobal: true},
		resources.ListOptions{Limit: 10},
		nil,
	)
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("list result=%#v err=%v", result, err)
	}
	rendered := registry.Render()
	for _, want := range []string{
		`kubepeep_resource_lists_total{resource="pods",strategy="global"} 1`,
		`kubepeep_resource_list_items_received_total{resource="pods"} 1`,
		`kubepeep_resource_list_items_returned_total{resource="pods"} 1`,
		`kubepeep_resource_list_duration_nanoseconds_total{resource="pods",strategy="global"}`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q: %s", want, rendered)
		}
	}
}
