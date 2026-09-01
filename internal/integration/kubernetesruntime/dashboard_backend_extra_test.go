package kubernetesruntime

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type logDenyingAuthorization struct {
	mu              sync.Mutex
	denySubresource string
}

func (stub *logDenyingAuthorization) capability(key authorization.Key) authorization.Capability {
	decision := authorization.DecisionAllowed
	reason := authorization.ReasonSARAllowed
	if key.Subresource == stub.denySubresource {
		decision = authorization.DecisionDenied
		reason = authorization.ReasonSARDenied
	}
	return authorization.Capability{
		Namespace: key.Namespace, APIGroup: key.APIGroup,
		Resource: key.Resource, Subresource: key.Subresource, Verb: key.Verb, ResourceName: key.ResourceName,
		Decision: decision, ReasonCode: reason, ExpiresAt: time.Now().Add(time.Minute),
	}
}

func (stub *logDenyingAuthorization) Check(_ context.Context, key authorization.Key) authorization.Capability {
	return stub.capability(key)
}

func (stub *logDenyingAuthorization) Refresh(ctx context.Context, key authorization.Key) authorization.Capability {
	return stub.Check(ctx, key)
}

func (stub *logDenyingAuthorization) Revalidate(ctx context.Context, key authorization.Key, _ authorization.OperationKind) (authorization.Capability, error) {
	capability := stub.Check(ctx, key)
	if capability.Decision == authorization.DecisionDenied {
		return capability, &authorization.PublicError{Code: authorization.CodeForbidden, Message: "Kubernetes denied this operation.", HTTPStatus: http.StatusForbidden}
	}
	return capability, nil
}

func (stub *logDenyingAuthorization) Guard(ctx context.Context, key authorization.Key, kind authorization.OperationKind, operation authorization.Operation) (authorization.GuardResult, error) {
	capability, err := stub.Revalidate(ctx, key, kind)
	result := authorization.GuardResult{Capability: capability}
	if err != nil {
		return result, err
	}
	result.Executed = true
	return result, operation(ctx)
}

func (*logDenyingAuthorization) InvalidateGeneration(string) {}
func (*logDenyingAuthorization) InvalidateAll()              {}

type scriptedPodPort struct {
	pages    []dashboard.PodPage
	err      error
	pageSize []int
}

func (port *scriptedPodPort) ListPods(_ context.Context, namespace string, page dashboard.PageRequest) (dashboard.PodPage, error) {
	index := len(port.pageSize)
	port.pageSize = append(port.pageSize, page.Limit)
	if port.err != nil {
		return dashboard.PodPage{}, port.err
	}
	if index >= len(port.pages) {
		return dashboard.PodPage{}, nil
	}
	result := dashboard.PodPage{Items: []corev1.Pod{}, Continue: port.pages[index].Continue}
	result.Items = append(result.Items, port.pages[index].Items...)
	return result, nil
}

type scriptedEventPort struct {
	err      error
	pages    []dashboard.EventPage
	pageSize []int
}

func (port *scriptedEventPort) ListEvents(_ context.Context, namespace string, page dashboard.PageRequest) (dashboard.EventPage, error) {
	index := len(port.pageSize)
	port.pageSize = append(port.pageSize, page.Limit)
	if port.err != nil {
		return dashboard.EventPage{}, port.err
	}
	if index >= len(port.pages) {
		return dashboard.EventPage{}, nil
	}
	result := dashboard.EventPage{Items: []dashboard.NormalizedEvent{}, Continue: port.pages[index].Continue}
	result.Items = append(result.Items, port.pages[index].Items...)
	return result, nil
}

func TestNewDashboardBackendValidatesDependencies(t *testing.T) {
	t.Parallel()
	if _, err := NewDashboardBackend(nil, nil, dashboard.DefaultQueryBudget()); err == nil {
		t.Fatal("nil runtime and authorizer were accepted")
	}
	if _, err := NewDashboardBackend(nil, &dashboardAuthorizationStub{}, dashboard.DefaultQueryBudget()); err == nil {
		t.Fatal("nil runtime was accepted")
	}
	if _, err := NewDashboardBackend(&Runtime{}, nil, dashboard.DefaultQueryBudget()); err == nil {
		t.Fatal("nil authorizer was accepted")
	}
	backend, err := NewDashboardBackend(&Runtime{}, &dashboardAuthorizationStub{}, dashboard.QueryBudget{Timeout: -1, PageSize: 0, MaxItems: 0, MaxPages: 0})
	if err != nil || backend == nil {
		t.Fatalf("valid construction failed: %v", err)
	}
	if backend.queryBudget.PageSize != dashboard.DefaultPageSize || backend.queryBudget.MaxPages != dashboard.DefaultMaxPages {
		t.Fatalf("budget was not normalized: %+v", backend.queryBudget)
	}
}

func TestNilDashboardBackendGenerationChangeIsSafe(t *testing.T) {
	t.Parallel()
	var backend *DashboardBackend
	backend.OnGeneration("gen")
}

func TestDashboardBackendBlockEndpointsReturnCompleteCollections(t *testing.T) {
	t.Parallel()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api"}}
	event := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "warning"}, Type: "Warning", Reason: "Unhealthy"}
	provider, _, _ := dashboardTestClients(t, pod, event)
	backend := newDashboardBackend(provider, &dashboardAuthorizationStub{}, dashboard.DefaultQueryBudget())
	binding := dashboardTestBinding()
	resolution := namespaces.ScopeResolution{ScopeName: "payments", Namespaces: []string{"payments"}}
	ctx := context.Background()

	health := backend.NamespaceHealth(ctx, binding, resolution)
	if !health.Complete || health.Coverage == nil || health.Coverage.RequestedNamespaces != 1 {
		t.Fatalf("namespace health block = %+v", health)
	}
	problems := backend.Problems(ctx, binding, resolution)
	if !problems.Complete || len(problems.Value) != 0 {
		t.Fatalf("problems block = %+v", problems)
	}
	restarts := backend.Restarts(ctx, binding, resolution, 5)
	if !restarts.Complete || len(restarts.Value) != 0 {
		t.Fatalf("restarts block = %+v", restarts)
	}
	events := backend.Events(ctx, binding, resolution)
	if !events.Complete || len(events.Value) != 1 || events.Value[0].Reason != "Unhealthy" {
		t.Fatalf("events block = %+v", events)
	}
	metrics := backend.Metrics(ctx, binding, resolution)
	if !metrics.Complete || len(metrics.Value.Pods) != 1 || metrics.Value.WindowSeconds <= 0 {
		t.Fatalf("metrics block = %+v", metrics)
	}
}

func TestDashboardBackendScanLogsValidatesRequestAndHandlesEmptyScope(t *testing.T) {
	t.Parallel()
	provider, _, _ := dashboardTestClients(t)
	backend := newDashboardBackend(provider, &dashboardAuthorizationStub{}, dashboard.DefaultQueryBudget())
	binding := dashboardTestBinding()
	resolution := namespaces.ScopeResolution{ScopeName: "payments", Namespaces: []string{"payments"}}

	block := backend.ScanLogs(context.Background(), binding, resolution, dashboard.LogScanRequest{Window: stringPointer("bogus")})
	if block.Complete || len(block.Value) != 0 || len(block.Errors) != 1 {
		t.Fatalf("invalid scan request block = %+v", block)
	}
	if block.Coverage == nil || len(block.Coverage.Failed) != 1 {
		t.Fatalf("validation coverage = %+v", block.Coverage)
	}
	if counter := backend.currentLogCounter(binding.Generation); counter.State != dashboard.CounterNotCollected {
		t.Fatalf("counter after validation failure = %+v", counter)
	}

	empty := backend.ScanLogs(context.Background(), binding, resolution, dashboard.LogScanRequest{})
	if !empty.Complete || len(empty.Value) != 0 || empty.Truncated {
		t.Fatalf("empty scan block = %+v", empty)
	}
	counter := backend.currentLogCounter(binding.Generation)
	if counter.State != dashboard.CounterAvailable || counter.Value == nil || *counter.Value != 0 {
		t.Fatalf("empty scan counter = %+v", counter)
	}
}

func TestDashboardBackendScanLogsDeniedLogAuthorizationProducesDeniedCounter(t *testing.T) {
	t.Parallel()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api"},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name: "api", RestartCount: 3,
		}}},
	}
	provider, _, _ := dashboardTestClients(t, pod)
	backend := newDashboardBackend(provider, &logDenyingAuthorization{denySubresource: "log"}, dashboard.DefaultQueryBudget())
	binding := dashboardTestBinding()
	resolution := namespaces.ScopeResolution{ScopeName: "payments", Namespaces: []string{"payments"}}

	block := backend.ScanLogs(context.Background(), binding, resolution, dashboard.LogScanRequest{})
	if block.Complete || len(block.Value) != 0 || len(block.Errors) != 1 {
		t.Fatalf("denied scan block = %+v", block)
	}
	if block.Errors[0].Code != dashboard.CodeForbidden {
		t.Fatalf("denied scan error code = %q", block.Errors[0].Code)
	}
	if counter := backend.currentLogCounter(binding.Generation); counter.State != dashboard.CounterDenied {
		t.Fatalf("denied scan counter = %+v", counter)
	}
}

func TestDashboardBackendScanLogsRecordsDeniedPodListings(t *testing.T) {
	t.Parallel()
	provider, _, _ := dashboardTestClients(t)
	backend := newDashboardBackend(provider, &dashboardAuthorizationStub{denyResource: "pods"}, dashboard.DefaultQueryBudget())
	binding := dashboardTestBinding()
	resolution := namespaces.ScopeResolution{ScopeName: "payments", Namespaces: []string{"payments"}}

	block := backend.ScanLogs(context.Background(), binding, resolution, dashboard.LogScanRequest{})
	if block.Complete || len(block.Value) != 0 || len(block.Errors) != 1 || block.Errors[0].Code != dashboard.CodeForbidden {
		t.Fatalf("denied pod list scan block = %+v", block)
	}
	if block.Coverage == nil || len(block.Coverage.DeniedNamespaces) != 1 || block.Coverage.DeniedNamespaces[0] != "payments" {
		t.Fatalf("denied namespaces coverage = %+v", block.Coverage)
	}
}

func TestCollectPodsAppliesBudgetsCanonicalOrderAndErrors(t *testing.T) {
	t.Parallel()
	podA := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "beta", Name: "worker"}}
	podB := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "alpha", Name: "api"}}
	provider, _, _ := dashboardTestClients(t, podA, podB)
	binding := dashboardTestBinding()

	backend := newDashboardBackend(provider, &dashboardAuthorizationStub{}, dashboard.DefaultQueryBudget())
	_, adapter := backend.service(binding)
	block := backend.collectPods(context.Background(), adapter, []string{"beta", "alpha", "beta", "INVALID_NAME"})
	if !block.Complete || len(block.Value) != 2 {
		t.Fatalf("complete collection block = %+v", block)
	}
	if block.Value[0].Namespace != "alpha" || block.Value[1].Namespace != "beta" {
		t.Fatalf("canonical order was not applied: %#v", block.Value)
	}
	if block.Coverage.CompletedNamespaces != 2 || block.Coverage.RequestedNamespaces != 2 {
		t.Fatalf("coverage = %+v", block.Coverage)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledBlock := backend.collectPods(canceled, adapter, []string{"alpha"})
	if canceledBlock.Complete || len(canceledBlock.Errors) != 1 || canceledBlock.Errors[0].Code != dashboard.CodeClientCanceled {
		t.Fatalf("canceled collection block = %+v", canceledBlock)
	}

	deniedBackend := newDashboardBackend(provider, &dashboardAuthorizationStub{denyResource: "pods"}, dashboard.DefaultQueryBudget())
	_, deniedAdapter := deniedBackend.service(binding)
	deniedBlock := deniedBackend.collectPods(context.Background(), deniedAdapter, []string{"alpha"})
	if deniedBlock.Complete || len(deniedBlock.Errors) != 1 || deniedBlock.Errors[0].Code != dashboard.CodeForbidden {
		t.Fatalf("denied collection block = %+v", deniedBlock)
	}
	if len(deniedBlock.Coverage.DeniedNamespaces) != 1 {
		t.Fatalf("denied namespaces = %+v", deniedBlock.Coverage)
	}

	scripted := &scriptedPodPort{pages: []dashboard.PodPage{{Items: []corev1.Pod{*podA, *podB}}}}
	itemBackend := newDashboardBackend(provider, &dashboardAuthorizationStub{}, dashboard.QueryBudget{MaxItems: 1, MaxPages: 5, PageSize: 100})
	itemBlock := itemBackend.collectPods(context.Background(), scripted, []string{"alpha"})
	if itemBlock.Complete || !itemBlock.Truncated || len(itemBlock.Value) != 1 {
		t.Fatalf("max items block = %+v", itemBlock)
	}

	paging := &scriptedPodPort{pages: []dashboard.PodPage{{Items: []corev1.Pod{*podA}, Continue: "next"}, {Items: []corev1.Pod{*podB}}}}
	pageBackend := newDashboardBackend(provider, &dashboardAuthorizationStub{}, dashboard.DefaultQueryBudget())
	pageBlock := pageBackend.collectPods(context.Background(), paging, []string{"alpha"})
	if !pageBlock.Complete || len(pageBlock.Value) != 2 || pageBlock.Coverage.CompletedNamespaces != 1 {
		t.Fatalf("paged collection block = %+v", pageBlock)
	}

	exhausted := &scriptedPodPort{pages: []dashboard.PodPage{{Items: []corev1.Pod{*podA}, Continue: "next-1"}, {Items: []corev1.Pod{*podB}, Continue: "next-2"}}}
	tightBackend := newDashboardBackend(provider, &dashboardAuthorizationStub{}, dashboard.QueryBudget{MaxPages: 2, PageSize: 100})
	exhaustedBlock := tightBackend.collectPods(context.Background(), exhausted, []string{"alpha"})
	if exhaustedBlock.Complete || !exhaustedBlock.Truncated || len(exhaustedBlock.Value) != 2 {
		t.Fatalf("page budget block = %+v", exhaustedBlock)
	}
	if len(exhausted.pageSize) != 2 || exhausted.pageSize[0] != 100 {
		t.Fatalf("page size was not propagated: %v", exhausted.pageSize)
	}

	failing := &scriptedPodPort{err: errors.New("upstream failure")}
	failureBlock := pageBackend.collectPods(context.Background(), failing, []string{"alpha"})
	if failureBlock.Complete || len(failureBlock.Errors) != 1 || failureBlock.Errors[0].Code != dashboard.CodeClusterUnavailable {
		t.Fatalf("failure block = %+v", failureBlock)
	}
}

func TestCollectEventsRecordsErrorsCompletionAndCanonicalization(t *testing.T) {
	t.Parallel()
	provider, _, _ := dashboardTestClients(t)
	binding := dashboardTestBinding()

	backend := newDashboardBackend(provider, &dashboardAuthorizationStub{}, dashboard.DefaultQueryBudget())
	_, adapter := backend.service(binding)
	block := backend.collectEvents(context.Background(), adapter, []string{"alpha", "alpha", "invalid!"})
	if !block.Complete || len(block.Value) != 0 || block.Coverage.CompletedNamespaces != 1 {
		t.Fatalf("empty collection block = %+v", block)
	}

	deniedBackend := newDashboardBackend(provider, &dashboardAuthorizationStub{denyResource: "events"}, dashboard.DefaultQueryBudget())
	_, deniedAdapter := deniedBackend.service(binding)
	deniedBlock := deniedBackend.collectEvents(context.Background(), deniedAdapter, []string{"alpha"})
	if deniedBlock.Complete || len(deniedBlock.Errors) != 1 || deniedBlock.Errors[0].Code != dashboard.CodeForbidden {
		t.Fatalf("denied collection block = %+v", deniedBlock)
	}
	if len(deniedBlock.Coverage.DeniedNamespaces) != 1 || deniedBlock.Coverage.DeniedNamespaces[0] != "alpha" {
		t.Fatalf("denied namespaces = %+v", deniedBlock.Coverage)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledBlock := backend.collectEvents(canceled, deniedAdapter, []string{"alpha"})
	if canceledBlock.Complete || canceledBlock.Errors[0].Code != dashboard.CodeClientCanceled {
		t.Fatalf("canceled collection block = %+v", canceledBlock)
	}

	scripted := &scriptedEventPort{pages: []dashboard.EventPage{{Continue: "next"}, {Continue: "next"}}}
	tightBackend := newDashboardBackend(provider, &dashboardAuthorizationStub{}, dashboard.QueryBudget{MaxPages: 2, PageSize: 100})
	exhaustedBlock := tightBackend.collectEvents(context.Background(), scripted, []string{"alpha"})
	if exhaustedBlock.Complete || !exhaustedBlock.Truncated {
		t.Fatalf("page budget block = %+v", exhaustedBlock)
	}

	failing := &scriptedEventPort{err: context.DeadlineExceeded}
	failureBlock := tightBackend.collectEvents(context.Background(), failing, []string{"alpha"})
	if failureBlock.Complete || failureBlock.Errors[0].Code != dashboard.CodeUpstreamTimeout {
		t.Fatalf("timeout block = %+v", failureBlock)
	}
}

func TestDashboardPartialErrorClassification(t *testing.T) {
	t.Parallel()
	type safeError interface {
		Code() string
		PublicMessage() string
		Denied() bool
	}
	partial, denied := dashboardPartialError("ns", dashboard.NewDeniedError())
	if !denied || partial.Code != dashboard.CodeForbidden || partial.Namespace != "ns" {
		t.Fatalf("denied partial = %+v denied=%v", partial, denied)
	}
	partial, denied = dashboardPartialError("ns", context.DeadlineExceeded)
	if denied || partial.Code != dashboard.CodeUpstreamTimeout {
		t.Fatalf("timeout partial = %+v", partial)
	}
	partial, denied = dashboardPartialError("ns", context.Canceled)
	if denied || partial.Code != dashboard.CodeClientCanceled {
		t.Fatalf("canceled partial = %+v", partial)
	}
	partial, denied = dashboardPartialError("ns", errors.New("boom"))
	if denied || partial.Code != dashboard.CodeClusterUnavailable || !strings.Contains(partial.Message, "temporarily") {
		t.Fatalf("unknown partial = %+v", partial)
	}
	var safe safeError = nil
	_ = safe
}

func TestAddCollectionErrorDeduplicatesSameNamespaceAndCode(t *testing.T) {
	t.Parallel()
	block := newCollectionBlock([]corev1.Pod{}, 1)
	addCollectionError(&block, "alpha", dashboard.NewDeniedError())
	addCollectionError(&block, "alpha", dashboard.NewDeniedError())
	if block.Complete || len(block.Errors) != 1 || len(block.Coverage.DeniedNamespaces) != 1 {
		t.Fatalf("dedup block = %+v", block)
	}
	addCollectionError(&block, "beta", dashboard.NewDeniedError())
	if len(block.Errors) != 2 || len(block.Coverage.DeniedNamespaces) != 2 {
		t.Fatalf("second namespace block = %+v", block)
	}
	addCollectionError(&block, "beta", errors.New("boom"))
	if len(block.Errors) != 3 || len(block.Coverage.Failed) != 1 {
		t.Fatalf("failed namespace block = %+v", block)
	}
}

func TestMergeScanInputsCombinesCoverageAndErrors(t *testing.T) {
	t.Parallel()
	pods := newCollectionBlock([]corev1.Pod{}, 2)
	pods.Complete = false
	pods.Errors = []dashboard.PartialError{{Namespace: "alpha", Code: dashboard.CodeForbidden}}
	pods.Coverage.CompletedNamespaces = 1
	pods.Coverage.DeniedNamespaces = []string{"alpha"}
	events := newCollectionBlock([]dashboard.NormalizedEvent{}, 2)
	events.Truncated = true
	events.Errors = []dashboard.PartialError{{Namespace: "beta", Code: dashboard.CodeUpstreamTimeout}}
	events.Coverage.CompletedNamespaces = 0
	events.Coverage.Failed = []dashboard.PartialError{{Namespace: "beta", Code: dashboard.CodeUpstreamTimeout}}

	result := dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]{
		Value: []dashboard.LogMatchDTO{}, Complete: true,
		Coverage: &dashboard.CoverageDTO{RequestedNamespaces: 2, CompletedNamespaces: 1, DeniedNamespaces: []string{"alpha"}},
	}
	mergeScanInputs(&result, pods, events, 2)
	if result.Complete || !result.Truncated || len(result.Errors) != 2 {
		t.Fatalf("merged block = %+v", result)
	}
	if result.Coverage.CompletedNamespaces != 0 || len(result.Coverage.DeniedNamespaces) != 1 || len(result.Coverage.Failed) != 1 {
		t.Fatalf("merged coverage = %+v", result.Coverage)
	}

	nilCoverage := dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]{Value: []dashboard.LogMatchDTO{}, Complete: true}
	completePods := newCollectionBlock([]corev1.Pod{}, 1)
	completeEvents := newCollectionBlock([]dashboard.NormalizedEvent{}, 1)
	completePods.Coverage.CompletedNamespaces = 1
	completeEvents.Coverage.CompletedNamespaces = 1
	mergeScanInputs(&nilCoverage, completePods, completeEvents, 1)
	if !nilCoverage.Complete || nilCoverage.Coverage.CompletedNamespaces != 1 {
		t.Fatalf("nil coverage merge = %+v", nilCoverage)
	}
}

func TestFinishScanIgnoresStaleSequencesAndHandlesDeadlines(t *testing.T) {
	t.Parallel()
	backend := newDashboardBackend(&fixedDashboardClients{}, &dashboardAuthorizationStub{}, dashboard.DefaultQueryBudget())
	result := dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]{Value: []dashboard.LogMatchDTO{{Pod: "api"}}, Complete: true}

	staleResult := result
	backend.finishScan(999, "gen", context.Background(), staleResult)
	if counter := backend.currentLogCounter("gen"); counter.State != dashboard.CounterNotCollected {
		t.Fatalf("stale sequence changed the counter: %+v", counter)
	}

	staleGeneration := result
	backend.finishScan(1, "other-generation", context.Background(), staleGeneration)
	if counter := backend.currentLogCounter("gen"); counter.State != dashboard.CounterNotCollected {
		t.Fatalf("stale generation changed the counter: %+v", counter)
	}

	ctx, cancel, sequence := backend.beginScan(context.Background(), "gen")
	defer cancel()
	backend.finishScan(sequence, "gen", ctx, result)
	counter := backend.currentLogCounter("gen")
	if counter.State != dashboard.CounterAvailable || counter.Value == nil || *counter.Value != 1 {
		t.Fatalf("complete counter = %+v", counter)
	}

	timeoutContext, timeoutCancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer timeoutCancel()
	<-timeoutContext.Done()
	_, secondCancel, secondSequence := backend.beginScan(context.Background(), "gen")
	defer secondCancel()
	backend.finishScan(secondSequence, "gen", timeoutContext, result)
	if counter := backend.currentLogCounter("gen"); counter.State != dashboard.CounterUnavailable {
		t.Fatalf("deadline counter = %+v", counter)
	}

	_, thirdCancel, thirdSequence := backend.beginScan(context.Background(), "gen")
	defer thirdCancel()
	truncated := dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]{Value: []dashboard.LogMatchDTO{{Pod: "api"}}, Truncated: true}
	backend.finishScan(thirdSequence, "gen", context.Background(), truncated)
	if counter := backend.currentLogCounter("gen"); counter.State != dashboard.CounterTruncated {
		t.Fatalf("truncated counter = %+v", counter)
	}
}

func TestDashboardScanHelperFunctions(t *testing.T) {
	t.Parallel()
	if got := appendUniqueString([]string{"a"}, "a"); len(got) != 1 {
		t.Fatalf("appendUniqueString duplicated: %v", got)
	}
	values := []string{"a"}
	values = appendUniqueString(values, "b")
	if len(values) != 2 {
		t.Fatalf("appendUniqueString dropped: %v", values)
	}

	partials := []dashboard.PartialError{{Namespace: "a", Code: dashboard.CodeForbidden}}
	appendUniquePartial(&partials, dashboard.PartialError{Namespace: "a", Code: dashboard.CodeForbidden})
	if len(partials) != 1 {
		t.Fatalf("appendUniquePartial duplicated: %v", partials)
	}
	appendUniquePartial(&partials, dashboard.PartialError{Namespace: "a", Code: dashboard.CodeUpstreamTimeout})
	if len(partials) != 2 {
		t.Fatalf("appendUniquePartial dropped: %v", partials)
	}

	if minIntValue(1, 2) != 1 || minIntValue(3, 2) != 2 || maxIntValue(1, 2) != 2 || maxIntValue(3, 2) != 3 {
		t.Fatal("min/max helpers misbehaved")
	}

	if partialErrorsOnlyDenied(nil) {
		t.Fatal("empty errors reported denied")
	}
	if !partialErrorsOnlyDenied([]dashboard.PartialError{{Code: dashboard.CodeForbidden}}) {
		t.Fatal("all-denied errors were not reported as denied")
	}
	if partialErrorsOnlyDenied([]dashboard.PartialError{{Code: dashboard.CodeForbidden}, {Code: dashboard.CodeUpstreamTimeout}}) {
		t.Fatal("mixed errors reported denied")
	}

	if counter := cloneCounter(dashboard.EmptyCounter(dashboard.CounterDenied)); counter.Value != nil || counter.State != dashboard.CounterDenied {
		t.Fatalf("nil value clone = %+v", counter)
	}
	original := dashboard.AvailableCounter(3)
	cloned := cloneCounter(original)
	*cloned.Value = 7
	if *original.Value != 3 {
		t.Fatalf("clone shares the counter value: %+v", original)
	}

	canonical := canonicalDashboardNamespaces([]string{"beta", "alpha", "beta", "BadName", "*", ""})
	if len(canonical) != 2 || canonical[0] != "alpha" || canonical[1] != "beta" {
		t.Fatalf("canonical namespaces = %v", canonical)
	}
	if got := canonicalDashboardNamespaces(nil); len(got) != 0 {
		t.Fatalf("nil namespaces = %v", got)
	}
}

func TestResolveLogTargetOwnersResolvesOncePerPodAndHandlesMissing(t *testing.T) {
	t.Parallel()
	owner := &dashboard.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "payments"}
	resolver := &countingOwnerResolver{owner: owner}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "api"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "web"}},
	}
	targets := []dashboard.LogTarget{
		{Namespace: "payments", Pod: "api"},
		{Namespace: "payments", Pod: "api"},
		{Namespace: "payments", Pod: "missing"},
		{Namespace: "other", Pod: "api"},
	}
	resolveLogTargetOwners(context.Background(), resolver, pods, targets)
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1 (owner resolution must be cached per pod)", resolver.calls)
	}
	if targets[0].Workload != owner || targets[1].Workload != owner {
		t.Fatalf("resolved workloads = %+v %+v", targets[0].Workload, targets[1].Workload)
	}
	if targets[2].Workload != nil || targets[3].Workload != nil {
		t.Fatalf("missing pods invented owners: %+v %+v", targets[2].Workload, targets[3].Workload)
	}
}

type countingOwnerResolver struct {
	owner *dashboard.ResourceRef
	calls int
	err   error
}

func (resolver *countingOwnerResolver) ResolvePodOwner(_ context.Context, _ *corev1.Pod) (*dashboard.ResourceRef, error) {
	resolver.calls++
	if resolver.err != nil {
		return nil, resolver.err
	}
	return resolver.owner, nil
}
