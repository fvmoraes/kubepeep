package dashboard

import (
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
)

type LogTarget struct {
	Namespace     string
	Pod           string
	Container     string
	ContainerType ContainerType
	Workload      *ResourceRef
	Previous      bool
	priority      int
	restarts      int64
}

type LogTargetSelection struct {
	Targets   []LogTarget
	Truncated bool
}

type logTargetKey struct {
	namespace string
	pod       string
	container string
	previous  bool
}

// SelectLogTargets gives priority to the documented three groups without
// broadening a scan to unrelated containers in the same namespace.
func SelectLogTargets(pods []corev1.Pod, problems []ProblemPodDTO, restarts []RestartDTO, now time.Time, request ResolvedLogScanRequest, budget LogBudget) LogTargetSelection {
	budget = budget.normalized()
	if request.Window <= 0 || request.Window > MaximumLogWindow {
		request.Window = DefaultLogWindow
	}
	if request.MaxPods <= 0 || request.MaxPods > MaximumLogMaxPods {
		request.MaxPods = DefaultLogMaxPods
	}
	problemByPod := make(map[string][]ProblemPodDTO)
	for _, problem := range problems {
		key := problem.Namespace + "\x00" + problem.Pod
		problemByPod[key] = append(problemByPod[key], problem)
	}
	restartByContainer := make(map[string]RestartDTO)
	for _, restart := range restarts {
		key := containerIdentity(restart.Namespace, restart.Pod, restart.ContainerType, restart.Container)
		if current, ok := restartByContainer[key]; !ok || restart.Restarts > current.Restarts {
			restartByContainer[key] = restart
		}
	}
	unique := make(map[logTargetKey]LogTarget)
	for podIndex := range pods {
		pod := &pods[podIndex]
		podProblems := problemByPod[pod.Namespace+"\x00"+pod.Name]
		owner := DirectPodOwner(pod)
		appendStatuses := func(statuses []corev1.ContainerStatus, containerType ContainerType) {
			for _, status := range statuses {
				priority := 99
				previous := false
				for _, problem := range podProblems {
					if problem.Container == nil || (*problem.Container == status.Name && problem.ContainerType != nil && *problem.ContainerType == containerType) {
						priority = 0
						break
					}
				}
				restart := restartByContainer[containerIdentity(pod.Namespace, pod.Name, containerType, status.Name)]
				if restart.Restarts > 0 && priority > 1 {
					priority = 1
				}
				if restart.Restarts > 0 && status.LastTerminationState.Terminated != nil {
					previous = true
				}
				if recentlyTerminated(status, now, request.Window) && priority > 2 {
					priority = 2
					previous = status.LastTerminationState.Terminated != nil
				}
				if priority == 99 {
					continue
				}
				target := LogTarget{
					Namespace:     pod.Namespace,
					Pod:           pod.Name,
					Container:     status.Name,
					ContainerType: containerType,
					Workload:      cloneResourceRef(owner),
					Previous:      previous,
					priority:      priority,
					restarts:      restart.Restarts,
				}
				key := logTargetKey{namespace: target.Namespace, pod: target.Pod, container: target.Container, previous: target.Previous}
				if current, ok := unique[key]; !ok || lessLogTarget(target, current) {
					unique[key] = target
				}
			}
		}
		appendStatuses(pod.Status.ContainerStatuses, ContainerRegular)
		appendStatuses(pod.Status.InitContainerStatuses, ContainerInit)
		appendStatuses(pod.Status.EphemeralContainerStatuses, ContainerEphemeral)
	}
	values := make([]LogTarget, 0, len(unique))
	for _, target := range unique {
		values = append(values, target)
	}
	sort.SliceStable(values, func(left, right int) bool { return lessLogTarget(values[left], values[right]) })
	selected := make([]LogTarget, 0, minInt(len(values), budget.MaxContainers))
	selectedPods := make(map[string]struct{})
	truncated := false
	for _, target := range values {
		podKey := target.Namespace + "\x00" + target.Pod
		if _, exists := selectedPods[podKey]; !exists && len(selectedPods) >= request.MaxPods {
			truncated = true
			continue
		}
		if len(selected) >= budget.MaxContainers {
			truncated = true
			break
		}
		selectedPods[podKey] = struct{}{}
		selected = append(selected, target)
	}
	return LogTargetSelection{Targets: selected, Truncated: truncated}
}

func recentlyTerminated(status corev1.ContainerStatus, now time.Time, window time.Duration) bool {
	for _, terminated := range []*corev1.ContainerStateTerminated{status.State.Terminated, status.LastTerminationState.Terminated} {
		if terminated == nil || terminated.FinishedAt.IsZero() {
			continue
		}
		age := now.Sub(terminated.FinishedAt.Time)
		if age >= 0 && age <= window {
			return true
		}
	}
	return false
}

func containerIdentity(namespace, pod string, containerType ContainerType, container string) string {
	return namespace + "\x00" + pod + "\x00" + string(containerType) + "\x00" + container
}

func lessLogTarget(left, right LogTarget) bool {
	if left.priority != right.priority {
		return left.priority < right.priority
	}
	if left.restarts != right.restarts {
		return left.restarts > right.restarts
	}
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	if left.Pod != right.Pod {
		return left.Pod < right.Pod
	}
	if typeRank(left.ContainerType) != typeRank(right.ContainerType) {
		return typeRank(left.ContainerType) < typeRank(right.ContainerType)
	}
	if left.Container != right.Container {
		return left.Container < right.Container
	}
	if left.Previous != right.Previous {
		return !left.Previous
	}
	return false
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
