// Package resourcecatalog defines the Kubernetes objects exposed by the app.
// Routes, authorization and the dynamic client share this fixed allowlist;
// submitted YAML can never choose its own API endpoint.
package resourcecatalog

import (
	"strings"
	"unicode"
)

type Resource struct {
	Collection, Group, Version, Resource, Kind string
	Namespaced                                 bool
}

func (r Resource) APIVersion() string {
	if r.Group == "" {
		return r.Version
	}
	return r.Group + "/" + r.Version
}

func (r Resource) UpdateCapability() string { return "yaml." + r.Collection + ".update" }

var resources = []Resource{
	{"gateway-classes", "gateway.networking.k8s.io", "v1", "gatewayclasses", "GatewayClass", false},
	{"gateways", "gateway.networking.k8s.io", "v1", "gateways", "Gateway", true},
	{"http-routes", "gateway.networking.k8s.io", "v1", "httproutes", "HTTPRoute", true},
	{"grpc-routes", "gateway.networking.k8s.io", "v1", "grpcroutes", "GRPCRoute", true},
	{"tcp-routes", "gateway.networking.k8s.io", "v1", "tcproutes", "TCPRoute", true},
	{"tls-routes", "gateway.networking.k8s.io", "v1", "tlsroutes", "TLSRoute", true},
	{"udp-routes", "gateway.networking.k8s.io", "v1", "udproutes", "UDPRoute", true},
	{"reference-grants", "gateway.networking.k8s.io", "v1", "referencegrants", "ReferenceGrant", true},
	{"backend-tls-policies", "gateway.networking.k8s.io", "v1", "backendtlspolicies", "BackendTLSPolicy", true},
	{"listener-sets", "gateway.networking.k8s.io", "v1", "listenersets", "ListenerSet", true},
	{"pods", "", "v1", "pods", "Pod", true},
	{"deployments", "apps", "v1", "deployments", "Deployment", true},
	{"statefulsets", "apps", "v1", "statefulsets", "StatefulSet", true},
	{"daemonsets", "apps", "v1", "daemonsets", "DaemonSet", true},
	{"replicasets", "apps", "v1", "replicasets", "ReplicaSet", true},
	{"jobs", "batch", "v1", "jobs", "Job", true},
	{"cronjobs", "batch", "v1", "cronjobs", "CronJob", true},
	{"services", "", "v1", "services", "Service", true},
	{"endpoints", "", "v1", "endpoints", "Endpoints", true},
	{"endpoint-slices", "discovery.k8s.io", "v1", "endpointslices", "EndpointSlice", true},
	{"ingresses", "networking.k8s.io", "v1", "ingresses", "Ingress", true},
	{"ingress-classes", "networking.k8s.io", "v1", "ingressclasses", "IngressClass", false},
	{"network-policies", "networking.k8s.io", "v1", "networkpolicies", "NetworkPolicy", true},
	{"configmaps", "", "v1", "configmaps", "ConfigMap", true},
	{"secrets", "", "v1", "secrets", "Secret", true},
	{"nodes", "", "v1", "nodes", "Node", false},
	{"namespaces", "", "v1", "namespaces", "Namespace", false},
	{"leases", "coordination.k8s.io", "v1", "leases", "Lease", true},
	{"persistent-volumes", "", "v1", "persistentvolumes", "PersistentVolume", false},
	{"persistent-volume-claims", "", "v1", "persistentvolumeclaims", "PersistentVolumeClaim", true},
	{"storage-classes", "storage.k8s.io", "v1", "storageclasses", "StorageClass", false},
	{"csi-drivers", "storage.k8s.io", "v1", "csidrivers", "CSIDriver", false},
	{"csi-nodes", "storage.k8s.io", "v1", "csinodes", "CSINode", false},
	{"volume-attachments", "storage.k8s.io", "v1", "volumeattachments", "VolumeAttachment", false},
	{"service-accounts", "", "v1", "serviceaccounts", "ServiceAccount", true},
	{"resource-quotas", "", "v1", "resourcequotas", "ResourceQuota", true},
	{"limit-ranges", "", "v1", "limitranges", "LimitRange", true},
	{"hpas", "autoscaling", "v2", "horizontalpodautoscalers", "HorizontalPodAutoscaler", true},
	{"pdbs", "policy", "v1", "poddisruptionbudgets", "PodDisruptionBudget", true},
	{"roles", "rbac.authorization.k8s.io", "v1", "roles", "Role", true},
	{"role-bindings", "rbac.authorization.k8s.io", "v1", "rolebindings", "RoleBinding", true},
	{"cluster-roles", "rbac.authorization.k8s.io", "v1", "clusterroles", "ClusterRole", false},
	{"cluster-role-bindings", "rbac.authorization.k8s.io", "v1", "clusterrolebindings", "ClusterRoleBinding", false},
	{"customresourcedefinitions", "apiextensions.k8s.io", "v1", "customresourcedefinitions", "CustomResourceDefinition", false},
	{"priority-classes", "scheduling.k8s.io", "v1", "priorityclasses", "PriorityClass", false},
	{"runtime-classes", "node.k8s.io", "v1", "runtimeclasses", "RuntimeClass", false},
	{"mutating-webhook-configurations", "admissionregistration.k8s.io", "v1", "mutatingwebhookconfigurations", "MutatingWebhookConfiguration", false},
	{"validating-webhook-configurations", "admissionregistration.k8s.io", "v1", "validatingwebhookconfigurations", "ValidatingWebhookConfiguration", false},
}

func All() []Resource { return append([]Resource(nil), resources...) }

func Lookup(collection string) (Resource, bool) {
	for _, resource := range resources {
		if resource.Collection == collection {
			return resource, true
		}
	}
	return Resource{}, false
}

// Kubernetes RBAC names can contain colons (for example system:discovery).
// A name is one nonempty URL path segment, never a path or URL escape.
func ValidName(name string) bool {
	return name != "" && len(name) <= 253 && name != "." && name != ".." &&
		!strings.ContainsAny(name, "/%\\") && strings.IndexFunc(name, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}
