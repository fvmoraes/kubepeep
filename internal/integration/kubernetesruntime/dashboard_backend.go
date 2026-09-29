package kubernetesruntime

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fvmoraes/kubepeep/internal/observability"
	problemdetector "github.com/fvmoraes/kubepeep/internal/problems"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
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
	discovery     *metricsDiscoveryCache
	metrics       *observability.Registry
	resources     *ResourceBackend

	scanMu         sync.Mutex
	scanSequence   uint64
	scanCancel     context.CancelFunc
	scanGeneration string
	scanCounter    dashboard.CounterDTO
}

func (backend *DashboardBackend) ConfigureDiagnostics(metrics *observability.Registry, resources *ResourceBackend) {
	if backend == nil {
		return
	}
	backend.metrics, backend.resources = metrics, resources
}

func NewDashboardBackend(runtime *Runtime, authorizer authorization.AuthorizationService, queryBudget dashboard.QueryBudget) (*DashboardBackend, error) {
	if runtime == nil || authorizer == nil {
		return nil, errors.New("dashboard backend: Kubernetes runtime and authorization are required")
	}
	return newDashboardBackend(runtimeDashboardClientProvider{runtime: runtime}, authorizer, queryBudget), nil
}

func newDashboardBackend(clients dashboardClientProvider, authorizer authorization.AuthorizationService, queryBudget dashboard.QueryBudget) *DashboardBackend {
	return &DashboardBackend{
		clients: clients, authorization: authorizer,
		queryBudget: queryBudget.Normalized(), logBudget: dashboard.DefaultLogBudget(), now: time.Now,
		discovery:   newMetricsDiscoveryCache(defaultDiscoveryCacheTTL, time.Now),
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
	backend.discovery.InvalidateAll()
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

func (backend *DashboardBackend) NamespaceHealth(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.NamespaceHealthDTO] {
	service, _ := backend.service(binding)
	return service.NamespaceHealth(ctx, dashboardSelection(binding, resolution))
}

func (backend *DashboardBackend) Problems(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO] {
	_, adapter := backend.service(binding)
	selection := dashboardSelection(binding, resolution)
	type podResult struct {
		value dashboard.DashboardBlockDTO[[]corev1.Pod]
	}
	type eventResult struct {
		value dashboard.DashboardBlockDTO[[]dashboard.NormalizedEvent]
	}
	type workloadResult struct {
		value dashboard.DashboardBlockDTO[[]dashboard.WorkloadDTO]
	}
	podsCh, eventsCh, workloadsCh := make(chan podResult, 1), make(chan eventResult, 1), make(chan workloadResult, 1)
	go func() { podsCh <- podResult{backend.collectPods(ctx, adapter, selection.Namespaces)} }()
	go func() { eventsCh <- eventResult{backend.collectEvents(ctx, adapter, selection.Namespaces)} }()
	go func() {
		workloadsCh <- workloadResult{dashboard.NewWorkloadService(adapter, nil, backend.queryBudget).List(ctx, selection)}
	}()
	pods, events, workloads := (<-podsCh).value, (<-eventsCh).value, (<-workloadsCh).value
	result := dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO]{Value: []dashboard.ProblemPodDTO{}, Complete: pods.Complete && events.Complete && workloads.Complete, Truncated: pods.Truncated || events.Truncated || workloads.Truncated, Coverage: pods.Coverage, Errors: []dashboard.PartialError{}}
	result.Errors = append(result.Errors, pods.Errors...)
	result.Errors = append(result.Errors, events.Errors...)
	result.Errors = append(result.Errors, workloads.Errors...)
	now := backend.now()
	for index := range pods.Value {
		pod := &pods.Value[index]
		owner := dashboard.DirectPodOwner(pod)
		for _, problem := range problemdetector.DetectPod(pod, now) {
			result.Value = append(result.Value, problemDTO(problem, owner, string(pod.UID)))
		}
	}
	for _, workload := range workloads.Value {
		observation := problemdetector.WorkloadObservation{Resource: problemdetector.Resource{APIGroup: problemWorkloadAPIGroup(workload.Kind), Kind: workload.Kind, Namespace: workload.Namespace, Name: workload.Name}, Status: string(workload.Status), Desired: workload.Desired, Available: workload.Available, AgeSeconds: workload.AgeSeconds}
		for _, problem := range problemdetector.DetectWorkload(observation) {
			result.Value = append(result.Value, problemDTO(problem, nil, ""))
		}
	}
	for _, event := range events.Value {
		if !strings.EqualFold(event.Type, "Warning") {
			continue
		}
		age := int64(0)
		if !event.ObservedAt.IsZero() && !now.Before(event.ObservedAt) {
			age = int64(now.Sub(event.ObservedAt) / time.Second)
		}
		for _, problem := range problemdetector.DetectWarningEvent(event.Namespace, event.RegardingKind, event.RegardingName, event.Reason, age) {
			result.Value = append(result.Value, problemDTO(problem, nil, string(event.RegardingUID)))
		}
	}
	if backend.resources != nil {
		index := backend.resources.LocalIndex(binding.Generation, resolution.Namespaces)
		pvcComplete := false
		for _, coverage := range index.Coverage {
			if coverage.Topic == resourcecore.TopicPVCs {
				pvcComplete = coverage.Complete
				break
			}
		}
		if !pvcComplete {
			result.Complete = false
			result.Errors = append(result.Errors, dashboard.PartialError{Code: dashboard.CodeFeatureUnavailable, Message: "PVC problem coverage is incomplete until the local cache is synchronized."})
		}
		for _, pvc := range index.PVCs {
			for _, problem := range problemdetector.DetectPVC(pvc.Namespace, pvc.Name, pvc.Status, pvc.AgeSeconds) {
				result.Value = append(result.Value, problemDTO(problem, nil, ""))
			}
		}
	}
	nodeProblems, nodeComplete, nodeErrors := backend.collectNodeProblems(ctx, binding, now)
	result.Value = append(result.Value, nodeProblems...)
	result.Complete = result.Complete && nodeComplete
	result.Errors = append(result.Errors, nodeErrors...)
	dashboard.SortProblems(result.Value)
	return result
}

func problemDTO(problem problemdetector.Problem, owner *dashboard.ResourceRef, uid string) dashboard.ProblemPodDTO {
	severity := dashboard.ProblemSeverity(problem.Severity)
	resource := dashboard.ResourceRef{APIGroup: problem.Resource.APIGroup, Kind: problem.Resource.Kind, Namespace: problem.Resource.Namespace, Name: problem.Resource.Name, UID: uid}
	var container *string
	var containerType *dashboard.ContainerType
	if problem.Container != "" {
		value := problem.Container
		kind := dashboard.ContainerRegular
		container = &value
		containerType = &kind
	}
	reason := problem.Reason
	summary := problem.Summary
	actions := []string{"inspect"}
	pod := ""
	if problem.Resource.Kind == "Pod" {
		pod = problem.Resource.Name
		actions = append(actions, "logs")
	} else if problem.Resource.Kind == "Deployment" || problem.Resource.Kind == "ReplicaSet" || problem.Resource.Kind == "StatefulSet" || problem.Resource.Kind == "DaemonSet" || problem.Resource.Kind == "Job" {
		actions = append(actions, "logs")
	}
	return dashboard.ProblemPodDTO{Resource: resource, Namespace: problem.Resource.Namespace, Pod: pod, Owner: owner, Container: container, ContainerType: containerType, Status: problem.Reason, Reason: &reason, Message: &summary, Summary: summary, Source: dashboard.ProblemSource(problem.Source), Severity: severity, AgeSeconds: problem.AgeSeconds, Actions: actions}
}

func problemWorkloadAPIGroup(kind string) string {
	if kind == "Job" || kind == "CronJob" {
		return "batch"
	}
	return "apps"
}

func (backend *DashboardBackend) collectNodeProblems(ctx context.Context, binding namespaces.SelectionBinding, now time.Time) ([]dashboard.ProblemPodDTO, bool, []dashboard.PartialError) {
	result := []dashboard.ProblemPodDTO{}
	errorsOut := []dashboard.PartialError{}
	requestContext, cancel, clients, err := (&dashboardAdapter{clients: backend.clients, authorization: backend.authorization, binding: binding, discovery: backend.discovery}).unary(ctx)
	if err != nil {
		return result, false, []dashboard.PartialError{{Code: dashboard.CodeClusterUnavailable, Message: "Node problem coverage is unavailable."}}
	}
	defer cancel()
	adapter := &dashboardAdapter{clients: backend.clients, authorization: backend.authorization, binding: binding, discovery: backend.discovery}
	var list *corev1.NodeList
	err = adapter.guard(requestContext, authorization.Key{Generation: binding.Generation, Resource: "nodes", Verb: "list"}, func(operationContext context.Context) error {
		var listErr error
		list, listErr = clients.kubernetes.CoreV1().Nodes().List(operationContext, metav1.ListOptions{Limit: 500})
		return listErr
	})
	if err != nil {
		partial, _ := nodePartialError(err)
		return result, false, []dashboard.PartialError{partial}
	}
	for index := range list.Items {
		node := &list.Items[index]
		ready := corev1.ConditionUnknown
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				ready = condition.Status
				break
			}
		}
		age := int64(0)
		if !node.CreationTimestamp.IsZero() && !now.Before(node.CreationTimestamp.Time) {
			age = int64(now.Sub(node.CreationTimestamp.Time) / time.Second)
		}
		for _, problem := range problemdetector.DetectNode(node.Name, ready, age) {
			result = append(result, problemDTO(problem, nil, string(node.UID)))
		}
	}
	return result, list.Continue == "", errorsOut
}

func nodePartialError(err error) (dashboard.PartialError, bool) {
	if authorization.ErrorCodeOf(err) == authorization.CodeForbidden {
		return dashboard.PartialError{Code: dashboard.CodeForbidden, Message: "Access to the requested resource was denied."}, true
	}
	return dashboard.PartialError{Code: dashboard.CodeClusterUnavailable, Message: "The Kubernetes API is temporarily unavailable."}, false
}

func (backend *DashboardBackend) Restarts(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, limit int) dashboard.DashboardBlockDTO[[]dashboard.RestartDTO] {
	service, _ := backend.service(binding)
	return service.Restarts(ctx, dashboardSelection(binding, resolution), limit)
}

func (backend *DashboardBackend) Events(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.EventDTO] {
	service, _ := backend.service(binding)
	return service.EventList(ctx, dashboardSelection(binding, resolution))
}

func (backend *DashboardBackend) Metrics(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DashboardBlockDTO[dashboard.MetricsDTO] {
	service, _ := backend.service(binding)
	return service.PodMetrics(ctx, dashboardSelection(binding, resolution))
}

func (backend *DashboardBackend) Diagnostics(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DiagnosticsDTO {
	diagnosticsContext, cancel := context.WithTimeout(ctx, backend.queryBudget.Timeout)
	defer cancel()
	ctx = diagnosticsContext
	result := dashboard.DiagnosticsDTO{Generation: binding.Generation, CollectedAt: backend.now().UTC().Format(time.RFC3339Nano), Complete: true, Errors: []dashboard.PartialError{}, Namespaces: []dashboard.NamespaceDiagnosticsDTO{}, CacheCoverage: []dashboard.CacheCoverageDiagnosticDTO{}}
	metricSnapshot := observability.Snapshot{}
	if backend.metrics != nil {
		metricSnapshot = backend.metrics.Snapshot()
	}
	result.Performance = performanceDiagnostics(metricSnapshot)
	index := resourcecore.LocalIndexSnapshot{Generation: binding.Generation}
	if backend.resources != nil {
		index = backend.resources.LocalIndex(binding.Generation, resolution.Namespaces)
		memory := backend.resources.MemoryStats()
		result.Cluster.KubePeep = dashboard.KubePeepDiagnosticsDTO{ResourceCacheEntries: memory.ResourceEntries, ResourceCacheBytes: memory.ResourceBytes, CollectionCacheEntries: memory.CollectionEntries, CollectionCacheBytes: memory.CollectionBytes, CursorBytes: memory.CursorBytes, ActiveWatches: result.Performance.ActiveWatches}
	}
	for _, coverage := range index.Coverage {
		result.CacheCoverage = append(result.CacheCoverage, dashboard.CacheCoverageDiagnosticDTO{Topic: string(coverage.Topic), State: string(coverage.State), Complete: coverage.Complete, LoadedNamespaces: append([]string(nil), coverage.LoadedNamespaces...)})
		if !coverage.Complete {
			result.Complete = false
		}
	}
	resourceCounts := make(map[string]map[string]int)
	for _, entry := range index.NamespaceCount {
		resourceCounts[entry.Namespace] = entry.Resources
	}
	problemsBlock := backend.Problems(ctx, binding, resolution)
	problemCounts := make(map[string]map[dashboard.ProblemSeverity]int)
	for _, problem := range problemsBlock.Value {
		if problemCounts[problem.Namespace] == nil {
			problemCounts[problem.Namespace] = make(map[dashboard.ProblemSeverity]int)
		}
		problemCounts[problem.Namespace][problem.Severity]++
	}
	result.Complete = result.Complete && problemsBlock.Complete
	result.Errors = append(result.Errors, problemsBlock.Errors...)
	namespaceValues := canonicalDashboardNamespaces(resolution.Namespaces)
	result.Cluster.Namespaces = len(namespaceValues)
	const maximumDiagnosticNamespaces = 200
	if len(namespaceValues) > maximumDiagnosticNamespaces {
		namespaceValues = namespaceValues[:maximumDiagnosticNamespaces]
		result.Complete = false
		result.Errors = append(result.Errors, dashboard.PartialError{Code: dashboard.CodeFeatureUnavailable, Message: "Namespace diagnostics were limited to the first 200 canonical namespaces."})
	}
	type namespaceResult struct {
		value  dashboard.NamespaceDiagnosticsDTO
		errors []dashboard.PartialError
	}
	jobs := make(chan string)
	completed := make(chan namespaceResult, len(namespaceValues))
	workers := min(4, len(namespaceValues))
	for worker := 0; worker < workers; worker++ {
		go func() {
			for namespace := range jobs {
				started := time.Now()
				service, _ := backend.service(binding)
				selection := dashboardSelection(binding, resolution)
				selection.Namespaces = []string{namespace}
				block := service.NamespaceHealth(ctx, selection)
				elapsed := time.Since(started).Milliseconds()
				item := dashboard.NamespaceDiagnosticsDTO{Namespace: namespace, Resources: copyCounts(resourceCounts[namespace]), Problems: copyProblemCounts(problemCounts[namespace]), Complete: block.Complete}
				if block.Complete && len(block.Value) == 1 {
					item.Restarts = &block.Value[0].ContainerRestarts
					item.ListLatencyMilliseconds = &elapsed
				}
				completed <- namespaceResult{value: item, errors: block.Errors}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, namespace := range namespaceValues {
			select {
			case jobs <- namespace:
			case <-ctx.Done():
				return
			}
		}
	}()
	for range namespaceValues {
		select {
		case item := <-completed:
			result.Namespaces = append(result.Namespaces, item.value)
			result.Errors = append(result.Errors, item.errors...)
			result.Complete = result.Complete && item.value.Complete
		case <-ctx.Done():
			result.Complete = false
			result.Errors = append(result.Errors, dashboard.PartialError{Code: dashboard.CodeClientCanceled, Message: "Diagnostics collection was canceled."})
			goto namespaceDone
		}
	}
namespaceDone:
	sort.Slice(result.Namespaces, func(left, right int) bool {
		leftLatency, rightLatency := result.Namespaces[left].ListLatencyMilliseconds, result.Namespaces[right].ListLatencyMilliseconds
		if leftLatency == nil || rightLatency == nil {
			if leftLatency == nil && rightLatency != nil {
				return false
			}
			if leftLatency != nil && rightLatency == nil {
				return true
			}
		} else if *leftLatency != *rightLatency {
			return *leftLatency > *rightLatency
		}
		return result.Namespaces[left].Namespace < result.Namespaces[right].Namespace
	})
	result.Cluster.Totals = make(map[string]int)
	for _, entry := range index.Resources {
		if entry.Kind != "Event" {
			result.Cluster.Totals[entry.Kind]++
		}
	}
	if version, err := backend.clusterVersion(ctx, binding); err == nil {
		result.Cluster.KubernetesVersion = &version
	} else {
		result.Complete = false
		result.Errors = append(result.Errors, dashboard.PartialError{Code: dashboard.CodeClusterUnavailable, Message: "Kubernetes version is unavailable."})
	}
	return result
}

func performanceDiagnostics(snapshot observability.Snapshot) dashboard.PerformanceDiagnosticsDTO {
	result := dashboard.PerformanceDiagnosticsDTO{ResourceSync: []dashboard.ResourceSyncDiagnosticDTO{}}
	apiSamples := []time.Duration{}
	for _, series := range snapshot.Durations[observability.KubernetesRequestDurationNanosecondsTotalName] {
		apiSamples = append(apiSamples, series.Samples...)
	}
	result.APIServerLatency = latencyPercentiles(apiSamples)
	byResource := make(map[string][]time.Duration)
	for _, series := range snapshot.Durations[observability.ResourceListDurationNanosecondsTotalName] {
		resource := series.Labels["resource"]
		if resource == "" {
			continue
		}
		byResource[resource] = append(byResource[resource], series.Samples...)
	}
	for resource, samples := range byResource {
		result.ResourceSync = append(result.ResourceSync, dashboard.ResourceSyncDiagnosticDTO{Resource: resource, Latency: latencyPercentiles(samples)})
	}
	sort.Slice(result.ResourceSync, func(left, right int) bool {
		return result.ResourceSync[left].Resource < result.ResourceSync[right].Resource
	})
	hits := counterTotal(snapshot, observability.ResourceCacheHitsTotalName) + counterTotal(snapshot, observability.CollectionCacheHitsTotalName)
	misses := counterTotal(snapshot, observability.ResourceCacheMissesTotalName) + counterTotal(snapshot, observability.CollectionCacheMissesTotalName)
	if hits+misses > 0 {
		ratio := float64(hits) / float64(hits+misses)
		result.CacheHitRatio = &ratio
	}
	result.ActiveWatches = gaugeTotal(snapshot, observability.WatchActiveName)
	result.Responses429 = counterTotal(snapshot, observability.Kubernetes429TotalName)
	result.WatchReconnects = counterTotal(snapshot, observability.WatchReconnectsTotalName)
	elapsed := time.Since(snapshot.StartedAt).Minutes()
	if !snapshot.StartedAt.IsZero() && elapsed > 0 {
		result.RequestsPerMinute = float64(counterTotal(snapshot, observability.KubernetesRequestsTotalName)) / elapsed
	}
	return result
}

func latencyPercentiles(values []time.Duration) dashboard.LatencyPercentilesDTO {
	if len(values) == 0 {
		return dashboard.LatencyPercentilesDTO{}
	}
	values = append([]time.Duration(nil), values...)
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	at := func(percentile float64) int64 {
		index := int(float64(len(values)-1)*percentile + 0.5)
		return values[index].Milliseconds()
	}
	return dashboard.LatencyPercentilesDTO{P50Milliseconds: at(0.50), P95Milliseconds: at(0.95), P99Milliseconds: at(0.99), MaxMilliseconds: values[len(values)-1].Milliseconds(), Samples: len(values)}
}
func counterTotal(snapshot observability.Snapshot, name string) uint64 {
	var total uint64
	for _, series := range snapshot.Counters[name] {
		total += series.Value
	}
	return total
}
func gaugeTotal(snapshot observability.Snapshot, name string) int64 {
	var total int64
	for _, series := range snapshot.Gauges[name] {
		total += series.Value
	}
	return total
}
func copyCounts(source map[string]int) map[string]int {
	result := make(map[string]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
func copyProblemCounts(source map[dashboard.ProblemSeverity]int) map[dashboard.ProblemSeverity]int {
	result := map[dashboard.ProblemSeverity]int{dashboard.ProblemCritical: 0, dashboard.ProblemWarning: 0, dashboard.ProblemInfo: 0}
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (backend *DashboardBackend) clusterVersion(ctx context.Context, binding namespaces.SelectionBinding) (string, error) {
	adapter := &dashboardAdapter{clients: backend.clients, authorization: backend.authorization, binding: binding, discovery: backend.discovery}
	requestContext, cancel, clients, err := adapter.unary(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()
	type response struct {
		version string
		err     error
	}
	done := make(chan response, 1)
	go func() {
		value, versionErr := clients.kubernetes.Discovery().ServerVersion()
		if versionErr != nil {
			done <- response{err: versionErr}
			return
		}
		done <- response{version: value.GitVersion}
	}()
	select {
	case <-requestContext.Done():
		return "", requestContext.Err()
	case value := <-done:
		return value.version, value.err
	}
}

func (backend *DashboardBackend) ScanLogs(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, request dashboard.LogScanRequest) dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO] {
	resolved, err := dashboard.ResolveLogScanRequest(request)
	if err != nil {
		return dashboardValidationBlock(err)
	}
	scanContext, cancel, sequence := backend.beginScan(ctx, binding.Generation)
	defer cancel()
	scanContext, timeoutCancel := context.WithTimeout(scanContext, backend.queryBudget.Timeout)
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
	adapter := &dashboardAdapter{clients: backend.clients, authorization: backend.authorization, binding: binding, discovery: backend.discovery}
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
			remaining = backend.queryBudget.MaxItems - len(block.Value)
			if remaining <= 0 {
				block.Complete = false
				block.Truncated = true
				break
			}
			if err := ctx.Err(); err != nil {
				addCollectionError(&block, namespace, err)
				break
			}
			response, err := port.ListPods(ctx, namespace, dashboard.PageRequest{Limit: backend.queryBudget.PageSize, Continue: continuation})
			if err != nil {
				addCollectionError(&block, namespace, err)
				break
			}
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
			remaining = backend.queryBudget.MaxItems - len(block.Value)
			if remaining <= 0 {
				block.Complete = false
				block.Truncated = true
				break
			}
			if err := ctx.Err(); err != nil {
				addCollectionError(&block, namespace, err)
				break
			}
			response, err := port.ListEvents(ctx, namespace, dashboard.PageRequest{Limit: backend.queryBudget.PageSize, Continue: continuation})
			if err != nil {
				addCollectionError(&block, namespace, err)
				break
			}
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
