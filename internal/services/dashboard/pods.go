package dashboard

import (
	"context"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodSnapshot struct {
	Pods   []corev1.Pod
	Events []NormalizedEvent
}

type PodOverview struct {
	Total       int64
	Healthy     int64
	Problematic int64
	Restarts    int64
}

type PodService struct {
	pods   PodPort
	events EventPort
	owners OwnerResolver
	clock  Clock
	budget QueryBudget
}

func NewPodService(pods PodPort, events EventPort, owners OwnerResolver, clock Clock, budget QueryBudget) *PodService {
	if clock == nil {
		clock = realClock{}
	}
	return &PodService{pods: pods, events: events, owners: owners, clock: clock, budget: budget.normalized()}
}

func RestartSeverityFor(restarts int64) RestartSeverity {
	switch {
	case restarts <= 0:
		return RestartHealthy
	case restarts <= 2:
		return RestartAttention
	case restarts <= 9:
		return RestartWarning
	default:
		return RestartCritical
	}
}

func PodRestarts(pod *corev1.Pod, owner *ResourceRef, now time.Time) []RestartDTO {
	if pod == nil {
		return []RestartDTO{}
	}
	rows := make([]RestartDTO, 0, len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses)+len(pod.Status.EphemeralContainerStatuses))
	appendStatuses := func(statuses []corev1.ContainerStatus, containerType ContainerType) {
		for _, status := range statuses {
			if status.RestartCount <= 0 {
				continue
			}
			current, last := containerStatus(status)
			rows = append(rows, RestartDTO{
				Namespace:     pod.Namespace,
				Pod:           pod.Name,
				Owner:         cloneResourceRef(owner),
				Container:     status.Name,
				ContainerType: containerType,
				Restarts:      int64(status.RestartCount),
				Severity:      RestartSeverityFor(int64(status.RestartCount)),
				Status:        current,
				LastReason:    last,
				AgeSeconds:    ageSeconds(pod.CreationTimestamp, now),
			})
		}
	}
	appendStatuses(pod.Status.ContainerStatuses, ContainerRegular)
	appendStatuses(pod.Status.InitContainerStatuses, ContainerInit)
	appendStatuses(pod.Status.EphemeralContainerStatuses, ContainerEphemeral)
	SortRestarts(rows)
	return rows
}

func containerStatus(status corev1.ContainerStatus) (string, string) {
	current := "Unknown"
	last := ""
	switch {
	case status.State.Waiting != nil:
		current = status.State.Waiting.Reason
		if current == "" {
			current = "Waiting"
		}
	case status.State.Running != nil:
		current = "Running"
	case status.State.Terminated != nil:
		current = status.State.Terminated.Reason
		if current == "" {
			current = "Terminated"
		}
	}
	if status.LastTerminationState.Terminated != nil {
		last = status.LastTerminationState.Terminated.Reason
	} else if status.State.Terminated != nil {
		last = status.State.Terminated.Reason
	}
	return sanitizeText(current, MaximumStatusBytes), sanitizeText(last, MaximumStatusBytes)
}

func SortRestarts(rows []RestartDTO) {
	sort.SliceStable(rows, func(left, right int) bool {
		if rows[left].Restarts != rows[right].Restarts {
			return rows[left].Restarts > rows[right].Restarts
		}
		if rows[left].Namespace != rows[right].Namespace {
			return rows[left].Namespace < rows[right].Namespace
		}
		if rows[left].Pod != rows[right].Pod {
			return rows[left].Pod < rows[right].Pod
		}
		if typeRank(rows[left].ContainerType) != typeRank(rows[right].ContainerType) {
			return typeRank(rows[left].ContainerType) < typeRank(rows[right].ContainerType)
		}
		return rows[left].Container < rows[right].Container
	})
}

func typeRank(value ContainerType) int {
	switch value {
	case ContainerRegular:
		return 0
	case ContainerInit:
		return 1
	case ContainerEphemeral:
		return 2
	default:
		return 3
	}
}

func DirectPodOwner(pod *corev1.Pod) *ResourceRef {
	if pod == nil {
		return nil
	}
	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && *owner.Controller {
			return resourceRef(owner.APIVersion, owner.Kind, pod.Namespace, owner.Name, owner.UID)
		}
	}
	return nil
}

func cloneResourceRef(value *ResourceRef) *ResourceRef {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type problemCandidate struct {
	priority      int
	severity      ProblemSeverity
	container     *string
	containerType *ContainerType
	reason        *string
	message       *string
	source        ProblemSource
	event         *NormalizedEvent
	stateOrder    int
}

func ClassifyProblemPod(pod *corev1.Pod, events []NormalizedEvent, owner *ResourceRef, now time.Time) (ProblemPodDTO, bool) {
	if pod == nil {
		return ProblemPodDTO{}, false
	}
	candidates := make([]problemCandidate, 0)
	status := string(pod.Status.Phase)
	if status == "" {
		status = "Unknown"
	}
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Reason == "Evicted" {
		candidates = append(candidates, problemCandidate{
			priority: 1,
			severity: ProblemCritical,
			reason:   stringPointer(sanitizeText(pod.Status.Reason, MaximumStatusBytes)),
			message:  stringPointer(sanitizeText(pod.Status.Message, MaximumProblemText)),
			source:   ProblemPodStatus,
		})
	}
	appendContainerProblems := func(statuses []corev1.ContainerStatus, containerType ContainerType) {
		for _, container := range statuses {
			name := container.Name
			typeCopy := containerType
			terminated := []*corev1.ContainerStateTerminated{container.State.Terminated, container.LastTerminationState.Terminated}
			for stateOrder, state := range terminated {
				if state == nil || state.Reason != "OOMKilled" {
					continue
				}
				candidates = append(candidates, problemCandidate{
					priority:      2,
					severity:      ProblemCritical,
					container:     stringPointer(name),
					containerType: &typeCopy,
					reason:        stringPointer(sanitizeText(state.Reason, MaximumStatusBytes)),
					message:       stringPointer(sanitizeText(state.Message, MaximumProblemText)),
					source:        ProblemContainerTerminated,
					stateOrder:    stateOrder,
				})
			}
			if waiting := container.State.Waiting; waiting != nil {
				priority, severity, ok := waitingProblem(waiting.Reason)
				if ok {
					candidates = append(candidates, problemCandidate{
						priority:      priority,
						severity:      severity,
						container:     stringPointer(name),
						containerType: &typeCopy,
						reason:        stringPointer(sanitizeText(waiting.Reason, MaximumStatusBytes)),
						message:       stringPointer(sanitizeText(waiting.Message, MaximumProblemText)),
						source:        ProblemContainerWaiting,
					})
				}
			}
			if pod.Status.Phase == corev1.PodRunning && !container.Ready && podAgeAtLeast(pod.CreationTimestamp, now, 2*time.Minute) {
				candidates = append(candidates, problemCandidate{
					priority:      9,
					severity:      ProblemWarning,
					container:     stringPointer(name),
					containerType: &typeCopy,
					reason:        stringPointer("NotReady"),
					source:        ProblemContainerStatus,
				})
			}
		}
	}
	appendContainerProblems(pod.Status.ContainerStatuses, ContainerRegular)
	appendContainerProblems(pod.Status.InitContainerStatuses, ContainerInit)
	appendContainerProblems(pod.Status.EphemeralContainerStatuses, ContainerEphemeral)

	for index := range pod.Status.Conditions {
		condition := pod.Status.Conditions[index]
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Reason == "Unschedulable" {
			candidates = append(candidates, problemCandidate{
				priority: 6,
				severity: ProblemWarning,
				reason:   stringPointer(sanitizeText(condition.Reason, MaximumStatusBytes)),
				message:  stringPointer(sanitizeText(condition.Message, MaximumProblemText)),
				source:   ProblemCondition,
			})
		}
	}

	for _, eventMatch := range matchingProblemEvents(pod, events, now) {
		eventCopy := eventMatch.event
		candidates = append(candidates, problemCandidate{
			priority: eventMatch.priority,
			severity: eventMatch.severity,
			reason:   stringPointer(sanitizeText(eventCopy.Reason, MaximumStatusBytes)),
			message:  stringPointer(sanitizeText(eventCopy.Message, MaximumProblemText)),
			source:   ProblemEvent,
			event:    &eventCopy,
		})
	}

	if pod.Status.Phase == corev1.PodPending && podAgeAtLeast(pod.CreationTimestamp, now, 5*time.Minute) {
		candidates = append(candidates, problemCandidate{
			priority: 10,
			severity: ProblemWarning,
			reason:   stringPointer(sanitizeText(pod.Status.Reason, MaximumStatusBytes)),
			message:  stringPointer(sanitizeText(pod.Status.Message, MaximumProblemText)),
			source:   ProblemPodStatus,
		})
	}
	if len(candidates) == 0 {
		return ProblemPodDTO{}, false
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		return lessProblemCandidate(candidates[left], candidates[right])
	})
	winner := candidates[0]
	return ProblemPodDTO{
		Namespace:     pod.Namespace,
		Pod:           pod.Name,
		Owner:         cloneResourceRef(owner),
		Container:     winner.container,
		ContainerType: winner.containerType,
		Status:        sanitizeText(status, MaximumStatusBytes),
		Reason:        winner.reason,
		Message:       winner.message,
		Source:        winner.source,
		Severity:      winner.severity,
		AgeSeconds:    ageSeconds(pod.CreationTimestamp, now),
	}, true
}

func waitingProblem(reason string) (int, ProblemSeverity, bool) {
	switch reason {
	case "CrashLoopBackOff", "CreateContainerConfigError", "RunContainerError":
		return 3, ProblemCritical, true
	case "ImagePullBackOff", "ErrImagePull":
		return 5, ProblemWarning, true
	default:
		return 0, "", false
	}
}

func podAgeAtLeast(created metav1.Time, now time.Time, minimum time.Duration) bool {
	return !created.IsZero() && !created.Time.After(now) && now.Sub(created.Time) >= minimum
}

type eventProblemMatch struct {
	priority int
	severity ProblemSeverity
	event    NormalizedEvent
}

func matchingProblemEvents(pod *corev1.Pod, events []NormalizedEvent, now time.Time) []eventProblemMatch {
	best := make(map[int]NormalizedEvent)
	for _, event := range events {
		if !strings.EqualFold(event.Type, "Warning") || event.RegardingKind != "Pod" || event.Namespace != pod.Namespace || event.RegardingName != pod.Name || event.RegardingUID == "" || event.RegardingUID != pod.UID {
			continue
		}
		if event.ObservedAt.IsZero() || event.ObservedAt.Before(now.Add(-15*time.Minute)) || event.ObservedAt.After(now.Add(time.Minute)) {
			continue
		}
		priority := 0
		severity := ProblemWarning
		switch {
		case event.Reason == "Unhealthy" && hasCaseInsensitivePrefix(event.Message, "Liveness probe failed"):
			priority, severity = 4, ProblemCritical
		case event.Reason == "FailedScheduling":
			priority, severity = 7, ProblemWarning
		case event.Reason == "Unhealthy" && hasCaseInsensitivePrefix(event.Message, "Readiness probe failed"):
			priority, severity = 8, ProblemWarning
		default:
			continue
		}
		current, exists := best[priority]
		if !exists || newerEvent(event, current) {
			best[priority] = event
		}
		_ = severity
	}
	result := make([]eventProblemMatch, 0, len(best))
	for priority, event := range best {
		severity := ProblemWarning
		if priority == 4 {
			severity = ProblemCritical
		}
		result = append(result, eventProblemMatch{priority: priority, severity: severity, event: event})
	}
	return result
}

func hasCaseInsensitivePrefix(value, prefix string) bool {
	if len(value) < len(prefix) {
		return false
	}
	return strings.EqualFold(value[:len(prefix)], prefix)
}

func newerEvent(left, right NormalizedEvent) bool {
	if !left.ObservedAt.Equal(right.ObservedAt) {
		return left.ObservedAt.After(right.ObservedAt)
	}
	if left.Count != right.Count {
		return left.Count > right.Count
	}
	if left.Reason != right.Reason {
		return left.Reason < right.Reason
	}
	return string(left.UID) < string(right.UID)
}

func lessProblemCandidate(left, right problemCandidate) bool {
	if left.severity != right.severity {
		return left.severity == ProblemCritical
	}
	if left.priority != right.priority {
		return left.priority < right.priority
	}
	if left.containerType != nil || right.containerType != nil {
		leftType, rightType := 99, 99
		if left.containerType != nil {
			leftType = typeRank(*left.containerType)
		}
		if right.containerType != nil {
			rightType = typeRank(*right.containerType)
		}
		if leftType != rightType {
			return leftType < rightType
		}
		leftName, rightName := "", ""
		if left.container != nil {
			leftName = *left.container
		}
		if right.container != nil {
			rightName = *right.container
		}
		if leftName != rightName {
			return leftName < rightName
		}
	}
	if left.event != nil && right.event != nil && newerEvent(*left.event, *right.event) {
		return true
	}
	return left.stateOrder < right.stateOrder
}

func SortProblems(values []ProblemPodDTO) {
	sort.SliceStable(values, func(left, right int) bool {
		if values[left].Severity != values[right].Severity {
			return values[left].Severity == ProblemCritical
		}
		if values[left].Namespace != values[right].Namespace {
			return values[left].Namespace < values[right].Namespace
		}
		if values[left].Pod != values[right].Pod {
			return values[left].Pod < values[right].Pod
		}
		leftContainer, rightContainer := "", ""
		if values[left].Container != nil {
			leftContainer = *values[left].Container
		}
		if values[right].Container != nil {
			rightContainer = *values[right].Container
		}
		return leftContainer < rightContainer
	})
}

func (s *PodService) Restarts(ctx context.Context, selection Selection, limit int) DashboardBlockDTO[[]RestartDTO] {
	if limit == 0 {
		limit = DefaultRestartLimit
	}
	result := blockWithValue(make([]RestartDTO, 0), emptyCoverage(len(canonicalNamespaces(selection.Namespaces))))
	if limit < 1 || limit > MaximumRestartLimit {
		addBlockError(&result, "", validationError("limit must be between 1 and 50"))
		return result
	}
	if s == nil {
		addBlockError(&result, "", NewFeatureUnavailableError())
		return result
	}
	requestContext, cancel := context.WithTimeout(ctx, s.budget.Timeout)
	defer cancel()
	pods := s.loadPods(requestContext, selection)
	copyBlockState(&result, pods)
	now := s.clock.Now()
	for index := range pods.Value {
		owner := s.resolveOwner(requestContext, &pods.Value[index])
		result.Value = append(result.Value, PodRestarts(&pods.Value[index], owner, now)...)
	}
	SortRestarts(result.Value)
	if len(result.Value) > limit {
		result.Value = result.Value[:limit]
		result.Truncated = true
		result.Complete = false
	}
	return result
}

func (s *PodService) Problems(ctx context.Context, selection Selection) DashboardBlockDTO[[]ProblemPodDTO] {
	if s == nil {
		return unavailableProblems()
	}
	requestContext, cancel := context.WithTimeout(ctx, s.budget.Timeout)
	defer cancel()
	pods := s.loadPods(requestContext, selection)
	result := blockWithValue(make([]ProblemPodDTO, 0), pods.Coverage)
	copyBlockState(&result, pods)
	events := s.loadEvents(requestContext, selection)
	mergeBlockState(&result, events.Complete, events.Truncated, events.Errors)
	if result.Coverage != nil && events.Coverage != nil {
		mergeCoverage(result.Coverage, events.Coverage)
	}
	now := s.clock.Now()
	for index := range pods.Value {
		owner := s.resolveOwner(requestContext, &pods.Value[index])
		if problem, ok := ClassifyProblemPod(&pods.Value[index], events.Value, owner, now); ok {
			result.Value = append(result.Value, problem)
		}
	}
	SortProblems(result.Value)
	return result
}

func (s *PodService) Overview(ctx context.Context, selection Selection) DashboardBlockDTO[PodOverview] {
	if s == nil {
		return unavailablePodOverview()
	}
	requestContext, cancel := context.WithTimeout(ctx, s.budget.Timeout)
	defer cancel()
	pods := s.loadPods(requestContext, selection)
	result := blockWithValue(PodOverview{}, pods.Coverage)
	copyBlockState(&result, pods)
	events := s.loadEvents(requestContext, selection)
	mergeBlockState(&result, events.Complete, events.Truncated, events.Errors)
	if result.Coverage != nil && events.Coverage != nil {
		mergeCoverage(result.Coverage, events.Coverage)
	}
	now := s.clock.Now()
	result.Value.Total = int64(len(pods.Value))
	for index := range pods.Value {
		pod := &pods.Value[index]
		for _, status := range pod.Status.ContainerStatuses {
			result.Value.Restarts += int64(maxInt32(status.RestartCount, 0))
		}
		for _, status := range pod.Status.InitContainerStatuses {
			result.Value.Restarts += int64(maxInt32(status.RestartCount, 0))
		}
		for _, status := range pod.Status.EphemeralContainerStatuses {
			result.Value.Restarts += int64(maxInt32(status.RestartCount, 0))
		}
		owner := s.resolveOwner(requestContext, pod)
		if _, problematic := ClassifyProblemPod(pod, events.Value, owner, now); problematic {
			result.Value.Problematic++
		} else if podIsPositivelyHealthy(pod) {
			result.Value.Healthy++
		}
	}
	return result
}

// podIsPositivelyHealthy is intentionally conservative. Absence of a known
// problem is not health evidence: Kubernetes must report a Running Pod, a
// positive Ready condition, and at least one positively running/ready regular
// container. Missing or unknown status therefore contributes to Total only.
func podIsPositivelyHealthy(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	podReady := false
	for index := range pod.Status.Conditions {
		condition := pod.Status.Conditions[index]
		if condition.Type == corev1.PodReady {
			podReady = condition.Status == corev1.ConditionTrue
			break
		}
	}
	if !podReady || len(pod.Status.ContainerStatuses) == 0 {
		return false
	}
	for index := range pod.Status.ContainerStatuses {
		status := pod.Status.ContainerStatuses[index]
		if !status.Ready || status.State.Running == nil {
			return false
		}
	}
	return true
}

func maxInt32(left, right int32) int32 {
	if left > right {
		return left
	}
	return right
}

func (s *PodService) resolveOwner(ctx context.Context, pod *corev1.Pod) *ResourceRef {
	if s != nil && s.owners != nil {
		owner, err := s.owners.ResolvePodOwner(ctx, pod)
		if err == nil {
			return cloneResourceRef(owner)
		}
	}
	return DirectPodOwner(pod)
}

func (s *PodService) loadPods(ctx context.Context, selection Selection) DashboardBlockDTO[[]corev1.Pod] {
	namespaces := canonicalNamespaces(selection.Namespaces)
	block := blockWithValue(make([]corev1.Pod, 0), emptyCoverage(len(namespaces)))
	if s == nil || s.pods == nil {
		addBlockError(&block, "", NewFeatureUnavailableError())
		return block
	}
	requestContext, cancel := context.WithTimeout(ctx, s.budget.Timeout)
	defer cancel()
	for _, namespace := range namespaces {
		remaining := s.budget.MaxItems - len(block.Value)
		if remaining <= 0 {
			block.Complete = false
			block.Truncated = true
			break
		}
		items, complete, truncated, err := s.loadPodNamespace(requestContext, namespace, remaining)
		block.Value = append(block.Value, items...)
		if err != nil {
			addBlockError(&block, namespace, err)
			continue
		}
		if complete {
			block.Coverage.CompletedNamespaces++
		}
		if truncated {
			block.Complete = false
			block.Truncated = true
		}
	}
	sort.Slice(block.Value, func(left, right int) bool {
		if block.Value[left].Namespace != block.Value[right].Namespace {
			return block.Value[left].Namespace < block.Value[right].Namespace
		}
		if block.Value[left].Name != block.Value[right].Name {
			return block.Value[left].Name < block.Value[right].Name
		}
		return string(block.Value[left].UID) < string(block.Value[right].UID)
	})
	return block
}

func (s *PodService) loadPodNamespace(ctx context.Context, namespace string, maximumItems int) ([]corev1.Pod, bool, bool, error) {
	result := make([]corev1.Pod, 0)
	continuation := ""
	for page := 0; page < s.budget.MaxPages; page++ {
		if err := ctx.Err(); err != nil {
			return result, false, false, err
		}
		response, err := s.pods.ListPods(ctx, namespace, PageRequest{Limit: s.budget.PageSize, Continue: continuation})
		if err != nil {
			return result, false, false, err
		}
		remaining := maximumItems - len(result)
		if remaining <= 0 {
			return result, false, true, nil
		}
		if len(response.Items) > remaining {
			result = append(result, response.Items[:remaining]...)
			return result, false, true, nil
		}
		result = append(result, response.Items...)
		continuation = response.Continue
		if continuation == "" {
			return result, true, false, nil
		}
	}
	return result, false, continuation != "", nil
}

func (s *PodService) loadEvents(ctx context.Context, selection Selection) DashboardBlockDTO[[]NormalizedEvent] {
	namespaces := canonicalNamespaces(selection.Namespaces)
	block := blockWithValue(make([]NormalizedEvent, 0), emptyCoverage(len(namespaces)))
	if s == nil || s.events == nil {
		// Events are supplemental: their absence is visible, but Pod status still
		// yields an honest partial result.
		addBlockError(&block, "", NewFeatureUnavailableError())
		return block
	}
	requestContext, cancel := context.WithTimeout(ctx, s.budget.Timeout)
	defer cancel()
	eventService := NewEventService(s.events, s.budget)
	for _, namespace := range namespaces {
		remaining := s.budget.MaxItems - len(block.Value)
		if remaining <= 0 {
			block.Complete = false
			block.Truncated = true
			break
		}
		items, complete, truncated, err := eventService.loadNamespace(requestContext, namespace, remaining)
		block.Value = append(block.Value, items...)
		if err != nil {
			addBlockError(&block, namespace, err)
			continue
		}
		if complete {
			block.Coverage.CompletedNamespaces++
		}
		if truncated {
			block.Complete = false
			block.Truncated = true
		}
	}
	return block
}

func copyBlockState[T, U any](destination *DashboardBlockDTO[T], source DashboardBlockDTO[U]) {
	destination.Complete = source.Complete
	destination.Truncated = source.Truncated
	destination.Coverage = source.Coverage
	destination.Errors = append([]PartialError(nil), source.Errors...)
	if destination.Errors == nil {
		destination.Errors = make([]PartialError, 0)
	}
}

func mergeBlockState[T any](destination *DashboardBlockDTO[T], complete, truncated bool, errors []PartialError) {
	destination.Complete = destination.Complete && complete
	destination.Truncated = destination.Truncated || truncated
	destination.Errors = append(destination.Errors, errors...)
}

func mergeCoverage(destination, source *CoverageDTO) {
	// Both collectors fan out over the same requested namespace set. A
	// namespace is complete only when both sources completed; conservatively
	// use the lower count and merge visible failures.
	if source.RequestedNamespaces > destination.RequestedNamespaces {
		destination.RequestedNamespaces = source.RequestedNamespaces
	}
	if source.CompletedNamespaces < destination.CompletedNamespaces {
		destination.CompletedNamespaces = source.CompletedNamespaces
	}
	for _, namespace := range source.DeniedNamespaces {
		destination.DeniedNamespaces = appendUnique(destination.DeniedNamespaces, namespace)
	}
	destination.Failed = append(destination.Failed, source.Failed...)
}
