package podhealth

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestControllingOwnerPicksControllerAndCopies(t *testing.T) {
	controller := true
	references := []metav1.OwnerReference{
		{Kind: "ReplicaSet", Name: "rs", UID: "u1"},
		{Kind: "Deployment", Name: "deploy", UID: "u2", Controller: &controller},
	}
	selected := ControllingOwner(references)
	if selected == nil || selected.Kind != "Deployment" {
		t.Fatalf("controlling owner = %+v", selected)
	}
	selected.Name = "mutated"
	if references[1].Name != "deploy" {
		t.Fatal("ControllingOwner must return a copy")
	}
	if ControllingOwner(references[:1]) != nil {
		t.Fatal("no controller reference must yield nil")
	}
	if ControllingOwner(nil) != nil {
		t.Fatal("nil references must yield nil")
	}
}

func statuses(waiting, terminated *int32) []corev1.ContainerStatus {
	status := corev1.ContainerStatus{}
	if waiting != nil {
		status.State.Waiting = &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}
	}
	if terminated != nil {
		status.State.Terminated = &corev1.ContainerStateTerminated{ExitCode: *terminated}
	}
	return []corev1.ContainerStatus{status}
}

func TestProblematicMatchesListBadgeSemantics(t *testing.T) {
	tests := []struct {
		name        string
		phase       string
		ready       int64
		desired     int64
		restarts    int64
		statuses    [][]corev1.ContainerStatus
		problematic bool
	}{
		{name: "running healthy", phase: "Running", ready: 1, desired: 1, restarts: 0, problematic: false},
		{name: "pending", phase: "Pending", ready: 0, desired: 1, problematic: true},
		{name: "failed", phase: "Failed", problematic: true},
		// Callers pass an already-normalized phase (resources normalizePodPhase
		// maps anything nonstandard to "Unknown"); a raw unknown phase is not
		// this function's job to classify.
		{name: "raw nonstandard phase passes through", phase: "SomethingElse", problematic: false},
		{name: "explicit unknown phase", phase: "Unknown", problematic: true},
		{name: "ready mismatch", phase: "Running", ready: 0, desired: 2, problematic: true},
		{name: "restarts threshold", phase: "Running", ready: 1, desired: 1, restarts: 3, problematic: true},
		{name: "restarts below threshold", phase: "Running", ready: 1, desired: 1, restarts: 2, problematic: false},
		{name: "waiting container", phase: "Running", ready: 1, desired: 1, statuses: [][]corev1.ContainerStatus{statuses(int32Ptr(1), nil)}, problematic: true},
		{name: "failed termination", phase: "Running", ready: 1, desired: 1, statuses: [][]corev1.ContainerStatus{statuses(nil, int32Ptr(1))}, problematic: true},
		{name: "clean termination", phase: "Running", ready: 1, desired: 1, statuses: [][]corev1.ContainerStatus{statuses(nil, int32Ptr(0))}, problematic: false},
		{name: "init container waiting", phase: "Running", ready: 1, desired: 1, statuses: [][]corev1.ContainerStatus{statuses(int32Ptr(1), nil)}, problematic: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Problematic(test.phase, test.ready, test.desired, test.restarts, test.statuses...)
			if got != test.problematic {
				t.Fatalf("Problematic = %v, want %v", got, test.problematic)
			}
		})
	}
}

func int32Ptr(value int32) *int32 { return &value }
