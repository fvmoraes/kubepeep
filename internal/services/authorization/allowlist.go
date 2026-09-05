package authorization

// ResourceNamePolicy controls whether /permissions may form a product with the
// resourceName query parameter for a capability.
type ResourceNamePolicy string

const (
	ResourceNameEmpty  ResourceNamePolicy = "empty"
	ResourceNameTarget ResourceNamePolicy = "target"
)

// CapabilityScope describes whether a capability is cluster or namespace
// scoped.
type CapabilityScope string

const (
	ScopeCluster   CapabilityScope = "cluster"
	ScopeNamespace CapabilityScope = "namespace"
)

// CapabilitySpec is one immutable entry from the docs/api.md MVP allowlist.
type CapabilitySpec struct {
	ID                 string
	APIGroup           string
	Resource           string
	Subresource        string
	Verb               string
	Scope              CapabilityScope
	ResourceNamePolicy ResourceNamePolicy
}

var capabilityAllowlist = [...]CapabilitySpec{
	{ID: "namespaces.list", Resource: "namespaces", Verb: "list", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "pods.list", Resource: "pods", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "pods.get", Resource: "pods", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "pods.watch", Resource: "pods", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "pods.logs.get", Resource: "pods", Subresource: "log", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "pods.delete", Resource: "pods", Verb: "delete", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "pods.exec.create", Resource: "pods", Subresource: "exec", Verb: "create", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "pods.portforward.create", Resource: "pods", Subresource: "portforward", Verb: "create", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "events.list", Resource: "events", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "events.watch", Resource: "events", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "deployments.list", APIGroup: "apps", Resource: "deployments", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "deployments.get", APIGroup: "apps", Resource: "deployments", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "deployments.watch", APIGroup: "apps", Resource: "deployments", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "deployments.restart", APIGroup: "apps", Resource: "deployments", Verb: "patch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "deployments.scale", APIGroup: "apps", Resource: "deployments", Subresource: "scale", Verb: "update", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "statefulsets.list", APIGroup: "apps", Resource: "statefulsets", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "statefulsets.get", APIGroup: "apps", Resource: "statefulsets", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "statefulsets.watch", APIGroup: "apps", Resource: "statefulsets", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "statefulsets.scale", APIGroup: "apps", Resource: "statefulsets", Subresource: "scale", Verb: "update", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "daemonsets.list", APIGroup: "apps", Resource: "daemonsets", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "daemonsets.get", APIGroup: "apps", Resource: "daemonsets", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "daemonsets.watch", APIGroup: "apps", Resource: "daemonsets", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "jobs.list", APIGroup: "batch", Resource: "jobs", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "jobs.get", APIGroup: "batch", Resource: "jobs", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "jobs.watch", APIGroup: "batch", Resource: "jobs", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "cronjobs.list", APIGroup: "batch", Resource: "cronjobs", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "cronjobs.get", APIGroup: "batch", Resource: "cronjobs", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "cronjobs.watch", APIGroup: "batch", Resource: "cronjobs", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "services.list", Resource: "services", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "services.get", Resource: "services", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "services.watch", Resource: "services", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "ingresses.list", APIGroup: "networking.k8s.io", Resource: "ingresses", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "ingresses.get", APIGroup: "networking.k8s.io", Resource: "ingresses", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "ingresses.watch", APIGroup: "networking.k8s.io", Resource: "ingresses", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "endpoint-slices.list", APIGroup: "discovery.k8s.io", Resource: "endpointslices", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "endpoint-slices.get", APIGroup: "discovery.k8s.io", Resource: "endpointslices", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "endpoint-slices.watch", APIGroup: "discovery.k8s.io", Resource: "endpointslices", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "configmaps.list", Resource: "configmaps", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "configmaps.get", Resource: "configmaps", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "configmaps.watch", Resource: "configmaps", Verb: "watch", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "secrets.list", Resource: "secrets", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "secrets.get", Resource: "secrets", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "nodes.list", Resource: "nodes", Verb: "list", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "nodes.get", Resource: "nodes", Verb: "get", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameTarget},
	{ID: "persistentvolumes.list", Resource: "persistentvolumes", Verb: "list", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "persistentvolumes.get", Resource: "persistentvolumes", Verb: "get", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameTarget},
	{ID: "persistentvolumeclaims.list", Resource: "persistentvolumeclaims", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "persistentvolumeclaims.get", Resource: "persistentvolumeclaims", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "storageclasses.list", Resource: "storageclasses", Verb: "list", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "storageclasses.get", Resource: "storageclasses", Verb: "get", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameTarget},
	{ID: "volumeattachments.list", Resource: "volumeattachments", Verb: "list", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "volumeattachments.get", Resource: "volumeattachments", Verb: "get", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameTarget},
	{ID: "csinodes.list", Resource: "csinodes", Verb: "list", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "csinodes.get", Resource: "csinodes", Verb: "get", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameTarget},
	{ID: "csidrivers.list", Resource: "csidrivers", Verb: "list", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "csidrivers.get", Resource: "csidrivers", Verb: "get", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameTarget},
	{ID: "leases.list", APIGroup: "coordination.k8s.io", Resource: "leases", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
	{ID: "leases.get", APIGroup: "coordination.k8s.io", Resource: "leases", Verb: "get", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameTarget},
	{ID: "namespaces.get", Resource: "namespaces", Verb: "get", Scope: ScopeCluster, ResourceNamePolicy: ResourceNameTarget},
	{ID: "metrics.pods.list", APIGroup: "metrics.k8s.io", Resource: "pods", Verb: "list", Scope: ScopeNamespace, ResourceNamePolicy: ResourceNameEmpty},
}

var capabilityByID = func() map[string]CapabilitySpec {
	result := make(map[string]CapabilitySpec, len(capabilityAllowlist))
	for _, specification := range capabilityAllowlist {
		result[specification.ID] = specification
	}
	return result
}()

// Allowlist returns a copy in the canonical docs/api.md order.
func Allowlist() []CapabilitySpec {
	return append([]CapabilitySpec(nil), capabilityAllowlist[:]...)
}

// LookupCapability resolves only an exact, case-sensitive allowlisted ID.
func LookupCapability(id string) (CapabilitySpec, bool) {
	specification, ok := capabilityByID[id]
	return specification, ok
}

// KeyForCapability creates a complete review key from an allowlisted ID.
func KeyForCapability(generation, namespace, id, resourceName string) (Key, error) {
	specification, ok := LookupCapability(id)
	if !ok {
		return Key{}, validationError()
	}
	if specification.Scope == ScopeCluster {
		if namespace != "" || resourceName != "" {
			return Key{}, validationError()
		}
	} else if namespace == "" {
		return Key{}, validationError()
	}
	if specification.ResourceNamePolicy == ResourceNameEmpty && resourceName != "" {
		return Key{}, validationError()
	}
	key := Key{
		Generation:   generation,
		Namespace:    namespace,
		APIGroup:     specification.APIGroup,
		Resource:     specification.Resource,
		Subresource:  specification.Subresource,
		Verb:         specification.Verb,
		ResourceName: resourceName,
	}
	if err := ValidateKey(key); err != nil {
		return Key{}, err
	}
	return key, nil
}
