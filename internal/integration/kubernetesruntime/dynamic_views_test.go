package kubernetesruntime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kubeadapter "github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var viewFixture = resources.DynamicResource{Group: "example.test", Version: "v1", Resource: "widgets", Kind: "Widget", Namespaced: true}
var viewBinding = namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Cluster: "cluster", Generation: "view-gen"}
var viewScope = namespaces.ScopeResolution{ScopeName: "web", Namespaces: []string{"web"}}

func viewBackend(t *testing.T, serve http.HandlerFunc) *ResourceBackend {
	t.Helper()
	server := httptest.NewServer(serve)
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: dev\nclusters:\n- name: cluster\n  cluster:\n    server: %s\ncontexts:\n- name: dev\n  context:\n    cluster: cluster\n    user: test\nusers:\n- name: test\n  user: {}\n", server.URL)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	resolution, err := kubeadapter.NewLoader(kubeadapter.LoaderOptions{}).Resolve(t.Context(), kubeadapter.ResolveRequest{ExplicitPath: &path, FirstReconcile: true})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := kubeadapter.NewClientFactory(kubeadapter.FactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	clients, err := factory.Build(t.Context(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(clients.CloseIdleConnections)
	return &ResourceBackend{clients: fixedResourceClientProvider{resourceClientSet{views: clients.ResourceViews(), metadata: clients.UnaryMetadata()}}, authorizer: &allowResourceAuthorization{}, now: time.Now}
}
func viewJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func viewDiscovery(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api":
		viewJSON(w, metav1.APIVersions{Versions: []string{"v1"}})
	case "/apis":
		viewJSON(w, metav1.APIGroupList{Groups: []metav1.APIGroup{{Name: "example.test", PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "example.test/v1", Version: "v1"}, Versions: []metav1.GroupVersionForDiscovery{{GroupVersion: "example.test/v1", Version: "v1"}}}}})
	case "/api/v1":
		viewJSON(w, metav1.APIResourceList{APIResources: []metav1.APIResource{{Name: "secrets", Kind: "Secret", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}}, {Name: "nodes", Kind: "Node", Verbs: metav1.Verbs{"list", "get"}}}})
	case "/apis/example.test/v1":
		viewJSON(w, metav1.APIResourceList{APIResources: []metav1.APIResource{{Name: "widgets", Kind: "Widget", Namespaced: true, ShortNames: []string{"wd"}, Verbs: metav1.Verbs{"list", "get"}}, {Name: "widgets/status", Kind: "Widget", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}}, {Name: "writeonly", Kind: "WriteOnly", Verbs: metav1.Verbs{"create"}}}})
	default:
		return false
	}
	return true
}
func viewTable(namespace, name, next string) metav1.Table {
	raw, _ := json.Marshal(map[string]any{"apiVersion": "meta.k8s.io/v1", "kind": "PartialObjectMetadata", "metadata": map[string]any{"name": name, "namespace": namespace, "uid": "u1", "resourceVersion": "r1", "creationTimestamp": "2026-01-01T00:00:00Z", "annotations": map[string]any{"private": "not-in-dto"}}})
	return metav1.Table{TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "Table"}, ListMeta: metav1.ListMeta{Continue: next, ResourceVersion: "r1"}, ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name", Type: "string", Format: "name"}, {Name: "Ready", Type: "boolean"}, {Name: "Replicas", Type: "integer"}, {Name: "Message", Type: "string", Priority: 1}}, Rows: []metav1.TableRow{{Cells: []any{name, true, 3, "healthy"}, Object: runtime.RawExtension{Raw: raw}}}}
}

func TestDynamicViewsDiscoveryPaginationDetailAndExplicitDocument(t *testing.T) {
	t.Parallel()
	var discovered, documents atomic.Int32
	backend := viewBackend(t, func(w http.ResponseWriter, r *http.Request) {
		if viewDiscovery(w, r) {
			discovered.Add(1)
			return
		}
		if r.URL.Path != "/apis/example.test/v1/namespaces/web/widgets" && r.URL.Path != "/apis/example.test/v1/namespaces/web/widgets/first" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Accept") == "application/json" {
			documents.Add(1)
			_, _ = w.Write([]byte(`{"metadata":{"name":"first","namespace":"web","managedFields":[{}]},"spec":{"exact":9007199254740993,"encoded":"c2VjcmV0","text":"olá"}}`))
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "as=Table") || r.URL.Query().Get("includeObject") != "Metadata" {
			t.Error("full objects requested for inventory")
		}
		name, next := "first", ""
		if !strings.HasSuffix(r.URL.Path, "/first") {
			if r.URL.Query().Get("continue") == "" {
				next = "next"
			} else {
				name = "second"
			}
			if r.URL.Query().Get("fieldSelector") != "metadata.name=first" {
				t.Error("field selector lost")
			}
		}
		viewJSON(w, viewTable("web", name, next))
	})
	catalog, err := backend.DiscoverResources(t.Context(), viewBinding, false)
	if err != nil || len(catalog.Resources) != 3 {
		t.Fatalf("catalog: %#v / %v", catalog, err)
	}
	first, err := backend.ListDynamicResources(t.Context(), viewBinding, viewScope, viewFixture, resources.ListOptions{Limit: 1, FieldSelector: "metadata.name=first"}, nil)
	if err != nil || len(first.Items) != 1 || first.Cursor == nil {
		t.Fatalf("first: %#v / %v", first, err)
	}
	second, err := backend.ListDynamicResources(t.Context(), viewBinding, viewScope, viewFixture, resources.ListOptions{Limit: 1, FieldSelector: "metadata.name=first"}, first.Cursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].Name != "second" {
		t.Fatalf("second: %#v / %v", second, err)
	}
	detail, err := backend.GetDynamicResource(t.Context(), viewBinding, viewScope, viewFixture, "web", "first")
	encoded, _ := json.Marshal(detail)
	if err != nil || detail.Cells[2] != "3" || detail.Columns[3].Priority != 1 || strings.Contains(string(encoded), "not-in-dto") {
		t.Fatalf("detail: %#v / %v", detail, err)
	}
	if documents.Load() != 0 || discovered.Load() != 4 {
		t.Fatalf("unexpected eager document or repeated discovery: %d / %d", documents.Load(), discovered.Load())
	}
	document, err := backend.GetDynamicYAML(t.Context(), viewBinding, viewScope, viewFixture, "web", "first")
	if err != nil || !strings.Contains(document.YAML, "9007199254740993") || !strings.Contains(document.YAML, "c2VjcmV0") || !strings.Contains(document.YAML, "olá") || strings.Contains(document.YAML, "managedFields") {
		t.Fatalf("document lost fields: %q / %v", document.YAML, err)
	}
	changed := viewBinding
	changed.Generation = "new-generation"
	_, err = backend.DiscoverResources(t.Context(), changed, false)
	if err != nil || discovered.Load() != 8 {
		t.Fatal("catalog leaked across selection generation")
	}
}

func TestDynamicViewsAuthorizationScopeMetadataFallbackAndLimits(t *testing.T) {
	t.Parallel()
	t.Run("denied details never read object", func(t *testing.T) {
		var objects atomic.Int32
		backend := viewBackend(t, func(w http.ResponseWriter, r *http.Request) {
			if viewDiscovery(w, r) {
				return
			}
			objects.Add(1)
			http.Error(w, "unexpected", 500)
		})
		backend.authorizer = &selectiveResourceAuthorization{denied: map[string]authorization.Decision{"example.test/widgets/get": authorization.DecisionDenied}}
		_, err := backend.GetDynamicResource(t.Context(), viewBinding, viewScope, viewFixture, "web", "first")
		if resources.ErrorCodeOf(err) != resources.CodeForbidden || objects.Load() != 0 {
			t.Fatalf("denied read: %v / %d", err, objects.Load())
		}
	})
	t.Run("unsupported table falls back to metadata only", func(t *testing.T) {
		backend := viewBackend(t, func(w http.ResponseWriter, r *http.Request) {
			if viewDiscovery(w, r) {
				return
			}
			if strings.Contains(r.Header.Get("Accept"), "as=Table") {
				w.WriteHeader(406)
				viewJSON(w, metav1.Status{Status: "Failure", Code: 406, Reason: metav1.StatusReasonNotAcceptable})
				return
			}
			if !strings.Contains(r.Header.Get("Accept"), "PartialObjectMetadata") {
				t.Error("unsafe fallback")
			}
			viewJSON(w, map[string]any{"apiVersion": "meta.k8s.io/v1", "kind": "PartialObjectMetadataList", "metadata": map[string]any{"resourceVersion": "1"}, "items": []any{map[string]any{"apiVersion": "meta.k8s.io/v1", "kind": "PartialObjectMetadata", "metadata": map[string]any{"name": "first", "namespace": "web"}}}})
		})
		result, err := backend.ListDynamicResources(t.Context(), viewBinding, viewScope, viewFixture, resources.ListOptions{Limit: 10}, nil)
		if err != nil || len(result.Items) != 1 || len(result.Items[0].Cells) != 0 {
			t.Fatalf("metadata fallback: %#v / %v", result, err)
		}
	})
	t.Run("document bounded before decode", func(t *testing.T) {
		backend := viewBackend(t, func(w http.ResponseWriter, r *http.Request) {
			if viewDiscovery(w, r) {
				return
			}
			_, _ = w.Write([]byte(`{"spec":"` + strings.Repeat("x", 2<<20) + `"}`))
		})
		_, err := backend.GetDynamicYAML(t.Context(), viewBinding, viewScope, viewFixture, "web", "first")
		if resources.ErrorCodeOf(err) != resources.CodeLimitExceeded {
			t.Fatalf("oversized: %v", err)
		}
	})
	for _, code := range []int{401, 403} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			backend := viewBackend(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				reason := metav1.StatusReasonForbidden
				if code == 401 {
					reason = metav1.StatusReasonUnauthorized
				}
				viewJSON(w, metav1.Status{Status: "Failure", Code: int32(code), Reason: reason, Message: "private upstream text"})
			})
			_, err := backend.DiscoverResources(t.Context(), viewBinding, false)
			want := resources.CodeForbidden
			if code == 401 {
				want = resources.CodeAuthenticationUnavailable
			}
			if resources.ErrorCodeOf(err) != want || strings.Contains(resources.PublicMessage(err), "private upstream text") {
				t.Fatalf("classification: %v", err)
			}
		})
	}
}
