package dashboard

import (
	"context"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func ClassifyDeployment(value *appsv1.Deployment, now time.Time) WorkloadDTO {
	if value == nil {
		return unknownWorkload("Deployment")
	}
	desired := int32(1)
	if value.Spec.Replicas != nil {
		desired = maxInt32(*value.Spec.Replicas, 0)
	}
	result := WorkloadDTO{
		Namespace:  value.Namespace,
		Kind:       "Deployment",
		Name:       value.Name,
		Ready:      int32Pointer(maxInt32(value.Status.ReadyReplicas, 0)),
		Desired:    int32Pointer(desired),
		Available:  int32Pointer(maxInt32(value.Status.AvailableReplicas, 0)),
		Updated:    int32Pointer(maxInt32(value.Status.UpdatedReplicas, 0)),
		Status:     WorkloadUnknown,
		AgeSeconds: ageSeconds(value.CreationTimestamp, now),
	}
	if !observedCurrent(value.Generation, value.Status.ObservedGeneration) {
		return result
	}
	deadlineExceeded := false
	for _, condition := range value.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse && condition.Reason == "ProgressDeadlineExceeded" {
			deadlineExceeded = true
			break
		}
	}
	ready := int64(value.Status.ReadyReplicas)
	available := int64(value.Status.AvailableReplicas)
	updated := int64(value.Status.UpdatedReplicas)
	desired64 := int64(desired)
	switch {
	case deadlineExceeded || available < desired64:
		result.Status = WorkloadDegraded
	case updated < desired64 || ready < desired64:
		result.Status = WorkloadProgressing
	case ready == desired64 && available == desired64 && updated == desired64:
		result.Status = WorkloadHealthy
	}
	return result
}

// ClassifyStatefulSetWithAvailability keeps the schema-presence fact that the
// typed Kubernetes object cannot retain after decoding. Adapters backed by
// unstructured/metadata-aware decoding must pass false when the field was not
// returned; such a value can never be classified healthy.
func ClassifyStatefulSetWithAvailability(value *appsv1.StatefulSet, availableCollected bool, now time.Time) WorkloadDTO {
	if value == nil {
		return unknownWorkload("StatefulSet")
	}
	desired := int32(1)
	if value.Spec.Replicas != nil {
		desired = maxInt32(*value.Spec.Replicas, 0)
	}
	result := WorkloadDTO{
		Namespace:  value.Namespace,
		Kind:       "StatefulSet",
		Name:       value.Name,
		Ready:      int32Pointer(maxInt32(value.Status.ReadyReplicas, 0)),
		Desired:    int32Pointer(desired),
		Updated:    int32Pointer(maxInt32(value.Status.UpdatedReplicas, 0)),
		Status:     WorkloadUnknown,
		AgeSeconds: ageSeconds(value.CreationTimestamp, now),
	}
	if availableCollected {
		result.Available = int32Pointer(maxInt32(value.Status.AvailableReplicas, 0))
	}
	if !availableCollected || !observedCurrent(value.Generation, value.Status.ObservedGeneration) {
		return result
	}
	ready := int64(value.Status.ReadyReplicas)
	updated := int64(value.Status.UpdatedReplicas)
	desired64 := int64(desired)
	switch {
	case ready < desired64:
		result.Status = WorkloadDegraded
	case updated < desired64:
		result.Status = WorkloadProgressing
	default:
		result.Status = WorkloadHealthy
	}
	return result
}

// ClassifyStatefulSet is appropriate for a typed client response, where the
// field is part of the negotiated API schema.
func ClassifyStatefulSet(value *appsv1.StatefulSet, now time.Time) WorkloadDTO {
	return ClassifyStatefulSetWithAvailability(value, true, now)
}

func ClassifyDaemonSet(value *appsv1.DaemonSet, now time.Time) WorkloadDTO {
	if value == nil {
		return unknownWorkload("DaemonSet")
	}
	result := WorkloadDTO{
		Namespace:  value.Namespace,
		Kind:       "DaemonSet",
		Name:       value.Name,
		Ready:      int32Pointer(maxInt32(value.Status.NumberReady, 0)),
		Desired:    int32Pointer(maxInt32(value.Status.DesiredNumberScheduled, 0)),
		Available:  int32Pointer(maxInt32(value.Status.NumberAvailable, 0)),
		Updated:    int32Pointer(maxInt32(value.Status.UpdatedNumberScheduled, 0)),
		Status:     WorkloadUnknown,
		AgeSeconds: ageSeconds(value.CreationTimestamp, now),
	}
	if !observedCurrent(value.Generation, value.Status.ObservedGeneration) {
		return result
	}
	desired := int64(value.Status.DesiredNumberScheduled)
	switch {
	case value.Status.NumberUnavailable > 0 || int64(value.Status.NumberReady) < desired:
		result.Status = WorkloadDegraded
	case int64(value.Status.UpdatedNumberScheduled) < desired:
		result.Status = WorkloadProgressing
	default:
		result.Status = WorkloadHealthy
	}
	return result
}

func ClassifyJob(value *batchv1.Job, now time.Time) WorkloadDTO {
	if value == nil {
		return unknownWorkload("Job")
	}
	desired := int32(1)
	if value.Spec.Completions != nil {
		desired = maxInt32(*value.Spec.Completions, 0)
	}
	result := WorkloadDTO{
		Namespace:  value.Namespace,
		Kind:       "Job",
		Name:       value.Name,
		Desired:    int32Pointer(desired),
		Available:  int32Pointer(maxInt32(value.Status.Succeeded, 0)),
		Status:     WorkloadUnknown,
		AgeSeconds: ageSeconds(value.CreationTimestamp, now),
	}
	if value.Status.Ready != nil {
		result.Ready = int32Pointer(maxInt32(*value.Status.Ready, 0))
	}
	failedCondition := jobCondition(value, batchv1.JobFailed)
	completeCondition := jobCondition(value, batchv1.JobComplete)
	switch {
	case failedCondition != nil && failedCondition.Status == corev1.ConditionTrue:
		result.Status = WorkloadFailed
	case value.Status.Failed > 0:
		result.Status = WorkloadFailed
	case value.Spec.Suspend != nil && *value.Spec.Suspend:
		result.Status = WorkloadSuspended
	case value.Status.Active > 0:
		result.Status = WorkloadProgressing
	case completeCondition != nil && completeCondition.Status == corev1.ConditionTrue:
		result.Status = WorkloadCompleted
	case int64(value.Status.Succeeded) >= int64(desired):
		result.Status = WorkloadCompleted
	}
	return result
}

func ClassifyCronJob(value *batchv1.CronJob, jobs []batchv1.Job, now time.Time) WorkloadDTO {
	if value == nil {
		return unknownWorkload("CronJob")
	}
	ready := int64(len(value.Status.Active))
	result := WorkloadDTO{
		Namespace:  value.Namespace,
		Kind:       "CronJob",
		Name:       value.Name,
		Ready:      &ready,
		Status:     WorkloadUnknown,
		AgeSeconds: ageSeconds(value.CreationTimestamp, now),
	}
	latest, timestamp, exists := latestOwnedJob(value, jobs)
	if exists && jobFailed(latest) && !timestamp.IsZero() {
		age := now.Sub(timestamp)
		if age >= 0 && age <= 24*time.Hour {
			result.Status = WorkloadFailed
			return result
		}
	}
	if value.Spec.Suspend != nil && *value.Spec.Suspend {
		result.Status = WorkloadSuspended
		return result
	}
	if len(value.Status.Active) > 0 {
		result.Status = WorkloadProgressing
		return result
	}
	if value.Status.LastScheduleTime != nil && !value.Status.LastScheduleTime.IsZero() {
		result.Status = WorkloadHealthy
	}
	return result
}

func observedCurrent(generation, observed int64) bool {
	return generation > 0 && observed > 0 && observed >= generation
}

func jobCondition(value *batchv1.Job, conditionType batchv1.JobConditionType) *batchv1.JobCondition {
	for index := range value.Status.Conditions {
		if value.Status.Conditions[index].Type == conditionType {
			return &value.Status.Conditions[index]
		}
	}
	return nil
}

func jobFailed(value *batchv1.Job) bool {
	if value == nil {
		return false
	}
	condition := jobCondition(value, batchv1.JobFailed)
	return condition != nil && condition.Status == corev1.ConditionTrue
}

func latestOwnedJob(cronJob *batchv1.CronJob, jobs []batchv1.Job) (*batchv1.Job, time.Time, bool) {
	var winner *batchv1.Job
	var winnerTime time.Time
	for index := range jobs {
		job := &jobs[index]
		if job.Namespace != cronJob.Namespace || !ownedByUID(job.OwnerReferences, cronJob.UID) {
			continue
		}
		timestamp := jobTerminalTime(job)
		if timestamp.IsZero() {
			timestamp = job.CreationTimestamp.Time
		}
		if winner == nil || timestamp.After(winnerTime) || (timestamp.Equal(winnerTime) && job.Name < winner.Name) {
			winner = job
			winnerTime = timestamp
		}
	}
	return winner, winnerTime, winner != nil
}

func ownedByUID(references []metav1.OwnerReference, uid types.UID) bool {
	if uid == "" {
		return false
	}
	for _, reference := range references {
		if reference.UID == uid {
			return true
		}
	}
	return false
}

func jobTerminalTime(value *batchv1.Job) time.Time {
	latest := time.Time{}
	if value.Status.CompletionTime != nil {
		latest = value.Status.CompletionTime.Time
	}
	for index := range value.Status.Conditions {
		condition := value.Status.Conditions[index]
		if condition.Status != corev1.ConditionTrue || (condition.Type != batchv1.JobFailed && condition.Type != batchv1.JobComplete) {
			continue
		}
		if condition.LastTransitionTime.Time.After(latest) {
			latest = condition.LastTransitionTime.Time
		}
	}
	return latest
}

func unknownWorkload(kind string) WorkloadDTO {
	return WorkloadDTO{Kind: kind, Status: WorkloadUnknown}
}

func workloadKindRank(kind string) int {
	switch kind {
	case "Deployment":
		return 0
	case "StatefulSet":
		return 1
	case "DaemonSet":
		return 2
	case "Job":
		return 3
	case "CronJob":
		return 4
	default:
		return 5
	}
}

func SortWorkloads(values []WorkloadDTO) {
	sort.SliceStable(values, func(left, right int) bool {
		if workloadKindRank(values[left].Kind) != workloadKindRank(values[right].Kind) {
			return workloadKindRank(values[left].Kind) < workloadKindRank(values[right].Kind)
		}
		if values[left].Namespace != values[right].Namespace {
			return values[left].Namespace < values[right].Namespace
		}
		return values[left].Name < values[right].Name
	})
}

type WorkloadService struct {
	port   WorkloadPort
	clock  Clock
	budget QueryBudget
}

func NewWorkloadService(port WorkloadPort, clock Clock, budget QueryBudget) *WorkloadService {
	if clock == nil {
		clock = realClock{}
	}
	return &WorkloadService{port: port, clock: clock, budget: budget.Normalized()}
}

func (s *WorkloadService) List(ctx context.Context, selection Selection) DashboardBlockDTO[[]WorkloadDTO] {
	namespaces := canonicalNamespaces(selection.Namespaces)
	block := blockWithValue(make([]WorkloadDTO, 0), emptyCoverage(len(namespaces)))
	if s == nil || s.port == nil {
		addBlockError(&block, "", NewFeatureUnavailableError())
		return block
	}
	requestContext, cancel := context.WithTimeout(ctx, s.budget.Timeout)
	defer cancel()
	now := s.clock.Now()
	collectedItems := 0
	for _, namespace := range namespaces {
		remaining := s.budget.MaxItems - collectedItems
		if remaining <= 0 {
			block.Complete = false
			block.Truncated = true
			break
		}
		pages, complete, truncated, err := s.loadNamespace(requestContext, namespace, remaining)
		if err != nil {
			addBlockError(&block, namespace, err)
		}
		if complete {
			block.Coverage.CompletedNamespaces++
		}
		if truncated {
			block.Complete = false
			block.Truncated = true
		}
		for _, page := range pages {
			collectedItems += workloadPageCount(page)
			for _, issue := range page.Issues {
				if issue.Err != nil {
					addBlockError(&block, namespace, issue.Err)
				}
			}
		}
		allJobs := make([]batchv1.Job, 0)
		for pageIndex := range pages {
			allJobs = append(allJobs, pages[pageIndex].Jobs...)
		}
		for pageIndex := range pages {
			page := &pages[pageIndex]
			for index := range page.Deployments {
				block.Value = append(block.Value, ClassifyDeployment(&page.Deployments[index], now))
			}
			for index := range page.StatefulSets {
				availableCollected := true
				if page.StatefulSetAvailable != nil {
					availableCollected = page.StatefulSetAvailable[page.StatefulSets[index].UID]
				}
				block.Value = append(block.Value, ClassifyStatefulSetWithAvailability(&page.StatefulSets[index], availableCollected, now))
			}
			for index := range page.DaemonSets {
				block.Value = append(block.Value, ClassifyDaemonSet(&page.DaemonSets[index], now))
			}
			for index := range page.Jobs {
				block.Value = append(block.Value, ClassifyJob(&page.Jobs[index], now))
			}
			for index := range page.CronJobs {
				block.Value = append(block.Value, ClassifyCronJob(&page.CronJobs[index], allJobs, now))
			}
		}
	}
	SortWorkloads(block.Value)
	return block
}

func (s *WorkloadService) Degraded(ctx context.Context, selection Selection) DashboardBlockDTO[[]WorkloadDTO] {
	all := s.List(ctx, selection)
	result := blockWithValue(make([]WorkloadDTO, 0), all.Coverage)
	copyBlockState(&result, all)
	for _, workload := range all.Value {
		switch workload.Status {
		case WorkloadDegraded, WorkloadFailed:
			result.Value = append(result.Value, workload)
		case WorkloadUnknown:
			// A zero degraded count is authoritative only when every collected
			// workload has enough status evidence to be classified.
			result.Complete = false
		}
	}
	return result
}

func (s *WorkloadService) DegradedByNamespace(ctx context.Context, selection Selection) DashboardBlockDTO[map[string]int64] {
	if s == nil {
		result := blockWithValue(make(map[string]int64), nil)
		addBlockError(&result, "", NewFeatureUnavailableError())
		return result
	}
	all := s.List(ctx, selection)
	result := blockWithValue(make(map[string]int64), all.Coverage)
	copyBlockState(&result, all)
	for _, workload := range all.Value {
		switch workload.Status {
		case WorkloadDegraded, WorkloadFailed:
			result.Value[workload.Namespace]++
		case WorkloadUnknown:
			result.Complete = false
		}
	}
	return result
}

func (s *WorkloadService) loadNamespace(ctx context.Context, namespace string, maximumItems int) ([]WorkloadPage, bool, bool, error) {
	result := make([]WorkloadPage, 0)
	continuation := ""
	items := 0
	fullyCollected := true
	for page := 0; page < s.budget.MaxPages; page++ {
		if items >= maximumItems {
			return result, false, true, nil
		}
		if err := ctx.Err(); err != nil {
			return result, false, false, err
		}
		response, err := s.port.ListWorkloads(ctx, namespace, PageRequest{Limit: s.budget.PageSize, Continue: continuation})
		if err != nil {
			return result, false, false, err
		}
		if len(response.Issues) > 0 {
			fullyCollected = false
		}
		pageItems := workloadPageCount(response)
		if items+pageItems > maximumItems {
			if len(response.Issues) > 0 {
				result = append(result, WorkloadPage{Issues: response.Issues})
			}
			return result, false, true, nil
		}
		result = append(result, response)
		items += pageItems
		continuation = response.Continue
		if continuation == "" {
			return result, fullyCollected, false, nil
		}
	}
	return result, false, continuation != "", nil
}

func workloadPageCount(page WorkloadPage) int {
	return len(page.Deployments) + len(page.StatefulSets) + len(page.DaemonSets) + len(page.Jobs) + len(page.CronJobs)
}
