package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/fvmoraes/ginger/pkg/router"

	"github.com/fvmoraes/kubepeep/internal/api"
	actionservice "github.com/fvmoraes/kubepeep/internal/services/actions"
)

type Dependencies struct {
	Snapshots    api.SnapshotProvider
	Sessions     *api.SessionStore
	Generation   api.GenerationSource
	Profiles     ClusterProfileService
	Scopes       NamespaceScopeService
	Namespaces   NamespaceCatalog
	Permissions  PermissionMatrixService
	Selection    SelectionReader
	Contexts     ContextService
	Dashboard    DashboardService
	Resources    ResourceService
	Preferences  PreferenceService
	Actions      actionservice.ActionService
	PortForwards actionservice.PortForwardService
	Exec         actionservice.ExecService
	Cursors      *api.CursorCodec
	Origin       string
	Port         int
	Build        api.BuildInfo
	ExtraOrigins []string
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
		apiRouter.GET("/dashboard/namespace-health", dashboard.NamespaceHealth)
		apiRouter.GET("/dashboard/problems", dashboard.Problems)
		apiRouter.GET("/dashboard/restarts", dashboard.Restarts)
		apiRouter.GET("/dashboard/events", dashboard.Events)
		apiRouter.POST("/dashboard/log-scan", dashboard.LogScan)
		apiRouter.GET("/metrics", dashboard.Metrics)
	}
	if dependencies.Resources != nil && dependencies.Selection != nil {
		resourceHandler := NewResources(dependencies.Resources, dependencies.Preferences, dependencies.Selection, dependencies.Cursors)
		apiRouter.GET("/workloads", resourceHandler.Workloads)
		apiRouter.GET("/workloads/{kind}/{namespace}/{name}", resourceHandler.WorkloadDetail)
		apiRouter.GET("/workloads/{kind}/{namespace}/{name}/yaml", resourceHandler.WorkloadYAML)
		apiRouter.GET("/pods", resourceHandler.Pods)
		apiRouter.GET("/pods/{namespace}/{name}", resourceHandler.PodDetail)
		apiRouter.GET("/pods/{namespace}/{name}/yaml", resourceHandler.PodYAML)
		apiRouter.GET("/pods/{namespace}/{name}/logs", resourceHandler.PodLogs)
		apiRouter.GET("/events", resourceHandler.Events)
		apiRouter.GET("/services", resourceHandler.Services)
		apiRouter.GET("/services/{namespace}/{name}", resourceHandler.ServiceDetail)
		apiRouter.GET("/services/{namespace}/{name}/yaml", resourceHandler.ServiceYAML)
		apiRouter.GET("/ingresses", resourceHandler.Ingresses)
		apiRouter.GET("/ingresses/{namespace}/{name}", resourceHandler.IngressDetail)
		apiRouter.GET("/ingresses/{namespace}/{name}/yaml", resourceHandler.IngressYAML)
		apiRouter.GET("/endpoint-slices", resourceHandler.EndpointSlices)
		apiRouter.GET("/endpoint-slices/{namespace}/{name}", resourceHandler.EndpointSliceDetail)
		apiRouter.GET("/endpoint-slices/{namespace}/{name}/yaml", resourceHandler.EndpointSliceYAML)
		apiRouter.GET("/configmaps", resourceHandler.ConfigMaps)
		apiRouter.GET("/configmaps/{namespace}/{name}", resourceHandler.ConfigMapDetail)
		apiRouter.GET("/configmaps/{namespace}/{name}/yaml", resourceHandler.ConfigMapYAML)
		apiRouter.GET("/resources/{collection}/{namespace}/{name}/yaml-diff", resourceHandler.ResourceYAMLDiff)
		apiRouter.GET("/secrets", resourceHandler.Secrets)
		apiRouter.GET("/secrets/{namespace}/{name}", resourceHandler.SecretDetail)
		apiRouter.GET("/nodes", resourceHandler.Nodes)
		apiRouter.GET("/nodes/{name}", resourceHandler.NodeDetail)
		apiRouter.GET("/nodes/{name}/yaml", resourceHandler.NodeYAML)
		apiRouter.GET("/leases", resourceHandler.Leases)
		apiRouter.GET("/leases/{namespace}/{name}", resourceHandler.LeaseDetail)
		apiRouter.GET("/leases/{namespace}/{name}/yaml", resourceHandler.LeaseYAML)
		apiRouter.GET("/persistent-volumes", resourceHandler.PersistentVolumes)
		apiRouter.GET("/persistent-volumes/{name}", resourceHandler.PersistentVolumeDetail)
		apiRouter.GET("/persistent-volumes/{name}/yaml", resourceHandler.PersistentVolumeYAML)
		apiRouter.GET("/persistent-volume-claims", resourceHandler.PersistentVolumeClaims)
		apiRouter.GET("/persistent-volume-claims/{namespace}/{name}", resourceHandler.PersistentVolumeClaimDetail)
		apiRouter.GET("/persistent-volume-claims/{namespace}/{name}/yaml", resourceHandler.PersistentVolumeClaimYAML)
		apiRouter.GET("/storage-classes", resourceHandler.StorageClasses)
		apiRouter.GET("/storage-classes/{name}", resourceHandler.StorageClassDetail)
		apiRouter.GET("/storage-classes/{name}/yaml", resourceHandler.StorageClassYAML)
		apiRouter.GET("/csi-drivers", resourceHandler.CSIDrivers)
		apiRouter.GET("/csi-drivers/{name}", resourceHandler.CSIDriverDetail)
		apiRouter.GET("/csi-nodes", resourceHandler.CSINodes)
		apiRouter.GET("/csi-nodes/{name}", resourceHandler.CSINodeDetail)
		apiRouter.GET("/volume-attachments", resourceHandler.VolumeAttachments)
		apiRouter.GET("/volume-attachments/{name}", resourceHandler.VolumeAttachmentDetail)
		apiRouter.GET("/namespaces/{name}", resourceHandler.NamespaceDetail)
		apiRouter.GET("/service-accounts", resourceHandler.ServiceAccounts)
		apiRouter.GET("/service-accounts/{namespace}/{name}", resourceHandler.ServiceAccountDetail)
		apiRouter.GET("/resource-quotas", resourceHandler.ResourceQuotas)
		apiRouter.GET("/resource-quotas/{namespace}/{name}", resourceHandler.ResourceQuotaDetail)
		apiRouter.GET("/limit-ranges", resourceHandler.LimitRanges)
		apiRouter.GET("/limit-ranges/{namespace}/{name}", resourceHandler.LimitRangeDetail)
		apiRouter.GET("/hpas", resourceHandler.HPAs)
		apiRouter.GET("/hpas/{namespace}/{name}", resourceHandler.HPADetail)
		apiRouter.GET("/pdbs", resourceHandler.PDBs)
		apiRouter.GET("/pdbs/{namespace}/{name}", resourceHandler.PDBDetail)
		apiRouter.GET("/roles", resourceHandler.Roles)
		apiRouter.GET("/roles/{namespace}/{name}", resourceHandler.RoleDetail)
		apiRouter.GET("/role-bindings", resourceHandler.RoleBindings)
		apiRouter.GET("/role-bindings/{namespace}/{name}", resourceHandler.RoleBindingDetail)
		apiRouter.GET("/network-policies", resourceHandler.NetworkPolicies)
		apiRouter.GET("/network-policies/{namespace}/{name}", resourceHandler.NetworkPolicyDetail)
		apiRouter.GET("/endpoints", resourceHandler.Endpoints)
		apiRouter.GET("/endpoints/{namespace}/{name}", resourceHandler.EndpointsDetail)
		apiRouter.GET("/cluster-roles", resourceHandler.ClusterRoles)
		apiRouter.GET("/cluster-roles/{name}", resourceHandler.ClusterRoleDetail)
		apiRouter.GET("/cluster-role-bindings", resourceHandler.ClusterRoleBindings)
		apiRouter.GET("/cluster-role-bindings/{name}", resourceHandler.ClusterRoleBindingDetail)
		apiRouter.GET("/customresourcedefinitions", resourceHandler.CustomResourceDefinitions)
		apiRouter.GET("/customresourcedefinitions/{name}", resourceHandler.CustomResourceDefinitionDetail)
		apiRouter.GET("/priority-classes", resourceHandler.PriorityClasses)
		apiRouter.GET("/priority-classes/{name}", resourceHandler.PriorityClassDetail)
		apiRouter.GET("/runtime-classes", resourceHandler.RuntimeClasses)
		apiRouter.GET("/runtime-classes/{name}", resourceHandler.RuntimeClassDetail)
		apiRouter.GET("/mutating-webhook-configurations", resourceHandler.MutatingWebhookConfigurations)
		apiRouter.GET("/mutating-webhook-configurations/{name}", resourceHandler.MutatingWebhookConfigurationDetail)
		apiRouter.GET("/validating-webhook-configurations", resourceHandler.ValidatingWebhookConfigurations)
		apiRouter.GET("/validating-webhook-configurations/{name}", resourceHandler.ValidatingWebhookConfigurationDetail)
		apiRouter.GET("/ingress-classes", resourceHandler.IngressClasses)
		apiRouter.GET("/ingress-classes/{name}", resourceHandler.IngressClassDetail)
	}
	if dependencies.Preferences != nil {
		preferenceHandler := NewResources(nil, dependencies.Preferences, dependencies.Selection, dependencies.Cursors)
		apiRouter.GET("/preferences", preferenceHandler.PreferencesGet)
		apiRouter.PUT("/preferences", preferenceHandler.PreferencesPut)
	}
	if dependencies.Selection != nil && (dependencies.Actions != nil || dependencies.PortForwards != nil || dependencies.Exec != nil) {
		actions := NewActionHandlers(dependencies.Actions, dependencies.PortForwards, dependencies.Exec, dependencies.Selection)
		if dependencies.Actions != nil {
			apiRouter.POST("/workloads/{kind}/{namespace}/{name}/restart", actions.Restart)
			apiRouter.PUT("/workloads/{kind}/{namespace}/{name}/scale", actions.Scale)
			apiRouter.DELETE("/pods/{namespace}/{name}", actions.DeletePod)
		}
		if dependencies.PortForwards != nil {
			apiRouter.POST("/pods/{namespace}/{name}/port-forward", actions.CreatePortForward)
			apiRouter.GET("/port-forwards", actions.ListPortForwards)
			apiRouter.DELETE("/port-forwards/{id}", actions.ClosePortForward)
		}
		if dependencies.Exec != nil {
			apiRouter.POST("/pods/{namespace}/{name}/exec", actions.CreateExecTicket)
		}
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
		apiPrefix + "/dashboard/summary", apiPrefix + "/dashboard/namespace-health",
		apiPrefix + "/dashboard/problems",
		apiPrefix + "/dashboard/restarts", apiPrefix + "/dashboard/events", apiPrefix + "/metrics",
		apiPrefix + "/port-forwards", apiPrefix + "/workloads", apiPrefix + "/pods",
		apiPrefix + "/events", apiPrefix + "/services", apiPrefix + "/ingresses",
		apiPrefix + "/endpoint-slices", apiPrefix + "/configmaps", apiPrefix + "/secrets",
		apiPrefix + "/nodes", apiPrefix + "/leases",
		apiPrefix + "/persistent-volumes", apiPrefix + "/persistent-volume-claims",
		apiPrefix + "/storage-classes", apiPrefix + "/csi-drivers", apiPrefix + "/csi-nodes",
		apiPrefix + "/volume-attachments", apiPrefix + "/service-accounts",
		apiPrefix + "/resource-quotas", apiPrefix + "/limit-ranges", apiPrefix + "/hpas", apiPrefix + "/pdbs",
		apiPrefix + "/roles", apiPrefix + "/role-bindings", apiPrefix + "/network-policies", apiPrefix + "/endpoints",
		apiPrefix + "/cluster-roles", apiPrefix + "/cluster-role-bindings",
		apiPrefix + "/customresourcedefinitions", apiPrefix + "/priority-classes", apiPrefix + "/runtime-classes",
		apiPrefix + "/mutating-webhook-configurations", apiPrefix + "/validating-webhook-configurations",
		apiPrefix + "/ingress-classes":
		return "GET, HEAD", true
	case apiPrefix + "/preferences":
		return "GET, HEAD, PUT", true
	case apiPrefix + "/stream":
		return "GET", true
	case apiPrefix + "/contexts/select", apiPrefix + "/namespace-scopes/validate", apiPrefix + "/dashboard/log-scan":
		return "POST", true
	case apiPrefix + "/namespace-scopes":
		return "GET, HEAD, POST", true
	}
	if actionAllow, actionKnown := actionAllowedMethods(path); actionKnown {
		if resourceAllow, resourceKnown := resourceAllowedMethods(path); resourceKnown {
			return mergeAllowMethods(actionAllow, resourceAllow), true
		}
		return actionAllow, true
	}
	if allow, known := resourceAllowedMethods(path); known {
		return allow, true
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

func resourceAllowedMethods(path string) (string, bool) {
	if !strings.HasPrefix(path, apiPrefix+"/") {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, apiPrefix+"/"), "/")
	for _, part := range parts {
		if part == "" {
			return "", false
		}
	}
	switch {
	case len(parts) == 4 && parts[0] == "workloads":
		return "GET, HEAD", true
	case len(parts) == 5 && parts[0] == "workloads" && parts[4] == "yaml":
		return "GET, HEAD", true
	case len(parts) == 3 && parts[0] == "pods":
		return "GET, HEAD", true
	case len(parts) == 4 && parts[0] == "pods" && (parts[3] == "yaml" || parts[3] == "logs"):
		return "GET, HEAD", true
	case len(parts) == 5 && parts[0] == "pods" && parts[3] == "logs" && parts[4] == "stream":
		return "GET", true
	case len(parts) == 3 && (parts[0] == "services" || parts[0] == "ingresses" || parts[0] == "endpoint-slices" || parts[0] == "configmaps" || parts[0] == "secrets" || parts[0] == "leases" || parts[0] == "persistent-volume-claims" || parts[0] == "service-accounts" || parts[0] == "resource-quotas" || parts[0] == "limit-ranges" || parts[0] == "hpas" || parts[0] == "pdbs" || parts[0] == "roles" || parts[0] == "role-bindings" || parts[0] == "network-policies" || parts[0] == "endpoints"):
		return "GET, HEAD", true
	case len(parts) == 4 && parts[3] == "yaml" && (parts[0] == "services" || parts[0] == "ingresses" || parts[0] == "endpoint-slices" || parts[0] == "configmaps"):
		return "GET, HEAD", true
	case len(parts) == 2 && parts[0] == "nodes":
		return "GET, HEAD", true
	case len(parts) == 3 && parts[0] == "nodes" && parts[2] == "yaml":
		return "GET, HEAD", true
	case len(parts) == 2 && clusterNameRoutes[parts[0]]:
		return "GET, HEAD", true
	case len(parts) == 3 && parts[2] == "yaml" && clusterYAMLRoutes[parts[0]]:
		return "GET, HEAD", true
	case len(parts) == 4 && parts[3] == "yaml" && (parts[0] == "leases" || parts[0] == "persistent-volume-claims"):
		return "GET, HEAD", true
	default:
		return "", false
	}
}

// clusterNameRoutes are cluster-scoped detail collections ({collection}/{name}).
var clusterNameRoutes = map[string]bool{
	"nodes": true, "persistent-volumes": true, "storage-classes": true,
	"csi-drivers": true, "csi-nodes": true, "volume-attachments": true,
	"namespaces": true, "cluster-roles": true, "cluster-role-bindings": true,
	"customresourcedefinitions": true, "priority-classes": true,
	"runtime-classes": true, "mutating-webhook-configurations": true,
	"validating-webhook-configurations": true, "ingress-classes": true,
}

// clusterYAMLRoutes are the cluster-scoped collections with a YAML action.
var clusterYAMLRoutes = map[string]bool{
	"nodes": true, "persistent-volumes": true, "storage-classes": true,
}

func mergeAllowMethods(left, right string) string {
	seen := map[string]bool{}
	result := []string{}
	for _, group := range []string{left, right} {
		for _, method := range strings.Split(group, ", ") {
			if method != "" && !seen[method] {
				seen[method] = true
				result = append(result, method)
			}
		}
	}
	return strings.Join(result, ", ")
}

func actionAllowedMethods(path string) (string, bool) {
	if !strings.HasPrefix(path, apiPrefix+"/") {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, apiPrefix+"/"), "/")
	for _, part := range parts {
		if part == "" {
			return "", false
		}
	}
	switch {
	case len(parts) == 5 && parts[0] == "workloads" && parts[4] == "restart":
		return "POST", true
	case len(parts) == 5 && parts[0] == "workloads" && parts[4] == "scale":
		return "PUT", true
	case len(parts) == 3 && parts[0] == "pods":
		return "DELETE", true
	case len(parts) == 4 && parts[0] == "pods" && parts[3] == "port-forward":
		return "POST", true
	case len(parts) == 4 && parts[0] == "pods" && parts[3] == "exec":
		return "POST", true
	case len(parts) == 2 && parts[0] == "port-forwards":
		return "DELETE", true
	case len(parts) == 3 && parts[0] == "exec" && parts[2] == "stream":
		return "GET", true
	default:
		return "", false
	}
}

func writeEnvelope(w http.ResponseWriter, value any) error {
	return json.NewEncoder(w).Encode(value)
}
