package handlers

import (
	"context"
	"net/http"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type DiagnosticsService interface {
	Diagnostics(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) dashboard.DiagnosticsDTO
}

type DiagnosticsHandler struct {
	service   DiagnosticsService
	selection SelectionReader
}

func NewDiagnostics(service DiagnosticsService, selection SelectionReader) *DiagnosticsHandler {
	return &DiagnosticsHandler{service: service, selection: selection}
}

func (handler *DiagnosticsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if handler == nil || handler.service == nil || handler.selection == nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "Diagnostics are unavailable.", nil, nil))
		return
	}
	if r.URL.RawQuery != "" {
		api.WriteError(w, r, api.NewHTTPError(http.StatusBadRequest, api.CodeValidationFailed, "The diagnostics endpoint does not accept query parameters.", nil, nil))
		return
	}
	binding, resolution := handler.selection.Snapshot()
	if binding.Generation == "" {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "No active Kubernetes selection is available.", nil, nil))
		return
	}
	value := handler.service.Diagnostics(r.Context(), binding, resolution)
	(&InvestigationHandler{selection: handler.selection}).writeCurrent(w, r, binding, value)
}
