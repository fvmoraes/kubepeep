package handlers

import (
	"context"
	"net/http"

	"github.com/fvmoraes/ginger/pkg/response"
	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type resourceYAMLService interface {
	ReadResourceYAML(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, actions.RouteTarget) (actions.ResourceYAMLDocument, error)
	UpdateResourceYAML(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, actions.RouteTarget, actions.ResourceYAMLRequest) (actions.ActionAcceptedDTO, error)
}

func (handler *ActionHandlers) ResourceYAML(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if err := validateDetailRequest(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	service, ok := handler.actions.(resourceYAMLService)
	if !ok {
		api.WriteError(w, r, api.NewHTTPError(http.StatusNotImplemented, api.CodeInternal, "YAML editing is unavailable.", nil, nil))
		return
	}
	binding, resolution := handler.selection.Snapshot()
	route := actions.RouteTarget{Kind: r.PathValue("collection"), Namespace: r.PathValue("namespace"), Name: r.PathValue("name")}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		document, err := service.ReadResourceYAML(r.Context(), binding, resolution, route)
		if err != nil {
			api.WriteError(w, r, actionHTTPError(err))
			return
		}
		current, _ := handler.selection.Snapshot()
		if current.Generation != binding.Generation {
			api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed.", nil, nil))
			return
		}
		response.OK(w, document)
		return
	}
	var request actions.ResourceYAMLRequest
	// JSON escaping can expand every source byte to six bytes.
	if err := api.DecodeStrict(w, r, &request, actions.MaximumResourceYAMLBytes*6+8192); err != nil {
		api.WriteError(w, r, err)
		return
	}
	result, err := service.UpdateResourceYAML(r.Context(), binding, resolution, route, request)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	response.OK(w, result)
}
