package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
	"k8s.io/apimachinery/pkg/util/validation"
)

type InvestigationService interface {
	Investigation(generation, kind, namespace, name string, expectedNamespaces []string) resourcecore.Investigation
	LocalIndex(generation string, expectedNamespaces []string) resourcecore.LocalIndexSnapshot
}

type InvestigationHandler struct {
	service   InvestigationService
	selection SelectionReader
}

func NewInvestigation(service InvestigationService, selection SelectionReader) *InvestigationHandler {
	return &InvestigationHandler{service: service, selection: selection}
}

func (handler *InvestigationHandler) Get(w http.ResponseWriter, r *http.Request) {
	if handler == nil || handler.service == nil || handler.selection == nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The local investigation index is unavailable.", nil, nil))
		return
	}
	if r.URL.RawQuery != "" {
		api.WriteError(w, r, api.NewHTTPError(http.StatusBadRequest, api.CodeValidationFailed, "The investigation endpoint does not accept query parameters.", nil, nil))
		return
	}
	binding, resolution := handler.selection.Snapshot()
	if binding.Generation == "" {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The local investigation index is unavailable.", nil, nil))
		return
	}
	kind, namespace, name := strings.TrimSpace(r.PathValue("kind")), strings.TrimSpace(r.PathValue("namespace")), strings.TrimSpace(r.PathValue("name"))
	if !investigationKind(kind) || len(validation.IsDNS1123Subdomain(namespace)) > 0 || len(validation.IsDNS1123Subdomain(name)) > 0 || !investigationInScope(resolution.Namespaces, namespace) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusBadRequest, api.CodeValidationFailed, "The investigation target is invalid or outside the active namespace scope.", nil, nil))
		return
	}
	value := handler.service.Investigation(binding.Generation, kind, namespace, name, resolution.Namespaces)
	handler.writeCurrent(w, r, binding, value)
}

func (handler *InvestigationHandler) Index(w http.ResponseWriter, r *http.Request) {
	if handler == nil || handler.service == nil || handler.selection == nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The local resource index is unavailable.", nil, nil))
		return
	}
	binding, resolution := handler.selection.Snapshot()
	if binding.Generation == "" {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The local resource index is unavailable.", nil, nil))
		return
	}
	if r.URL.RawQuery != "" {
		api.WriteError(w, r, api.NewHTTPError(http.StatusBadRequest, api.CodeValidationFailed, "The local index endpoint does not accept query parameters.", nil, nil))
		return
	}
	handler.writeCurrent(w, r, binding, handler.service.LocalIndex(binding.Generation, resolution.Namespaces))
}

func (handler *InvestigationHandler) writeCurrent(w http.ResponseWriter, r *http.Request, binding namespaces.SelectionBinding, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "The investigation response could not be encoded.", nil, err))
		return
	}
	write := func() {
		noStore(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}
	if fenced, ok := handler.selection.(interface {
		IfCurrent(namespaces.SelectionBinding, func()) bool
	}); ok {
		if !fenced.IfCurrent(binding, write) {
			api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed before the investigation was published.", nil, nil))
		}
		return
	}
	current, _ := handler.selection.Snapshot()
	if !sameSelectionBinding(current, binding) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed before the investigation was published.", nil, nil))
		return
	}
	write()
}

func investigationKind(value string) bool {
	switch strings.ToLower(value) {
	case "pod", "deployment", "replicaset", "statefulset", "daemonset", "job", "cronjob":
		return true
	default:
		return false
	}
}

func investigationInScope(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
