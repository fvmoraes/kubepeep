package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
)

type investigationServiceStub struct {
	index resourcecore.LocalIndexSnapshot
}

func (stub investigationServiceStub) Investigation(generation, kind, namespace, name string, _ []string) resourcecore.Investigation {
	return stub.index.Investigate(kind, namespace, name)
}

func (stub investigationServiceStub) LocalIndex(generation string, _ []string) resourcecore.LocalIndexSnapshot {
	value := stub.index
	value.Generation = generation
	return value
}

type diagnosticsServiceStub struct{}

func (diagnosticsServiceStub) Diagnostics(_ context.Context, binding namespaces.SelectionBinding, _ namespaces.ScopeResolution) dashboard.DiagnosticsDTO {
	return dashboard.DiagnosticsDTO{Generation: binding.Generation, Complete: true, Errors: []dashboard.PartialError{}}
}

func phaseFiveSelection() *dashboardSelectionStub {
	return &dashboardSelectionStub{
		binding:    namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen-5"},
		resolution: namespaces.ScopeResolution{ScopeName: "portal", ScopeSource: "saved", Namespaces: []string{"portal"}},
	}
}

func TestInvestigationHandlersFenceGenerationAndScope(t *testing.T) {
	t.Parallel()
	selection := phaseFiveSelection()
	service := investigationServiceStub{index: resourcecore.LocalIndexSnapshot{Resources: []resourcecore.IndexedResource{{Kind: "Pod", Namespace: "portal", Name: "portal-api"}}}}
	handler := NewInvestigation(service, selection)

	indexResponse := httptest.NewRecorder()
	handler.Index(indexResponse, httptest.NewRequest(http.MethodGet, "/api/v1/local-index", nil))
	if indexResponse.Code != http.StatusOK || indexResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("index status=%d headers=%v body=%s", indexResponse.Code, indexResponse.Header(), indexResponse.Body.String())
	}
	var index resourcecore.LocalIndexSnapshot
	if err := json.Unmarshal(indexResponse.Body.Bytes(), &index); err != nil || index.Generation != "gen-5" {
		t.Fatalf("index=%#v err=%v", index, err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/investigation/Pod/portal/portal-api", nil)
	request.SetPathValue("kind", "Pod")
	request.SetPathValue("namespace", "portal")
	request.SetPathValue("name", "portal-api")
	response := httptest.NewRecorder()
	handler.Get(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("investigation status=%d body=%s", response.Code, response.Body.String())
	}

	outside := httptest.NewRequest(http.MethodGet, "/api/v1/investigation/Pod/other/api", nil)
	outside.SetPathValue("kind", "Pod")
	outside.SetPathValue("namespace", "other")
	outside.SetPathValue("name", "api")
	outsideResponse := httptest.NewRecorder()
	handler.Get(outsideResponse, outside)
	if outsideResponse.Code != http.StatusBadRequest {
		t.Fatalf("out-of-scope status=%d body=%s", outsideResponse.Code, outsideResponse.Body.String())
	}

	selection.rejectFence = true
	fenced := httptest.NewRecorder()
	handler.Index(fenced, httptest.NewRequest(http.MethodGet, "/api/v1/local-index", nil))
	if fenced.Code != http.StatusConflict {
		t.Fatalf("fenced status=%d body=%s", fenced.Code, fenced.Body.String())
	}
}

func TestDiagnosticsPublishesGenerationAndRejectsQueries(t *testing.T) {
	t.Parallel()
	handler := NewDiagnostics(diagnosticsServiceStub{}, phaseFiveSelection())
	response := httptest.NewRecorder()
	handler.Get(response, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics", nil))
	if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var value dashboard.DiagnosticsDTO
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil || value.Generation != "gen-5" {
		t.Fatalf("diagnostics=%#v err=%v", value, err)
	}

	invalid := httptest.NewRecorder()
	handler.Get(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics?namespace=portal", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("query status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestNilPhaseFiveHandlersReturnUnavailable(t *testing.T) {
	t.Parallel()
	for _, invoke := range []func(http.ResponseWriter, *http.Request){
		(*InvestigationHandler)(nil).Get,
		(*InvestigationHandler)(nil).Index,
		(*DiagnosticsHandler)(nil).Get,
	} {
		response := httptest.NewRecorder()
		invoke(response, httptest.NewRequest(http.MethodGet, "/api/v1/phase-five", nil))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
}
