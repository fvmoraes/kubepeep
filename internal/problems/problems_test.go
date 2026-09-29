package problems

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDetectPodCoversCriticalWarningAndInfoWithoutInventingHealth(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "portal", CreationTimestamp: metav1.NewTime(now.Add(-time.Minute))}, Status: corev1.PodStatus{Phase: corev1.PodPending, ContainerStatuses: []corev1.ContainerStatus{
		{Name: "critical", RestartCount: 8, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"}}},
		{Name: "recent", RestartCount: 1},
	}}}
	values := DetectPod(pod, now)
	seen := map[string]Severity{}
	for _, value := range values {
		seen[value.Reason] = value.Severity
	}
	for reason, severity := range map[string]Severity{"Pending": SeverityWarning, "CrashLoopBackOff": SeverityCritical, "OOMKilled": SeverityCritical, "HighRestarts": SeverityWarning, "RecentRestart": SeverityInfo} {
		if seen[reason] != severity {
			t.Fatalf("%s: got %q want %q (%#v)", reason, seen[reason], severity, values)
		}
	}
}

func TestWorkloadPVCNodeAndEventSeverity(t *testing.T) {
	available, desired := int64(0), int64(3)
	if got := DetectWorkload(WorkloadObservation{Resource: Resource{Kind: "Deployment", Name: "portal"}, Status: "Degraded", Available: &available, Desired: &desired}); len(got) != 1 || got[0].Severity != SeverityCritical {
		t.Fatalf("workload: %#v", got)
	}
	if got := DetectPVC("portal", "data", "Pending", 10); len(got) != 1 || got[0].Reason != "PVCPending" {
		t.Fatalf("pvc: %#v", got)
	}
	if got := DetectNode("worker", corev1.ConditionUnknown, 10); len(got) != 1 || got[0].Reason != "NodeNotReady" {
		t.Fatalf("node: %#v", got)
	}
	if got := DetectWarningEvent("portal", "Pod", "api", "Unhealthy", 10); len(got) != 1 || got[0].Severity != SeverityWarning || got[0].Reason != "ProbeFailed" {
		t.Fatalf("event: %#v", got)
	}
}
