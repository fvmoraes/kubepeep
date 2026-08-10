package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	contextservice "github.com/fvmoraes/kubepeep/internal/services/contexts"
)

type fakeContexts struct {
	items    []contextservice.ContextDTO
	selected contextservice.SelectionDTO
	err      error
	request  contextservice.SelectRequest
	profile  int64
}

func (f *fakeContexts) List(_ context.Context, profileID int64) ([]contextservice.ContextDTO, error) {
	f.profile = profileID
	return f.items, f.err
}

func (f *fakeContexts) Select(_ context.Context, request contextservice.SelectRequest) (contextservice.SelectionDTO, error) {
	f.request = request
	return f.selected, f.err
}

func TestContextsListRequiresOnePositiveProfileAndReturnsSanitizedDTO(t *testing.T) {
	service := &fakeContexts{items: []contextservice.ContextDTO{{ClusterProfileID: 7, Name: "dev", Cluster: "cluster-dev", Selected: true}}}
	handler := NewContexts(service)

	for _, target := range []string{"/api/v1/contexts", "/api/v1/contexts?clusterProfileId=", "/api/v1/contexts?clusterProfileId=0", "/api/v1/contexts?clusterProfileId=7&extra=x"} {
		recorder := httptest.NewRecorder()
		handler.List(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"VALIDATION_FAILED"`) {
			t.Fatalf("target=%s status=%d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}

	recorder := httptest.NewRecorder()
	handler.List(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/contexts?clusterProfileId=7", nil))
	if recorder.Code != http.StatusOK || service.profile != 7 || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d profile=%d headers=%v", recorder.Code, service.profile, recorder.Header())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"cluster":"cluster-dev"`) || strings.Contains(body, "token") || strings.Contains(body, "/home/") {
		t.Fatalf("unsafe or incomplete DTO: %s", body)
	}
}

func TestContextSelectUsesStrictBoundedJSONAndStableErrors(t *testing.T) {
	handler := NewContexts(&fakeContexts{})
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{name: "unknown", body: `{"clusterProfileId":1,"context":"dev","setDefault":false,"expectedGeneration":"gen","token":"secret"}`, status: http.StatusBadRequest, code: api.CodeUnknownField},
		{name: "trailing", body: `{"clusterProfileId":1,"context":"dev","setDefault":false,"expectedGeneration":"gen"}{}`, status: http.StatusBadRequest, code: api.CodeInvalidJSON},
		{name: "empty", body: ``, status: http.StatusBadRequest, code: api.CodeInvalidJSON},
		{name: "large", body: `{"clusterProfileId":1,"context":"` + strings.Repeat("x", maxContextBodyBytes) + `","setDefault":false,"expectedGeneration":"gen"}`, status: http.StatusRequestEntityTooLarge, code: api.CodeBodyTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.Select(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/contexts/select", strings.NewReader(test.body)))
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "secret") {
				t.Fatalf("error leaked rejected value: %s", recorder.Body.String())
			}
		})
	}
}

func TestContextSelectPublishesCompleteSelectionAndMapsSafeFailures(t *testing.T) {
	checked := time.Now().UTC()
	service := &fakeContexts{selected: contextservice.SelectionDTO{
		ClusterProfileID: 1, Context: "dev", Cluster: "cluster-dev", ScopeSource: "none", Generation: "gen_next",
		Components: contextservice.SelectionComponents{Cluster: api.ComponentState{Status: api.StatusDegraded, Code: api.CodeClusterUnavailable, Message: "offline", CheckedAt: &checked}},
	}}
	handler := NewContexts(service)
	body := `{"clusterProfileId":1,"context":"dev","setDefault":true,"expectedGeneration":"gen_old"}`
	recorder := httptest.NewRecorder()
	handler.Select(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/contexts/select", strings.NewReader(body)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"scopeSource":"none"`) || !strings.Contains(recorder.Body.String(), `"generation":"gen_next"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.request.Context != "dev" || !service.request.SetDefault {
		t.Fatalf("request not decoded: %#v", service.request)
	}

	service.err = &contextservice.ExternalError{Code: api.CodeContextNotFound, Message: "The context does not exist."}
	recorder = httptest.NewRecorder()
	handler.Select(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/contexts/select", strings.NewReader(body)))
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"CONTEXT_NOT_FOUND"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	service.err = errors.New("token=should-never-escape")
	recorder = httptest.NewRecorder()
	handler.List(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/contexts?clusterProfileId=1", nil))
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "should-never-escape") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAPIFallbackRecognizesAllPhaseFourRouteShapes(t *testing.T) {
	handler := NewAPIFallback()
	tests := []struct {
		path  string
		allow string
	}{
		{"/api/v1/contexts", "GET, HEAD"},
		{"/api/v1/contexts/select", "POST"},
		{"/api/v1/namespace-scopes", "GET, HEAD, POST"},
		{"/api/v1/namespace-scopes/42", "GET, HEAD, PUT, DELETE"},
		{"/api/v1/namespace-scopes/42/select", "POST"},
		{"/api/v1/permissions", "GET, HEAD"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, test.path, nil))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != test.allow {
			t.Fatalf("path=%s status=%d allow=%q body=%s", test.path, recorder.Code, recorder.Header().Get("Allow"), recorder.Body.String())
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/not-real", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
