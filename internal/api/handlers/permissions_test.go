package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type fakePermissionMatrix struct {
	request authorization.PermissionsRequest
	matrix  authorization.CapabilityMatrix
	err     error
	onCall  func()
}

func (f *fakePermissionMatrix) Matrix(_ context.Context, request authorization.PermissionsRequest) (authorization.CapabilityMatrix, error) {
	f.request = request
	if f.onCall != nil {
		f.onCall()
	}
	return f.matrix, f.err
}

type fakeSelectionReader struct {
	binding    namespaces.SelectionBinding
	resolution namespaces.ScopeResolution
}

func (f *fakeSelectionReader) Snapshot() (namespaces.SelectionBinding, namespaces.ScopeResolution) {
	return f.binding, f.resolution
}

func TestPermissionsQueryBindsActiveGenerationAndScope(t *testing.T) {
	service := &fakePermissionMatrix{matrix: authorization.CapabilityMatrix{Generation: "gen_active", Decisions: []authorization.Capability{}, Errors: []authorization.PartialError{}, Complete: true}}
	selection := &fakeSelectionReader{
		binding:    namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen_active"},
		resolution: namespaces.ScopeResolution{Namespaces: []string{"payments", "billing"}},
	}
	target := "/api/v1/permissions?namespace=payments&capability=pods.get&resourceName=pod-a&refresh=true"
	recorder := httptest.NewRecorder()
	NewPermissions(service, selection).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	if service.request.Generation != "gen_active" || !service.request.Refresh || len(service.request.ActiveNamespaces) != 2 || service.request.CapabilityIDs[0] != "pods.get" {
		t.Fatalf("request not bound to active selection: %#v", service.request)
	}
}

func TestPermissionsRejectsInvalidGrammarAndMissingSelection(t *testing.T) {
	service := &fakePermissionMatrix{}
	active := &fakeSelectionReader{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}}
	for _, target := range []string{
		"/api/v1/permissions?unknown=x",
		"/api/v1/permissions?refresh=true&refresh=false",
		"/api/v1/permissions?refresh=maybe",
		"/api/v1/permissions?namespace=",
	} {
		recorder := httptest.NewRecorder()
		NewPermissions(service, active).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"VALIDATION_FAILED"`) {
			t.Fatalf("target=%s status=%d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}

	recorder := httptest.NewRecorder()
	NewPermissions(service, &fakeSelectionReader{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/permissions", nil))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"GENERATION_CHANGED"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPermissionsMapsTotalUnavailableWithPartialSafeDetails(t *testing.T) {
	matrix := authorization.CapabilityMatrix{
		Generation: "gen", Decisions: []authorization.Capability{{CapabilityID: "pods.list", Decision: authorization.DecisionUnknown}},
		Errors: []authorization.PartialError{{Code: authorization.CodeAuthorizationUnavailable, Message: "Authorization could not be confirmed."}},
	}
	service := &fakePermissionMatrix{matrix: matrix, err: &authorization.PublicError{
		Code: authorization.CodeAuthorizationUnavailable, Message: "Authorization could not be confirmed.", HTTPStatus: http.StatusServiceUnavailable,
	}}
	selection := &fakeSelectionReader{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}, resolution: namespaces.ScopeResolution{Namespaces: []string{"default"}}}
	recorder := httptest.NewRecorder()
	NewPermissions(service, selection).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/permissions?capability=pods.list", nil))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"decision":"unknown"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	service.err = errors.New("upstream secret response")
	recorder = httptest.NewRecorder()
	NewPermissions(service, selection).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/permissions?capability=pods.list", nil))
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "upstream secret") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPermissionsReturnsGenerationChangedWhenSelectionMovesDuringReview(t *testing.T) {
	selection := &fakeSelectionReader{
		binding:    namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen_old"},
		resolution: namespaces.ScopeResolution{Namespaces: []string{"default"}},
	}
	service := &fakePermissionMatrix{matrix: authorization.CapabilityMatrix{Generation: "gen_old"}}
	service.onCall = func() { selection.binding.Generation = "gen_new" }
	recorder := httptest.NewRecorder()
	NewPermissions(service, selection).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/permissions?capability=pods.list", nil))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"GENERATION_CHANGED"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
