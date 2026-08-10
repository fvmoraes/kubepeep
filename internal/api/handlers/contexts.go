package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/fvmoraes/ginger/pkg/response"

	"github.com/fvmoraes/kubepeep/internal/api"
	contextservice "github.com/fvmoraes/kubepeep/internal/services/contexts"
)

const maxContextBodyBytes = 1 << 20

type ContextService interface {
	List(context.Context, int64) ([]contextservice.ContextDTO, error)
	Select(context.Context, contextservice.SelectRequest) (contextservice.SelectionDTO, error)
}

type Contexts struct{ service ContextService }

func NewContexts(service ContextService) *Contexts { return &Contexts{service: service} }

func (h *Contexts) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query) != 1 || len(query["clusterProfileId"]) != 1 || query.Get("clusterProfileId") == "" {
		api.WriteError(w, r, validationHTTPError("clusterProfileId is required and must be unique.", nil))
		return
	}
	profileID, err := strconv.ParseInt(query.Get("clusterProfileId"), 10, 64)
	if err != nil || profileID <= 0 {
		api.WriteError(w, r, validationHTTPError("clusterProfileId must be a positive integer.", nil))
		return
	}
	items, err := h.service.List(r.Context(), profileID)
	if err != nil {
		api.WriteError(w, r, contextHTTPError(err))
		return
	}
	noStore(w)
	response.OK(w, items)
}

func (h *Contexts) Select(w http.ResponseWriter, r *http.Request) {
	var request contextservice.SelectRequest
	limited := http.MaxBytesReader(w, r.Body, maxContextBodyBytes)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			api.WriteError(w, r, api.NewHTTPError(http.StatusRequestEntityTooLarge, api.CodeBodyTooLarge, "The request body is too large.", nil, nil))
			return
		}
		if isUnknownJSONField(err) {
			api.WriteError(w, r, api.NewHTTPError(http.StatusBadRequest, api.CodeUnknownField, "The JSON body contains an unknown field.", nil, nil))
			return
		}
		api.WriteError(w, r, api.NewHTTPError(http.StatusBadRequest, api.CodeInvalidJSON, "The JSON body is not valid.", nil, nil))
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusBadRequest, api.CodeInvalidJSON, "The JSON body contains trailing content.", nil, nil))
		return
	}
	selected, err := h.service.Select(r.Context(), request)
	if err != nil {
		api.WriteError(w, r, contextHTTPError(err))
		return
	}
	noStore(w)
	response.OK(w, selected)
}

func contextHTTPError(err error) error {
	switch {
	case errors.Is(err, contextservice.ErrValidation):
		return validationHTTPError("The context selection is invalid.", nil)
	case errors.Is(err, contextservice.ErrNotFound):
		return api.NewHTTPError(http.StatusNotFound, api.CodeNotFound, "The cluster profile was not found.", nil, err)
	case errors.Is(err, contextservice.ErrGenerationChange):
		return api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed before this operation.", nil, err)
	}
	var external *contextservice.ExternalError
	if errors.As(err, &external) {
		status := http.StatusServiceUnavailable
		switch external.Code {
		case api.CodeKubeconfigNotFound, api.CodeContextNotFound:
			status = http.StatusNotFound
		case api.CodeKubeconfigInvalid:
			status = http.StatusBadRequest
		case api.CodeGenerationChanged:
			status = http.StatusConflict
		}
		return api.NewHTTPError(status, external.Code, external.Message, nil, err)
	}
	return api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Contexts are temporarily unavailable.", nil, err)
}

func isUnknownJSONField(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "json: unknown field ")
}
