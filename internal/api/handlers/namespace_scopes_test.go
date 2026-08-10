package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type fakeNamespaceService struct {
	scopes           []namespaces.Scope
	selected         namespaces.ScopeResolution
	result           namespaces.SelectionResult
	err              error
	onSelect         func()
	validatedRequest namespaces.ScopeWriteRequest
	createdRequest   namespaces.ScopeWriteRequest
}

func (f *fakeNamespaceService) List(context.Context, int64, string) ([]namespaces.Scope, error) {
	return f.scopes, f.err
}
func (f *fakeNamespaceService) Get(_ context.Context, id int64) (namespaces.Scope, error) {
	for _, scope := range f.scopes {
		if scope.ID == id {
			return scope, f.err
		}
	}
	return namespaces.Scope{}, namespaces.ErrNotFound
}
func (f *fakeNamespaceService) Validate(_ context.Context, request namespaces.ScopeWriteRequest, _ bool) (namespaces.ValidationReport, error) {
	f.validatedRequest = request
	return namespaces.ValidationReport{}, f.err
}
func (f *fakeNamespaceService) Create(_ context.Context, request namespaces.ScopeWriteRequest) (namespaces.Scope, error) {
	f.createdRequest = request
	return firstScope(f.scopes), f.err
}
func (f *fakeNamespaceService) Update(context.Context, int64, namespaces.ScopeWriteRequest) (namespaces.Scope, namespaces.SelectionResult, error) {
	return firstScope(f.scopes), f.result, f.err
}
func (f *fakeNamespaceService) Delete(context.Context, int64, namespaces.ScopeDeleteRequest) (namespaces.SelectionResult, error) {
	return f.result, f.err
}
func (f *fakeNamespaceService) Select(context.Context, int64, namespaces.ScopeSelectRequest) (namespaces.ScopeResolution, namespaces.SelectionResult, error) {
	if f.onSelect != nil {
		f.onSelect()
	}
	return f.selected, f.result, f.err
}

func firstScope(scopes []namespaces.Scope) namespaces.Scope {
	if len(scopes) == 0 {
		return namespaces.Scope{}
	}
	return scopes[0]
}

type fakeNamespaceCatalog struct {
	items []string
	err   error
}

func (f fakeNamespaceCatalog) List(context.Context, namespaces.SelectionBinding) ([]string, error) {
	return f.items, f.err
}

type fakePagedNamespaceCatalog struct {
	pages map[string]namespaces.NamespacePage
	err   error
}

func (f fakePagedNamespaceCatalog) List(context.Context, namespaces.SelectionBinding) ([]string, error) {
	return nil, f.err
}

func (f fakePagedNamespaceCatalog) ListPage(_ context.Context, _ namespaces.SelectionBinding, request namespaces.NamespacePageRequest) (namespaces.NamespacePage, error) {
	if f.err != nil {
		return namespaces.NamespacePage{}, f.err
	}
	return f.pages[request.Continue], nil
}

type staticSnapshots struct{ snapshot api.Snapshot }

func (s *staticSnapshots) Snapshot(context.Context) (api.Snapshot, error) { return s.snapshot, nil }
func (s *staticSnapshots) SetSelection(value *api.SelectionSummary)       { s.snapshot.Selection = value }

func TestNamespaceListMarksActiveItemsAndSeparatesForbidden(t *testing.T) {
	selection := &fakeSelectionReader{
		binding:    namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"},
		resolution: namespaces.ScopeResolution{Namespaces: []string{"payments"}},
	}
	handler := NewNamespaceScopes(&fakeNamespaceService{}, selection, fakeNamespaceCatalog{items: []string{"payments", "billing"}})
	recorder := httptest.NewRecorder()
	handler.ListNamespaces(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"name":"payments","phase":"Active","selected":true`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	handler = NewNamespaceScopes(&fakeNamespaceService{}, selection, fakeNamespaceCatalog{err: namespaces.ErrNamespaceListForbidden})
	recorder = httptest.NewRecorder()
	handler.ListNamespaces(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces", nil))
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"code":"FORBIDDEN"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestScopeSelectionReturnsCompleteSelectionDTOAndRefreshesSnapshot(t *testing.T) {
	defaultNamespace := "payments"
	resolution := namespaces.ScopeResolution{
		ScopeID: 9, ScopeName: "Finance", ScopeMode: namespaces.ScopeModeList, ScopeSource: "saved",
		DefaultNamespace: &defaultNamespace, Namespaces: []string{"payments", "billing"},
	}
	selection := &fakeSelectionReader{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Cluster: "cluster-dev", Generation: "gen_old"}}
	resultBinding := namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Cluster: "cluster-dev", ActiveScopeID: 9, Generation: "gen_new"}
	service := &fakeNamespaceService{selected: resolution, result: namespaces.SelectionResult{
		Generation: "gen_new", Binding: resultBinding, Resolution: resolution, Changed: true,
	}}
	service.onSelect = func() {
		selection.binding.Generation = "gen_new"
		selection.binding.ActiveScopeID = 9
		selection.resolution = resolution
	}
	checked := time.Now().UTC()
	snapshots := &staticSnapshots{snapshot: api.Snapshot{Components: api.StatusComponents{Cluster: api.ComponentState{Status: api.StatusHealthy, Code: "OK", Message: "reachable", CheckedAt: &checked}}}}
	handler := NewNamespaceScopes(service, selection, fakeNamespaceCatalog{}, snapshots)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/namespace-scopes/9/select", strings.NewReader(`{"expectedGeneration":"gen_old"}`))
	request.SetPathValue("id", "9")
	recorder := httptest.NewRecorder()
	handler.Select(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{`"cluster":"cluster-dev"`, `"scopeName":"Finance"`, `"scopeMode":"list"`, `"scopeSource":"saved"`, `"defaultNamespace":"payments"`, `"generation":"gen_new"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s in %s", expected, body)
		}
	}
	if recorder.Code != http.StatusOK || snapshots.snapshot.Selection == nil || snapshots.snapshot.Selection.ScopeID == nil {
		t.Fatalf("status=%d snapshot=%#v body=%s", recorder.Code, snapshots.snapshot.Selection, body)
	}
}

func TestNamespaceHandlersMapStrictBodyAndStorageErrorsSafely(t *testing.T) {
	selection := &fakeSelectionReader{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}}
	handler := NewNamespaceScopes(&fakeNamespaceService{}, selection, fakeNamespaceCatalog{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/namespace-scopes", strings.NewReader(`{"clusterProfileId":1,"context":"dev","name":"x","mode":"single","namespaces":["default"],"credential":"secret"}`))
	recorder := httptest.NewRecorder()
	handler.Create(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"UNKNOWN_FIELD"`) || strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	service := &fakeNamespaceService{err: errors.New("database secret detail")}
	handler = NewNamespaceScopes(service, selection, fakeNamespaceCatalog{})
	recorder = httptest.NewRecorder()
	handler.List(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/namespace-scopes", nil))
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "database secret") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNamespaceListUsesSignedNativeCursorAndRealPhase(t *testing.T) {
	selection := &fakeSelectionReader{
		binding:    namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", ActiveScopeID: 7, Generation: "gen"},
		resolution: namespaces.ScopeResolution{ScopeSource: "saved", Namespaces: []string{"payments"}},
	}
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	catalog := fakePagedNamespaceCatalog{pages: map[string]namespaces.NamespacePage{
		"":                     {Items: []namespaces.NamespaceRecord{{Name: "payments", UID: "uid-1", Phase: "Terminating"}}, Continue: "raw-kubernetes-token"},
		"raw-kubernetes-token": {Items: []namespaces.NamespaceRecord{{Name: "billing", UID: "uid-2", Phase: "Active"}}},
	}}
	handler := NewNamespaceScopes(&fakeNamespaceService{}, selection, catalog).WithCursors(codec)
	first := httptest.NewRecorder()
	handler.ListNamespaces(first, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces?limit=1", nil))
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"phase":"Terminating"`) || strings.Contains(first.Body.String(), "raw-kubernetes-token") {
		t.Fatalf("status=%d body=%s", first.Code, first.Body.String())
	}
	var envelope struct {
		Meta struct {
			Page struct {
				Next string `json:"next"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &envelope); err != nil || envelope.Meta.Page.Next == "" {
		t.Fatalf("cursor missing: err=%v body=%s", err, first.Body.String())
	}
	second := httptest.NewRecorder()
	target := "/api/v1/namespaces?limit=1&continue=" + url.QueryEscape(envelope.Meta.Page.Next)
	handler.ListNamespaces(second, httptest.NewRequest(http.MethodGet, target, nil))
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"name":"billing"`) || !strings.Contains(second.Body.String(), `"complete":true`) {
		t.Fatalf("status=%d body=%s", second.Code, second.Body.String())
	}
	mismatch := httptest.NewRecorder()
	handler.ListNamespaces(mismatch, httptest.NewRequest(http.MethodGet, target+"&search=bill", nil))
	if mismatch.Code != http.StatusBadRequest || !strings.Contains(mismatch.Body.String(), `"code":"CURSOR_MISMATCH"`) {
		t.Fatalf("status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}
}

func TestScopeListKeysetCursorSurvivesInsertionBeforeBoundary(t *testing.T) {
	selection := &fakeSelectionReader{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}}
	service := &fakeNamespaceService{scopes: []namespaces.Scope{{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Beta"}, {ID: 3, Name: "Charlie"}}}
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	handler := NewNamespaceScopes(service, selection, fakeNamespaceCatalog{}).WithCursors(codec)
	first := httptest.NewRecorder()
	handler.List(first, httptest.NewRequest(http.MethodGet, "/api/v1/namespace-scopes?limit=2", nil))
	var envelope struct {
		Meta struct {
			Page struct {
				Next string `json:"next"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &envelope); err != nil || envelope.Meta.Page.Next == "" {
		t.Fatalf("cursor missing: err=%v body=%s", err, first.Body.String())
	}
	service.scopes = append(service.scopes, namespaces.Scope{ID: 4, Name: "Aardvark"})
	second := httptest.NewRecorder()
	handler.List(second, httptest.NewRequest(http.MethodGet, "/api/v1/namespace-scopes?limit=2&continue="+url.QueryEscape(envelope.Meta.Page.Next), nil))
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"name":"Charlie"`) || strings.Contains(second.Body.String(), `"name":"Beta"`) {
		t.Fatalf("status=%d body=%s", second.Code, second.Body.String())
	}
}

func TestValidateAndCreateBindActiveGenerationWithoutClientField(t *testing.T) {
	selection := &fakeSelectionReader{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen_active"}}
	service := &fakeNamespaceService{scopes: []namespaces.Scope{{ID: 1, ClusterProfileID: 1, Context: "dev", Name: "One", Mode: namespaces.ScopeModeSingle}}}
	handler := NewNamespaceScopes(service, selection, fakeNamespaceCatalog{})
	body := `{"clusterProfileId":1,"context":"dev","name":"One","mode":"single","namespaces":["default"]}`
	validated := httptest.NewRecorder()
	handler.Validate(validated, httptest.NewRequest(http.MethodPost, "/api/v1/namespace-scopes/validate", strings.NewReader(body)))
	if validated.Code != http.StatusOK || service.validatedRequest.ExpectedGeneration != "gen_active" {
		t.Fatalf("status=%d request=%#v body=%s", validated.Code, service.validatedRequest, validated.Body.String())
	}
	created := httptest.NewRecorder()
	handler.Create(created, httptest.NewRequest(http.MethodPost, "/api/v1/namespace-scopes", strings.NewReader(body)))
	if created.Code != http.StatusCreated || service.createdRequest.ExpectedGeneration != "gen_active" {
		t.Fatalf("status=%d request=%#v body=%s", created.Code, service.createdRequest, created.Body.String())
	}

	wrongOrigin := httptest.NewRecorder()
	wrongBody := strings.Replace(body, `"context":"dev"`, `"context":"prod"`, 1)
	handler.Create(wrongOrigin, httptest.NewRequest(http.MethodPost, "/api/v1/namespace-scopes", strings.NewReader(wrongBody)))
	if wrongOrigin.Code != http.StatusConflict || !strings.Contains(wrongOrigin.Body.String(), `"code":"SELECTION_MISMATCH"`) {
		t.Fatalf("status=%d body=%s", wrongOrigin.Code, wrongOrigin.Body.String())
	}
}
