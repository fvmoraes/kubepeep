package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/fvmoraes/ginger/pkg/response"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
)

type ClusterProfileService interface {
	List(context.Context) ([]clusterprofiles.DTO, error)
	Active(context.Context) (clusterprofiles.DTO, error)
}

type ClusterProfiles struct {
	service ClusterProfileService
}

func NewClusterProfiles(service ClusterProfileService) *ClusterProfiles {
	return &ClusterProfiles{service: service}
}

func (h *ClusterProfiles) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.service.List(r.Context())
	if err != nil {
		api.WriteError(w, r, profileHTTPError(err))
		return
	}
	noStore(w)
	response.OK(w, profiles)
}

func (h *ClusterProfiles) Active(w http.ResponseWriter, r *http.Request) {
	profile, err := h.service.Active(r.Context())
	if err != nil {
		api.WriteError(w, r, profileHTTPError(err))
		return
	}
	noStore(w)
	response.OK(w, profile)
}

func (h *ClusterProfiles) Import(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	service, ok := h.service.(interface {
		Import(context.Context, clusterprofiles.ImportRequest) (clusterprofiles.DTO, error)
	})
	if !ok {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "Kubeconfig import is unavailable.", nil, nil))
		return
	}
	var request clusterprofiles.ImportRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		api.WriteError(w, r, validationHTTPError("Provide one kubeconfig file path or its contents (up to 1 MiB).", nil))
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		api.WriteError(w, r, validationHTTPError("The request contains trailing content.", nil))
		return
	}
	profile, err := service.Import(r.Context(), request)
	if err != nil {
		switch {
		case errors.Is(err, clusterprofiles.ErrImportRelativePaths):
			api.WriteError(w, r, validationHTTPError("This kubeconfig uses relative credential paths. Use its original local file path, or embed certificate data before importing.", nil))
		case errors.Is(err, clusterprofiles.ErrImportInvalid):
			api.WriteError(w, r, validationHTTPError("The kubeconfig must be a valid file up to 1 MiB with at least one complete context.", nil))
		case errors.Is(err, clusterprofiles.ErrImportConflict):
			api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, "KUBECONFIG_CONFLICT", "An existing kubeconfig entry has different settings. Rename the imported entry or use its original file path. Existing entries were preserved.", nil, nil))
		default:
			api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "The kubeconfig could not be saved or opened. Check the local path and file permissions.", nil, nil))
		}
		return
	}
	response.OK(w, profile)
}

func profileHTTPError(err error) error {
	if errors.Is(err, clusterprofiles.ErrNotFound) {
		return api.NewHTTPError(http.StatusNotFound, api.CodeNotFound, "No active cluster profile was found.", nil, err)
	}
	return api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Cluster profiles are temporarily unavailable.", nil, err)
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
