// Package podhealth owns the shared pod classification primitives used by
// both the resource lists (problem badge, owner column) and the dashboard
// (problem diagnosis fallback, restart ranking). Keeping them in one leaf
// package prevents the two layers from drifting apart.
package podhealth

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ControllingOwner returns a copy of the owner reference flagged as
// controller, or nil when there is none. Callers decide their own fallback
// (dashboard refuses to guess; the resource list falls back to the first
// reference to preserve historical behavior).
func ControllingOwner(references []metav1.OwnerReference) *metav1.OwnerReference {
	for index := range references {
		if references[index].Controller != nil && *references[index].Controller {
			selected := references[index]
			return &selected
		}
	}
	return nil
}

// Problematic is the canonical fast heuristic behind the resource-list
// "problem" badge. It is intentionally cheaper and stricter-timed than the
// dashboard's ClassifyProblemPod diagnosis: the badge must react immediately
// (no grace periods), while the dashboard adds evidence, severity, and
// reasons. Any change here changes what every resource list marks as
// problematic.
func Problematic(phase string, readyCurrent, readyDesired int64, restarts int64, statusGroups ...[]corev1.ContainerStatus) bool {
	if phase == "Pending" || phase == "Failed" || phase == "Unknown" {
		return true
	}
	if phase == "Running" && readyCurrent != readyDesired {
		return true
	}
	if restarts >= 3 {
		return true
	}
	for _, statuses := range statusGroups {
		for _, status := range statuses {
			if status.State.Waiting != nil {
				return true
			}
			if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
				return true
			}
		}
	}
	return false
}
