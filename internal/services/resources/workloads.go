package resources

import (
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkloadStatus string

const (
	WorkloadHealthy     WorkloadStatus = "Healthy"
	WorkloadProgressing WorkloadStatus = "Progressing"
	WorkloadDegraded    WorkloadStatus = "Degraded"
	WorkloadSuspended   WorkloadStatus = "Suspended"
	WorkloadCompleted   WorkloadStatus = "Completed"
	WorkloadFailed      WorkloadStatus = "Failed"
	WorkloadUnknown     WorkloadStatus = "Unknown"
)

type WorkloadDTO struct {
	Namespace  string         `json:"namespace"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Ready      *int64         `json:"ready"`
	Desired    *int64         `json:"desired"`
	Available  *int64         `json:"available"`
	Updated    *int64         `json:"updated"`
	Status     WorkloadStatus `json:"status"`
	AgeSeconds int64          `json:"ageSeconds"`
}

func (WorkloadDTO) resourceListItem() {}

type WorkloadDetailDTO struct {
	Metadata   ResourceMetadataDTO `json:"metadata"`
	Kind       string              `json:"kind"`
	Ready      *int64              `json:"ready"`
	Desired    *int64              `json:"desired"`
	Available  *int64              `json:"available"`
	Updated    *int64              `json:"updated"`
	Status     WorkloadStatus      `json:"status"`
	Selector   map[string]string   `json:"selector"`
	RestartAt  *string             `json:"restartAt"`
	Conditions []ConditionDTO      `json:"conditions"`
	Containers []ContainerSpecDTO  `json:"containers"`
	Related    []ResourceRef       `json:"related"`
}

func (WorkloadDetailDTO) resourceDetailItem() {}

func ConvertDeployment(value *appsv1.Deployment, now time.Time) WorkloadDTO {
	return workloadFromDashboard(dashboard.ClassifyDeployment(value, now))
}

func ConvertStatefulSet(value *appsv1.StatefulSet, now time.Time) WorkloadDTO {
	return workloadFromDashboard(dashboard.ClassifyStatefulSetWithAvailability(value, true, now))
}

func ConvertDaemonSet(value *appsv1.DaemonSet, now time.Time) WorkloadDTO {
	return workloadFromDashboard(dashboard.ClassifyDaemonSet(value, now))
}

func ConvertJob(value *batchv1.Job, now time.Time) WorkloadDTO {
	return workloadFromDashboard(dashboard.ClassifyJob(value, now))
}

func ConvertCronJob(value *batchv1.CronJob, jobs []batchv1.Job, now time.Time) WorkloadDTO {
	return ConvertCronJobWithHistory(value, jobs, jobs != nil, now)
}

// ConvertCronJobWithHistory refuses to infer a prioritized CronJob state from
// incomplete Job evidence. In particular, a truncated or unauthorized Job
// scan must not turn absence of a recent failure into Healthy.
func ConvertCronJobWithHistory(value *batchv1.CronJob, jobs []batchv1.Job, historyComplete bool, now time.Time) WorkloadDTO {
	result := workloadFromDashboard(dashboard.ClassifyCronJob(value, jobs, now))
	if !historyComplete {
		result.Status = WorkloadUnknown
	}
	return result
}

func workloadFromDashboard(value dashboard.WorkloadDTO) WorkloadDTO {
	return WorkloadDTO{
		Namespace:  value.Namespace,
		Kind:       value.Kind,
		Name:       value.Name,
		Ready:      value.Ready,
		Desired:    value.Desired,
		Available:  value.Available,
		Updated:    value.Updated,
		Status:     WorkloadStatus(value.Status),
		AgeSeconds: value.AgeSeconds,
	}
}

func DeploymentDetail(value *appsv1.Deployment, related []ResourceRef, now time.Time) WorkloadDetailDTO {
	summary := ConvertDeployment(value, now)
	conditions := make([]ConditionDTO, 0, min(len(value.Status.Conditions), maximumConditions))
	for _, condition := range value.Status.Conditions {
		if len(conditions) == maximumConditions {
			break
		}
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	return workloadDetail(value, summary, selectorMap(value.Spec.Selector), value.Spec.Template.Annotations, conditions, value.Spec.Template.Spec.Containers, related)
}

func StatefulSetDetail(value *appsv1.StatefulSet, related []ResourceRef, now time.Time) WorkloadDetailDTO {
	summary := ConvertStatefulSet(value, now)
	conditions := make([]ConditionDTO, 0, min(len(value.Status.Conditions), maximumConditions))
	for _, condition := range value.Status.Conditions {
		if len(conditions) == maximumConditions {
			break
		}
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	return workloadDetail(value, summary, selectorMap(value.Spec.Selector), value.Spec.Template.Annotations, conditions, value.Spec.Template.Spec.Containers, related)
}

func DaemonSetDetail(value *appsv1.DaemonSet, related []ResourceRef, now time.Time) WorkloadDetailDTO {
	summary := ConvertDaemonSet(value, now)
	conditions := make([]ConditionDTO, 0, min(len(value.Status.Conditions), maximumConditions))
	for _, condition := range value.Status.Conditions {
		if len(conditions) == maximumConditions {
			break
		}
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	return workloadDetail(value, summary, selectorMap(value.Spec.Selector), value.Spec.Template.Annotations, conditions, value.Spec.Template.Spec.Containers, related)
}

func JobDetail(value *batchv1.Job, related []ResourceRef, now time.Time) WorkloadDetailDTO {
	summary := ConvertJob(value, now)
	conditions := make([]ConditionDTO, 0, min(len(value.Status.Conditions), maximumConditions))
	for _, condition := range value.Status.Conditions {
		if len(conditions) == maximumConditions {
			break
		}
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	return workloadDetail(value, summary, selectorMap(value.Spec.Selector), value.Spec.Template.Annotations, conditions, value.Spec.Template.Spec.Containers, related)
}

func CronJobDetail(value *batchv1.CronJob, jobs []batchv1.Job, related []ResourceRef, now time.Time) WorkloadDetailDTO {
	summary := ConvertCronJob(value, jobs, now)
	return workloadDetail(value, summary, nil, value.Spec.JobTemplate.Spec.Template.Annotations, []ConditionDTO{}, value.Spec.JobTemplate.Spec.Template.Spec.Containers, related)
}

func CronJobDetailWithHistory(value *batchv1.CronJob, jobs []batchv1.Job, historyComplete bool, related []ResourceRef, now time.Time) WorkloadDetailDTO {
	summary := ConvertCronJobWithHistory(value, jobs, historyComplete, now)
	return workloadDetail(value, summary, nil, value.Spec.JobTemplate.Spec.Template.Annotations, []ConditionDTO{}, value.Spec.JobTemplate.Spec.Template.Spec.Containers, related)
}

func workloadDetail(value metav1.Object, summary WorkloadDTO, selector map[string]string, annotations map[string]string, conditions []ConditionDTO, containers []corev1.Container, related []ResourceRef) WorkloadDetailDTO {
	if len(related) > maximumRelated {
		related = related[:maximumRelated]
	}
	return WorkloadDetailDTO{
		Metadata: ConvertMetadata(value), Kind: summary.Kind,
		Ready: summary.Ready, Desired: summary.Desired, Available: summary.Available, Updated: summary.Updated,
		Status: summary.Status, Selector: selector, RestartAt: restartAnnotation(annotations),
		Conditions: conditions, Containers: ContainerSpecs(containers), Related: append([]ResourceRef(nil), related...),
	}
}

func restartAnnotation(annotations map[string]string) *string {
	value := annotations["kubectl.kubernetes.io/restartedAt"]
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	canonical := parsed.UTC().Format(time.RFC3339)
	return &canonical
}
