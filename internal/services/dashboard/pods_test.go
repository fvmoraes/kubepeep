package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestRestartSeverityThresholds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		restarts int64
		want     RestartSeverity
	}{
		{-1, RestartHealthy}, {0, RestartHealthy}, {1, RestartAttention},
		{2, RestartAttention}, {3, RestartWarning}, {9, RestartWarning},
		{10, RestartCritical}, {100, RestartCritical},
	}
	for _, test := range tests {
		if got := RestartSeverityFor(test.restarts); got != test.want {
			t.Fatalf("RestartSeverityFor(%d) = %q, want %q", test.restarts, got, test.want)
		}
	}
}

func TestPodHealthyRequiresPositiveRunningReadyContainerEvidence(t *testing.T) {
	t.Parallel()
	now := time.Now()
	healthy := healthyTestPod(now)
	if !podIsPositivelyHealthy(&healthy) {
		t.Fatal("positive Kubernetes health evidence was rejected")
	}

	tests := []struct {
		name   string
		mutate func(*corev1.Pod)
	}{
		{"missing ready condition", func(pod *corev1.Pod) { pod.Status.Conditions = nil }},
		{"unknown ready condition", func(pod *corev1.Pod) { pod.Status.Conditions[0].Status = corev1.ConditionUnknown }},
		{"not running", func(pod *corev1.Pod) { pod.Status.Phase = corev1.PodPending }},
		{"missing containers", func(pod *corev1.Pod) { pod.Status.ContainerStatuses = nil }},
		{"container not ready", func(pod *corev1.Pod) { pod.Status.ContainerStatuses[0].Ready = false }},
		{"container state unknown", func(pod *corev1.Pod) { pod.Status.ContainerStatuses[0].State = corev1.ContainerState{} }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			pod := healthyTestPod(now)
			test.mutate(&pod)
			if podIsPositivelyHealthy(&pod) {
				t.Fatalf("%s was presented as healthy", test.name)
			}
		})
	}
}

func TestPodRestartsIncludesAllContainerTypesAndSortsDeterministically(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api-abc", CreationTimestamp: metav1.NewTime(now.Add(-14 * time.Minute))},
		Status: corev1.PodStatus{
			ContainerStatuses:          []corev1.ContainerStatus{{Name: "api", RestartCount: 4, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error"}}}},
			InitContainerStatuses:      []corev1.ContainerStatus{{Name: "migrate", RestartCount: 2, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
			EphemeralContainerStatuses: []corev1.ContainerStatus{{Name: "debug", RestartCount: 10, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}}}},
		},
	}
	owner := &ResourceRef{APIGroup: "apps", Kind: "Deployment", Namespace: "payments", Name: "api", UID: "owner-1"}
	rows := PodRestarts(&pod, owner, now)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0].ContainerType != ContainerEphemeral || rows[0].Severity != RestartCritical {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].ContainerType != ContainerRegular || rows[1].Status != "CrashLoopBackOff" || rows[1].LastReason != "Error" {
		t.Fatalf("unexpected regular row: %+v", rows[1])
	}
	if rows[2].ContainerType != ContainerInit || rows[2].Severity != RestartAttention {
		t.Fatalf("unexpected init row: %+v", rows[2])
	}
	if rows[0].AgeSeconds != 840 || rows[0].Owner == owner {
		t.Fatalf("age/owner copy mismatch: %+v", rows[0])
	}
}

func TestClassifyProblemPodClosedTable(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		mutate   func(*corev1.Pod) []NormalizedEvent
		want     ProblemSource
		severity ProblemSeverity
	}{
		{"failed phase", func(p *corev1.Pod) []NormalizedEvent {
			p.Status.Phase = corev1.PodFailed
			p.Status.Reason = "Failed"
			return nil
		}, ProblemPodStatus, ProblemCritical},
		{"evicted", func(p *corev1.Pod) []NormalizedEvent { p.Status.Reason = "Evicted"; return nil }, ProblemPodStatus, ProblemCritical},
		{"oom current", func(p *corev1.Pod) []NormalizedEvent {
			p.Status.ContainerStatuses = []corev1.ContainerStatus{terminatedStatus("api", "OOMKilled", false)}
			return nil
		}, ProblemContainerTerminated, ProblemCritical},
		{"oom last", func(p *corev1.Pod) []NormalizedEvent {
			p.Status.ContainerStatuses = []corev1.ContainerStatus{terminatedStatus("api", "OOMKilled", true)}
			return nil
		}, ProblemContainerTerminated, ProblemCritical},
		{"crash loop", waitingMutation("CrashLoopBackOff"), ProblemContainerWaiting, ProblemCritical},
		{"config error", waitingMutation("CreateContainerConfigError"), ProblemContainerWaiting, ProblemCritical},
		{"run error", waitingMutation("RunContainerError"), ProblemContainerWaiting, ProblemCritical},
		{"image backoff", waitingMutation("ImagePullBackOff"), ProblemContainerWaiting, ProblemWarning},
		{"image pull", waitingMutation("ErrImagePull"), ProblemContainerWaiting, ProblemWarning},
		{"unschedulable", func(p *corev1.Pod) []NormalizedEvent {
			p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "no nodes"}}
			return nil
		}, ProblemCondition, ProblemWarning},
		{"liveness", eventMutation(now, "Unhealthy", "Liveness probe failed: status 500"), ProblemEvent, ProblemCritical},
		{"failed scheduling", eventMutation(now, "FailedScheduling", "insufficient cpu"), ProblemEvent, ProblemWarning},
		{"readiness", eventMutation(now, "Unhealthy", "readiness PROBE FAILED: timeout"), ProblemEvent, ProblemWarning},
		{"not ready", func(p *corev1.Pod) []NormalizedEvent {
			p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "api", Ready: false}}
			return nil
		}, ProblemContainerStatus, ProblemWarning},
		{"pending", func(p *corev1.Pod) []NormalizedEvent { p.Status.Phase = corev1.PodPending; return nil }, ProblemPodStatus, ProblemWarning},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pod := healthyTestPod(now)
			events := test.mutate(&pod)
			problem, ok := ClassifyProblemPod(&pod, events, nil, now)
			if !ok {
				t.Fatal("expected problem")
			}
			if problem.Source != test.want || problem.Severity != test.severity {
				t.Fatalf("got source/severity %q/%q, want %q/%q", problem.Source, problem.Severity, test.want, test.severity)
			}
		})
	}
}

func TestProblemBoundariesAreInclusiveAndUIDIsMandatory(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		pod    corev1.Pod
		events []NormalizedEvent
		want   bool
	}{
		{"ready exactly two minutes", runningUnreadyPod(now.Add(-2 * time.Minute)), nil, true},
		{"ready below two minutes", runningUnreadyPod(now.Add(-2*time.Minute + time.Nanosecond)), nil, false},
		{"pending exactly five minutes", pendingPod(now.Add(-5 * time.Minute)), nil, true},
		{"pending below five minutes", pendingPod(now.Add(-5*time.Minute + time.Nanosecond)), nil, false},
		{"event exactly fifteen minutes", healthyTestPod(now), []NormalizedEvent{problemEvent(now.Add(-15*time.Minute), "event", "Unhealthy", "Liveness probe failed")}, true},
		{"event older than fifteen minutes", healthyTestPod(now), []NormalizedEvent{problemEvent(now.Add(-15*time.Minute-time.Nanosecond), "event", "Unhealthy", "Liveness probe failed")}, false},
		{"event at future tolerance", healthyTestPod(now), []NormalizedEvent{problemEvent(now.Add(time.Minute), "event", "Unhealthy", "Liveness probe failed")}, true},
		{"event beyond future tolerance", healthyTestPod(now), []NormalizedEvent{problemEvent(now.Add(time.Minute+time.Nanosecond), "event", "Unhealthy", "Liveness probe failed")}, false},
		{"event without regarding uid", healthyTestPod(now), []NormalizedEvent{{UID: "event", Namespace: "payments", RegardingKind: "Pod", RegardingName: "api", Reason: "Unhealthy", Message: "Liveness probe failed", ObservedAt: now}}, false},
		{"normal event ignored", healthyTestPod(now), []NormalizedEvent{{UID: "event", Namespace: "payments", RegardingKind: "Pod", RegardingName: "api", RegardingUID: "pod-uid", Type: "Normal", Reason: "Unhealthy", Message: "Liveness probe failed", ObservedAt: now}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, got := ClassifyProblemPod(&test.pod, test.events, nil, now)
			if got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestProblemPriorityContainerTieAndConditionBeforeEvent(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	pod := healthyTestPod(now)
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{terminatedStatus("zeta", "OOMKilled", false)}
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{terminatedStatus("alpha", "OOMKilled", false)}
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "condition wins"}}
	events := []NormalizedEvent{problemEvent(now, "event", "FailedScheduling", "event loses")}
	problem, ok := ClassifyProblemPod(&pod, events, nil, now)
	if !ok || problem.Source != ProblemContainerTerminated || problem.Container == nil || *problem.Container != "zeta" || problem.ContainerType == nil || *problem.ContainerType != ContainerRegular {
		t.Fatalf("unexpected priority winner: %+v", problem)
	}

	pod.Status.ContainerStatuses = nil
	pod.Status.InitContainerStatuses = nil
	problem, ok = ClassifyProblemPod(&pod, events, nil, now)
	if !ok || problem.Source != ProblemCondition || problem.Message == nil || *problem.Message != "condition wins" {
		t.Fatalf("condition did not precede equivalent Event: %+v", problem)
	}
}

func TestProblemEventTieBreakAndMissingDiagnosis(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	pod := healthyTestPod(now)
	events := []NormalizedEvent{
		problemEvent(now, "z", "Unhealthy", "Liveness probe failed: old"),
		problemEvent(now, "a", "Unhealthy", "Liveness probe failed: winner"),
	}
	events[1].Count = 2
	problem, ok := ClassifyProblemPod(&pod, events, nil, now)
	if !ok || problem.Message == nil || *problem.Message != "Liveness probe failed: winner" {
		t.Fatalf("unexpected event tie winner: %+v", problem)
	}

	pod = pendingPod(now.Add(-5 * time.Minute))
	problem, ok = ClassifyProblemPod(&pod, nil, nil, now)
	if !ok || problem.Reason != nil || problem.Message != nil {
		t.Fatalf("missing diagnosis was fabricated: %+v", problem)
	}
}

func healthyTestPod(now time.Time) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api", UID: types.UID("pod-uid"), CreationTimestamp: metav1.NewTime(now.Add(-10 * time.Minute))},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "api", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
}

func waitingMutation(reason string) func(*corev1.Pod) []NormalizedEvent {
	return func(p *corev1.Pod) []NormalizedEvent {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "api", Ready: true, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: "real message"}}}}
		return nil
	}
}

func eventMutation(now time.Time, reason, message string) func(*corev1.Pod) []NormalizedEvent {
	return func(*corev1.Pod) []NormalizedEvent {
		return []NormalizedEvent{problemEvent(now, "event", reason, message)}
	}
}

func problemEvent(at time.Time, uid, reason, message string) NormalizedEvent {
	return NormalizedEvent{UID: types.UID(uid), Namespace: "payments", RegardingKind: "Pod", RegardingName: "api", RegardingUID: "pod-uid", Type: "Warning", Reason: reason, Message: message, Count: 1, ObservedAt: at}
}

func terminatedStatus(name, reason string, previous bool) corev1.ContainerStatus {
	state := corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: reason, Message: "terminated"}}
	if previous {
		return corev1.ContainerStatus{Name: name, Ready: true, LastTerminationState: state}
	}
	return corev1.ContainerStatus{Name: name, Ready: true, State: state}
}

func runningUnreadyPod(created time.Time) corev1.Pod {
	pod := healthyTestPod(created.Add(10 * time.Minute))
	pod.CreationTimestamp = metav1.NewTime(created)
	pod.Status.ContainerStatuses[0].Ready = false
	return pod
}

func pendingPod(created time.Time) corev1.Pod {
	pod := healthyTestPod(created.Add(10 * time.Minute))
	pod.CreationTimestamp = metav1.NewTime(created)
	pod.Status.Phase = corev1.PodPending
	pod.Status.ContainerStatuses = nil
	return pod
}
