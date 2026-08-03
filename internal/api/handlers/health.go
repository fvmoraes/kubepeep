package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/fvmoraes/ginger/pkg/response"

	"github.com/fvmoraes/kubepeep/internal/api"
)

const handlerSnapshotTimeout = 3 * time.Second

type Health struct {
	snapshots api.SnapshotProvider
}

func NewHealth(snapshots api.SnapshotProvider) *Health {
	return &Health{snapshots: snapshots}
}

func (h *Health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	ctx, cancel := context.WithTimeout(r.Context(), handlerSnapshotTimeout)
	defer cancel()
	snapshot, err := h.snapshots.Snapshot(ctx)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err := api.ValidateSnapshot(snapshot); err != nil {
		api.WriteError(w, r, err)
		return
	}
	data := api.HealthDataFromSnapshot(snapshot)
	status := http.StatusOK
	if data.Status == api.StatusUnhealthy {
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// response.Envelope is the Ginger envelope reused by the Kube Peep DTO.
	_ = writeEnvelope(w, response.Envelope[api.HealthData]{Data: data})
}
