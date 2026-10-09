package resources

import (
	"slices"
	"sort"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/podhealth"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodDTO struct {
	ContainerCount     int                          `json:"containerCount"`
	Containers         []PodContainerSummaryDTO     `json:"containers"`
	ContainerResources map[string]ResourceBudgetDTO `json:"containerResources,omitempty"`
	Resources          ResourceBudgetDTO            `json:"resources"`
	Secrets            []string                     `json:"secrets,omitempty"`
	Namespace          string                       `json:"namespace"`
	Name               string                       `json:"name"`
	Labels             map[string]string            `json:"labels,omitempty"`
	Status             string                       `json:"status"`
	Ready              ReadyDTO                     `json:"ready"`
	Restarts           int64                        `json:"restarts"`
	Node               *string                      `json:"node"`
	IP                 *string                      `json:"ip"`
	Owner              *OwnerDTO                    `json:"owner"`
	AgeSeconds         int64                        `json:"ageSeconds"`
	Problematic        bool                         `json:"problematic"`
	Starting           bool                         `json:"starting"`
	ConfigMaps         []string                     `json:"configMaps,omitempty"`
	PVCs               []string                     `json:"pvcs,omitempty"`
}

// Inventory status only: never include container environment or Secret values.
type PodContainerSummaryDTO struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	State  string  `json:"state"`
	Status string  `json:"status"`
	Reason *string `json:"reason"`
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
		ContainerCount:     len(value.Spec.Containers) + len(value.Spec.InitContainers) + len(value.Spec.EphemeralContainers),
		Containers:         podContainerSummaries(value, now),
		Namespace:          value.Namespace,
		Resources:          podBudget(value.Spec),
		ContainerResources: podContainerBudgets(value.Spec),
		Secrets:            podSecretRefs(value.Spec),
		Name:               value.Name,
		Labels:             limitedStringMap(value.Labels),
		Status:             status,
		Ready:              ReadyDTO{Current: ready, Desired: int64(len(value.Spec.Containers))},
		Restarts:           restarts,
		Node:               nullableString(value.Spec.NodeName),
		IP:                 nullableString(value.Status.PodIP),
		Owner:              owner,
		AgeSeconds:         ageSeconds(value.CreationTimestamp.Time, now),
		ConfigMaps:         podConfigMapRefs(value.Spec),
		PVCs:               podPVCRefs(value.Spec),
	}
	summary.Starting = podStarting(value, summary, now)
	summary.Problematic = !summary.Starting && podProblematic(value, summary)
	return summary
}

func podStarting(value *corev1.Pod, summary PodDTO, now time.Time) bool {
	if value.Status.Phase != corev1.PodPending && value.Status.Phase != corev1.PodRunning {
		return false
	}
	if summary.Restarts > 0 {
		return false
	}
	for _, condition := range value.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Reason == "Unschedulable" {
			return false
		}
	}
	for _, container := range summary.Containers {
		if container.Status == "problem" {
			return false
		}
	}
	for _, container := range summary.Containers {
		if container.Status == "starting" {
			return true
		}
	}
	for _, container := range value.Status.InitContainerStatuses {
		if container.State.Terminated == nil && !container.Ready {
			return true
		}
	}
	for _, container := range value.Status.ContainerStatuses {
		// Kubernetes keeps Started false until the startup probe succeeds.
		if container.Started != nil && !*container.Started && container.State.Running != nil {
			return true
		}
	}
	// Match the dashboard's readiness grace period for a newly created Pod.
	return summary.Ready.Current < summary.Ready.Desired && !value.CreationTimestamp.IsZero() && now.Sub(value.CreationTimestamp.Time) < 2*time.Minute
}

func podContainerSummaries(value *corev1.Pod, now time.Time) []PodContainerSummaryDTO {
	result := make([]PodContainerSummaryDTO, 0)
	appendGroup := func(names []string, statuses []corev1.ContainerStatus, kind string) {
		byName := make(map[string]corev1.ContainerStatus, len(statuses))
		for _, status := range statuses {
			byName[status.Name] = status
		}
		for _, name := range names {
			if len(result) >= maximumContainers {
				return
			}
			status := byName[name]
			detail := podContainer(ContainerSpecDTO{Name: name}, status, kind)
			tone := "inactive"
			switch {
			case status.State.Running != nil:
				tone = "running"
				if !status.Ready && kind != "ephemeral" {
					if kind == "init" || status.Started != nil && !*status.Started || !value.CreationTimestamp.IsZero() && now.Sub(value.CreationTimestamp.Time) < 2*time.Minute {
						tone = "starting"
					} else {
						tone = "problem"
					}
				}
			case status.State.Terminated != nil && status.State.Terminated.ExitCode != 0:
				tone = "problem"
			case status.State.Waiting != nil:
				switch status.State.Waiting.Reason {
				case "", "ContainerCreating", "PodInitializing":
					tone = "starting"
				default:
					tone = "problem"
				}
			case status.Name == "" && value.Status.Phase == corev1.PodPending:
				tone = "starting"
			}
			result = append(result, PodContainerSummaryDTO{Name: name, Type: kind, State: detail.State, Status: tone, Reason: detail.Reason})
		}
	}
	regular := make([]string, 0, len(value.Spec.Containers))
	for _, container := range value.Spec.Containers {
		regular = append(regular, container.Name)
	}
	init := make([]string, 0, len(value.Spec.InitContainers))
	for _, container := range value.Spec.InitContainers {
		init = append(init, container.Name)
	}
	ephemeral := make([]string, 0, len(value.Spec.EphemeralContainers))
	for _, container := range value.Spec.EphemeralContainers {
		ephemeral = append(ephemeral, container.Name)
	}
	appendGroup(regular, value.Status.ContainerStatuses, "regular")
	appendGroup(init, value.Status.InitContainerStatuses, "init")
	appendGroup(ephemeral, value.Status.EphemeralContainerStatuses, "ephemeral")
	return result
}

func podConfigMapRefs(spec corev1.PodSpec) []string {
	values := make([]string, 0)
	add := func(value string) {
		if value == "" {
			return
		}
		for _, existing := range values {
			if existing == value {
				return
			}
		}
		values = append(values, value)
	}
	for _, volume := range spec.Volumes {
		if volume.ConfigMap != nil {
			add(volume.ConfigMap.Name)
		}
		if volume.Projected != nil {
			for _, source := range volume.Projected.Sources {
				if source.ConfigMap != nil {
					add(source.ConfigMap.Name)
				}
			}
		}
	}
	containers := make([]corev1.Container, 0, len(spec.InitContainers)+len(spec.Containers))
	containers = append(containers, spec.InitContainers...)
	containers = append(containers, spec.Containers...)
	for _, ephemeral := range spec.EphemeralContainers {
		containers = append(containers, corev1.Container{Env: ephemeral.Env, EnvFrom: ephemeral.EnvFrom})
	}
	for _, container := range containers {
		for _, source := range container.EnvFrom {
			if source.ConfigMapRef != nil {
				add(source.ConfigMapRef.Name)
			}
		}
		for _, variable := range container.Env {
			if variable.ValueFrom != nil && variable.ValueFrom.ConfigMapKeyRef != nil {
				add(variable.ValueFrom.ConfigMapKeyRef.Name)
			}
		}
	}
	sort.Strings(values)
	return values
}

func podPVCRefs(spec corev1.PodSpec) []string {
	values := make([]string, 0)
	for _, volume := range spec.Volumes {
		if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName == "" {
			continue
		}
		if len(values) == 64 {
			break
		}
		values = append(values, volume.PersistentVolumeClaim.ClaimName)
	}
	sort.Strings(values)
	return slices.Compact(values)
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
		RelatedEvents:       append([]ResourceRef{}, relatedEvents...),
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
	selected := podhealth.ControllingOwner(references)
	if selected == nil {
		// Historical behavior: no controller means the first reference wins.
		selected = &references[0]
	}
	if selected.Kind == "" || selected.Name == "" {
		return nil
	}
	return &OwnerDTO{Kind: selected.Kind, Name: selected.Name}
}

func podProblematic(value *corev1.Pod, summary PodDTO) bool {
	var statusGroups [][]corev1.ContainerStatus
	if value != nil {
		statusGroups = append(statusGroups, value.Status.InitContainerStatuses, value.Status.ContainerStatuses)
	}
	return podhealth.Problematic(summary.Status, summary.Ready.Current, summary.Ready.Desired, summary.Restarts, statusGroups...)
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
		converted := ContainerSpecDTO{Name: spec.Name, Image: spec.Image, Ports: []ContainerPortDTO{}, Resources: containerBudget(spec.Resources), Environment: containerEnvironment(spec.Env), EnvFrom: containerEnvFrom(spec.EnvFrom)}
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
