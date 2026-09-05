package resources

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// ClusterCollections are read without any namespace scope: the origin carries
// an empty namespace and authorization keys use cluster-scoped capabilities.
var clusterGVR = map[Collection]Origin{
	CollectionNodes:             {Version: "v1", Resource: "nodes"},
	CollectionPersistentVolumes: {Version: "v1", Resource: "persistentvolumes"},
	CollectionVolumeAttachments: {APIGroup: "storage.k8s.io", Version: "v1", Resource: "volumeattachments"},
	CollectionStorageClasses:    {APIGroup: "storage.k8s.io", Version: "v1", Resource: "storageclasses"},
	CollectionCSINodes:          {APIGroup: "storage.k8s.io", Version: "v1", Resource: "csinodes"},
	CollectionCSIDrivers:        {APIGroup: "storage.k8s.io", Version: "v1", Resource: "csidrivers"},
}

// ClusterOriginFor returns the single cluster-scoped origin of a collection.
func ClusterOriginFor(collection Collection) (Origin, error) {
	origin, ok := clusterGVR[collection]
	if !ok {
		return Origin{}, validationError("collection is not cluster-scoped")
	}
	return origin, nil
}

// isClusterScoped reports whether a collection is read without namespaces.
func isClusterScoped(collection Collection) bool {
	_, ok := clusterGVR[collection]
	return ok
}

const (
	maximumNodeConditions = 20
	maximumNodeAddresses  = 10
	maximumNodeTaints     = 32
	maximumNodeRoles      = 8
	maximumNodeLabels     = 32
)

// NodeDTO is the list view of one Node. Only identity, readiness, roles,
// version, age and the authorized internal address leave the boundary.
type NodeDTO struct {
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	Ready          bool     `json:"ready"`
	Roles          []string `json:"roles"`
	KubeletVersion string   `json:"kubeletVersion"`
	InternalIP     *string  `json:"internalIP"`
	AgeSeconds     int64    `json:"ageSeconds"`
}

func (NodeDTO) resourceListItem() {}

// NodeDetailDTO bounds conditions, capacity, allocatable and taints. Provider
// IDs, annotations, arbitrary labels beyond the role set and pod CIDRs never
// leave the cluster adapter.
type NodeDetailDTO struct {
	Metadata       ResourceMetadataDTO `json:"metadata"`
	Status         string              `json:"status"`
	Ready          bool                `json:"ready"`
	Roles          []string            `json:"roles"`
	KubeletVersion string              `json:"kubeletVersion"`
	InternalIP     *string             `json:"internalIP"`
	Conditions     []ConditionDTO      `json:"conditions"`
	Capacity       map[string]string   `json:"capacity"`
	Allocatable    map[string]string   `json:"allocatable"`
	Taints         []NodeTaintDTO      `json:"taints"`
	Truncated      bool                `json:"truncated"`
}

func (NodeDetailDTO) resourceDetailItem() {}

// NodeTaintDTO keeps the operator-useful fields; time added/extra value text
// is preserved but the count of taints is bounded.
type NodeTaintDTO struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

// ConvertNode projects one Node onto the bounded list DTO.
func ConvertNode(value *corev1.Node, now time.Time) NodeDTO {
	roles := nodeRoles(value.Labels)
	status, ready := nodeReadiness(value)
	return NodeDTO{
		Name:           value.Name,
		Status:         status,
		Ready:          ready,
		Roles:          roles,
		KubeletVersion: value.Status.NodeInfo.KubeletVersion,
		InternalIP:     nodeInternalIP(value.Status.Addresses),
		AgeSeconds:     int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertNodeDetail projects one Node onto the bounded detail DTO.
func ConvertNodeDetail(value *corev1.Node, now time.Time) NodeDetailDTO {
	summary := ConvertNode(value, now)
	conditions := make([]ConditionDTO, 0, min(len(value.Status.Conditions), maximumNodeConditions))
	for _, condition := range value.Status.Conditions {
		if len(conditions) == maximumNodeConditions {
			break
		}
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	capacity := boundedQuantity(value.Status.Capacity)
	allocatable := boundedQuantity(value.Status.Allocatable)
	taints := make([]NodeTaintDTO, 0, min(len(value.Spec.Taints), maximumNodeTaints))
	for _, taint := range value.Spec.Taints {
		if len(taints) == maximumNodeTaints {
			break
		}
		taints = append(taints, NodeTaintDTO{Key: taint.Key, Value: taint.Value, Effect: string(taint.Effect)})
	}
	metadata := ConvertMetadata(value)
	metadata.Namespace = ""
	truncated := len(value.Status.Conditions) > maximumNodeConditions ||
		len(value.Spec.Taints) > maximumNodeTaints ||
		len(value.Status.Capacity) > maximumNodeLabels ||
		len(value.Status.Allocatable) > maximumNodeLabels
	return NodeDetailDTO{
		Metadata: metadata, Status: summary.Status, Ready: summary.Ready, Roles: summary.Roles,
		KubeletVersion: summary.KubeletVersion, InternalIP: summary.InternalIP,
		Conditions: conditions, Capacity: capacity, Allocatable: allocatable,
		Taints: taints, Truncated: truncated,
	}
}

func nodeReadiness(value *corev1.Node) (string, bool) {
	for _, condition := range value.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			switch condition.Status {
			case corev1.ConditionTrue:
				return "Ready", true
			case corev1.ConditionUnknown:
				return "Unknown", false
			default:
				return "NotReady", false
			}
		}
	}
	return "Unknown", false
}

// nodeRoles extracts only the kubernetes.io/role label family; arbitrary
// labels are never projected.
func nodeRoles(labels map[string]string) []string {
	roles := make([]string, 0, 2)
	for label, value := range labels {
		if value != "true" {
			continue
		}
		switch {
		case label == "node-role.kubernetes.io/control-plane":
			roles = append(roles, "control-plane")
		case label == corev1.LabelOSStable || label == corev1.LabelArchStable:
			// base OS/arch labels are not roles
		case len(label) > len("node-role.kubernetes.io/") && label[:len("node-role.kubernetes.io/")] == "node-role.kubernetes.io/":
			role := label[len("node-role.kubernetes.io/"):]
			if isValidLabelSegment(role) {
				roles = append(roles, role)
			}
		}
		if len(roles) == maximumNodeRoles {
			break
		}
	}
	sortStrings(roles)
	return roles
}

func isValidLabelSegment(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for _, character := range value {
		alphanumeric := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
		if !alphanumeric && character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

func nodeInternalIP(addresses []corev1.NodeAddress) *string {
	for _, address := range addresses {
		if address.Type == corev1.NodeInternalIP {
			value := address.Address
			return &value
		}
	}
	return nil
}

func boundedQuantity(values corev1.ResourceList) map[string]string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, string(key))
	}
	sortStrings(keys)
	if len(keys) > maximumNodeLabels {
		keys = keys[:maximumNodeLabels]
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		quantity := values[corev1.ResourceName(key)]
		result[key] = quantity.String()
	}
	return result
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// NamespaceObjectDetailDTO is the V2-01 Namespace object inspection. It is
// distinct from local scope management: phases and conditions of the cluster
// object, never the saved scope contents.
type NamespaceObjectDetailDTO struct {
	Metadata   ResourceMetadataDTO `json:"metadata"`
	Status     string              `json:"status"`
	Conditions []ConditionDTO      `json:"conditions"`
}

func (NamespaceObjectDetailDTO) resourceDetailItem() {}

// ConvertNamespaceObjectDetail projects one Namespace object onto the bounded
// inspection DTO.
func ConvertNamespaceObjectDetail(value *corev1.Namespace) NamespaceObjectDetailDTO {
	conditions := make([]ConditionDTO, 0, min(len(value.Status.Conditions), maximumNodeConditions))
	for _, condition := range value.Status.Conditions {
		if len(conditions) == maximumNodeConditions {
			break
		}
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	metadata := ConvertMetadata(value)
	metadata.Namespace = ""
	return NamespaceObjectDetailDTO{
		Metadata: metadata, Status: string(value.Status.Phase), Conditions: conditions,
	}
}
