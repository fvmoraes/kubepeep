package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
)

type yamlDiffServiceStub struct {
	ResourceService
	diff      resourcecore.LastAppliedDiffDTO
	err       error
	calls     int
	collecton string
}

func (stub *yamlDiffServiceStub) ResourceLastAppliedDiff(context.Context, namespaces.SelectionBinding, string, string, string) (resourcecore.LastAppliedDiffDTO, error) {
	stub.calls++
	return stub.diff, stub.err
}

func newYAMLDiffHandler(t *testing.T, service *yamlDiffServiceStub) *Resources {
	t.Helper()
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	selection := &resourceSelectionStub{
		binding:    namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"},
		resolution: namespaces.ScopeResolution{ScopeName: "scope", ScopeSource: "saved", Namespaces: []string{"default"}},
	}
	return NewResources(service, nil, selection, codec)
}

func TestResourceYAMLDiffWritesGenerationBoundEnvelope(t *testing.T) {
	service := &yamlDiffServiceStub{diff: resourcecore.LastAppliedDiffDTO{Lines: []resourcecore.DiffLineDTO{{Kind: resourcecore.DiffRemoved, Text: "b: 2"}, {Kind: resourcecore.DiffAdded, Text: "b: 9"}}}}
	handler := newYAMLDiffHandler(t, service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/resources/deployments/default/api/yaml-diff", nil)
	request.SetPathValue("collection", "deployments")
	request.SetPathValue("namespace", "default")
	request.SetPathValue("name", "api")
	response := httptest.NewRecorder()
	handler.ResourceYAMLDiff(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.calls != 1 || service.collecton != "" && service.calls != 1 {
		t.Fatalf("service calls=%d", service.calls)
	}
	var envelope struct {
		Data resourcecore.LastAppliedDiffDTO `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Lines) != 2 || envelope.Data.Lines[0].Text != "b: 2" || envelope.Data.Absent {
		t.Fatalf("diff payload = %#v", envelope.Data)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control=%q", response.Header().Get("Cache-Control"))
	}
}

func TestResourceYAMLDiffAbsentBaselineAndUnknownCollection(t *testing.T) {
	service := &yamlDiffServiceStub{diff: resourcecore.LastAppliedDiffDTO{Absent: true, Lines: []resourcecore.DiffLineDTO{}}}
	handler := newYAMLDiffHandler(t, service)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/resources/pods/default/api/yaml-diff", nil)
	request.SetPathValue("collection", "pods")
	request.SetPathValue("namespace", "default")
	request.SetPathValue("name", "api")
	response := httptest.NewRecorder()
	handler.ResourceYAMLDiff(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("absent status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data resourcecore.LastAppliedDiffDTO `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Data.Absent || len(envelope.Data.Lines) != 0 {
		t.Fatalf("absent payload = %#v", envelope.Data)
	}

	unknown := httptest.NewRequest(http.MethodGet, "/api/v1/resources/secrets/default/store/yaml-diff", nil)
	unknown.SetPathValue("collection", "secrets")
	unknown.SetPathValue("namespace", "default")
	unknown.SetPathValue("name", "store")
	unknownResponse := httptest.NewRecorder()
	handler.ResourceYAMLDiff(unknownResponse, unknown)
	if unknownResponse.Code != http.StatusNotFound {
		t.Fatalf("secrets must never diff, status=%d", unknownResponse.Code)
	}
	if service.calls != 1 {
		t.Fatalf("unknown collection must not reach the service: calls=%d", service.calls)
	}
}
