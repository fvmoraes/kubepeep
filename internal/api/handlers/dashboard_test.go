package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type dashboardSelectionStub struct {
	mu          sync.Mutex
	binding     namespaces.SelectionBinding
	resolution  namespaces.ScopeResolution
	rejectFence bool
}

func (stub *dashboardSelectionStub) IfCurrent(binding namespaces.SelectionBinding, publish func()) bool {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.rejectFence || !sameSelectionBinding(stub.binding, binding) {
		return false
	}
	publish()
	return true
}

func (stub *dashboardSelectionStub) Snapshot() (namespaces.SelectionBinding, namespaces.ScopeResolution) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	resolution := stub.resolution
	resolution.Namespaces = append([]string(nil), resolution.Namespaces...)
	return stub.binding, resolution
}

func (stub *dashboardSelectionStub) generation(value string) {
	stub.mu.Lock()
	stub.binding.Generation = value
	stub.mu.Unlock()
}

type dashboardServiceStub struct {
	selectionSeen namespaces.ScopeResolution
	onSummary     func()
	logRequest    dashboard.LogScanRequest
	problemsCalls int
	problemsValue *dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO]
	eventsValue   *dashboard.DashboardBlockDTO[[]dashboard.EventDTO]
	metricsValue  *dashboard.DashboardBlockDTO[dashboard.MetricsDTO]
}

func (stub *dashboardServiceStub) Summary(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) dashboard.DashboardBlockDTO[dashboard.SummaryDTO] {
	if stub.onSummary != nil {
		stub.onSummary()
	}
	return dashboard.DashboardBlockDTO[dashboard.SummaryDTO]{Value: dashboard.SummaryDTO{Namespaces: dashboard.AvailableCounter(2)}, Complete: true, Errors: []dashboard.PartialError{}}
}

func (stub *dashboardServiceStub) Problems(_ context.Context, _ namespaces.SelectionBinding, resolution namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO] {
	stub.problemsCalls++
	stub.selectionSeen = resolution
	if stub.problemsValue != nil {
		return *stub.problemsValue
	}
	reason := "CrashLoopBackOff"
	return dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO]{
		Value: []dashboard.ProblemPodDTO{
			{Namespace: "payments", Pod: "api", Severity: dashboard.ProblemCritical, Reason: &reason},
			{Namespace: "other", Pod: "worker", Severity: dashboard.ProblemWarning},
		},
		Complete: true, Errors: []dashboard.PartialError{},
	}
}

func (stub *dashboardServiceStub) Restarts(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, int) dashboard.DashboardBlockDTO[[]dashboard.RestartDTO] {
	return dashboard.DashboardBlockDTO[[]dashboard.RestartDTO]{Value: []dashboard.RestartDTO{}, Complete: true, Errors: []dashboard.PartialError{}}
}

func (stub *dashboardServiceStub) Events(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.EventDTO] {
	if stub.eventsValue != nil {
		return *stub.eventsValue
	}
	return dashboard.DashboardBlockDTO[[]dashboard.EventDTO]{Value: []dashboard.EventDTO{}, Complete: true, Errors: []dashboard.PartialError{}}
}

func (stub *dashboardServiceStub) ScanLogs(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, request dashboard.LogScanRequest) dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO] {
	stub.logRequest = request
	return dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]{Value: []dashboard.LogMatchDTO{}, Complete: true, Errors: []dashboard.PartialError{}}
}

func (stub *dashboardServiceStub) Metrics(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) dashboard.DashboardBlockDTO[dashboard.MetricsDTO] {
	if stub.metricsValue != nil {
		return *stub.metricsValue
	}
	return dashboard.DashboardBlockDTO[dashboard.MetricsDTO]{Value: dashboard.MetricsDTO{Pods: []dashboard.PodMetricDTO{}, TopCPU: []dashboard.MetricRankDTO{}, TopMemory: []dashboard.MetricRankDTO{}}, Complete: true, Errors: []dashboard.PartialError{}}
}

func testDashboardHandler() (*Dashboard, *dashboardServiceStub, *dashboardSelectionStub) {
	selection := &dashboardSelectionStub{
		binding:    namespaces.SelectionBinding{ClusterProfileID: 7, Context: "dev", Cluster: "cluster-a", Generation: "gen-7"},
		resolution: namespaces.ScopeResolution{ScopeName: "backend", ScopeSource: "saved", Namespaces: []string{"payments", "other"}},
	}
	service := &dashboardServiceStub{}
	cursors, err := api.NewCursorCodec()
	if err != nil {
		panic(err)
	}
	handler := NewDashboard(service, selection, cursors)
	handler.now = func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) }
	return handler, service, selection
}

func TestDashboardCursorIsOpaqueAndBoundToQueryAndSelection(t *testing.T) {
	handler, _, _ := testDashboardHandler()
	recorder := httptest.NewRecorder()
	handler.Problems(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/problems?limit=1", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var first struct {
		Data dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO] `json:"data"`
		Meta dashboardMeta                                          `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Meta.Page == nil || first.Meta.Page.Next == "" || first.Meta.Page.Complete || !first.Meta.Page.Truncated || len(first.Data.Value) != 1 {
		t.Fatalf("first page = %+v, data = %#v", first.Meta.Page, first.Data.Value)
	}

	token := url.QueryEscape(first.Meta.Page.Next)
	recorder = httptest.NewRecorder()
	handler.Problems(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/problems?limit=1&continue="+token, ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var second struct {
		Data dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO] `json:"data"`
		Meta dashboardMeta                                          `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.Meta.Page == nil || second.Meta.Page.Next != "" || !second.Meta.Page.Complete || len(second.Data.Value) != 1 || second.Data.Value[0].Pod != "worker" {
		t.Fatalf("second page = %+v, data = %#v", second.Meta.Page, second.Data.Value)
	}

	recorder = httptest.NewRecorder()
	handler.Problems(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/problems?limit=2&continue="+token, ""))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), api.CodeCursorMismatch) {
		t.Fatalf("mismatch status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func dashboardRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	return request.WithContext(api.WithRequestID(request.Context(), "req-dashboard"))
}

func TestDashboardSummaryWritesGenerationBoundEnvelope(t *testing.T) {
	handler, _, _ := testDashboardHandler()
	recorder := httptest.NewRecorder()
	handler.Summary(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/summary", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
	}
	var response struct {
		Data dashboard.DashboardBlockDTO[dashboard.SummaryDTO] `json:"data"`
		Meta dashboardMeta                                     `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Meta.RequestID != "req-dashboard" || response.Meta.Generation != "gen-7" || response.Meta.Selection.Scope != "backend" {
		t.Fatalf("unexpected meta: %+v", response.Meta)
	}
	if response.Data.Value.Namespaces.Value == nil || *response.Data.Value.Namespaces.Value != 2 {
		t.Fatalf("unexpected summary: %+v", response.Data.Value)
	}
}

func TestDashboardProblemsNarrowsScopeAndFiltersWithoutBroadening(t *testing.T) {
	handler, service, _ := testDashboardHandler()
	recorder := httptest.NewRecorder()
	handler.Problems(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/problems?namespace=payments&status=critical&search=crash&limit=10", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(service.selectionSeen.Namespaces) != 1 || service.selectionSeen.Namespaces[0] != "payments" {
		t.Fatalf("backend namespaces = %#v", service.selectionSeen.Namespaces)
	}
	var response struct {
		Data dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO] `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Value) != 1 || response.Data.Value[0].Pod != "api" {
		t.Fatalf("problems = %#v", response.Data.Value)
	}

	recorder = httptest.NewRecorder()
	handler.Problems(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/problems?namespace=outside", ""))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), api.CodeValidationFailed) {
		t.Fatalf("outside scope status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDashboardLogScanUsesStrictBoundedJSON(t *testing.T) {
	handler, service, _ := testDashboardHandler()
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "unknown", body: `{"tailLines":10,"token":"secret"}`, code: api.CodeUnknownField},
		{name: "wrong type", body: `{"tailLines":"10"}`, code: api.CodeValidationFailed},
		{name: "out of range", body: `{"tailLines":0}`, code: api.CodeValidationFailed},
		{name: "trailing", body: `{} {}`, code: api.CodeInvalidJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.LogScan(recorder, dashboardRequest(http.MethodPost, "/api/v1/dashboard/log-scan", test.body))
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), test.code) {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}

	recorder := httptest.NewRecorder()
	handler.LogScan(recorder, dashboardRequest(http.MethodPost, "/api/v1/dashboard/log-scan", `{"window":"30m","tailLines":25,"maxPods":3,"maxConcurrentContainers":2}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("valid status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.logRequest.TailLines == nil || *service.logRequest.TailLines != 25 || service.logRequest.Window == nil || *service.logRequest.Window != "30m" {
		t.Fatalf("request = %+v", service.logRequest)
	}
}

func TestDashboardDiscardsResultAfterGenerationChange(t *testing.T) {
	handler, service, selection := testDashboardHandler()
	service.onSummary = func() { selection.generation("gen-8") }
	recorder := httptest.NewRecorder()
	handler.Summary(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/summary", ""))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), api.CodeGenerationChanged) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDashboardRequiresActiveSelectionAndRejectsUnknownQuery(t *testing.T) {
	handler, _, selection := testDashboardHandler()
	selection.mu.Lock()
	selection.binding = namespaces.SelectionBinding{}
	selection.mu.Unlock()
	recorder := httptest.NewRecorder()
	handler.Summary(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/summary", ""))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d", recorder.Code)
	}

	handler, _, _ = testDashboardHandler()
	recorder = httptest.NewRecorder()
	handler.Events(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/events?raw=true", ""))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), api.CodeValidationFailed) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestContainsFoldedUsesUnicodeSimpleCaseFolding(t *testing.T) {
	if !containsFolded("ος", "ΟΣ") || !containsFolded("οσ", "Ος") {
		t.Fatal("Greek sigma variants should match under Unicode simple folding")
	}
}

func TestDashboardRejectsInvalidCursorBeforeKubernetesCollection(t *testing.T) {
	handler, service, _ := testDashboardHandler()
	recorder := httptest.NewRecorder()
	handler.Problems(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/problems?continue=not-a-cursor", ""))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), api.CodeCursorInvalid) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.problemsCalls != 0 {
		t.Fatalf("invalid cursor triggered %d collections", service.problemsCalls)
	}
}

func TestDashboardMapsOnlyTotalFailureToAuthoritativeHTTPError(t *testing.T) {
	handler, service, _ := testDashboardHandler()
	service.metricsValue = &dashboard.DashboardBlockDTO[dashboard.MetricsDTO]{
		Value:    dashboard.MetricsDTO{Pods: []dashboard.PodMetricDTO{}, TopCPU: []dashboard.MetricRankDTO{}, TopMemory: []dashboard.MetricRankDTO{}},
		Coverage: &dashboard.CoverageDTO{RequestedNamespaces: 2, DeniedNamespaces: []string{}, Failed: []dashboard.PartialError{}},
		Errors:   []dashboard.PartialError{{Code: dashboard.CodeFeatureUnavailable, Message: "safe"}},
	}
	recorder := httptest.NewRecorder()
	handler.Metrics(recorder, dashboardRequest(http.MethodGet, "/api/v1/metrics", ""))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), dashboard.CodeFeatureUnavailable) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	problem := dashboard.ProblemPodDTO{Namespace: "payments", Pod: "api", Severity: dashboard.ProblemCritical}
	service.problemsValue = &dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO]{
		Value: []dashboard.ProblemPodDTO{problem}, Coverage: &dashboard.CoverageDTO{RequestedNamespaces: 2},
		Errors: []dashboard.PartialError{{Namespace: "other", Code: dashboard.CodeForbidden, Message: "safe"}},
	}
	recorder = httptest.NewRecorder()
	handler.Problems(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/problems", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("partial status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	service.problemsValue.Value = []dashboard.ProblemPodDTO{}
	recorder = httptest.NewRecorder()
	handler.Problems(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/problems", ""))
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), api.CodeForbidden) {
		t.Fatalf("total denial status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	service.problemsValue.Errors = []dashboard.PartialError{{Code: dashboard.CodeAuthenticationUnavailable, Message: "safe"}}
	recorder = httptest.NewRecorder()
	handler.Problems(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/problems", ""))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), api.CodeAuthenticationUnavailable) {
		t.Fatalf("authentication status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDashboardGenerationFenceWinsOverAStaleTotalFailure(t *testing.T) {
	handler, service, selection := testDashboardHandler()
	service.metricsValue = &dashboard.DashboardBlockDTO[dashboard.MetricsDTO]{
		Value:  dashboard.MetricsDTO{Pods: []dashboard.PodMetricDTO{}, TopCPU: []dashboard.MetricRankDTO{}, TopMemory: []dashboard.MetricRankDTO{}},
		Errors: []dashboard.PartialError{{Code: dashboard.CodeFeatureUnavailable, Message: "safe"}},
	}
	selection.mu.Lock()
	selection.rejectFence = true
	selection.mu.Unlock()
	recorder := httptest.NewRecorder()
	handler.Metrics(recorder, dashboardRequest(http.MethodGet, "/api/v1/metrics", ""))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), api.CodeGenerationChanged) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDashboardEventsDefaultToWarningsAndAllowClosedExplicitTypes(t *testing.T) {
	handler, service, _ := testDashboardHandler()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	grouped := dashboard.GroupEvents([]dashboard.NormalizedEvent{
		{UID: "normal", Namespace: "payments", RegardingKind: "Pod", RegardingName: "api", RegardingUID: "normal-pod", Type: "Normal", Reason: "Pulled", Message: "image", Count: 1, ObservedAt: now},
		{UID: "warning", Namespace: "payments", RegardingKind: "Pod", RegardingName: "api", RegardingUID: "warning-pod", Type: "Warning", Reason: "BackOff", Message: "retry", Count: 2, ObservedAt: now.Add(-time.Minute)},
		{UID: "unknown", Namespace: "payments", RegardingKind: "Pod", RegardingName: "api", RegardingUID: "unknown-pod", Type: "vendor", Reason: "Odd", Message: "custom", Count: 3, ObservedAt: now.Add(-2 * time.Minute)},
	})
	service.eventsValue = &dashboard.DashboardBlockDTO[[]dashboard.EventDTO]{Value: grouped, Complete: true, Errors: []dashboard.PartialError{}}

	recorder := httptest.NewRecorder()
	handler.Events(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/events", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("default status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data dashboard.DashboardBlockDTO[[]dashboard.EventDTO] `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Value) != 1 || response.Data.Value[0].Type != "Warning" {
		t.Fatalf("default event filter = %+v", response.Data.Value)
	}

	recorder = httptest.NewRecorder()
	handler.Events(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/events?status=Normal&status=Unknown", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("explicit status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Value) != 2 || response.Data.Value[0].Type != "Normal" || response.Data.Value[1].Type != "Unknown" {
		t.Fatalf("explicit event filter = %+v", response.Data.Value)
	}
}

func TestDashboardEventCursorUsesHiddenKubernetesIdentity(t *testing.T) {
	handler, service, _ := testDashboardHandler()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	service.eventsValue = &dashboard.DashboardBlockDTO[[]dashboard.EventDTO]{
		Value: dashboard.GroupEvents([]dashboard.NormalizedEvent{
			{UID: "event-a", Namespace: "payments", RegardingKind: "Pod", RegardingName: "api", RegardingUID: "pod-a", Type: "Warning", Reason: "BackOff", Message: "same", Count: 1, Source: "kubelet", ObservedAt: now},
			{UID: "event-b", Namespace: "payments", RegardingKind: "Pod", RegardingName: "api", RegardingUID: "pod-b", Type: "Warning", Reason: "BackOff", Message: "same", Count: 1, Source: "kubelet", ObservedAt: now},
			{UID: "event-c", Namespace: "payments", RegardingKind: "Pod", RegardingName: "api", RegardingUID: "pod-c", Type: "Warning", Reason: "BackOff", Message: "same", Count: 1, Source: "kubelet", ObservedAt: now},
		}),
		Complete: true,
		Errors:   []dashboard.PartialError{},
	}
	recorder := httptest.NewRecorder()
	handler.Events(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/events?limit=2", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var first struct {
		Data dashboard.DashboardBlockDTO[[]dashboard.EventDTO] `json:"data"`
		Meta dashboardMeta                                     `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Data.Value) != 2 || first.Meta.Page == nil || first.Meta.Page.Next == "" {
		t.Fatalf("first event page = %+v meta=%+v", first.Data.Value, first.Meta.Page)
	}
	recorder = httptest.NewRecorder()
	handler.Events(recorder, dashboardRequest(http.MethodGet, "/api/v1/dashboard/events?limit=2&continue="+url.QueryEscape(first.Meta.Page.Next), ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("second status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var second struct {
		Data dashboard.DashboardBlockDTO[[]dashboard.EventDTO] `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Data.Value) != 1 {
		t.Fatalf("duplicate or missing event at cursor boundary: %+v", second.Data.Value)
	}
}

func TestDashboardDescendingSortKeepsCanonicalTieBreakAscending(t *testing.T) {
	t.Parallel()
	problems := dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO]{Value: []dashboard.ProblemPodDTO{
		{Namespace: "zeta", Pod: "pod", Severity: dashboard.ProblemCritical},
		{Namespace: "alpha", Pod: "pod", Severity: dashboard.ProblemCritical},
	}}
	filterProblems(&problems, dashboardListQuery{sort: "severity", order: "desc", statuses: map[string]struct{}{}})
	if problems.Value[0].Namespace != "alpha" {
		t.Fatalf("problem tie-break was reversed: %+v", problems.Value)
	}

	timestamp := "2026-08-10T12:00:00Z"
	events := dashboard.DashboardBlockDTO[[]dashboard.EventDTO]{Value: []dashboard.EventDTO{
		{Timestamp: &timestamp, Namespace: "zeta", Type: "Warning"},
		{Timestamp: &timestamp, Namespace: "alpha", Type: "Warning"},
	}}
	filterEvents(&events, dashboardListQuery{sort: "timestamp", order: "desc", statuses: map[string]struct{}{"Warning": {}}})
	if events.Value[0].Namespace != "alpha" {
		t.Fatalf("event tie-break was reversed: %+v", events.Value)
	}

	metrics := dashboard.DashboardBlockDTO[dashboard.MetricsDTO]{Value: dashboard.MetricsDTO{Pods: []dashboard.PodMetricDTO{
		{Namespace: "zeta", Pod: "pod", CPUMillicores: 100},
		{Namespace: "alpha", Pod: "pod", CPUMillicores: 100},
	}}}
	filterMetrics(&metrics, dashboardListQuery{sort: "cpu", order: "desc"})
	if metrics.Value.Pods[0].Namespace != "alpha" {
		t.Fatalf("metric tie-break was reversed: %+v", metrics.Value.Pods)
	}
}
