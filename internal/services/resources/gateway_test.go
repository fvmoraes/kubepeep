package resources

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func gatewayCondition(kind, status, reason string, generation int64) any {
	return map[string]any{"type": kind, "status": status, "reason": reason, "observedGeneration": generation}
}

func TestConvertGatewayConditionMeaning(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, kind, expected string
		status               map[string]any
	}{
		{"unobserved", "Gateway", "Unknown", map[string]any{}},
		{"accepted awaiting programming", "Gateway", "Pending", map[string]any{"conditions": []any{gatewayCondition("Accepted", "True", "Accepted", 2)}}},
		{"programmed", "Gateway", "Ready", map[string]any{"conditions": []any{gatewayCondition("Accepted", "True", "Accepted", 2), gatewayCondition("Programmed", "True", "Programmed", 2)}}},
		{"stale positive", "Gateway", "Pending", map[string]any{"conditions": []any{gatewayCondition("Programmed", "True", "Programmed", 1)}}},
		{"stale negative", "GatewayClass", "Pending", map[string]any{"conditions": []any{gatewayCondition("Accepted", "False", "InvalidParameters", 1)}}},
		{"pending is not failure", "Gateway", "Pending", map[string]any{"conditions": []any{gatewayCondition("Accepted", "True", "Accepted", 2), gatewayCondition("Programmed", "False", "Pending", 2)}}},
		{"unresolved route backend", "HTTPRoute", "Degraded", map[string]any{"parents": []any{map[string]any{"parentRef": map[string]any{"name": "edge"}, "conditions": []any{gatewayCondition("Accepted", "True", "Accepted", 2), gatewayCondition("ResolvedRefs", "False", "BackendNotFound", 2)}}}}},
		{"listener conflict", "ListenerSet", "Degraded", map[string]any{"listeners": []any{map[string]any{"name": "https", "conditions": []any{gatewayCondition("Conflicted", "True", "HostnameConflict", 2)}}}}},
		{"policy ancestors", "BackendTLSPolicy", "Ready", map[string]any{"ancestors": []any{map[string]any{"ancestorRef": map[string]any{"name": "edge"}, "conditions": []any{gatewayCondition("Accepted", "True", "Accepted", 2)}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := &unstructured.Unstructured{Object: map[string]any{"kind": test.kind, "metadata": map[string]any{"name": "edge", "generation": int64(2)}, "status": test.status}}
			got := ConvertGateway(value, time.Now())
			if got.Status != test.expected {
				t.Fatalf("status = %s, want %s", got.Status, test.expected)
			}
			if strings.HasPrefix(test.name, "stale") && (got.Conditions[0].Status != "Unknown" || *got.Conditions[0].Reason != "AwaitingReconciliation") {
				t.Fatalf("stale condition = %#v", got.Conditions)
			}
		})
	}
}

func TestConvertGatewayRelatedIdentityAndBoundedPayload(t *testing.T) {
	t.Parallel()
	value := &unstructured.Unstructured{Object: map[string]any{
		"kind": "HTTPRoute", "metadata": map[string]any{"namespace": "web", "name": "route", "annotations": map[string]any{"private": "not-in-inventory"}},
		"spec": map[string]any{"parentRefs": []any{map[string]any{"name": "edge", "namespace": "infra"}}, "rules": []any{map[string]any{"backendRefs": []any{map[string]any{"name": "api"}, map[string]any{"name": "api"}, map[string]any{"name": "custom", "group": "example.org", "kind": "Service"}}}}},
	}}
	want := []ResourceRef{{APIGroup: resourcecatalog.GatewayGroup, Kind: "Gateway", Namespace: "infra", Name: "edge"}, {Kind: "Service", Namespace: "web", Name: "api"}, {APIGroup: "example.org", Kind: "Service", Namespace: "web", Name: "custom"}}
	got := ConvertGateway(value, time.Now())
	if !reflect.DeepEqual(got.Related, want) {
		t.Fatalf("related = %#v", got.Related)
	}
	for i := 0; i < 10; i++ {
		if !reflect.DeepEqual(ConvertGateway(value, time.Now()).Related, want) {
			t.Fatal("references are not deterministic")
		}
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "not-in-inventory") {
		t.Fatal("inventory leaked arbitrary annotations")
	}
	conditions := []any{}
	for i := 0; i < maximumGatewayEntries+10; i++ {
		condition := gatewayCondition("Accepted", "True", "Accepted", 0).(map[string]any)
		condition["message"] = strings.Repeat("message", 500)
		conditions = append(conditions, condition)
	}
	value.Object["status"] = map[string]any{"conditions": conditions}
	got = ConvertGateway(value, time.Now())
	if !got.Truncated || len(got.Conditions) != maximumGatewayEntries || len(*got.Conditions[0].Message) > 2048 {
		t.Fatal("unbounded condition projection")
	}
}

func TestGatewayCollectionsSupportColumnPreferences(t *testing.T) {
	t.Parallel()
	preferences := DefaultPreferences()
	for _, resource := range resourcecatalog.GatewayResources() {
		preferences.Columns.Hidden[resource.Collection] = []string{"version"}
		if _, err := OriginsFor(Collection(resource.Collection), []string{"web"}, nil); err != nil && resource.Namespaced {
			t.Fatal(err)
		}
	}
	if err := ValidatePreferences(preferences); err != nil {
		t.Fatal(err)
	}
}
