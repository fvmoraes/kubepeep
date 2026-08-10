package kubernetesruntime

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

// DashboardBackend composes the pure Phase 5 services with request-bound
// Kubernetes adapters. It retains only a summary counter for the latest scan;
// log lines and match DTOs exist solely for the lifetime of the HTTP request.
type DashboardBackend struct {
	clients       dashboardClientProvider
	authorization authorization.AuthorizationService
	queryBudget   dashboard.QueryBudget
	logBudget     dashboard.LogBudget
	now           func() time.Time

	scanMu         sync.Mutex
	scanSequence   uint64
	scanCancel     context.CancelFunc
	scanGeneration string
	scanCounter    dashboard.CounterDTO
}

func NewDashboardBackend(runtime *Runtime, authorizer authorization.AuthorizationService) (*DashboardBackend, error) {
	if runtime == nil || authorizer == nil {
		return nil, errors.New("dashboard backend: Kubernetes runtime and authorization are required")
	}
	return newDashboardBackend(runtimeDashboardClientProvider{runtime: runtime}, authorizer), nil
}

func newDashboardBackend(clients dashboardClientProvider, authorizer authorization.AuthorizationService) *DashboardBackend {
	return &DashboardBackend{
		clients: clients, authorization: authorizer,
		queryBudget: dashboard.DefaultQueryBudget(), logBudget: dashboard.DefaultLogBudget(), now: time.Now,
		scanCounter: dashboard.EmptyCounter(dashboard.CounterNotCollected),
	}
}

// OnGeneration immediately cancels an explicit scan and forgets its counter.
// KubernetesRuntime independently cancels transport work; this also stops
// local target selection, detection, and redaction that no longer belong to
// the active selection.
func (backend *DashboardBackend) OnGeneration(generation string) {
	if backend == nil {
		return
	}
	backend.scanMu.Lock()
	defer backend.scanMu.Unlock()
	if backend.scanCancel != nil {
		backend.scanCancel()
	}
	backend.scanSequence++
	backend.scanCancel = nil
	backend.scanGeneration = generation
	backend.scanCounter = dashboard.EmptyCounter(dashboard.CounterNotCollected)
}

func (backend *DashboardBackend) Summary(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DashboardBlockDTO[dashboard.SummaryDTO] {
	service, _ := backend.service(binding)
	return service.Summary(ctx, dashboardSelection(binding, resolution), dashboard.SummaryOptions{
		PossibleLogMatches: backend.currentLogCounter(binding.Generation),
	})
}

func (backend *DashboardBackend) Problems(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO] {
	service, _ := backend.service(binding)
	return service.Problems(ctx, dashboardSelection(binding, resolution))
}

func (backend *DashboardBackend) Restarts(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, limit int) dashboard.DashboardBlockDTO[[]dashboard.RestartDTO] {
	service, _ := backend.service(binding)
	return service.Restarts(ctx, dashboardSelection(binding, resolution), limit)
}

func (backend *DashboardBackend) Events(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.EventDTO] {
	service, _ := backend.service(binding)
	return service.Warnings(ctx, dashboardSelection(binding, resolution))
}

func (backend *DashboardBackend) Metrics(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DashboardBlockDTO[dashboard.MetricsDTO] {
	service, _ := backend.service(binding)
	return service.PodMetrics(ctx, dashboardSelection(binding, resolution))
}

func (backend *DashboardBackend) ScanLogs(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, request dashboard.LogScanRequest) dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO] {
	resolved, err := dashboard.ResolveLogScanRequest(request)
	if err != nil {
		return dashboardValidationBlock(err)
	}
	scanContext, cancel, sequence := backend.beginScan(ctx, binding.Generation)
	defer cancel()
	scanContext, timeoutCancel := context.WithTimeout(scanContext, dashboard.DefaultBlockTimeout)
	defer timeoutCancel()
	service, adapter := backend.service(binding)
	selection := dashboardSelection(binding, resolution)

	type podCollection struct {
		block dashboard.DashboardBlockDTO[[]corev1.Pod]
	}
	type eventCollection struct {
		block dashboard.DashboardBlockDTO[[]dashboard.NormalizedEvent]
	}
	podsChannel := make(chan podCollection, 1)
	eventsChannel := make(chan eventCollection, 1)
	go func() { podsChannel <- podCollection{backend.collectPods(scanContext, adapter, selection.Namespaces)} }()
	go func() {
		eventsChannel <- eventCollection{backend.collectEvents(scanContext, adapter, selection.Namespaces)}
	}()
	pods := (<-podsChannel).block
	events := (<-eventsChannel).block

	problems := make([]dashboard.ProblemPodDTO, 0)
	restarts := make([]dashboard.RestartDTO, 0)
	capturedAt := backend.now()
	for index := range pods.Value {
		pod := &pods.Value[index]
		owner := dashboard.DirectPodOwner(pod)
		if problem, ok := dashboard.ClassifyProblemPod(pod, events.Value, owner, capturedAt); ok {
			problems = append(problems, problem)
		}
		restarts = append(restarts, dashboard.PodRestarts(pod, owner, capturedAt)...)
	}
	targets := dashboard.SelectLogTargets(pods.Value, problems, restarts, capturedAt, resolved, backend.logBudget)
	resolveLogTargetOwners(scanContext, adapter, pods.Value, targets.Targets)
	result := service.ScanLogs(scanContext, request, targets.Targets)
	mergeScanInputs(&result, pods, events, len(selection.Namespaces))
	if targets.Truncated {
		result.Complete = false
		result.Truncated = true
	}
	backend.finishScan(sequence, binding.Generation, scanContext, result)
	return result
}

func resolveLogTargetOwners(ctx context.Context, resolver dashboard.OwnerResolver, pods []corev1.Pod, targets []dashboard.LogTarget) {
	podsByName := make(map[string]*corev1.Pod, len(pods))
	for index := range pods {
		podsByName[pods[index].Namespace+"\x00"+pods[index].Name] = &pods[index]
	}
	owners := make(map[string]*dashboard.ResourceRef)
	for index := range targets {
		key := targets[index].Namespace + "\x00" + targets[index].Pod
		owner, resolved := owners[key]
		if !resolved {
			pod := podsByName[key]
			if pod != nil {
				owner, _ = resolver.ResolvePodOwner(ctx, pod)
			}
			owners[key] = owner
		}
		targets[index].Workload = owner
	}
}

func (backend *DashboardBackend) service(binding namespaces.SelectionBinding) (*dashboard.DashboardService, *dashboardAdapter) {
	adapter := &dashboardAdapter{clients: backend.clients, authorization: backend.authorization, binding: binding}
	pods := dashboard.NewPodService(adapter, adapter, adapter, nil, backend.queryBudget)
	return &dashboard.DashboardService{
		Pods:      pods,
		Workloads: dashboard.NewWorkloadService(adapter, nil, backend.queryBudget),
		Events:    dashboard.NewEventService(adapter, backend.queryBudget),
		Logs:      dashboard.NewLogService(adapter, adapter, nil, backend.logBudget),
		Metrics:   dashboard.NewMetricsService(adapter, adapter, nil, backend.queryBudget),
	}, adapter
}

func dashboardSelection(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.Selection {
	scope := resolution.ScopeName
	if scope == "" {
		scope = resolution.ScopeSource
	}
	return dashboard.Selection{
		Generation: binding.Generation, Context: binding.Context, Cluster: binding.Cluster,
		Scope: scope, Namespaces: append([]string(nil), resolution.Namespaces...),
	}
}

func (backend *DashboardBackend) collectPods(ctx context.Context, port dashboard.PodPort, namespaces []string) dashboard.DashboardBlockDTO[[]corev1.Pod] {
	values := canonicalDashboardNamespaces(namespaces)
	block := newCollectionBlock(make([]corev1.Pod, 0), len(values))
	for _, namespace := range values {
		remaining := backend.queryBudget.MaxItems - len(block.Value)
		if remaining <= 0 {
			block.Complete = false
			block.Truncated = true
			break
		}
		continuation := ""
		completed := false
		for page := 0; page < backend.queryBudget.MaxPages; page++ {
			if err := ctx.Err(); err != nil {
				addCollectionError(&block, namespace, err)
				break
			}
			response, err := port.ListPods(ctx, namespace, dashboard.PageRequest{Limit: backend.queryBudget.PageSize, Continue: continuation})
			if err != nil {
				addCollectionError(&block, namespace, err)
				break
			}
			remaining = backend.queryBudget.MaxItems - len(block.Value)
			if len(response.Items) > remaining {
				block.Value = append(block.Value, response.Items[:remaining]...)
				block.Complete = false
				block.Truncated = true
				break
			}
			block.Value = append(block.Value, response.Items...)
			continuation = response.Continue
			if continuation == "" {
				completed = true
				break
			}
			if page+1 == backend.queryBudget.MaxPages {
				block.Complete = false
				block.Truncated = true
			}
		}
		if completed {
			block.Coverage.CompletedNamespaces++
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

func (backend *DashboardBackend) collectEvents(ctx context.Context, port dashboard.EventPort, namespaces []string) dashboard.DashboardBlockDTO[[]dashboard.NormalizedEvent] {
	values := canonicalDashboardNamespaces(namespaces)
	block := newCollectionBlock(make([]dashboard.NormalizedEvent, 0), len(values))
	for _, namespace := range values {
		remaining := backend.queryBudget.MaxItems - len(block.Value)
		if remaining <= 0 {
			block.Complete = false
			block.Truncated = true
			break
		}
		continuation := ""
		completed := false
		for page := 0; page < backend.queryBudget.MaxPages; page++ {
			if err := ctx.Err(); err != nil {
				addCollectionError(&block, namespace, err)
				break
			}
			response, err := port.ListEvents(ctx, namespace, dashboard.PageRequest{Limit: backend.queryBudget.PageSize, Continue: continuation})
			if err != nil {
				addCollectionError(&block, namespace, err)
				break
			}
			remaining = backend.queryBudget.MaxItems - len(block.Value)
			if len(response.Items) > remaining {
				block.Value = append(block.Value, response.Items[:remaining]...)
				block.Complete = false
				block.Truncated = true
				break
			}
			block.Value = append(block.Value, response.Items...)
			continuation = response.Continue
			if continuation == "" {
				completed = true
				break
			}
			if page+1 == backend.queryBudget.MaxPages {
				block.Complete = false
				block.Truncated = true
			}
		}
		if completed {
			block.Coverage.CompletedNamespaces++
		}
	}
	return block
}

func newCollectionBlock[T any](value T, requested int) dashboard.DashboardBlockDTO[T] {
	return dashboard.DashboardBlockDTO[T]{
		Value: value, Complete: true, Errors: make([]dashboard.PartialError, 0),
		Coverage: &dashboard.CoverageDTO{RequestedNamespaces: requested, DeniedNamespaces: []string{}, Failed: []dashboard.PartialError{}},
	}
}

func addCollectionError[T any](block *dashboard.DashboardBlockDTO[T], namespace string, err error) {
	partial, denied := dashboardPartialError(namespace, err)
	for _, existing := range block.Errors {
		if existing.Namespace == partial.Namespace && existing.Code == partial.Code {
			block.Complete = false
			return
		}
	}
	block.Complete = false
	block.Errors = append(block.Errors, partial)
	if denied {
		block.Coverage.DeniedNamespaces = appendUniqueString(block.Coverage.DeniedNamespaces, namespace)
	} else {
		block.Coverage.Failed = append(block.Coverage.Failed, partial)
	}
}

func dashboardPartialError(namespace string, err error) (dashboard.PartialError, bool) {
	type safeError interface {
		Code() string
		PublicMessage() string
		Denied() bool
	}
	var safe safeError
	if errors.As(err, &safe) {
		return dashboard.PartialError{Namespace: namespace, Code: safe.Code(), Message: safe.PublicMessage()}, safe.Denied()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return dashboard.PartialError{Namespace: namespace, Code: dashboard.CodeUpstreamTimeout, Message: "Collection timed out."}, false
	}
	if errors.Is(err, context.Canceled) {
		return dashboard.PartialError{Namespace: namespace, Code: dashboard.CodeClientCanceled, Message: "Collection was canceled."}, false
	}
	return dashboard.PartialError{Namespace: namespace, Code: dashboard.CodeClusterUnavailable, Message: "The Kubernetes API is temporarily unavailable."}, false
}

func mergeScanInputs(
	result *dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO],
	pods dashboard.DashboardBlockDTO[[]corev1.Pod],
	events dashboard.DashboardBlockDTO[[]dashboard.NormalizedEvent],
	requested int,
) {
	result.Complete = result.Complete && pods.Complete && events.Complete
	result.Truncated = result.Truncated || pods.Truncated || events.Truncated
	for _, partial := range append(append([]dashboard.PartialError{}, pods.Errors...), events.Errors...) {
		appendUniquePartial(&result.Errors, partial)
	}
	coverage := &dashboard.CoverageDTO{RequestedNamespaces: requested, DeniedNamespaces: []string{}, Failed: []dashboard.PartialError{}}
	baseComplete := minIntValue(pods.Coverage.CompletedNamespaces, events.Coverage.CompletedNamespaces)
	logComplete := requested
	if result.Coverage != nil {
		logComplete = requested - result.Coverage.RequestedNamespaces + result.Coverage.CompletedNamespaces
		for _, namespace := range result.Coverage.DeniedNamespaces {
			coverage.DeniedNamespaces = appendUniqueString(coverage.DeniedNamespaces, namespace)
		}
		for _, failure := range result.Coverage.Failed {
			appendUniquePartial(&coverage.Failed, failure)
		}
	}
	coverage.CompletedNamespaces = minIntValue(baseComplete, maxIntValue(logComplete, 0))
	for _, source := range []*dashboard.CoverageDTO{pods.Coverage, events.Coverage} {
		for _, namespace := range source.DeniedNamespaces {
			coverage.DeniedNamespaces = appendUniqueString(coverage.DeniedNamespaces, namespace)
		}
		for _, failure := range source.Failed {
			appendUniquePartial(&coverage.Failed, failure)
		}
	}
	result.Coverage = coverage
}

func dashboardValidationBlock(err error) dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO] {
	partial, _ := dashboardPartialError("", err)
	return dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]{
		Value: []dashboard.LogMatchDTO{}, Complete: false, Coverage: &dashboard.CoverageDTO{DeniedNamespaces: []string{}, Failed: []dashboard.PartialError{partial}},
		Errors: []dashboard.PartialError{partial},
	}
}

func (backend *DashboardBackend) beginScan(parent context.Context, generation string) (context.Context, context.CancelFunc, uint64) {
	backend.scanMu.Lock()
	defer backend.scanMu.Unlock()
	if backend.scanCancel != nil {
		backend.scanCancel()
	}
	ctx, cancel := context.WithCancel(parent)
	backend.scanSequence++
	backend.scanCancel = cancel
	backend.scanGeneration = generation
	backend.scanCounter = dashboard.EmptyCounter(dashboard.CounterCollecting)
	return ctx, cancel, backend.scanSequence
}

func (backend *DashboardBackend) finishScan(sequence uint64, generation string, ctx context.Context, result dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]) {
	backend.scanMu.Lock()
	defer backend.scanMu.Unlock()
	if sequence != backend.scanSequence || generation != backend.scanGeneration {
		return
	}
	backend.scanCancel = nil
	if errors.Is(ctx.Err(), context.Canceled) {
		backend.scanCounter = dashboard.EmptyCounter(dashboard.CounterNotCollected)
		return
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		backend.scanCounter = dashboard.EmptyCounter(dashboard.CounterUnavailable)
		return
	}
	switch {
	case result.Truncated:
		backend.scanCounter = dashboard.TruncatedCounter(int64(len(result.Value)))
	case result.Complete:
		backend.scanCounter = dashboard.AvailableCounter(int64(len(result.Value)))
	case partialErrorsOnlyDenied(result.Errors):
		backend.scanCounter = dashboard.EmptyCounter(dashboard.CounterDenied)
	default:
		backend.scanCounter = dashboard.EmptyCounter(dashboard.CounterUnavailable)
	}
}

func (backend *DashboardBackend) currentLogCounter(generation string) dashboard.CounterDTO {
	backend.scanMu.Lock()
	defer backend.scanMu.Unlock()
	if generation == "" || generation != backend.scanGeneration {
		return dashboard.EmptyCounter(dashboard.CounterNotCollected)
	}
	return cloneCounter(backend.scanCounter)
}

func cloneCounter(value dashboard.CounterDTO) dashboard.CounterDTO {
	if value.Value == nil {
		return dashboard.CounterDTO{State: value.State}
	}
	copy := *value.Value
	return dashboard.CounterDTO{State: value.State, Value: &copy}
}

func partialErrorsOnlyDenied(values []dashboard.PartialError) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if value.Code != dashboard.CodeForbidden {
			return false
		}
	}
	return true
}

func canonicalDashboardNamespaces(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !namespaces.ValidNamespaceName(value) {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniquePartial(values *[]dashboard.PartialError, value dashboard.PartialError) {
	for _, existing := range *values {
		if existing.Namespace == value.Namespace && existing.Code == value.Code {
			return
		}
	}
	*values = append(*values, value)
}

func minIntValue(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxIntValue(left, right int) int {
	if left > right {
		return left
	}
	return right
}
