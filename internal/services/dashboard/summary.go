package dashboard

import (
	"context"
	"math"
)

type DashboardPodService interface {
	Overview(context.Context, Selection) DashboardBlockDTO[PodOverview]
	Problems(context.Context, Selection) DashboardBlockDTO[[]ProblemPodDTO]
	Restarts(context.Context, Selection, int) DashboardBlockDTO[[]RestartDTO]
}

type DashboardWorkloadService interface {
	Degraded(context.Context, Selection) DashboardBlockDTO[[]WorkloadDTO]
}

type DashboardEventService interface {
	Warnings(context.Context, Selection) DashboardBlockDTO[[]EventDTO]
}

type DashboardLogService interface {
	Scan(context.Context, LogScanRequest, []LogTarget) DashboardBlockDTO[[]LogMatchDTO]
}

type DashboardMetricsService interface {
	Collect(context.Context, Selection) DashboardBlockDTO[MetricsDTO]
}

// DashboardService only orchestrates the reusable resource services. It does
// not query Kubernetes or retain a previous log scan itself.
type DashboardService struct {
	Pods      DashboardPodService
	Workloads DashboardWorkloadService
	Events    DashboardEventService
	Logs      DashboardLogService
	Metrics   DashboardMetricsService
}

type SummaryOptions struct {
	// PossibleLogMatches is supplied by the caller's current in-memory view.
	// The zero value means that no scan has been executed.
	PossibleLogMatches CounterDTO
}

func (s *DashboardService) Summary(ctx context.Context, selection Selection, options SummaryOptions) DashboardBlockDTO[SummaryDTO] {
	type podResult struct {
		value DashboardBlockDTO[PodOverview]
	}
	type workloadResult struct {
		value DashboardBlockDTO[[]WorkloadDTO]
	}
	type eventResult struct{ value DashboardBlockDTO[[]EventDTO] }
	podsChannel := make(chan podResult, 1)
	workloadsChannel := make(chan workloadResult, 1)
	eventsChannel := make(chan eventResult, 1)
	if s != nil && s.Pods != nil {
		go func() { podsChannel <- podResult{s.Pods.Overview(ctx, selection)} }()
	} else {
		podsChannel <- podResult{unavailablePodOverview()}
	}
	if s != nil && s.Workloads != nil {
		go func() { workloadsChannel <- workloadResult{s.Workloads.Degraded(ctx, selection)} }()
	} else {
		workloadsChannel <- workloadResult{unavailableWorkloads()}
	}
	if s != nil && s.Events != nil {
		go func() { eventsChannel <- eventResult{s.Events.Warnings(ctx, selection)} }()
	} else {
		eventsChannel <- eventResult{unavailableEvents()}
	}
	pods := (<-podsChannel).value
	workloads := (<-workloadsChannel).value
	events := (<-eventsChannel).value
	logCounter := normalizeLogCounter(options.PossibleLogMatches)
	value := SummaryDTO{
		Namespaces:         AvailableCounter(int64(len(canonicalNamespaces(selection.Namespaces)))),
		PodsTotal:          counterForBlock(pods.Value.Total, pods.Complete, pods.Truncated, pods.Errors),
		PodsHealthy:        counterForBlock(pods.Value.Healthy, pods.Complete, pods.Truncated, pods.Errors),
		PodsProblematic:    counterForBlock(pods.Value.Problematic, pods.Complete, pods.Truncated, pods.Errors),
		WorkloadsDegraded:  counterForBlock(int64(len(workloads.Value)), workloads.Complete, workloads.Truncated, workloads.Errors),
		Restarts:           counterForBlock(pods.Value.Restarts, pods.Complete, pods.Truncated, pods.Errors),
		WarningEvents:      counterForBlock(eventOccurrenceCount(events.Value), events.Complete, events.Truncated, events.Errors),
		PossibleLogMatches: logCounter,
	}
	block := blockWithValue(value, nil)
	block.Complete = pods.Complete && workloads.Complete && events.Complete && logCounter.State != CounterCollecting && logCounter.State != CounterTruncated
	block.Truncated = pods.Truncated || workloads.Truncated || events.Truncated || logCounter.State == CounterTruncated
	block.Errors = append(block.Errors, pods.Errors...)
	block.Errors = append(block.Errors, workloads.Errors...)
	block.Errors = append(block.Errors, events.Errors...)
	return block
}

func (s *DashboardService) Problems(ctx context.Context, selection Selection) DashboardBlockDTO[[]ProblemPodDTO] {
	if s == nil || s.Pods == nil {
		return unavailableProblems()
	}
	return s.Pods.Problems(ctx, selection)
}

func (s *DashboardService) Restarts(ctx context.Context, selection Selection, limit int) DashboardBlockDTO[[]RestartDTO] {
	if s == nil || s.Pods == nil {
		result := blockWithValue([]RestartDTO{}, emptyCoverage(len(canonicalNamespaces(selection.Namespaces))))
		addBlockError(&result, "", NewFeatureUnavailableError())
		return result
	}
	return s.Pods.Restarts(ctx, selection, limit)
}

func (s *DashboardService) Warnings(ctx context.Context, selection Selection) DashboardBlockDTO[[]EventDTO] {
	if s == nil || s.Events == nil {
		return unavailableEvents()
	}
	return s.Events.Warnings(ctx, selection)
}

func (s *DashboardService) ScanLogs(ctx context.Context, request LogScanRequest, targets []LogTarget) DashboardBlockDTO[[]LogMatchDTO] {
	if s == nil || s.Logs == nil {
		result := blockWithValue([]LogMatchDTO{}, emptyCoverage(len(targetNamespaces(targets))))
		addBlockError(&result, "", NewFeatureUnavailableError())
		return result
	}
	return s.Logs.Scan(ctx, request, targets)
}

func (s *DashboardService) PodMetrics(ctx context.Context, selection Selection) DashboardBlockDTO[MetricsDTO] {
	if s == nil || s.Metrics == nil {
		result := blockWithValue(MetricsDTO{Pods: []PodMetricDTO{}, TopCPU: []MetricRankDTO{}, TopMemory: []MetricRankDTO{}}, emptyCoverage(len(canonicalNamespaces(selection.Namespaces))))
		addBlockError(&result, "", NewFeatureUnavailableError())
		return result
	}
	return s.Metrics.Collect(ctx, selection)
}

func counterForBlock(value int64, complete, truncated bool, errors []PartialError) CounterDTO {
	if truncated {
		return TruncatedCounter(value)
	}
	if complete {
		return AvailableCounter(value)
	}
	if onlyDenied(errors) {
		return EmptyCounter(CounterDenied)
	}
	return EmptyCounter(CounterUnavailable)
}

func onlyDenied(errors []PartialError) bool {
	if len(errors) == 0 {
		return false
	}
	for _, item := range errors {
		if item.Code != CodeForbidden {
			return false
		}
	}
	return true
}

func normalizeLogCounter(value CounterDTO) CounterDTO {
	switch value.State {
	case CounterAvailable, CounterTruncated:
		if value.Value == nil || *value.Value < 0 {
			return EmptyCounter(CounterUnavailable)
		}
		copy := *value.Value
		return CounterDTO{State: value.State, Value: &copy}
	case CounterDenied, CounterUnavailable, CounterNotCollected, CounterCollecting:
		return CounterDTO{State: value.State}
	default:
		return EmptyCounter(CounterNotCollected)
	}
}

func eventOccurrenceCount(events []EventDTO) int64 {
	var result int64
	for _, event := range events {
		if event.Count <= 0 {
			continue
		}
		if result > math.MaxInt64-event.Count {
			return math.MaxInt64
		}
		result += event.Count
	}
	return result
}

func unavailablePodOverview() DashboardBlockDTO[PodOverview] {
	result := blockWithValue(PodOverview{}, nil)
	addBlockError(&result, "", NewFeatureUnavailableError())
	return result
}

func unavailableWorkloads() DashboardBlockDTO[[]WorkloadDTO] {
	result := blockWithValue([]WorkloadDTO{}, nil)
	addBlockError(&result, "", NewFeatureUnavailableError())
	return result
}

func unavailableEvents() DashboardBlockDTO[[]EventDTO] {
	result := blockWithValue([]EventDTO{}, nil)
	addBlockError(&result, "", NewFeatureUnavailableError())
	return result
}

func unavailableProblems() DashboardBlockDTO[[]ProblemPodDTO] {
	result := blockWithValue([]ProblemPodDTO{}, nil)
	addBlockError(&result, "", NewFeatureUnavailableError())
	return result
}
