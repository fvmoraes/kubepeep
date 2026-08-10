package handlers

import (
	"context"
	"errors"
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
