package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/fvmoraes/ginger/pkg/router"

	"github.com/fvmoraes/kubepeep/internal/api"
)

type Dependencies struct {
	Snapshots   api.SnapshotProvider
	Sessions    *api.SessionStore
	Generation  api.GenerationSource
	Profiles    ClusterProfileService
	Scopes      NamespaceScopeService
	Namespaces  NamespaceCatalog
	Permissions PermissionMatrixService
	Selection   SelectionReader
	Contexts    ContextService
	Dashboard   DashboardService
	Cursors     *api.CursorCodec
	Origin      string
	Port        int
	Build       api.BuildInfo
}

const (
	apiPrefix    = "/api/v1"
	statusPath   = apiPrefix + "/status"
	sessionPath  = apiPrefix + "/session"
	profilesPath = apiPrefix + "/cluster/profiles"
	profilePath  = apiPrefix + "/cluster/profile"
)

func Register(applicationRouter *router.Router, dependencies Dependencies) {
	apiRouter := applicationRouter.Group(apiPrefix)
	status := NewStatus(dependencies.Snapshots, dependencies.Build, dependencies.Port, dependencies.Generation)
	session := NewSession(dependencies.Sessions, dependencies.Generation, dependencies.Origin)
	apiRouter.GET("/status", status.ServeHTTP)
	apiRouter.GET("/session", session.ServeHTTP)
	if dependencies.Profiles != nil {
		profiles := NewClusterProfiles(dependencies.Profiles)
		apiRouter.GET("/cluster/profiles", profiles.List)
		apiRouter.GET("/cluster/profile", profiles.Active)
	}
	if dependencies.Contexts != nil {
		contexts := NewContexts(dependencies.Contexts)
		apiRouter.GET("/contexts", contexts.List)
		apiRouter.POST("/contexts/select", contexts.Select)
	}
	if dependencies.Scopes != nil && dependencies.Selection != nil {
		scopes := NewNamespaceScopes(dependencies.Scopes, dependencies.Selection, dependencies.Namespaces, dependencies.Snapshots).WithCursors(dependencies.Cursors)
		apiRouter.GET("/namespaces", scopes.ListNamespaces)
		apiRouter.GET("/namespace-scopes", scopes.List)
		apiRouter.POST("/namespace-scopes", scopes.Create)
		apiRouter.POST("/namespace-scopes/validate", scopes.Validate)
		apiRouter.GET("/namespace-scopes/{id}", scopes.Get)
		apiRouter.PUT("/namespace-scopes/{id}", scopes.Update)
		apiRouter.DELETE("/namespace-scopes/{id}", scopes.Delete)
		apiRouter.POST("/namespace-scopes/{id}/select", scopes.Select)
	}
	if dependencies.Permissions != nil && dependencies.Selection != nil {
		apiRouter.GET("/permissions", NewPermissions(dependencies.Permissions, dependencies.Selection).ServeHTTP)
	}
	if dependencies.Dashboard != nil && dependencies.Selection != nil {
		dashboard := NewDashboard(dependencies.Dashboard, dependencies.Selection, dependencies.Cursors)
		apiRouter.GET("/dashboard/summary", dashboard.Summary)
		apiRouter.GET("/dashboard/problems", dashboard.Problems)
		apiRouter.GET("/dashboard/restarts", dashboard.Restarts)
		apiRouter.GET("/dashboard/events", dashboard.Events)
		apiRouter.POST("/dashboard/log-scan", dashboard.LogScan)
		apiRouter.GET("/metrics", dashboard.Metrics)
	}
}

// NewAPIFallback keeps reserved API paths out of the SPA while preserving the
// public JSON error contract. It is registered as a method-agnostic fallback,
// after which the ServeMux still gives the concrete method routes precedence.
func NewAPIFallback() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allow, known := allowedMethods(r.URL.Path); known {
			w.Header().Set("Allow", allow)
			api.WriteError(w, r, api.NewHTTPError(
				http.StatusMethodNotAllowed,
				api.CodeMethodNotAllowed,
				"The HTTP method is not allowed for this API route.",
				nil,
				nil,
			))
			return
		}
		api.WriteError(w, r, api.NewHTTPError(
			http.StatusNotFound,
			api.CodeNotFound,
			"The requested API route was not found.",
			nil,
			nil,
		))
	})
}

func allowedMethods(path string) (string, bool) {
	switch path {
	case statusPath, sessionPath, profilesPath, profilePath,
		apiPrefix + "/contexts", apiPrefix + "/namespaces", apiPrefix + "/permissions",
		apiPrefix + "/dashboard/summary", apiPrefix + "/dashboard/problems",
		apiPrefix + "/dashboard/restarts", apiPrefix + "/dashboard/events", apiPrefix + "/metrics":
		return "GET, HEAD", true
	case apiPrefix + "/contexts/select", apiPrefix + "/namespace-scopes/validate", apiPrefix + "/dashboard/log-scan":
		return "POST", true
	case apiPrefix + "/namespace-scopes":
		return "GET, HEAD, POST", true
	}
	prefix := apiPrefix + "/namespace-scopes/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	remainder := strings.TrimPrefix(path, prefix)
	if remainder == "" || strings.Contains(remainder, "/") && !strings.HasSuffix(remainder, "/select") {
		return "", false
	}
	if strings.HasSuffix(remainder, "/select") {
		id := strings.TrimSuffix(remainder, "/select")
		return "POST", id != "" && !strings.Contains(id, "/")
	}
	return "GET, HEAD, PUT, DELETE", true
}

func writeEnvelope(w http.ResponseWriter, value any) error {
	return json.NewEncoder(w).Encode(value)
}
