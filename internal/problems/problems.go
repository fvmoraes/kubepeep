// Package problems contains deterministic detectors over observed Kubernetes
// state. It performs no I/O and never converts missing evidence into health.
package problems

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

type Resource struct {
	APIGroup  string
	Kind      string
	Namespace string
	Name      string
}

type Problem struct {
	Resource   Resource
	Severity   Severity
	Reason     string
	Summary    string
	Source     string
	Container  string
	AgeSeconds int64
}

type WorkloadObservation struct {
	Resource   Resource
	Status     string
	Desired    *int64
	Available  *int64
	AgeSeconds int64
}

const HighRestartThreshold int32 = 5

func DetectPod(pod *corev1.Pod, now time.Time) []Problem {
	if pod == nil || pod.Name == "" {
		return nil
	}
	result := make([]Problem, 0)
	resource := Resource{Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name}
	add := func(severity Severity, reason, summary, source, container string) {
		result = append(result, Problem{Resource: resource, Severity: severity, Reason: reason, Summary: summary, Source: source, Container: container, AgeSeconds: age(pod.CreationTimestamp.Time, now)})
	}
	if pod.Status.Phase == corev1.PodPending {
		add(SeverityWarning, "Pending", "Pod is pending.", "podStatus", "")
	}
	statuses := append(append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...), pod.Status.EphemeralContainerStatuses...)
	for _, status := range statuses {
		if waiting := status.State.Waiting; waiting != nil {
			switch waiting.Reason {
			case "CrashLoopBackOff":
				add(SeverityCritical, waiting.Reason, "Container is repeatedly crashing.", "containerWaiting", status.Name)
			case "ImagePullBackOff", "ErrImagePull":
				add(SeverityCritical, waiting.Reason, "Container image cannot be pulled.", "containerWaiting", status.Name)
			case "CreateContainerConfigError":
				add(SeverityWarning, waiting.Reason, "Container configuration is invalid.", "containerWaiting", status.Name)
			}
		}
		if terminated := status.LastTerminationState.Terminated; terminated != nil && terminated.Reason == "OOMKilled" {
			add(SeverityCritical, "OOMKilled", "Container was terminated after exceeding its memory limit.", "containerTerminated", status.Name)
		}
		if status.RestartCount >= HighRestartThreshold {
			add(SeverityWarning, "HighRestarts", fmt.Sprintf("Container restarted %d times.", status.RestartCount), "containerStatus", status.Name)
		} else if status.RestartCount > 0 {
			add(SeverityInfo, "RecentRestart", fmt.Sprintf("Container restarted %d times.", status.RestartCount), "containerStatus", status.Name)
		}
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Status != corev1.ConditionFalse {
			continue
		}
		reason := strings.ToLower(condition.Reason + " " + condition.Message)
		if strings.Contains(reason, "probe") || strings.Contains(reason, "unhealthy") {
			add(SeverityWarning, "ProbeFailed", "A readiness, liveness or startup probe is failing.", "condition", "")
		}
	}
	return deduplicate(result)
}

func DetectWorkload(value WorkloadObservation) []Problem {
	if value.Resource.Name == "" {
		return nil
	}
	switch value.Status {
	case "Failed":
		return []Problem{{Resource: value.Resource, Severity: SeverityCritical, Reason: "JobFailed", Summary: "Workload has failed.", Source: "workloadStatus", AgeSeconds: value.AgeSeconds}}
	case "Degraded":
		summary := "Workload has unavailable replicas."
		if value.Desired != nil && value.Available != nil {
			summary = fmt.Sprintf("%d/%d replicas available.", *value.Available, *value.Desired)
		}
		return []Problem{{Resource: value.Resource, Severity: SeverityCritical, Reason: "ReplicasUnavailable", Summary: summary, Source: "workloadStatus", AgeSeconds: value.AgeSeconds}}
	case "Progressing":
		return []Problem{{Resource: value.Resource, Severity: SeverityInfo, Reason: "RolloutProgressing", Summary: "Workload rollout is progressing.", Source: "workloadStatus", AgeSeconds: value.AgeSeconds}}
	default:
		return nil
	}
}

func DetectPVC(namespace, name, phase string, ageSeconds int64) []Problem {
	if phase != string(corev1.ClaimPending) || name == "" {
		return nil
	}
	return []Problem{{Resource: Resource{Kind: "PersistentVolumeClaim", Namespace: namespace, Name: name}, Severity: SeverityCritical, Reason: "PVCPending", Summary: "PersistentVolumeClaim is pending and may block its workload.", Source: "pvcStatus", AgeSeconds: ageSeconds}}
}

func DetectNode(name string, ready corev1.ConditionStatus, ageSeconds int64) []Problem {
	if name == "" || ready == corev1.ConditionTrue {
		return nil
	}
	return []Problem{{Resource: Resource{Kind: "Node", Name: name}, Severity: SeverityCritical, Reason: "NodeNotReady", Summary: "Node Ready condition is not true.", Source: "nodeCondition", AgeSeconds: ageSeconds}}
}

func DetectWarningEvent(namespace, objectKind, objectName, reason string, ageSeconds int64) []Problem {
	if objectName == "" {
		return nil
	}
	summary := "Kubernetes reported a Warning event."
	if strings.EqualFold(reason, "Unhealthy") || strings.Contains(strings.ToLower(reason), "probe") {
		reason = "ProbeFailed"
		summary = "Kubernetes reported a failing readiness, liveness or startup probe."
	}
	return []Problem{{Resource: Resource{Kind: objectKind, Namespace: namespace, Name: objectName}, Severity: SeverityWarning, Reason: reason, Summary: summary, Source: "event", AgeSeconds: ageSeconds}}
}

func Sort(values []Problem) {
	rank := func(value Severity) int {
		switch value {
		case SeverityCritical:
			return 0
		case SeverityWarning:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(values, func(left, right int) bool {
		if rank(values[left].Severity) != rank(values[right].Severity) {
			return rank(values[left].Severity) < rank(values[right].Severity)
		}
		if values[left].Resource.Namespace != values[right].Resource.Namespace {
			return values[left].Resource.Namespace < values[right].Resource.Namespace
		}
		if values[left].Resource.Kind != values[right].Resource.Kind {
			return values[left].Resource.Kind < values[right].Resource.Kind
		}
		if values[left].Resource.Name != values[right].Resource.Name {
			return values[left].Resource.Name < values[right].Resource.Name
		}
		return values[left].Reason < values[right].Reason
	})
}

func deduplicate(values []Problem) []Problem {
	seen := make(map[string]struct{})
	result := make([]Problem, 0, len(values))
	for _, value := range values {
		key := string(value.Severity) + "\x00" + value.Reason + "\x00" + value.Container
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}
func age(created, now time.Time) int64 {
	if created.IsZero() || now.Before(created) {
		return 0
	}
	return int64(now.Sub(created) / time.Second)
}
