package resources

import "time"

const (
	DefaultListLimit   = 100
	MaximumListLimit   = 500
	MaximumNamespaces  = 100
	MaximumSearchBytes = 256
	MaximumCursorBytes = 16 << 10
	MaximumFanout      = 4
)

type Collection string

const (
	CollectionWorkloads      Collection = "workloads"
	CollectionPods           Collection = "pods"
	CollectionEvents         Collection = "events"
	CollectionServices       Collection = "services"
	CollectionIngresses      Collection = "ingresses"
	CollectionEndpointSlices Collection = "endpoint-slices"
	CollectionConfigMaps     Collection = "configmaps"
	CollectionSecrets        Collection = "secrets"
	CollectionNodes          Collection = "nodes"
	CollectionLeases         Collection = "leases"
	CollectionPersistentVolumes       Collection = "persistent-volumes"
	CollectionPersistentVolumeClaims  Collection = "persistent-volume-claims"
	CollectionVolumeAttachments       Collection = "volume-attachments"
	CollectionStorageClasses          Collection = "storage-classes"
	CollectionCSINodes                Collection = "csi-nodes"
	CollectionCSIDrivers              Collection = "csi-drivers"
)

type WorkloadKind string

const (
	WorkloadDeployments  WorkloadKind = "deployments"
	WorkloadStatefulSets WorkloadKind = "statefulsets"
	WorkloadDaemonSets   WorkloadKind = "daemonsets"
	WorkloadJobs         WorkloadKind = "jobs"
	WorkloadCronJobs     WorkloadKind = "cronjobs"
)

var canonicalWorkloadKinds = []WorkloadKind{
	WorkloadDeployments,
	WorkloadStatefulSets,
	WorkloadDaemonSets,
	WorkloadJobs,
	WorkloadCronJobs,
}

type SortOrder string

const (
	OrderAscending  SortOrder = "asc"
	OrderDescending SortOrder = "desc"
)

type FilterScope string

const (
	FilterScopePage       FilterScope = "page"
	FilterScopeCollection FilterScope = "collection"
)

// Selection is an immutable request binding. Callers should obtain it from the
// active selection coordinator and fence publication against Generation.
type Selection struct {
	Generation string
	Context    string
	Scope      string
	Namespaces []string
}

type PartialErrorDTO struct {
	Namespace string    `json:"namespace,omitempty"`
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
}

type CoverageDTO struct {
	RequestedNamespaces int               `json:"requestedNamespaces"`
	CompletedNamespaces int               `json:"completedNamespaces"`
	DeniedNamespaces    []string          `json:"deniedNamespaces"`
	Failed              []PartialErrorDTO `json:"failed"`
}

type PageDTO struct {
	Limit       int         `json:"limit"`
	Next        string      `json:"next"`
	Complete    bool        `json:"complete"`
	Truncated   bool        `json:"truncated"`
	FilterScope FilterScope `json:"filterScope"`
}

// ListItem and DetailItem are sealed so adapters can only return DTOs owned by
// this package. In particular, corev1.Secret cannot instantiate a generic
// LIST/GET port.
type ListItem interface{ resourceListItem() }
type DetailItem interface{ resourceDetailItem() }

type ListResult[T ListItem] struct {
	Items       []T
	Cursor      *CompositeCursor[T]
	Page        PageDTO
	Coverage    CoverageDTO
	CollectedAt time.Time
}

type ResourceRef struct {
	APIGroup  string `json:"apiGroup"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
}

type ResourceMetadataDTO struct {
	Namespace         string            `json:"namespace"`
	Name              string            `json:"name"`
	UID               string            `json:"uid"`
	ResourceVersion   string            `json:"resourceVersion"`
	CreationTimestamp string            `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels"`
}

type ConditionDTO struct {
	Type               string  `json:"type"`
	Status             string  `json:"status"`
	Reason             *string `json:"reason"`
	Message            *string `json:"message"`
	LastTransitionTime *string `json:"lastTransitionTime"`
}

type ContainerPortDTO struct {
	Name          *string `json:"name"`
	ContainerPort int32   `json:"containerPort"`
	Protocol      string  `json:"protocol"`
}

type ContainerSpecDTO struct {
	Name  string             `json:"name"`
	Image string             `json:"image"`
	Ports []ContainerPortDTO `json:"ports"`
}

type ReadyDTO struct {
	Current int64 `json:"current"`
	Desired int64 `json:"desired"`
}

type OwnerDTO struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type PodContainerDTO struct {
	Spec         ContainerSpecDTO `json:"spec"`
	Type         string           `json:"type"`
	Ready        *bool            `json:"ready"`
	RestartCount int64            `json:"restartCount"`
	State        string           `json:"state"`
	Reason       *string          `json:"reason"`
}
