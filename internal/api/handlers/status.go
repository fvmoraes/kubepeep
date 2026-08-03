package handlers

import (
	"context"
	"net/http"

	"github.com/fvmoraes/ginger/pkg/response"

	"github.com/fvmoraes/kubepeep/internal/api"
)

type Status struct {
	snapshots  api.SnapshotProvider
	build      api.BuildInfo
	port       int
	generation api.GenerationSource
}

func NewStatus(snapshots api.SnapshotProvider, build api.BuildInfo, port int, generation api.GenerationSource) *Status {
	return &Status{snapshots: snapshots, build: normalizedBuild(build), port: port, generation: generation}
}

func (h *Status) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	if snapshot.Selection != nil {
		if h.generation == nil {
			api.WriteError(w, r, api.NewHTTPError(
				http.StatusInternalServerError,
				api.CodeInternal,
				"Status is temporarily unavailable.",
				nil,
				nil,
			))
			return
		}
		currentGeneration := h.generation.Current()
		if currentGeneration == "" {
			api.WriteError(w, r, api.NewHTTPError(
				http.StatusInternalServerError,
				api.CodeInternal,
				"Status is temporarily unavailable.",
				nil,
				nil,
			))
			return
		}
		if snapshot.Selection.Generation != "" && snapshot.Selection.Generation != currentGeneration {
			api.WriteError(w, r, api.NewHTTPError(
				http.StatusInternalServerError,
				api.CodeInternal,
				"Status is temporarily unavailable.",
				nil,
				nil,
			))
			return
		}
		selection := *snapshot.Selection
		selection.Generation = currentGeneration
		snapshot.Selection = &selection
	}
	response.OK(w, api.StatusData{
		Version:    h.build.Version,
		Commit:     h.build.Commit,
		BuildDate:  h.build.BuildDate,
		Port:       h.port,
		Components: snapshot.Components,
		Selection:  snapshot.Selection,
	})
}

func normalizedBuild(build api.BuildInfo) api.BuildInfo {
	if build.Version == "" {
		build.Version = "unknown"
	}
	if build.Commit == "" {
		build.Commit = "unknown"
	}
	if build.BuildDate == "" {
		build.BuildDate = "unknown"
	}
	return build
}
