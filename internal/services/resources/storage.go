package resources

import (
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	maximumVolumeConditions  = 16
	maximumCSIDriversPerNode = 64
	maximumTopoKeys          = 16
)

// LeaseDTO is the bounded list view of one coordination.k8s.io Lease.
type LeaseDTO struct {
	Namespace       string  `json:"namespace"`
	Name            string  `json:"name"`
	HolderName      string  `json:"holderName"`
	DurationSeconds int32   `json:"durationSeconds"`
	RenewTime       *string `json:"renewTime"`
	AgeSeconds      int64   `json:"ageSeconds"`
}

func (LeaseDTO) resourceListItem() {}

// LeaseDetailDTO bounds the lease identity and timing fields.
type LeaseDetailDTO struct {
	Metadata        ResourceMetadataDTO `json:"metadata"`
	HolderName      string              `json:"holderName"`
	DurationSeconds int32               `json:"durationSeconds"`
	RenewTime       *string             `json:"renewTime"`
	AcquireTime     *string             `json:"acquireTime"`
}

func (LeaseDetailDTO) resourceDetailItem() {}

// ConvertLease projects one Lease onto the bounded list DTO.
func ConvertLease(value *coordinationv1.Lease, now time.Time) LeaseDTO {
	renew := rfc3339MicroOrNil(value.Spec.RenewTime)
	holder := ""
	if value.Spec.HolderIdentity != nil {
		holder = *value.Spec.HolderIdentity
	}
	duration := int32(0)
	if value.Spec.LeaseDurationSeconds != nil {
		duration = *value.Spec.LeaseDurationSeconds
	}
	return LeaseDTO{
		Namespace: value.Namespace, Name: value.Name, HolderName: holder,
		DurationSeconds: duration, RenewTime: renew,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertLeaseDetail projects one Lease onto the bounded detail DTO.
func ConvertLeaseDetail(value *coordinationv1.Lease) LeaseDetailDTO {
	summary := ConvertLease(value, time.Time{})
	return LeaseDetailDTO{
		Metadata: ConvertMetadata(value), HolderName: summary.HolderName,
		DurationSeconds: summary.DurationSeconds, RenewTime: summary.RenewTime,
		AcquireTime: rfc3339MicroOrNil(value.Spec.AcquireTime),
	}
}

// PersistentVolumeDTO is the bounded list view of one PV.
type PersistentVolumeDTO struct {
	Name          string             `json:"name"`
	Status        string             `json:"status"`
	Capacity      string             `json:"capacity"`
	AccessModes   []string           `json:"accessModes"`
	ReclaimPolicy string             `json:"reclaimPolicy"`
	StorageClass  string             `json:"storageClass"`
	Claim         *VolumeClaimRefDTO `json:"claim"`
	AgeSeconds    int64              `json:"ageSeconds"`
}

func (PersistentVolumeDTO) resourceListItem() {}

// VolumeClaimRefDTO names the bound claim without prefetching its object.
type VolumeClaimRefDTO struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// PersistentVolumeDetailDTO keeps operator-useful PV fields. CSI volume
// attributes, secret references and source-specific fields never cross here.
type PersistentVolumeDetailDTO struct {
	Metadata      ResourceMetadataDTO `json:"metadata"`
	Status        string              `json:"status"`
	Capacity      map[string]string   `json:"capacity"`
	AccessModes   []string            `json:"accessModes"`
	ReclaimPolicy string              `json:"reclaimPolicy"`
	StorageClass  string              `json:"storageClass"`
	VolumeMode    string              `json:"volumeMode"`
	Claim         *VolumeClaimRefDTO  `json:"claim"`
	Reason        *string             `json:"reason"`
	Message       *string             `json:"message"`
	Omitted       []string            `json:"omitted"`
}

func (PersistentVolumeDetailDTO) resourceDetailItem() {}

// ConvertPersistentVolume projects one PV onto the bounded list DTO.
func ConvertPersistentVolume(value *corev1.PersistentVolume, now time.Time) PersistentVolumeDTO {
	reclaim := string(value.Spec.PersistentVolumeReclaimPolicy)
	capacity := ""
	if quantity, ok := value.Spec.Capacity[corev1.ResourceStorage]; ok {
		capacity = quantity.String()
	}
	return PersistentVolumeDTO{
		Name: value.Name, Status: string(value.Status.Phase), Capacity: capacity,
		AccessModes: accessModes(value.Spec.AccessModes), ReclaimPolicy: reclaim,
		StorageClass: value.Spec.StorageClassName,
		Claim:        volumeClaimRef(value.Spec.ClaimRef),
		AgeSeconds:   int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertPersistentVolumeDetail projects one PV onto the curated detail DTO.
func ConvertPersistentVolumeDetail(value *corev1.PersistentVolume) PersistentVolumeDetailDTO {
	summary := ConvertPersistentVolume(value, time.Time{})
	reason := value.Status.Reason
	message := value.Status.Message
	capacity := boundedQuantity(value.Spec.Capacity)
	return PersistentVolumeDetailDTO{
		Metadata: ConvertMetadata(value), Status: summary.Status, Capacity: capacity,
		AccessModes: summary.AccessModes, ReclaimPolicy: summary.ReclaimPolicy,
		StorageClass: summary.StorageClass,
		VolumeMode:   string(pointerValue(value.Spec.VolumeMode)),
		Claim:        summary.Claim,
		Reason:       stringOrNil(reason), Message: stringOrNil(message),
		Omitted: []string{"metadata.annotations", "metadata.managedFields", "metadata.finalizers",
			"spec.csi", "spec.nfs", "spec.hostPath", "spec.local", "spec.cephfs", "spec.rbd", "spec.iscsi",
			"secretRef fields of every source"},
	}
}

// PersistentVolumeClaimDTO is the bounded list view of one PVC. Absent
// capacity stays distinct from zero.
type PersistentVolumeClaimDTO struct {
	Namespace    string   `json:"namespace"`
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	VolumeName   string   `json:"volumeName"`
	Capacity     *string  `json:"capacity"`
	AccessModes  []string `json:"accessModes"`
	StorageClass *string  `json:"storageClass"`
	AgeSeconds   int64    `json:"ageSeconds"`
}

func (PersistentVolumeClaimDTO) resourceListItem() {}

// PersistentVolumeClaimDetailDTO bounds PVC identity, spec and conditions.
type PersistentVolumeClaimDetailDTO struct {
	Metadata     ResourceMetadataDTO `json:"metadata"`
	Status       string              `json:"status"`
	VolumeName   string              `json:"volumeName"`
	Capacity     map[string]string   `json:"capacity"`
	AccessModes  []string            `json:"accessModes"`
	StorageClass *string             `json:"storageClass"`
	VolumeMode   string              `json:"volumeMode"`
	Conditions   []ConditionDTO      `json:"conditions"`
	Truncated    bool                `json:"truncated"`
}

func (PersistentVolumeClaimDetailDTO) resourceDetailItem() {}

// ConvertPersistentVolumeClaim projects one PVC onto the bounded list DTO.
func ConvertPersistentVolumeClaim(value *corev1.PersistentVolumeClaim, now time.Time) PersistentVolumeClaimDTO {
	capacityQuantity, capacityPresent := value.Status.Capacity[corev1.ResourceStorage]
	capacity := quantityStringOrNil(capacityQuantity, capacityPresent)
	return PersistentVolumeClaimDTO{
		Namespace: value.Namespace, Name: value.Name, Status: string(value.Status.Phase),
		VolumeName: value.Spec.VolumeName, Capacity: capacity,
		AccessModes: accessModes(value.Spec.AccessModes), StorageClass: value.Spec.StorageClassName,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertPersistentVolumeClaimDetail projects one PVC onto the bounded detail.
func ConvertPersistentVolumeClaimDetail(value *corev1.PersistentVolumeClaim) PersistentVolumeClaimDetailDTO {
	summary := ConvertPersistentVolumeClaim(value, time.Time{})
	conditions := make([]ConditionDTO, 0, min(len(value.Status.Conditions), maximumVolumeConditions))
	for _, condition := range value.Status.Conditions {
		if len(conditions) == maximumVolumeConditions {
			break
		}
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	return PersistentVolumeClaimDetailDTO{
		Metadata: ConvertMetadata(value), Status: summary.Status, VolumeName: summary.VolumeName,
		Capacity: boundedQuantity(value.Status.Capacity), AccessModes: summary.AccessModes,
		StorageClass: summary.StorageClass, VolumeMode: string(pointerValue(value.Spec.VolumeMode)),
		Conditions: conditions, Truncated: len(value.Status.Conditions) > maximumVolumeConditions,
	}
}

// StorageClassDTO is the bounded list view of one StorageClass. Free-form
// parameters (which may carry provider credentials) are always omitted.
type StorageClassDTO struct {
	Name                 string `json:"name"`
	Provisioner          string `json:"provisioner"`
	Default              bool   `json:"default"`
	ReclaimPolicy        string `json:"reclaimPolicy"`
	VolumeBindingMode    string `json:"volumeBindingMode"`
	AllowVolumeExpansion bool   `json:"allowVolumeExpansion"`
	AgeSeconds           int64  `json:"ageSeconds"`
}

func (StorageClassDTO) resourceListItem() {}

// StorageClassDetailDTO repeats the bounded fields and labels the omission.
type StorageClassDetailDTO struct {
	Metadata             ResourceMetadataDTO `json:"metadata"`
	Provisioner          string              `json:"provisioner"`
	Default              bool                `json:"default"`
	ReclaimPolicy        string              `json:"reclaimPolicy"`
	VolumeBindingMode    string              `json:"volumeBindingMode"`
	AllowVolumeExpansion bool                `json:"allowVolumeExpansion"`
	Omitted              []string            `json:"omitted"`
}

func (StorageClassDetailDTO) resourceDetailItem() {}

// ConvertStorageClass projects one StorageClass onto the bounded list DTO.
func ConvertStorageClass(value *storagev1.StorageClass, now time.Time) StorageClassDTO {
	reclaim := ""
	if value.ReclaimPolicy != nil {
		reclaim = string(*value.ReclaimPolicy)
	}
	binding := ""
	if value.VolumeBindingMode != nil {
		binding = string(*value.VolumeBindingMode)
	}
	return StorageClassDTO{
		Name: value.Name, Provisioner: value.Provisioner,
		Default:       value.Annotations["storageclass.kubernetes.io/is-default-class"] == "true",
		ReclaimPolicy: reclaim, VolumeBindingMode: binding,
		AllowVolumeExpansion: value.AllowVolumeExpansion != nil && *value.AllowVolumeExpansion,
		AgeSeconds:           int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertStorageClassDetail projects one StorageClass onto the bounded detail.
func ConvertStorageClassDetail(value *storagev1.StorageClass) StorageClassDetailDTO {
	summary := ConvertStorageClass(value, time.Time{})
	return StorageClassDetailDTO{
		Metadata: ConvertMetadata(value), Provisioner: summary.Provisioner, Default: summary.Default,
		ReclaimPolicy: summary.ReclaimPolicy, VolumeBindingMode: summary.VolumeBindingMode,
		AllowVolumeExpansion: summary.AllowVolumeExpansion,
		Omitted:              []string{"metadata.annotations (beyond default-class)", "metadata.managedFields", "parameters", "allowedTopologies", "mountOptions"},
	}
}

// CSIDriverDTO is the bounded list view of one CSIDriver.
type CSIDriverDTO struct {
	Name            string `json:"name"`
	AttachRequired  bool   `json:"attachRequired"`
	PodInfoOnMount  bool   `json:"podInfoOnMount"`
	StorageCapacity bool   `json:"storageCapacity"`
	AgeSeconds      int64  `json:"ageSeconds"`
}

func (CSIDriverDTO) resourceListItem() {}

// CSIDriverDetailDTO bounds the driver spec flags. Tokens and service
// accounts of the driver deployment never cross here.
type CSIDriverDetailDTO struct {
	Metadata        ResourceMetadataDTO `json:"metadata"`
	AttachRequired  bool                `json:"attachRequired"`
	PodInfoOnMount  bool                `json:"podInfoOnMount"`
	StorageCapacity bool                `json:"storageCapacity"`
	FSGroupPolicy   string              `json:"fsGroupPolicy"`
}

func (CSIDriverDetailDTO) resourceDetailItem() {}

// ConvertCSIDriver projects one CSIDriver onto the bounded list DTO.
func ConvertCSIDriver(value *storagev1.CSIDriver, now time.Time) CSIDriverDTO {
	return CSIDriverDTO{
		Name:            value.Name,
		AttachRequired:  value.Spec.AttachRequired != nil && *value.Spec.AttachRequired,
		PodInfoOnMount:  value.Spec.PodInfoOnMount != nil && *value.Spec.PodInfoOnMount,
		StorageCapacity: value.Spec.StorageCapacity != nil && *value.Spec.StorageCapacity,
		AgeSeconds:      int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertCSIDriverDetail projects one CSIDriver onto the bounded detail DTO.
func ConvertCSIDriverDetail(value *storagev1.CSIDriver) CSIDriverDetailDTO {
	summary := ConvertCSIDriver(value, time.Time{})
	return CSIDriverDetailDTO{
		Metadata: ConvertMetadata(value), AttachRequired: summary.AttachRequired,
		PodInfoOnMount: summary.PodInfoOnMount, StorageCapacity: summary.StorageCapacity,
		FSGroupPolicy: string(pointerValue(value.Spec.FSGroupPolicy)),
	}
}

// CSINodeDTO is the bounded list view of one CSINode.
type CSINodeDTO struct {
	Name        string `json:"name"`
	DriverCount int32  `json:"driverCount"`
	AgeSeconds  int64  `json:"ageSeconds"`
}

func (CSINodeDTO) resourceListItem() {}

// CSINodeDriverDTO names one driver registered on a node, without node IDs
// beyond the driver's own nodeID string.
type CSINodeDriverDTO struct {
	Name         string   `json:"name"`
	NodeID       string   `json:"nodeID"`
	TopologyKeys []string `json:"topologyKeys"`
}

// CSINodeDetailDTO bounds the registered driver list per node.
type CSINodeDetailDTO struct {
	Metadata    ResourceMetadataDTO `json:"metadata"`
	DriverCount int32               `json:"driverCount"`
	Drivers     []CSINodeDriverDTO  `json:"drivers"`
	Truncated   bool                `json:"truncated"`
}

func (CSINodeDetailDTO) resourceDetailItem() {}

// ConvertCSINode projects one CSINode onto the bounded list DTO.
func ConvertCSINode(value *storagev1.CSINode, now time.Time) CSINodeDTO {
	return CSINodeDTO{
		Name: value.Name, DriverCount: int32(len(value.Spec.Drivers)),
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertCSINodeDetail projects one CSINode onto the bounded detail DTO.
func ConvertCSINodeDetail(value *storagev1.CSINode) CSINodeDetailDTO {
	drivers := make([]CSINodeDriverDTO, 0, min(len(value.Spec.Drivers), maximumCSIDriversPerNode))
	for _, driver := range value.Spec.Drivers {
		if len(drivers) == maximumCSIDriversPerNode {
			break
		}
		keys := append([]string(nil), driver.TopologyKeys...)
		if len(keys) > maximumTopoKeys {
			keys = keys[:maximumTopoKeys]
		}
		drivers = append(drivers, CSINodeDriverDTO{Name: driver.Name, NodeID: driver.NodeID, TopologyKeys: keys})
	}
	return CSINodeDetailDTO{
		Metadata: ConvertMetadata(value), DriverCount: int32(len(value.Spec.Drivers)),
		Drivers: drivers, Truncated: len(value.Spec.Drivers) > maximumCSIDriversPerNode,
	}
}

// VolumeAttachmentDTO is the bounded list view of one VolumeAttachment.
type VolumeAttachmentDTO struct {
	Name       string `json:"name"`
	NodeName   string `json:"nodeName"`
	Attacher   string `json:"attacher"`
	VolumeName string `json:"volumeName"`
	Attached   bool   `json:"attached"`
	AgeSeconds int64  `json:"ageSeconds"`
}

func (VolumeAttachmentDTO) resourceListItem() {}

// VolumeAttachmentDetailDTO deliberately omits status.attachmentMetadata,
// raw driver errors and any free-form attributes.
type VolumeAttachmentDetailDTO struct {
	Metadata             ResourceMetadataDTO `json:"metadata"`
	NodeName             string              `json:"nodeName"`
	Attacher             string              `json:"attacher"`
	VolumeName           string              `json:"volumeName"`
	PersistentVolumeName string              `json:"persistentVolumeName"`
	Attached             bool                `json:"attached"`
	Omitted              []string            `json:"omitted"`
}

func (VolumeAttachmentDetailDTO) resourceDetailItem() {}

// ConvertVolumeAttachment projects one VolumeAttachment onto the bounded list DTO.
func ConvertVolumeAttachment(value *storagev1.VolumeAttachment, now time.Time) VolumeAttachmentDTO {
	return VolumeAttachmentDTO{
		Name: value.Name, NodeName: value.Spec.NodeName, Attacher: value.Spec.Attacher,
		VolumeName: volumeHandle(value), Attached: value.Status.Attached,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ConvertVolumeAttachmentDetail projects one VolumeAttachment onto the bounded
// detail DTO with the forbidden fields labeled as omitted.
func ConvertVolumeAttachmentDetail(value *storagev1.VolumeAttachment) VolumeAttachmentDetailDTO {
	summary := ConvertVolumeAttachment(value, time.Time{})
	pvName := ""
	if value.Spec.Source.PersistentVolumeName != nil {
		pvName = *value.Spec.Source.PersistentVolumeName
	}
	return VolumeAttachmentDetailDTO{
		Metadata: ConvertMetadata(value), NodeName: summary.NodeName, Attacher: summary.Attacher,
		VolumeName: summary.VolumeName, PersistentVolumeName: pvName, Attached: summary.Attached,
		Omitted: []string{"metadata.annotations", "metadata.managedFields", "metadata.finalizers",
			"spec.source.inlineVolumeSpec", "status.attachmentMetadata", "status.attachmentError (raw driver message)"},
	}
}

func volumeHandle(value *storagev1.VolumeAttachment) string {
	if value.Spec.Source.InlineVolumeSpec != nil {
		return "inline spec"
	}
	return ""
}

func accessModes(modes []corev1.PersistentVolumeAccessMode) []string {
	result := make([]string, 0, len(modes))
	for _, mode := range modes {
		result = append(result, string(mode))
	}
	return result
}

func volumeClaimRef(ref *corev1.ObjectReference) *VolumeClaimRefDTO {
	if ref == nil || ref.Namespace == "" || ref.Name == "" {
		return nil
	}
	return &VolumeClaimRefDTO{Namespace: ref.Namespace, Name: ref.Name}
}

func rfc3339OrNil(value *metav1.Time) *string {
	if value == nil {
		return nil
	}
	canonical := value.UTC().Format(time.RFC3339)
	return &canonical
}

func rfc3339MicroOrNil(value *metav1.MicroTime) *string {
	if value == nil {
		return nil
	}
	canonical := value.UTC().Format(time.RFC3339)
	return &canonical
}

func quantityStringOrNil(quantity resource.Quantity, present bool) *string {
	if !present {
		return nil
	}
	value := quantity.String()
	return &value
}

func stringOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func pointerValue[T any](value *T) T {
	var zero T
	if value == nil {
		return zero
	}
	return *value
}

func pointerStringOr(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}
