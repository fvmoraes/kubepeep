package kubernetes

import (
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func yamlFixture(resource resourcecatalog.Resource) (*unstructured.Unstructured, actions.MutationTarget) {
	namespace := ""
	if resource.Namespaced {
		namespace = "payments"
	}
	object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": resource.APIVersion(), "kind": resource.Kind, "metadata": map[string]any{"name": "sample", "uid": "uid-sample", "resourceVersion": "17"}, "data": map[string]any{"token": "c2VjcmV0"}, "spec": map[string]any{"preserveUnknownField": "unchanged"}}}
	object.SetNamespace(namespace)
	object.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "fixture"}})
	return object, actions.MutationTarget{Kind: resource.Kind, Name: "sample", Namespace: namespace}
}

func TestResourceYAMLRoundTripEveryRegisteredKind(t *testing.T) {
	t.Parallel()
	for _, resource := range resourcecatalog.All() {
		t.Run(resource.Collection, func(t *testing.T) {
			object, target := yamlFixture(resource)
			dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
			// The fake's English plural guessing produces "gatewaies". Register
			// objects under the API catalog's actual resource, just like a server.
			gvr := schema.GroupVersionResource{Group: resource.Group, Version: resource.Version, Resource: resource.Resource}
			if err := dynamic.Tracker().Create(gvr, object, target.Namespace); err != nil {
				t.Fatal(err)
			}
			client := &ActionClient{dynamic: dynamic}
			document, err := client.ReadResourceYAML(t.Context(), target, resource.Collection)
			if err != nil || strings.Contains(document, "managedFields") || !strings.Contains(document, "preserveUnknownField") {
				t.Fatalf("lossy YAML read: %v", err)
			}
			dynamic.PrependReactor("update", resource.Resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
				update := action.(clienttesting.UpdateAction)
				value := update.GetObject().(*unstructured.Unstructured)
				if value.GetUID() != types.UID("uid-sample") || value.GetResourceVersion() != "17" || action.GetNamespace() != target.Namespace || action.GetResource().Group != resource.Group || action.(interface{ GetUpdateOptions() metav1.UpdateOptions }).GetUpdateOptions().FieldValidation != "Strict" {
					t.Fatalf("wrong update/preconditions: %#v", action)
				}
				if value.Object["spec"].(map[string]any)["preserveUnknownField"] != "changed" || value.Object["data"].(map[string]any)["token"] != "c2VjcmV0" {
					t.Fatal("lost round-trip fields")
				}
				value.SetResourceVersion("18")
				return true, value, nil
			})
			result, err := client.UpdateResourceYAML(t.Context(), actions.ResourceYAMLCommand{Collection: resource.Collection, DeploymentYAMLCommand: actions.DeploymentYAMLCommand{Target: target, ExpectedUID: "uid-sample", ExpectedResourceVersion: "17", YAML: strings.Replace(document, "unchanged", "changed", 1)}})
			if err != nil || result.ResourceVersion != "18" {
				t.Fatalf("update failed: %v", err)
			}
		})
	}
}

func TestResourceYAMLRejectsMalformedOrRetargetedDocuments(t *testing.T) {
	resource, _ := resourcecatalog.Lookup("secrets")
	object, target := yamlFixture(resource)
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)
	client := &ActionClient{dynamic: dynamic}
	baseline, err := client.ReadResourceYAML(t.Context(), target, "secrets")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, document string }{
		{"namespace", strings.Replace(baseline, "namespace: payments", "namespace: outside", 1)},
		{"name", strings.Replace(baseline, "name: sample", "name: other", 1)},
		{"kind", strings.Replace(baseline, "kind: Secret", "kind: ConfigMap", 1)},
		{"api version", strings.Replace(baseline, "apiVersion: v1", "apiVersion: v2", 1)},
		{"uid", strings.Replace(baseline, "uid-sample", "uid-other", 1)},
		{"version", strings.Replace(baseline, `"17"`, `"18"`, 1)},
		{"multiple", baseline + "---\nkind: Secret\n"},
		{"duplicate", baseline + "kind: Secret\n"},
		{"invalid", "data: [PRIVATE_VALUE"},
		{"null", "null"},
		{"oversized", strings.Repeat("x", actions.MaximumResourceYAMLBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			dynamic.ClearActions()
			_, err := client.UpdateResourceYAML(t.Context(), actions.ResourceYAMLCommand{Collection: "secrets", DeploymentYAMLCommand: actions.DeploymentYAMLCommand{Target: target, ExpectedUID: "uid-sample", ExpectedResourceVersion: "17", YAML: test.document}})
			if !apierrors.IsBadRequest(err) || len(dynamic.Actions()) != 0 || strings.Contains(err.Error(), "PRIVATE_VALUE") {
				t.Fatalf("unsafe document: %v", err)
			}
		})
	}
	for _, upstream := range []error{apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, "sample", nil), apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "sample", nil), apierrors.NewBadRequest("PRIVATE_VALUE")} {
		dynamic.PrependReactor("update", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) { return true, nil, upstream })
		_, err := client.UpdateResourceYAML(t.Context(), actions.ResourceYAMLCommand{Collection: "secrets", DeploymentYAMLCommand: actions.DeploymentYAMLCommand{Target: target, ExpectedUID: "uid-sample", ExpectedResourceVersion: "17", YAML: baseline}})
		if err == nil || apierrors.IsConflict(err) != apierrors.IsConflict(upstream) || apierrors.IsForbidden(err) != apierrors.IsForbidden(upstream) || strings.Contains(err.Error(), "PRIVATE_VALUE") {
			t.Fatalf("lost error classification or leaked value: %v", err)
		}
	}
}

func TestGatewayYAMLRetainsServedVersionForUpdate(t *testing.T) {
	t.Parallel()
	for _, registered := range resourcecatalog.GatewayResources() {
		for _, version := range resourcecatalog.Versions(registered)[1:] {
			t.Run(registered.Kind+"/"+version, func(t *testing.T) {
				t.Parallel()
				resource := registered
				resource.Version = version
				object, target := yamlFixture(resource)
				dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
				gvr := schema.GroupVersionResource{Group: resource.Group, Version: version, Resource: resource.Resource}
				if err := dynamic.Tracker().Create(gvr, object, target.Namespace); err != nil {
					t.Fatal(err)
				}
				client := &ActionClient{dynamic: dynamic}
				document, err := client.ReadResourceYAML(t.Context(), target, resource.Collection)
				if err != nil || !strings.Contains(document, resource.APIVersion()) {
					t.Fatalf("read served version: %q / %v", document, err)
				}
				dynamic.ClearActions()
				_, err = client.UpdateResourceYAML(t.Context(), actions.ResourceYAMLCommand{Collection: resource.Collection, DeploymentYAMLCommand: actions.DeploymentYAMLCommand{Target: target, ExpectedUID: "uid-sample", ExpectedResourceVersion: "17", YAML: document}})
				if err != nil || len(dynamic.Actions()) != 1 || dynamic.Actions()[0].GetResource() != gvr || dynamic.Actions()[0].GetVerb() != "update" {
					t.Fatalf("update switched API version: %v / %#v", err, dynamic.Actions())
				}
			})
		}
	}
}
