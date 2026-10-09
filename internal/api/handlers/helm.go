package handlers

import (
	"context"
	"net/http"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type HelmService interface {
	ListHelmReleases(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, resourcecore.ListOptions, *resourcecore.CompositeCursor[resourcecore.HelmReleaseDTO]) (resourcecore.ListResult[resourcecore.HelmReleaseDTO], error)
	GetHelmRelease(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string, string) (resourcecore.HelmReleaseDTO, error)
	HelmDocument(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string, string, string) (resourcecore.HelmDocumentDTO, error)
	MutateHelmRelease(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string, string, resourcecore.HelmMutationRequest) (resourcecore.HelmMutationResult, error)
}

func (handler *Resources) helmService(w http.ResponseWriter, r *http.Request) (HelmService, bool) {
	if _, ok := resourcecore.HelmCollection(r.PathValue("driver")); !ok {
		api.WriteError(w, r, validationHTTPError("Choose Secrets or ConfigMaps as Helm storage.", nil))
		return nil, false
	}
	service, ok := handler.service.(HelmService)
	if !ok {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The Helm integration is unavailable.", nil, nil))
	}
	return service, ok
}
func (handler *Resources) HelmList(w http.ResponseWriter, r *http.Request) {
	service, ok := handler.helmService(w, r)
	if !ok {
		return
	}
	collection, _ := resourcecore.HelmCollection(r.PathValue("driver"))
	handleList(handler, w, r, collection, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution, o resourcecore.ListOptions, c *resourcecore.CompositeCursor[resourcecore.HelmReleaseDTO]) (resourcecore.ListResult[resourcecore.HelmReleaseDTO], error) {
		return service.ListHelmReleases(ctx, b, s, r.PathValue("driver"), o, c)
	})
}
func (handler *Resources) HelmDetail(w http.ResponseWriter, r *http.Request) {
	service, ok := handler.helmService(w, r)
	if !ok {
		return
	}
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return service.GetHelmRelease(ctx, b, s, r.PathValue("driver"), r.PathValue("namespace"), r.PathValue("name"))
	})
}
func (handler *Resources) HelmDocument(w http.ResponseWriter, r *http.Request) {
	service, ok := handler.helmService(w, r)
	if !ok {
		return
	}
	if r.PathValue("format") != "values" && r.PathValue("format") != "manifest" {
		api.WriteError(w, r, validationHTTPError("Choose values or manifest.", nil))
		return
	}
	handler.detail(w, r, func(ctx context.Context, b namespaces.SelectionBinding, s namespaces.ScopeResolution) (any, error) {
		return service.HelmDocument(ctx, b, s, r.PathValue("driver"), r.PathValue("namespace"), r.PathValue("name"), r.PathValue("format"))
	})
}
func (handler *Resources) HelmApply(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	service, ok := handler.helmService(w, r)
	if !ok {
		return
	}
	if err := validateDetailRequest(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, scope, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err := validateResourcePath(r, scope); err != nil {
		api.WriteError(w, r, err)
		return
	}
	var command resourcecore.HelmMutationRequest
	if err := api.DecodeStrict(w, r, &command, resourcecore.MaximumHelmDocumentBytes*6+8192); err != nil {
		api.WriteError(w, r, err)
		return
	}
	result, err := service.MutateHelmRelease(r.Context(), binding, scope, r.PathValue("driver"), r.PathValue("namespace"), r.PathValue("name"), command)
	if err != nil {
		if apierrors.IsConflict(err) {
			api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeConflict, "The Helm release changed or another operation is pending. Reload its history and resource status before retrying; an operation already started may have applied some resources.", nil, err))
		} else {
			api.WriteError(w, r, resourceHTTPError(err))
		}
		return
	}
	handler.writeJSONIfCurrent(w, r, binding, map[string]any{"data": result, "meta": map[string]any{"generation": binding.Generation}})
}
