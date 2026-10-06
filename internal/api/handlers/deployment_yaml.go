package handlers

import (
	"context"
	"net/http"

	"github.com/fvmoraes/ginger/pkg/response"
	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type deploymentYAMLService interface {
	UpdateDeploymentYAML(context.Context, namespaces.SelectionBinding, actions.RouteTarget, actions.DeploymentYAMLRequest) (actions.ActionAcceptedDTO, error)
}

func (handler *ActionHandlers) UpdateDeploymentYAML(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var request actions.DeploymentYAMLRequest
	if err := api.DecodeStrict(w, r, &request, actions.MaximumDeploymentYAMLBytes*2); err != nil {
		api.WriteError(w, r, err)
		return
	}
	service, ok := handler.actions.(deploymentYAMLService)
	if !ok {
		api.WriteError(w, r, api.NewHTTPError(http.StatusNotImplemented, api.CodeInternal, "Deployment editing is unavailable.", nil, nil))
		return
	}
	binding, _ := handler.selection.Snapshot()
	result, err := service.UpdateDeploymentYAML(r.Context(), binding, workloadRouteTarget(r), request)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	response.OK(w, result)
}
