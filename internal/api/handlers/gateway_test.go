package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

type gatewayServiceStub struct {
	ResourceService
	collection, namespace, name string
	options                     resources.ListOptions
	calls                       int
}

func (service *gatewayServiceStub) ListGatewayResources(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, collection string, options resources.ListOptions, _ *resources.CompositeCursor[resources.GatewayDTO]) (resources.ListResult[resources.GatewayDTO], error) {
	service.collection, service.options = collection, options
	service.calls++
	return resources.ListResult[resources.GatewayDTO]{Items: []resources.GatewayDTO{{Name: "edge"}}, Page: resources.PageDTO{Limit: options.Limit, Complete: true, FilterScope: resources.FilterScopeCollection}}, nil
}

func (service *gatewayServiceStub) GetGatewayResource(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, collection, namespace, name string) (resources.GatewayDTO, error) {
	service.collection, service.namespace, service.name = collection, namespace, name
	service.calls++
	return resources.GatewayDTO{Name: name, Namespace: namespace}, nil
}

func TestGatewayHandlersEveryKindPreservesSelectionAndIdentity(t *testing.T) {
	t.Parallel()
	for _, resource := range resourcecatalog.GatewayResources() {
		t.Run(resource.Collection, func(t *testing.T) {
			t.Parallel()
			codec, err := api.NewCursorCodec()
			if err != nil {
				t.Fatal(err)
			}
			service := &gatewayServiceStub{}
			selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "web", ScopeSource: "saved", Namespaces: []string{"web"}}}
			handler := NewResources(service, nil, selection, codec)
			query := "?limit=50&fieldSelector=metadata.name%3Dedge"
			namespace := ""
			if resource.Namespaced {
				query += "&namespace=web"
				namespace = "web"
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/"+resource.Collection+query, nil)
			response := httptest.NewRecorder()
			handler.GatewayList(resource.Collection)(response, request)
			if response.Code != http.StatusOK || service.collection != resource.Collection || service.options.FieldSelector != "metadata.name=edge" || service.options.Limit != 50 || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("list=%d %s / %#v", response.Code, response.Body, service)
			}
			request = httptest.NewRequest(http.MethodGet, "/api/v1/"+resource.Collection+"/edge", nil)
			request.SetPathValue("namespace", namespace)
			request.SetPathValue("name", "edge")
			response = httptest.NewRecorder()
			handler.GatewayDetail(resource.Collection)(response, request)
			if response.Code != http.StatusOK || service.namespace != namespace || service.name != "edge" || service.calls != 2 {
				t.Fatalf("detail=%d %s / %#v", response.Code, response.Body, service)
			}
			var envelope struct {
				Data resources.GatewayDTO `json:"data"`
				Meta struct {
					Generation string `json:"generation"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Meta.Generation != "gen" || envelope.Data.Name != "edge" {
				t.Fatalf("invalid envelope: %#v %v", envelope, err)
			}
		})
	}
}

func TestGatewayHandlersRejectUnavailableServiceAndOutsideScope(t *testing.T) {
	t.Parallel()
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "web", ScopeSource: "saved", Namespaces: []string{"web"}}}
	handler := NewResources(&resourceServiceStub{}, nil, selection, codec)
	response := httptest.NewRecorder()
	handler.GatewayList("gateways")(response, httptest.NewRequest(http.MethodGet, "/api/v1/gateways", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable service = %d", response.Code)
	}
	service := &gatewayServiceStub{}
	handler = NewResources(service, nil, selection, codec)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/gateways/infra/edge", nil)
	request.SetPathValue("namespace", "infra")
	request.SetPathValue("name", "edge")
	response = httptest.NewRecorder()
	handler.GatewayDetail("gateways")(response, request)
	if response.Code != http.StatusBadRequest || service.calls != 0 {
		t.Fatalf("outside scope: %d %s, calls=%d", response.Code, response.Body, service.calls)
	}
}
