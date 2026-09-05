package resources

import (
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	maximumQuotaEntries  = 64
	maximumLimitItems    = 32
	maximumHPAConditions = 16
	maximumHPAMetrics    = 16
)

// ServiceAccountDTO is metadata-only: tokens, Secret references and
// annotations are never projected (V3-07).
type ServiceAccountDTO struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	UID        string `json:"uid"`
	AgeSeconds int64  `json:"ageSeconds"`
}

func (ServiceAccountDTO) resourceListItem() {}

func (ServiceAccountDTO) resourceDetailItem() {}

// ConvertServiceAccount projects one ServiceAccount onto the metadata DTO.
// The same DTO serves list and detail: there is nothing else to disclose.
func ConvertServiceAccount(value *corev1.ServiceAccount, now time.Time) ServiceAccountDTO {
	return ServiceAccountDTO{
		Namespace: value.Namespace, Name: value.Name, UID: string(value.UID),
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

// ResourceQuotaDTO keeps hard/used as quantity strings with units preserved;
// object counts stay strings and are never interpreted as content.
type ResourceQuotaDTO struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Hard      map[string]string `json:"hard"`
	Used      map[string]string `json:"used"`
	Truncated bool              `json:"truncated"`
}

func (ResourceQuotaDTO) resourceListItem() {}

func (ResourceQuotaDTO) resourceDetailItem() {}

// ConvertResourceQuota projects one ResourceQuota onto the bounded DTO.
func ConvertResourceQuota(value *corev1.ResourceQuota) ResourceQuotaDTO {
	return ResourceQuotaDTO{
		Namespace: value.Namespace, Name: value.Name,
		Hard: boundedQuantity(value.Status.Hard), Used: boundedQuantity(value.Status.Used),
		Truncated: len(value.Status.Hard) > maximumQuotaEntries || len(value.Status.Used) > maximumQuotaEntries,
	}
}

// LimitRangeItemDTO is one bounded LimitRange entry.
type LimitRangeItemDTO struct {
	Type                 string            `json:"type"`
	Max                  map[string]string `json:"max"`
	Min                  map[string]string `json:"min"`
	Default              map[string]string `json:"default"`
	DefaultRequest       map[string]string `json:"defaultRequest"`
	MaxLimitRequestRatio map[string]string `json:"maxLimitRequestRatio"`
}

// LimitRangeDTO lists the bounded items of one LimitRange.
type LimitRangeDTO struct {
	Namespace string              `json:"namespace"`
	Name      string              `json:"name"`
	UID       string              `json:"uid"`
	Items     []LimitRangeItemDTO `json:"items"`
	Truncated bool                `json:"truncated"`
}

func (LimitRangeDTO) resourceListItem() {}

func (LimitRangeDTO) resourceDetailItem() {}

// ConvertLimitRange projects one LimitRange onto the bounded DTO.
func ConvertLimitRange(value *corev1.LimitRange) LimitRangeDTO {
	items := make([]LimitRangeItemDTO, 0, min(len(value.Spec.Limits), maximumLimitItems))
	for _, limit := range value.Spec.Limits {
		if len(items) == maximumLimitItems {
			break
		}
		items = append(items, LimitRangeItemDTO{
			Type:                 string(limit.Type),
			Max:                  boundedQuantity(limit.Max),
			Min:                  boundedQuantity(limit.Min),
			Default:              boundedQuantity(limit.Default),
			DefaultRequest:       boundedQuantity(limit.DefaultRequest),
			MaxLimitRequestRatio: boundedQuantity(limit.MaxLimitRequestRatio),
		})
	}
	return LimitRangeDTO{
		Namespace: value.Namespace, Name: value.Name, UID: string(value.UID),
		Items: items, Truncated: len(value.Spec.Limits) > maximumLimitItems,
	}
}

// HorizontalPodAutoscalerDTO reports current/desired replicas and conditions.
// Absence of metrics stays unknown; zero is never invented.
type HorizontalPodAutoscalerDTO struct {
	Namespace       string         `json:"namespace"`
	Name            string         `json:"name"`
	TargetKind      string         `json:"targetKind"`
	TargetName      string         `json:"targetName"`
	MinReplicas     *int32         `json:"minReplicas"`
	MaxReplicas     int32          `json:"maxReplicas"`
	CurrentReplicas int32          `json:"currentReplicas"`
	DesiredReplicas int32          `json:"desiredReplicas"`
	Conditions      []ConditionDTO `json:"conditions"`
	MetricNames     []string       `json:"metricNames"`
	Truncated       bool           `json:"truncated"`
	AgeSeconds      int64          `json:"ageSeconds"`
}

func (HorizontalPodAutoscalerDTO) resourceListItem() {}

func (HorizontalPodAutoscalerDTO) resourceDetailItem() {}

// ConvertHorizontalPodAutoscaler projects one HPA onto the bounded DTO.
func ConvertHorizontalPodAutoscaler(value *autoscalingv2.HorizontalPodAutoscaler, now time.Time) HorizontalPodAutoscalerDTO {
	conditions := make([]ConditionDTO, 0, min(len(value.Status.Conditions), maximumHPAConditions))
	for _, condition := range value.Status.Conditions {
		if len(conditions) == maximumHPAConditions {
			break
		}
		conditions = append(conditions, conditionDTO(string(condition.Type), string(condition.Status), condition.Reason, condition.Message, condition.LastTransitionTime))
	}
	metricNames := make([]string, 0, min(len(value.Spec.Metrics), maximumHPAMetrics))
	for _, metric := range value.Spec.Metrics {
		if len(metricNames) == maximumHPAMetrics {
			break
		}
		metricNames = append(metricNames, hpaMetricName(metric))
	}
	return HorizontalPodAutoscalerDTO{
		Namespace: value.Namespace, Name: value.Name,
		TargetKind: value.Spec.ScaleTargetRef.Kind, TargetName: value.Spec.ScaleTargetRef.Name,
		MinReplicas: value.Spec.MinReplicas, MaxReplicas: value.Spec.MaxReplicas,
		CurrentReplicas: value.Status.CurrentReplicas, DesiredReplicas: value.Status.DesiredReplicas,
		Conditions: conditions, MetricNames: metricNames,
		Truncated:  len(value.Status.Conditions) > maximumHPAConditions || len(value.Spec.Metrics) > maximumHPAMetrics,
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

func hpaMetricName(metric autoscalingv2.MetricSpec) string {
	switch metric.Type {
	case autoscalingv2.ResourceMetricSourceType:
		return "resource/" + string(metric.Resource.Name)
	case autoscalingv2.ContainerResourceMetricSourceType:
		return "containerResource/" + string(metric.ContainerResource.Name)
	case autoscalingv2.PodsMetricSourceType:
		return "pods/" + metric.Pods.Metric.Name
	case autoscalingv2.ObjectMetricSourceType:
		return "object/" + metric.Object.Metric.Name
	case autoscalingv2.ExternalMetricSourceType:
		return "external/" + metric.External.Metric.Name
	default:
		return string(metric.Type)
	}
}

// IntOrStringDTO preserves the Kubernetes IntOrString union without guessing.
type IntOrStringDTO struct {
	IsInt  bool   `json:"isInt"`
	Int    int32  `json:"int"`
	String string `json:"string"`
}

// PodDisruptionBudgetDTO reports disruption state; selectors are bounded.
type PodDisruptionBudgetDTO struct {
	Namespace          string            `json:"namespace"`
	Name               string            `json:"name"`
	MinAvailable       *IntOrStringDTO   `json:"minAvailable"`
	MaxUnavailable     *IntOrStringDTO   `json:"maxUnavailable"`
	CurrentHealthy     int32             `json:"currentHealthy"`
	DesiredHealthy     int32             `json:"desiredHealthy"`
	DisruptionsAllowed int32             `json:"disruptionsAllowed"`
	ExpectedPods       int32             `json:"expectedPods"`
	Selector           map[string]string `json:"selector"`
	AgeSeconds         int64             `json:"ageSeconds"`
}

func (PodDisruptionBudgetDTO) resourceListItem() {}

func (PodDisruptionBudgetDTO) resourceDetailItem() {}

// ConvertPodDisruptionBudget projects one PDB onto the bounded DTO.
func ConvertPodDisruptionBudget(value *policyv1.PodDisruptionBudget, now time.Time) PodDisruptionBudgetDTO {
	return PodDisruptionBudgetDTO{
		Namespace: value.Namespace, Name: value.Name,
		MinAvailable: intOrStringDTO(value.Spec.MinAvailable), MaxUnavailable: intOrStringDTO(value.Spec.MaxUnavailable),
		CurrentHealthy: value.Status.CurrentHealthy, DesiredHealthy: value.Status.DesiredHealthy,
		DisruptionsAllowed: value.Status.DisruptionsAllowed, ExpectedPods: value.Status.ExpectedPods,
		Selector:   selectorMap(value.Spec.Selector),
		AgeSeconds: int64(now.Sub(value.CreationTimestamp.Time) / time.Second),
	}
}

func intOrStringDTO(value *intstr.IntOrString) *IntOrStringDTO {
	if value == nil {
		return nil
	}
	if value.Type == intstr.Int {
		return &IntOrStringDTO{IsInt: true, Int: value.IntVal}
	}
	return &IntOrStringDTO{IsInt: false, String: value.StrVal}
}
