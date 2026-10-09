package kubernetesruntime

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func gatewayFake(resource resourcecatalog.Resource) *dynamicfake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{}
	for _, version := range resourcecatalog.Versions(resource) {
		kinds[schema.GroupVersionResource{Group: resource.Group, Version: version, Resource: resource.Resource}] = resource.Kind + "List"
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds)
}

func gatewayObject(resource resourcecatalog.Resource, version, namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{"apiVersion": resource.Group + "/" + version, "kind": resource.Kind, "metadata": map[string]any{"namespace": namespace, "name": name, "uid": "uid-" + name, "resourceVersion": "27"}}}
}

func TestGatewayEveryKindListAndDetailFallsBackOnlyOnMissingAPI(t *testing.T) {
	t.Parallel()
	for _, resource := range resourcecatalog.GatewayResources() {
		for _, version := range resourcecatalog.Versions(resource) {
			t.Run(resource.Kind+"/"+version, func(t *testing.T) {
				t.Parallel()
				namespace := ""
				if resource.Namespaced {
					namespace = "web"
				}
				client := gatewayFake(resource)
				value := gatewayObject(resource, version, namespace, "edge")
				client.PrependReactor("*", resource.Resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
					if action.GetNamespace() != namespace {
						t.Errorf("namespace = %q, want %q", action.GetNamespace(), namespace)
					}
					if action.GetResource().Version != version {
						return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), "")
					}
					if action.GetVerb() == "list" {
						return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*value}}, nil
					}
					if action.(clienttesting.GetAction).GetName() != "edge" {
						t.Error("GET did not target exact object")
					}
					return true, value.DeepCopy(), nil
				})
				authorizer := &allowResourceAuthorization{}
				backend := &ResourceBackend{clients: fixedResourceClientProvider{resourceClientSet{dynamic: client}}, authorizer: authorizer, now: time.Now}
				binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}
				resolution := namespaces.ScopeResolution{ScopeName: "web", Namespaces: []string{"web"}}
				page, err := backend.ListGatewayResources(t.Context(), binding, resolution, resource.Collection, resources.ListOptions{Limit: 10}, nil)
				if err != nil || len(page.Items) != 1 || page.Items[0].APIVersion != resource.Group+"/"+version {
					t.Fatalf("list: %#v / %v", page, err)
				}
				detail, err := backend.GetGatewayResource(t.Context(), binding, resolution, resource.Collection, namespace, "edge")
				if err != nil || detail.Name != "edge" || detail.APIVersion != resource.Group+"/"+version {
					t.Fatalf("detail: %#v / %v", detail, err)
				}
				for _, key := range authorizer.keys {
					if key.Resource != resource.Resource || key.APIGroup != resource.Group || key.Namespace != namespace {
						t.Fatalf("wrong authorization identity: %#v", key)
					}
				}
			})
		}
	}
}

func TestGatewayNativePaginationCarriesContinuationAndSelector(t *testing.T) {
	t.Parallel()
	resource, _ := resourcecatalog.Lookup("http-routes")
	client := gatewayFake(resource)
	var mu sync.Mutex
	tokens := []string{}
	client.PrependReactor("list", resource.Resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
		options := action.(interface{ GetListOptions() metav1.ListOptions }).GetListOptions()
		mu.Lock()
		tokens = append(tokens, options.Continue)
		mu.Unlock()
		if options.FieldSelector != "metadata.name=edge" {
			t.Errorf("field selector = %q", options.FieldSelector)
		}
		name, next := "edge", "native-next"
		if options.Continue != "" {
			name, next = "edge-2", ""
		}
		result := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*gatewayObject(resource, "v1", "web", name)}}
		result.SetContinue(next)
		result.SetResourceVersion("27")
		return true, result, nil
	})
	backend := &ResourceBackend{clients: fixedResourceClientProvider{resourceClientSet{dynamic: client}}, authorizer: &allowResourceAuthorization{}, now: time.Now}
	binding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}
	resolution := namespaces.ScopeResolution{ScopeName: "web", Namespaces: []string{"web"}}
	options := resources.ListOptions{Limit: 1, FieldSelector: "metadata.name=edge"}
	first, err := backend.ListGatewayResources(t.Context(), binding, resolution, resource.Collection, options, nil)
	if err != nil || first.Cursor == nil || len(first.Items) != 1 || first.Items[0].Name != "edge" {
		t.Fatalf("first page = %#v / %v", first, err)
	}
	second, err := backend.ListGatewayResources(t.Context(), binding, resolution, resource.Collection, options, first.Cursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].Name != "edge-2" {
		t.Fatalf("second page = %#v / %v", second, err)
	}
	if fmt.Sprint(tokens) != "[ native-next]" {
		t.Fatalf("continuations = %v", tokens)
	}
}

func TestGatewayMissingCRDAndDeniedReadsAreDistinguished(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		err      error
		decision authorization.Decision
		expected resources.ErrorCode
		calls    int
	}{
		{"CRD absent", apierrors.NewNotFound(schema.GroupResource{Group: resourcecatalog.GatewayGroup, Resource: "gatewayclasses"}, ""), authorization.DecisionAllowed, resources.CodeFeatureUnavailable, 4},
		{"server denied", apierrors.NewForbidden(schema.GroupResource{Resource: "gatewayclasses"}, "", errors.New("denied")), authorization.DecisionAllowed, resources.CodeForbidden, 1},
		{"review denied", nil, authorization.DecisionDenied, resources.CodeForbidden, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			resource, _ := resourcecatalog.Lookup("gateway-classes")
			client := gatewayFake(resource)
			calls := 0
			client.PrependReactor("list", resource.Resource, func(clienttesting.Action) (bool, runtime.Object, error) { calls++; return true, nil, test.err })
			backend := &ResourceBackend{clients: fixedResourceClientProvider{resourceClientSet{dynamic: client}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{resource.Group + "/" + resource.Resource + "/list": test.decision}}, now: time.Now}
			_, err := backend.ListGatewayResources(t.Context(), namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}, namespaces.ScopeResolution{}, resource.Collection, resources.ListOptions{Limit: 10}, nil)
			if resources.ErrorCodeOf(err) != test.expected || calls != test.calls {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
		})
	}
}

func TestGatewayDetailDenialNeverReachesKubernetes(t *testing.T) {
	t.Parallel()
	resource, _ := resourcecatalog.Lookup("gateways")
	client := gatewayFake(resource)
	backend := &ResourceBackend{clients: fixedResourceClientProvider{resourceClientSet{dynamic: client}}, authorizer: &selectiveResourceAuthorization{denied: map[string]authorization.Decision{resource.Group + "/" + resource.Resource + "/get": authorization.DecisionDenied}}, now: time.Now}
	_, err := backend.GetGatewayResource(t.Context(), namespaces.SelectionBinding{Generation: "gen"}, namespaces.ScopeResolution{Namespaces: []string{"web"}}, resource.Collection, "outside", "edge")
	if resources.ErrorCodeOf(err) != resources.CodeForbidden || len(client.Actions()) != 0 {
		t.Fatalf("denied GET reached Kubernetes: %v %#v", err, client.Actions())
	}
}

func TestGatewayListSearchMatchesDisplayedHostname(t *testing.T) {
	t.Parallel()
	resource, _ := resourcecatalog.Lookup("http-routes")
	client := gatewayFake(resource)
	object := gatewayObject(resource, "v1", "web", "edge")
	object.Object["spec"] = map[string]any{"hostnames": []any{"api.example.test"}}
	gvr := schema.GroupVersionResource{Group: resource.Group, Version: "v1", Resource: resource.Resource}
	if err := client.Tracker().Create(gvr, object, "web"); err != nil {
		t.Fatal(err)
	}
	backend := &ResourceBackend{clients: fixedResourceClientProvider{resourceClientSet{dynamic: client}}, authorizer: &allowResourceAuthorization{}, now: time.Now}
	page, err := backend.ListGatewayResources(t.Context(), namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}, namespaces.ScopeResolution{ScopeName: "web", Namespaces: []string{"web"}}, resource.Collection, resources.ListOptions{Limit: 10, Search: "EXAMPLE.TEST"}, nil)
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != "edge" {
		t.Fatalf("hostname search = %#v / %v", page.Items, err)
	}
}
