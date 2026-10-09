package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

type dynamicServiceStub struct {
	ResourceService
	target          resources.DynamicResource
	namespace, name string
	options         resources.ListOptions
	calls           int
}

func (s *dynamicServiceStub) DiscoverResources(context.Context, namespaces.SelectionBinding, bool) (resources.ResourceDiscoveryDTO, error) {
	s.calls++
	return resources.ResourceDiscoveryDTO{Resources: []resources.DynamicResource{}}, nil
}
func (s *dynamicServiceStub) ListDynamicResources(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, target resources.DynamicResource, options resources.ListOptions, _ *resources.CompositeCursor[resources.DynamicRow]) (resources.ListResult[resources.DynamicRow], error) {
	s.calls++
	s.target, s.options = target, options
	return resources.ListResult[resources.DynamicRow]{Items: []resources.DynamicRow{{Name: "sample"}}, Page: resources.PageDTO{Limit: options.Limit, Complete: true, FilterScope: resources.FilterScopeCollection}}, nil
}
func (s *dynamicServiceStub) GetDynamicResource(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, target resources.DynamicResource, namespace, name string) (resources.DynamicRow, error) {
	s.calls++
	s.target, s.namespace, s.name = target, namespace, name
	return resources.DynamicRow{Name: name, Namespace: namespace}, nil
}
func (s *dynamicServiceStub) GetDynamicYAML(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, target resources.DynamicResource, namespace, name string) (resources.DynamicDocument, error) {
	s.calls++
	s.target, s.namespace, s.name = target, namespace, name
	return resources.DynamicDocument{YAML: "kind: Widget\n", Generation: "gen"}, nil
}

func TestDynamicHandlersValidateResourceScopeAndReadOnlyRoutes(t *testing.T) {
	t.Parallel()
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "web", ScopeSource: "saved", Namespaces: []string{"web"}}}
	for _, test := range []struct {
		name, group, resource, scope, namespace, object string
		status                                          int
	}{
		{"namespaced", "example.test", "widgets", "n", "web", "sample", 200},
		{"cluster and RBAC colon name", "rbac.authorization.k8s.io", "clusterroles", "c", "_", "system:discovery", 200},
		{"outside scope", "example.test", "widgets", "n", "outside", "sample", 400},
		{"cluster marker", "example.test", "widgets", "c", "web", "sample", 400},
		{"subresource", "example.test", "widgets/status", "n", "web", "sample", 400},
		{"path traversal", "..", "widgets", "n", "web", "sample", 400},
		{"object slash", "example.test", "widgets", "n", "web", "a/b", 400},
		{"invalid scope", "example.test", "widgets", "invalid", "web", "sample", 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &dynamicServiceStub{}
			handler := NewResources(s, nil, selection, codec)
			r := httptest.NewRequest(http.MethodGet, "/api/v1/dynamic-resources/test", nil)
			for key, value := range map[string]string{"group": test.group, "version": "v1", "resource": test.resource, "scope": test.scope, "namespace": test.namespace, "name": test.object} {
				r.SetPathValue(key, value)
			}
			w := httptest.NewRecorder()
			handler.DynamicDetail(w, r)
			if w.Code != test.status {
				t.Fatalf("status=%d / %s", w.Code, w.Body)
			}
			if test.status != 200 && s.calls != 0 {
				t.Fatal("invalid path reached backend")
			}
			if test.status == 200 {
				if s.name != test.object || s.target.Group != test.group || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("lost target or cache protection")
				}
				w = httptest.NewRecorder()
				handler.DynamicYAML(w, r)
				if w.Code != 200 || s.calls != 2 {
					t.Fatalf("document=%d", w.Code)
				}
			}
		})
	}
	for _, path := range []string{"/api/v1/resource-discovery", "/api/v1/dynamic-resources/example.test/v1/widgets/n", "/api/v1/dynamic-resources/example.test/v1/widgets/n/web/sample", "/api/v1/dynamic-resources/example.test/v1/widgets/n/web/sample/yaml"} {
		allow, ok := allowedMethods(path)
		if !ok || allow != "GET, HEAD" {
			t.Fatalf("dynamic route allows mutations: %s %s", path, allow)
		}
	}
}

func TestDynamicHandlersListKeepsPaginationNamespaceAndFocus(t *testing.T) {
	t.Parallel()
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "web", ScopeSource: "saved", Namespaces: []string{"web"}}}
	service := &dynamicServiceStub{}
	handler := NewResources(service, nil, selection, codec)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/dynamic-resources/example.test/v1/widgets/n?namespace=web&limit=50&fieldSelector=metadata.name%3Dsample", nil)
	for key, value := range map[string]string{"group": "example.test", "version": "v1", "resource": "widgets", "scope": "n"} {
		r.SetPathValue(key, value)
	}
	w := httptest.NewRecorder()
	handler.DynamicList(w, r)
	if w.Code != 200 || service.options.Limit != 50 || service.options.FieldSelector != "metadata.name=sample" || len(service.options.Namespaces) != 1 || service.options.Namespaces[0] != "web" {
		t.Fatalf("query changed: %d %s / %#v", w.Code, w.Body, service.options)
	}
}
