package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/fvmoraes/ginger/pkg/response"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type PermissionMatrixService interface {
	Matrix(context.Context, authorization.PermissionsRequest) (authorization.CapabilityMatrix, error)
}

type SelectionReader interface {
	Snapshot() (namespaces.SelectionBinding, namespaces.ScopeResolution)
}

type Permissions struct {
	service   PermissionMatrixService
	selection SelectionReader
}

func NewPermissions(service PermissionMatrixService, selection SelectionReader) *Permissions {
	return &Permissions{service: service, selection: selection}
}

func (h *Permissions) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, err := decodePermissionsQuery(r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution := h.selection.Snapshot()
	if binding.ClusterProfileID <= 0 || binding.Context == "" || binding.Generation == "" {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "No active Kubernetes selection is available.", nil, nil))
		return
	}
	request.Generation = binding.Generation
	request.ActiveNamespaces = resolution.Namespaces
	matrix, matrixErr := h.service.Matrix(r.Context(), request)
	current, _ := h.selection.Snapshot()
	if current.Generation != binding.Generation || current.ClusterProfileID != binding.ClusterProfileID || current.Context != binding.Context {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed during permission evaluation.", nil, nil))
		return
	}
	if matrixErr != nil {
		api.WriteError(w, r, authorizationHTTPError(matrixErr, matrix))
		return
	}
	noStore(w)
	response.OK(w, matrix)
}

func decodePermissionsQuery(r *http.Request) (authorization.PermissionsRequest, error) {
	query := r.URL.Query()
	for key, values := range query {
		switch key {
		case "namespace", "capability", "resourceName":
			for _, value := range values {
				if value == "" {
					return authorization.PermissionsRequest{}, validationHTTPError("Query values must not be empty.", nil)
				}
			}
		case "refresh":
			if len(values) != 1 || values[0] == "" {
				return authorization.PermissionsRequest{}, validationHTTPError("refresh must be one boolean value.", nil)
			}
		default:
			return authorization.PermissionsRequest{}, validationHTTPError("The permissions query contains an unknown field.", nil)
		}
	}
	request := authorization.PermissionsRequest{
		Namespaces:    append([]string(nil), query["namespace"]...),
		CapabilityIDs: append([]string(nil), query["capability"]...),
		ResourceNames: append([]string(nil), query["resourceName"]...),
	}
	if raw := query.Get("refresh"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return authorization.PermissionsRequest{}, validationHTTPError("refresh must be true or false.", nil)
		}
		request.Refresh = value
	}
	return request, nil
}

func authorizationHTTPError(err error, matrix authorization.CapabilityMatrix) error {
	var public *authorization.PublicError
	if errors.As(err, &public) {
		details := any(nil)
		if public.Code == authorization.CodeAuthorizationUnavailable && len(matrix.Decisions) != 0 {
			details = matrix
		}
		return api.NewHTTPError(public.HTTPStatus, string(public.Code), public.Message, details, err)
	}
	return api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Permissions are temporarily unavailable.", nil, err)
}
