package resources

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func appsv1ReplicaSet(desired, ready int32) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api"},
		Spec:       appsv1.ReplicaSetSpec{Replicas: &desired},
		Status:     appsv1.ReplicaSetStatus{ReadyReplicas: ready, AvailableReplicas: ready},
	}
}

func resourceQuantity(value string) resource.Quantity {
	return resource.MustParse(value)
}

func TestConvertReplicaSetClassification(t *testing.T) {
	now := time.Now()
	replicas := int32(3)
	fullyReady := appsv1ReplicaSet(replicas, 3)
	if status := ConvertReplicaSet(fullyReady, now).Status; status != WorkloadHealthy {
		t.Fatalf("ready==desired status=%s, want Healthy", status)
	}
	partial := appsv1ReplicaSet(replicas, 1)
	if status := ConvertReplicaSet(partial, now).Status; status != WorkloadProgressing {
		t.Fatalf("ready<desired status=%s, want Progressing", status)
	}
}

func TestConvertHPAKeepsUnknownMetricsDistinctFromZero(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api", CreationTimestamp: metav1.Now()},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "api"},
			MinReplicas:    nil,
			MaxReplicas:    5,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{},
	}
	dto := ConvertHorizontalPodAutoscaler(hpa, time.Now())
	if dto.MinReplicas != nil || dto.CurrentReplicas != 0 && dto.DesiredReplicas != 0 {
		t.Fatalf("unexpected replicas state: %#v", dto)
	}
	if len(dto.Conditions) != 0 || len(dto.MetricNames) != 0 {
		t.Fatalf("conditions/metrics must be absent, not zero-invented: %#v", dto)
	}
}

func TestConvertPDBPreservesIntOrString(t *testing.T) {
	intVal := intstr.FromInt32(2)
	strVal := intstr.FromString("50%")
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api", CreationTimestamp: metav1.Now()},
		Spec:       policyv1.PodDisruptionBudgetSpec{MinAvailable: &intVal, MaxUnavailable: &strVal},
		Status:     policyv1.PodDisruptionBudgetStatus{CurrentHealthy: 4, DesiredHealthy: 3, DisruptionsAllowed: 1, ExpectedPods: 4},
	}
	dto := ConvertPodDisruptionBudget(pdb, time.Now())
	if dto.MinAvailable == nil || !dto.MinAvailable.IsInt || dto.MinAvailable.Int != 2 {
		t.Fatalf("minAvailable=%#v", dto.MinAvailable)
	}
	if dto.MaxUnavailable == nil || dto.MaxUnavailable.IsInt || dto.MaxUnavailable.String != "50%" {
		t.Fatalf("maxUnavailable=%#v", dto.MaxUnavailable)
	}
	if dto.DisruptionsAllowed != 1 || dto.ExpectedPods != 4 {
		t.Fatalf("status=%#v", dto)
	}
}

func TestConvertQuotaKeepsObjectCountsAsStrings(t *testing.T) {
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "team"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{corev1.ResourceRequestsCPU: resourceQuantity("400m"), corev1.ResourceName("count/deployments.apps"): resourceQuantity("10")},
			Used: corev1.ResourceList{corev1.ResourceRequestsCPU: resourceQuantity("250m"), corev1.ResourceName("count/deployments.apps"): resourceQuantity("3")},
		},
	}
	dto := ConvertResourceQuota(quota)
	if dto.Hard["count/deployments.apps"] != "10" || dto.Used["count/deployments.apps"] != "3" {
		t.Fatalf("object counts must survive verbatim: %#v", dto)
	}
	if dto.Truncated {
		t.Fatal("small quota must not be truncated")
	}
}
