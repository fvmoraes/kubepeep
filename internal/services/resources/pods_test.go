package resources

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodInventoryContainerStatesAndStartup(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	notStarted := false
	for _, tt := range []struct {
		name              string
		phase             corev1.PodPhase
		state             corev1.ContainerState
		started           *bool
		age               time.Duration
		wantStatus        string
		starting, problem bool
	}{
		{"running", corev1.PodRunning, corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}, nil, 10 * time.Minute, "running", false, false},
		{"startup probe", corev1.PodRunning, corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}, &notStarted, 5 * time.Minute, "starting", true, false},
		{"readiness warming up", corev1.PodRunning, corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}, nil, time.Second, "starting", true, false},
		{"readiness failed", corev1.PodRunning, corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}, nil, 10 * time.Minute, "problem", false, true},
		{"slow image pull", corev1.PodPending, corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}, nil, 10 * time.Minute, "starting", true, false},
		{"creating", corev1.PodPending, corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}, nil, time.Second, "starting", true, false},
		{"crash loop", corev1.PodPending, corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}, nil, time.Second, "problem", false, true},
		{"image error", corev1.PodPending, corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}, nil, time.Second, "problem", false, true},
		{"failed", corev1.PodFailed, corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}, nil, time.Second, "problem", false, true},
		{"completed", corev1.PodSucceeded, corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}, nil, time.Second, "inactive", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-tt.age))}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Env: []corev1.EnvVar{{Name: "TOKEN", Value: "must-not-appear"}}}}}, Status: corev1.PodStatus{Phase: tt.phase, ContainerStatuses: []corev1.ContainerStatus{{Name: "app", State: tt.state, Started: tt.started, Ready: tt.name == "running"}}}}
			result := ConvertPod(pod, now)
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte("must-not-appear")) {
				t.Fatal("inventory exposed container environment")
			}
			if result.Starting != tt.starting || result.Problematic != tt.problem {
				t.Fatalf("starting=%v problem=%v", result.Starting, result.Problematic)
			}
			if result.ContainerCount != 1 || len(result.Containers) != 1 || result.Containers[0].Status != tt.wantStatus {
				t.Fatalf("containers = %#v", result.Containers)
			}
		})
	}
}

func TestPodDetailRelatedEventsJSON(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		events []ResourceRef
		want   int
	}{
		{name: "absent", want: 0},
		{name: "empty", events: []ResourceRef{}, want: 0},
		{name: "present", events: []ResourceRef{{Kind: "Event", Name: "scheduled"}}, want: 1},
		{name: "bounded", events: make([]ResourceRef, 101), want: 100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := json.Marshal(PodDetail(&corev1.Pod{}, tt.events, time.Unix(200, 0)))
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"conditions", "containers", "initContainers", "ephemeralContainers", "relatedEvents"} {
				if len(wire[field]) == 0 || wire[field][0] != '[' {
					t.Errorf("%s must serialize as an array, got %s", field, wire[field])
				}
			}
			var events []ResourceRef
			if err := json.Unmarshal(wire["relatedEvents"], &events); err != nil {
				t.Fatal(err)
			}
			if len(events) != tt.want {
				t.Errorf("events count = %d, want %d", len(events), tt.want)
			}
			if tt.want == 1 && events[0].Name != "scheduled" {
				t.Error("event reference was lost")
			}
		})
	}
}
