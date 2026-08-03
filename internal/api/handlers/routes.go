package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/fvmoraes/ginger/pkg/router"

	"github.com/fvmoraes/kubepeep/internal/api"
)

type Dependencies struct {
	Snapshots  api.SnapshotProvider
	Sessions   *api.SessionStore
	Generation api.GenerationSource
	Origin     string
	Port       int
	Build      api.BuildInfo
}

const (
	apiPrefix   = "/api/v1"
	statusPath  = apiPrefix + "/status"
	sessionPath = apiPrefix + "/session"
)

func Register(applicationRouter *router.Router, dependencies Dependencies) {
	apiRouter := applicationRouter.Group(apiPrefix)
	status := NewStatus(dependencies.Snapshots, dependencies.Build, dependencies.Port, dependencies.Generation)
	session := NewSession(dependencies.Sessions, dependencies.Generation, dependencies.Origin)
	apiRouter.GET("/status", status.ServeHTTP)
	apiRouter.GET("/session", session.ServeHTTP)
}

// NewAPIFallback keeps reserved API paths out of the SPA while preserving the
// public JSON error contract. It is registered as a method-agnostic fallback,
// after which the ServeMux still gives the concrete method routes precedence.
func NewAPIFallback() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case statusPath, sessionPath:
			w.Header().Set("Allow", "GET, HEAD")
			api.WriteError(w, r, api.NewHTTPError(
				http.StatusMethodNotAllowed,
				api.CodeMethodNotAllowed,
				"The HTTP method is not allowed for this API route.",
				nil,
				nil,
			))
		default:
			api.WriteError(w, r, api.NewHTTPError(
				http.StatusNotFound,
				api.CodeNotFound,
				"The requested API route was not found.",
				nil,
				nil,
			))
		}
	})
}

func writeEnvelope(w http.ResponseWriter, value any) error {
	return json.NewEncoder(w).Encode(value)
}
