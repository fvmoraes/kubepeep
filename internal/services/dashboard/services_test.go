package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

func TestPodServicePartialCoverageDoesNotInventCompleteness(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	port := &fakePodPort{responses: map[string][]PodPage{
		"allowed": {{Items: []corev1.Pod{healthyTestPod(now)}}},
	}, failures: map[string]error{"denied": NewDeniedError()}}
	events := &fakeEventPort{responses: map[string][]EventPage{"allowed": {{}}, "denied": {{}}}}
	service := NewPodService(port, events, nil, fixedClock{now}, QueryBudget{})
	block := service.Overview(context.Background(), Selection{Namespaces: []string{"denied", "allowed", "allowed"}})
	if block.Complete || block.Coverage.RequestedNamespaces != 2 || block.Coverage.CompletedNamespaces != 1 || len(block.Coverage.DeniedNamespaces) != 1 {
		t.Fatalf("unexpected partial coverage: %+v", block)
	}
	if block.Value.Total != 1 || block.Value.Healthy != 1 || block.Value.Problematic != 0 {
		t.Fatalf("partial values were fabricated/lost: %+v", block.Value)
	}
}

func TestPodOverviewDoesNotTurnUnknownStatusIntoHealthy(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	unknown := healthyTestPod(now)
	unknown.Status.Conditions = nil
	port := &fakePodPort{responses: map[string][]PodPage{"payments": {{Items: []corev1.Pod{unknown}}}}}
	events := &fakeEventPort{responses: map[string][]EventPage{"payments": {{}}}}
	block := NewPodService(port, events, nil, fixedClock{now}, QueryBudget{}).Overview(
		context.Background(), Selection{Namespaces: []string{"payments"}},
	)
	if !block.Complete || block.Value.Total != 1 || block.Value.Healthy != 0 || block.Value.Problematic != 0 {
		t.Fatalf("unknown Pod status was fabricated as healthy: %+v", block)
	}
}

func TestPodServiceMarksPendingCursorAndTopLimitTruncated(t *testing.T) {
	t.Parallel()
	now := time.Now()
	pods := make([]corev1.Pod, 11)
	for index := range pods {
		pods[index] = healthyTestPod(now)
		pods[index].Name = string(rune('a' + index))
		pods[index].Status.ContainerStatuses[0].RestartCount = int32(index + 1)
	}
	port := &fakePodPort{responses: map[string][]PodPage{"ns": {{Items: pods, Continue: "more"}}}}
	service := NewPodService(port, &fakeEventPort{}, nil, fixedClock{now}, QueryBudget{MaxPages: 1, MaxItems: 100, PageSize: 100})
	block := service.Restarts(context.Background(), Selection{Namespaces: []string{"ns"}}, 10)
	if block.Complete || !block.Truncated || len(block.Value) != 10 {
		t.Fatalf("cursor/top limit was presented as complete: %+v", block)
	}
}

func TestEventServiceGroupsOnlyWarningsAndSurfacesFailures(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	port := &fakeEventPort{
		responses: map[string][]EventPage{
			"a": {{Items: []NormalizedEvent{{Namespace: "a", Type: "Warning", Reason: "BackOff", Count: 2, ObservedAt: now}}}},
		},
		failures: map[string]error{"b": errors.New("synthetic upstream payload must not escape")},
	}
	block := NewEventService(port, QueryBudget{}).Warnings(context.Background(), Selection{Namespaces: []string{"a", "b"}})
	if block.Complete || len(block.Value) != 1 || block.Coverage.CompletedNamespaces != 1 || len(block.Errors) != 1 {
		t.Fatalf("unexpected Event block: %+v", block)
	}
	if block.Errors[0].Message == "synthetic upstream payload must not escape" {
		t.Fatal("raw upstream error escaped")
	}
}

func TestEventServiceAppliesItemBudgetAcrossWholeFanout(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	port := &fakeEventPort{responses: map[string][]EventPage{
		"a": {
			{Items: []NormalizedEvent{{Namespace: "a", Type: "Warning", UID: "a", Count: 1, ObservedAt: now}}, Continue: "more"},
			{Items: []NormalizedEvent{{Namespace: "a", Type: "Warning", UID: "a-extra", Count: 1, ObservedAt: now}}},
		},
		"b": {{Items: []NormalizedEvent{{Namespace: "b", Type: "Warning", UID: "b", Count: 1, ObservedAt: now}}}},
	}}
	block := NewEventService(port, QueryBudget{MaxItems: 1}).Warnings(context.Background(), Selection{Namespaces: []string{"a", "b"}})
	if block.Complete || !block.Truncated || len(block.Value) != 1 || block.Coverage.CompletedNamespaces != 0 || port.calls["a"] != 1 || port.calls["b"] != 0 {
		t.Fatalf("global fan-out budget not enforced: %+v calls=%+v", block, port.calls)
	}
}

func TestEventServiceAllReturnsNormalWarningAndUnknown(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	port := &fakeEventPort{responses: map[string][]EventPage{
		"a": {{Items: []NormalizedEvent{
			{Namespace: "a", Type: "Normal", UID: "normal", Count: 1, ObservedAt: now},
			{Namespace: "a", Type: "Warning", UID: "warning", Count: 1, ObservedAt: now.Add(-time.Second)},
			{Namespace: "a", Type: "custom", UID: "unknown", Count: 1, ObservedAt: now.Add(-2 * time.Second)},
		}}},
	}}
	block := NewEventService(port, QueryBudget{}).All(context.Background(), Selection{Namespaces: []string{"a"}})
	if !block.Complete || block.Truncated || len(block.Value) != 3 {
		t.Fatalf("unexpected all-events block: %+v", block)
	}
	if block.Value[0].Type != "Normal" || block.Value[1].Type != "Warning" || block.Value[2].Type != "Unknown" {
		t.Fatalf("closed type enum not preserved: %+v", block.Value)
	}
}

func TestSummaryKeepsCounterStatesAndPartialErrorsIndependent(t *testing.T) {
	t.Parallel()
	pods := &stubDashboardPods{overview: blockWithValue(PodOverview{Total: 4, Healthy: 3, Problematic: 1, Restarts: 7}, nil)}
	workloads := &stubDashboardWorkloads{value: blockWithValue([]WorkloadDTO{{Status: WorkloadDegraded}}, nil)}
	eventsBlock := blockWithValue([]EventDTO{}, nil)
	addBlockError(&eventsBlock, "restricted", NewDeniedError())
	events := &stubDashboardEvents{value: eventsBlock}
	service := &DashboardService{Pods: pods, Workloads: workloads, Events: events}
	block := service.Summary(context.Background(), Selection{Namespaces: []string{"b", "a"}}, SummaryOptions{})
	if block.Complete || len(block.Errors) != 1 {
		t.Fatalf("summary failure did not remain partial: %+v", block)
	}
	if block.Value.PodsTotal.State != CounterAvailable || block.Value.PodsTotal.Value == nil || *block.Value.PodsTotal.Value != 4 {
		t.Fatalf("pod counter lost: %+v", block.Value.PodsTotal)
	}
	if block.Value.WarningEvents.State != CounterDenied || block.Value.WarningEvents.Value != nil {
		t.Fatalf("denied was confused with zero: %+v", block.Value.WarningEvents)
	}
	if block.Value.PossibleLogMatches.State != CounterNotCollected || block.Value.PossibleLogMatches.Value != nil {
		t.Fatalf("not-collected state lost: %+v", block.Value.PossibleLogMatches)
	}
}

func TestCounterTruncationRetainsLowerBoundButUnavailableDoesNot(t *testing.T) {
	t.Parallel()
	truncated := counterForBlock(3, false, true, nil)
	if truncated.State != CounterTruncated || truncated.Value == nil || *truncated.Value != 3 {
		t.Fatalf("truncated lower bound lost: %+v", truncated)
	}
	unavailable := counterForBlock(3, false, false, []PartialError{{Code: CodeUpstreamUnavailable}})
	if unavailable.State != CounterUnavailable || unavailable.Value != nil {
		t.Fatalf("partial count presented as authoritative: %+v", unavailable)
	}
}

func TestNamespaceHealthAggregatesPodsAndWorkloadsPerNamespace(t *testing.T) {
	t.Parallel()
	pods := &stubDashboardPods{
		overview: blockWithValue(PodOverview{}, nil),
		namespaceHealth: blockWithValue(map[string]PodNamespaceHealth{
			"payments": {ProblematicPods: 1, ContainerRestarts: 7},
			"other":    {ProblematicPods: 0, ContainerRestarts: 2},
		}, nil),
	}
	workloads := &stubDashboardWorkloads{
		degradedByNamespace: blockWithValue(map[string]int64{
			"payments": 1,
			"other":    0,
		}, nil),
	}
	service := &DashboardService{Pods: pods, Workloads: workloads, Events: &stubDashboardEvents{}}
	block := service.NamespaceHealth(context.Background(), Selection{Namespaces: []string{"other", "payments"}})
	if !block.Complete || len(block.Errors) != 0 {
		t.Fatalf("unexpected block state: %+v", block)
	}
	if len(block.Value) != 2 {
		t.Fatalf("expected 2 namespaces, got %d: %+v", len(block.Value), block.Value)
	}
	var payments, other NamespaceHealthDTO
	for _, row := range block.Value {
		if row.Namespace == "payments" {
			payments = row
		}
		if row.Namespace == "other" {
			other = row
		}
	}
	if payments.Namespace != "payments" || payments.ProblematicPods != 1 || payments.ContainerRestarts != 7 || payments.DegradedWorkloads != 1 {
		t.Fatalf("unexpected payments row: %+v", payments)
	}
	if other.Namespace != "other" || other.ProblematicPods != 0 || other.ContainerRestarts != 2 || other.DegradedWorkloads != 0 {
		t.Fatalf("unexpected other row: %+v", other)
	}
}

func TestNamespaceHealthMergesPartialStateFromBothSources(t *testing.T) {
	t.Parallel()
	pods := &stubDashboardPods{
		overview: blockWithValue(PodOverview{}, nil),
		namespaceHealth: func() DashboardBlockDTO[map[string]PodNamespaceHealth] {
			block := blockWithValue(map[string]PodNamespaceHealth{"a": {}}, nil)
			block.Truncated = true
			block.Complete = false
			addBlockError(&block, "a", NewDeniedError())
			return block
		}(),
	}
	workloads := &stubDashboardWorkloads{
		degradedByNamespace: func() DashboardBlockDTO[map[string]int64] {
			block := blockWithValue(map[string]int64{"a": 0}, nil)
			addBlockError(&block, "b", NewAuthenticationUnavailableError())
			return block
		}(),
	}
	service := &DashboardService{Pods: pods, Workloads: workloads, Events: &stubDashboardEvents{}}
	block := service.NamespaceHealth(context.Background(), Selection{Namespaces: []string{"a", "b"}})
	if block.Complete || !block.Truncated {
		t.Fatalf("merged state lost: complete=%v truncated=%v", block.Complete, block.Truncated)
	}
	if len(block.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d: %+v", len(block.Errors), block.Errors)
	}
}

func TestNamespaceHealthOmitsUnscopedNamespaces(t *testing.T) {
	t.Parallel()
	pods := &stubDashboardPods{
		namespaceHealth: blockWithValue(map[string]PodNamespaceHealth{
			"scoped":   {ProblematicPods: 1},
			"unscoped": {ProblematicPods: 9},
		}, nil),
	}
	workloads := &stubDashboardWorkloads{
		degradedByNamespace: blockWithValue(map[string]int64{"scoped": 0}, nil),
	}
	service := &DashboardService{Pods: pods, Workloads: workloads, Events: &stubDashboardEvents{}}
	block := service.NamespaceHealth(context.Background(), Selection{Namespaces: []string{"scoped"}})
	if len(block.Value) != 1 || block.Value[0].Namespace != "scoped" || block.Value[0].ProblematicPods != 1 {
		t.Fatalf("selection was not respected: %+v", block.Value)
	}
}

type fakePodPort struct {
	responses map[string][]PodPage
	failures  map[string]error
	calls     map[string]int
}

func (port *fakePodPort) ListPods(_ context.Context, namespace string, _ PageRequest) (PodPage, error) {
	if err := port.failures[namespace]; err != nil {
		return PodPage{}, err
	}
	if port.calls == nil {
		port.calls = make(map[string]int)
	}
	index := port.calls[namespace]
	port.calls[namespace]++
	if index >= len(port.responses[namespace]) {
		return PodPage{}, nil
	}
	return port.responses[namespace][index], nil
}

type fakeEventPort struct {
	responses map[string][]EventPage
	failures  map[string]error
	calls     map[string]int
}

func (port *fakeEventPort) ListEvents(_ context.Context, namespace string, _ PageRequest) (EventPage, error) {
	if err := port.failures[namespace]; err != nil {
		return EventPage{}, err
	}
	if port.calls == nil {
		port.calls = make(map[string]int)
	}
	index := port.calls[namespace]
	port.calls[namespace]++
	if index >= len(port.responses[namespace]) {
		return EventPage{}, nil
	}
	return port.responses[namespace][index], nil
}

type stubDashboardPods struct {
	overview        DashboardBlockDTO[PodOverview]
	namespaceHealth DashboardBlockDTO[map[string]PodNamespaceHealth]
}

func (stub *stubDashboardPods) Overview(context.Context, Selection) DashboardBlockDTO[PodOverview] {
	return stub.overview
}
func (*stubDashboardPods) Problems(context.Context, Selection) DashboardBlockDTO[[]ProblemPodDTO] {
	return blockWithValue([]ProblemPodDTO{}, nil)
}
func (*stubDashboardPods) Restarts(context.Context, Selection, int) DashboardBlockDTO[[]RestartDTO] {
	return blockWithValue([]RestartDTO{}, nil)
}
func (stub *stubDashboardPods) NamespaceHealth(context.Context, Selection) DashboardBlockDTO[map[string]PodNamespaceHealth] {
	return stub.namespaceHealth
}

type stubDashboardWorkloads struct {
	value               DashboardBlockDTO[[]WorkloadDTO]
	degradedByNamespace DashboardBlockDTO[map[string]int64]
}

func (stub *stubDashboardWorkloads) Degraded(context.Context, Selection) DashboardBlockDTO[[]WorkloadDTO] {
	return stub.value
}
func (stub *stubDashboardWorkloads) DegradedByNamespace(context.Context, Selection) DashboardBlockDTO[map[string]int64] {
	return stub.degradedByNamespace
}

type stubDashboardEvents struct{ value DashboardBlockDTO[[]EventDTO] }

func (stub *stubDashboardEvents) Warnings(context.Context, Selection) DashboardBlockDTO[[]EventDTO] {
	return stub.value
}

func (stub *stubDashboardEvents) All(context.Context, Selection) DashboardBlockDTO[[]EventDTO] {
	return stub.value
}
