package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
	pathvalidation "k8s.io/apimachinery/pkg/api/validation/path"
)

type DynamicResourceService interface {
	DiscoverResources(context.Context, namespaces.SelectionBinding, bool) (resourcecore.ResourceDiscoveryDTO, error)
	ListDynamicResources(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.DynamicResource, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.DynamicRow]) (resourcecore.ListResult[resourcecore.DynamicRow], error)
	GetDynamicResource(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.DynamicResource, string, string) (resourcecore.DynamicRow, error)
	GetDynamicYAML(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.DynamicResource, string, string) (resourcecore.DynamicDocument, error)
}

func (handler *Resources) dynamicService(w http.ResponseWriter, r *http.Request) (DynamicResourceService, bool) {
	service, ok := handler.service.(DynamicResourceService)
	if !ok {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "Dynamic resource views are unavailable.", nil, nil))
	}
	return service, ok
}
func dynamicTarget(r *http.Request) (resourcecore.DynamicResource, error) {
	group := r.PathValue("group")
	if group == "_" {
		group = ""
	}
	target := resourcecore.DynamicResource{Group: group, Version: r.PathValue("version"), Resource: r.PathValue("resource"), Namespaced: r.PathValue("scope") == "n"}
	if !target.Valid() || (r.PathValue("scope") != "n" && r.PathValue("scope") != "c") {
		return target, validationHTTPError("The resource identity is invalid.", nil)
	}
	return target, nil
}
func (handler *Resources) ResourceDiscovery(w http.ResponseWriter, r *http.Request) {
	service, ok := handler.dynamicService(w, r)
	if !ok {
		return
	}
	values := r.URL.Query()
	if len(values) > 1 || (len(values) > 0 && (len(values["refresh"]) != 1 || values.Get("refresh") != "true")) {
		api.WriteError(w, r, validationHTTPError("Only refresh=true is supported.", nil))
		return
	}
	binding, _, err := handler.clusterSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	result, err := service.DiscoverResources(r.Context(), binding, values.Get("refresh") == "true")
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	handler.writeJSONIfCurrent(w, r, binding, map[string]any{"data": result, "meta": map[string]any{"generation": binding.Generation}})
}
func (handler *Resources) DynamicList(w http.ResponseWriter, r *http.Request) {
	service, ok := handler.dynamicService(w, r)
	if !ok {
		return
	}
	target, err := dynamicTarget(r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	call := func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.DynamicRow]) (resourcecore.ListResult[resourcecore.DynamicRow], error) {
		return service.ListDynamicResources(ctx, b, s, target, o, c)
	}
	if target.Namespaced {
		handleList(handler, w, r, target.Collection(), call)
	} else {
		handleClusterList(handler, w, r, target.Collection(), call)
	}
}
func (handler *Resources) DynamicDetail(w http.ResponseWriter, r *http.Request) {
	handler.dynamicDetail(w, r, false)
}
func (handler *Resources) DynamicYAML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	handler.dynamicDetail(w, r, true)
}
func (handler *Resources) dynamicDetail(w http.ResponseWriter, r *http.Request, document bool) {
	service, ok := handler.dynamicService(w, r)
	if !ok {
		return
	}
	target, err := dynamicTarget(r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err := validateDetailRequest(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.clusterSelection()
	if target.Namespaced {
		binding, resolution, err = handler.activeSelection()
	}
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	namespace, name := r.PathValue("namespace"), r.PathValue("name")
	if name == "" || len(name) > 1024 || len(pathvalidation.IsValidPathSegmentName(name)) > 0 || strings.ContainsAny(name, "?#") {
		api.WriteError(w, r, validationHTTPError("The resource name is invalid.", nil))
		return
	}
	if target.Namespaced {
		if _, err := resourcecore.ResolveNamespaces(resolution.Namespaces, []string{namespace}); err != nil {
			api.WriteError(w, r, resourceHTTPError(err))
			return
		}
	} else {
		if namespace != "_" {
			api.WriteError(w, r, validationHTTPError("Cluster resources use the cluster namespace marker.", nil))
			return
		}
		namespace = ""
	}
	var result any
	if document {
		result, err = service.GetDynamicYAML(r.Context(), binding, resolution, target, namespace, name)
	} else {
		result, err = service.GetDynamicResource(r.Context(), binding, resolution, target, namespace, name)
	}
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	handler.writeJSONIfCurrent(w, r, binding, map[string]any{"data": result, "meta": map[string]any{"generation": binding.Generation, "collectedAt": handler.now().UTC().Format(time.RFC3339Nano)}})
}
