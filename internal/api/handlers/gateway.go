package handlers

import (
	"context"
	"net/http"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resourcecatalog"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
)

type GatewayService interface {
	ListGatewayResources(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.GatewayDTO]) (resourcecore.ListResult[resourcecore.GatewayDTO], error)
	GetGatewayResource(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string, string) (resourcecore.GatewayDTO, error)
}

func (handler *Resources) GatewayList(collection string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		service, ok := handler.service.(GatewayService)
		if !ok {
			api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The Gateway API reader is unavailable.", nil, nil))
			return
		}
		resource, _ := resourcecatalog.Lookup(collection)
		call := func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.GatewayDTO]) (resourcecore.ListResult[resourcecore.GatewayDTO], error) {
			return service.ListGatewayResources(ctx, b, s, collection, o, c)
		}
		if resource.Namespaced {
			handleList(handler, w, r, resourcecore.Collection(collection), call)
		} else {
			handleClusterList(handler, w, r, resourcecore.Collection(collection), call)
		}
	}
}

func (handler *Resources) GatewayDetail(collection string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		service, ok := handler.service.(GatewayService)
		if !ok {
			api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The Gateway API reader is unavailable.", nil, nil))
			return
		}
		resource, _ := resourcecatalog.Lookup(collection)
		call := func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
			return service.GetGatewayResource(ctx, b, s, collection, r.PathValue("namespace"), r.PathValue("name"))
		}
		if resource.Namespaced {
			handler.detail(w, r, call)
		} else {
			handler.clusterDetail(w, r, call)
		}
	}
}
