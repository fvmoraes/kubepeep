package resources

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodDTO struct {
	Namespace   string    `json:"namespace"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Ready       ReadyDTO  `json:"ready"`
	Restarts    int64     `json:"restarts"`
	Node        *string   `json:"node"`
	IP          *string   `json:"ip"`
	Owner       *OwnerDTO `json:"owner"`
	AgeSeconds  int64     `json:"ageSeconds"`
	Problematic bool      `json:"problematic"`
}

func (PodDTO) resourceListItem() {}

type PodDetailDTO struct {
	Metadata            ResourceMetadataDTO `json:"metadata"`
	Summary             PodDTO              `json:"summary"`
	Conditions          []ConditionDTO      `json:"conditions"`
	Containers          []PodContainerDTO   `json:"containers"`
	InitContainers      []PodContainerDTO   `json:"initContainers"`
	EphemeralContainers []PodContainerDTO   `json:"ephemeralContainers"`
	RelatedEvents       []ResourceRef       `json:"relatedEvents"`
}

func (PodDetailDTO) resourceDetailItem() {}

func ConvertPod(value *corev1.Pod, now time.Time) PodDTO {
	ready := int64(0)
	restarts := int64(0)
	statuses := append(append(append([]corev1.ContainerStatus{}, value.Status.InitContainerStatuses...), value.Status.ContainerStatuses...), value.Status.EphemeralContainerStatuses...)
	for _, status := range statuses {
		if status.RestartCount > 0 {
			restarts += int64(status.RestartCount)
		}
	}
	for _, status := range value.Status.ContainerStatuses {
		if status.Ready {
			ready++
		}
	}
	owner := directOwner(value.OwnerReferences)
	status := normalizePodPhase(value.Status.Phase)
	summary := PodDTO{
		Namespace:  value.Namespace,
		Name:       value.Name,
		Status:     status,
		Ready:      ReadyDTO{Current: ready, Desired: int64(len(value.Spec.Containers))},
		Restarts:   restarts,
		Node:       nullableString(value.Spec.NodeName),
		IP:         nullableString(value.Status.PodIP),
		Owner:      owner,
		AgeSeconds: ageSeconds(value.CreationTimestamp.Time, now),
	}
	summary.Problematic = podProblematic(value, summary)
	return summary
}

func PodDetail(value *corev1.Pod, relatedEvents []ResourceRef, now time.Time) PodDetailDTO {
	conditions := make([]ConditionDTO, 0, min(len(value.Status.Conditions), maximumConditions))
	for _, condition := range value.Status.Conditions {
		if len(conditions) == maximumConditions {
			break
		}
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	if len(relatedEvents) > 100 {
		relatedEvents = relatedEvents[:100]
	}
	return PodDetailDTO{
		Metadata:            ConvertMetadata(value),
		Summary:             ConvertPod(value, now),
		Conditions:          conditions,
		Containers:          regularContainerDTOs(value.Spec.Containers, value.Status.ContainerStatuses, "regular"),
		InitContainers:      regularContainerDTOs(value.Spec.InitContainers, value.Status.InitContainerStatuses, "init"),
		EphemeralContainers: ephemeralContainerDTOs(value.Spec.EphemeralContainers, value.Status.EphemeralContainerStatuses),
		RelatedEvents:       append([]ResourceRef(nil), relatedEvents...),
	}
}

func normalizePodPhase(phase corev1.PodPhase) string {
	switch phase {
	case corev1.PodRunning, corev1.PodPending, corev1.PodSucceeded, corev1.PodFailed:
		return string(phase)
	default:
		return "Unknown"
	}
}

func directOwner(references []metav1.OwnerReference) *OwnerDTO {
	if len(references) == 0 {
		return nil
	}
	selected := references[0]
	for _, reference := range references {
		if reference.Controller != nil && *reference.Controller {
			selected = reference
			break
		}
	}
	if selected.Kind == "" || selected.Name == "" {
		return nil
	}
	return &OwnerDTO{Kind: selected.Kind, Name: selected.Name}
}

func podProblematic(value *corev1.Pod, summary PodDTO) bool {
	if summary.Status == "Pending" || summary.Status == "Failed" || summary.Status == "Unknown" {
		return true
	}
	if summary.Status == "Running" && summary.Ready.Current != summary.Ready.Desired {
		return true
	}
	if summary.Restarts >= 3 {
		return true
	}
	for _, status := range append(append([]corev1.ContainerStatus{}, value.Status.InitContainerStatuses...), value.Status.ContainerStatuses...) {
		if status.State.Waiting != nil || (status.State.Terminated != nil && status.State.Terminated.ExitCode != 0) {
			return true
		}
	}
	return false
}

func regularContainerDTOs(specs []corev1.Container, statuses []corev1.ContainerStatus, containerType string) []PodContainerDTO {
	if len(specs) > maximumContainers {
		specs = specs[:maximumContainers]
	}
	byName := make(map[string]corev1.ContainerStatus, len(statuses))
	for _, status := range statuses {
		byName[status.Name] = status
	}
	result := make([]PodContainerDTO, 0, len(specs))
	for _, spec := range specs {
		converted := ContainerSpecs([]corev1.Container{spec})[0]
		result = append(result, podContainer(converted, byName[spec.Name], containerType))
	}
	return result
}

func ephemeralContainerDTOs(specs []corev1.EphemeralContainer, statuses []corev1.ContainerStatus) []PodContainerDTO {
	if len(specs) > maximumContainers {
		specs = specs[:maximumContainers]
	}
	byName := make(map[string]corev1.ContainerStatus, len(statuses))
	for _, status := range statuses {
		byName[status.Name] = status
	}
	result := make([]PodContainerDTO, 0, len(specs))
	for _, spec := range specs {
		converted := ContainerSpecDTO{Name: spec.Name, Image: spec.Image, Ports: []ContainerPortDTO{}}
		result = append(result, podContainer(converted, byName[spec.Name], "ephemeral"))
	}
	return result
}

func podContainer(spec ContainerSpecDTO, status corev1.ContainerStatus, containerType string) PodContainerDTO {
	result := PodContainerDTO{Spec: spec, Type: containerType, State: "unknown"}
	if status.Name == "" {
		return result
	}
	ready := status.Ready
	result.Ready = &ready
	if status.RestartCount > 0 {
		result.RestartCount = int64(status.RestartCount)
	}
	switch {
	case status.State.Waiting != nil:
		result.State = "waiting"
		result.Reason = nullableSanitized(status.State.Waiting.Reason, maximumMessageBytes)
	case status.State.Running != nil:
		result.State = "running"
	case status.State.Terminated != nil:
		result.State = "terminated"
		result.Reason = nullableSanitized(status.State.Terminated.Reason, maximumMessageBytes)
	}
	return result
}
