package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

const maximumDashboardBodyBytes = 1 << 20

type DashboardService interface {
	Summary(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) dashboard.DashboardBlockDTO[dashboard.SummaryDTO]
	NamespaceHealth(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.NamespaceHealthDTO]
	Problems(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO]
	Restarts(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, int) dashboard.DashboardBlockDTO[[]dashboard.RestartDTO]
	Events(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) dashboard.DashboardBlockDTO[[]dashboard.EventDTO]
	ScanLogs(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, dashboard.LogScanRequest) dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]
	Metrics(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution) dashboard.DashboardBlockDTO[dashboard.MetricsDTO]
}

type Dashboard struct {
	service   DashboardService
	selection SelectionReader
	cursors   *api.CursorCodec
	now       func() time.Time
}

func NewDashboard(service DashboardService, selection SelectionReader, cursors ...*api.CursorCodec) *Dashboard {
	var codec *api.CursorCodec
	if len(cursors) > 0 {
		codec = cursors[0]
	}
	return &Dashboard{service: service, selection: selection, cursors: codec, now: time.Now}
}

func (handler *Dashboard) Summary(w http.ResponseWriter, r *http.Request) {
	if err := rejectDashboardQuery(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	result := handler.service.Summary(r.Context(), binding, resolution)
	handler.writeResult(w, r, binding, resolution, result, nil)
}

func (handler *Dashboard) NamespaceHealth(w http.ResponseWriter, r *http.Request) {
	if err := rejectDashboardQuery(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	result := handler.service.NamespaceHealth(r.Context(), binding, resolution)
	handler.writeResult(w, r, binding, resolution, result, nil)
}

func (handler *Dashboard) Problems(w http.ResponseWriter, r *http.Request) {
	query, err := decodeDashboardListQuery(r, dashboardQueryProblems)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err := narrowDashboardNamespaces(&resolution, query.namespaces); err != nil {
		api.WriteError(w, r, err)
		return
	}
	cursor, err := handler.decodeDashboardCursor(query, binding, resolution)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	result := handler.service.Problems(r.Context(), binding, resolution)
	filterProblems(&result, query)
	page, err := paginateDashboardBlock(handler, &result, query, binding, resolution, cursor, problemCursorIdentity)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	handler.writeResult(w, r, binding, resolution, result, page)
}

func (handler *Dashboard) Restarts(w http.ResponseWriter, r *http.Request) {
	query, err := decodeDashboardListQuery(r, dashboardQueryRestarts)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err := narrowDashboardNamespaces(&resolution, query.namespaces); err != nil {
		api.WriteError(w, r, err)
		return
	}
	result := handler.service.Restarts(r.Context(), binding, resolution, query.limit)
	handler.writeResult(w, r, binding, resolution, result, nil)
}

func (handler *Dashboard) Events(w http.ResponseWriter, r *http.Request) {
	query, err := decodeDashboardListQuery(r, dashboardQueryEvents)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err := narrowDashboardNamespaces(&resolution, query.namespaces); err != nil {
		api.WriteError(w, r, err)
		return
	}
	cursor, err := handler.decodeDashboardCursor(query, binding, resolution)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	result := handler.service.Events(r.Context(), binding, resolution)
	filterEvents(&result, query)
	page, err := paginateDashboardBlock(handler, &result, query, binding, resolution, cursor, eventCursorIdentity)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	handler.writeResult(w, r, binding, resolution, result, page)
}

func (handler *Dashboard) LogScan(w http.ResponseWriter, r *http.Request) {
	if err := rejectDashboardQuery(r); err != nil {
		api.WriteError(w, r, err)
		return
	}
	request, err := decodeLogScanRequest(w, r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	result := handler.service.ScanLogs(r.Context(), binding, resolution, request)
	handler.writeResult(w, r, binding, resolution, result, nil)
}

func (handler *Dashboard) Metrics(w http.ResponseWriter, r *http.Request) {
	query, err := decodeDashboardListQuery(r, dashboardQueryMetrics)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err := narrowDashboardNamespaces(&resolution, query.namespaces); err != nil {
		api.WriteError(w, r, err)
		return
	}
	cursor, err := handler.decodeDashboardCursor(query, binding, resolution)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	result := handler.service.Metrics(r.Context(), binding, resolution)
	filterMetrics(&result, query)
	podsBlock := dashboard.DashboardBlockDTO[[]dashboard.PodMetricDTO]{
		Value: result.Value.Pods, Complete: result.Complete, Truncated: result.Truncated,
		Coverage: result.Coverage, Errors: result.Errors,
	}
	page, err := paginateDashboardBlock(handler, &podsBlock, query, binding, resolution, cursor, metricCursorIdentity)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	result.Value.Pods = podsBlock.Value
	result.Complete, result.Truncated = podsBlock.Complete, podsBlock.Truncated
	rebuildMetricRanks(&result.Value)
	handler.writeResult(w, r, binding, resolution, result, page)
}

func (handler *Dashboard) activeSelection() (namespaces.SelectionBinding, namespaces.ScopeResolution, error) {
	if handler == nil || handler.service == nil || handler.selection == nil {
		return namespaces.SelectionBinding{}, namespaces.ScopeResolution{}, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeClusterUnavailable, "The dashboard is temporarily unavailable.", nil, nil)
	}
	binding, resolution := handler.selection.Snapshot()
	if binding.ClusterProfileID <= 0 || binding.Context == "" || binding.Generation == "" {
		return binding, resolution, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "No active Kubernetes selection is available.", nil, nil)
	}
	return binding, resolution, nil
}

type dashboardEnvelope struct {
	Data any           `json:"data"`
	Meta dashboardMeta `json:"meta"`
}

type dashboardMeta struct {
	RequestID   string                 `json:"requestId"`
	Generation  string                 `json:"generation"`
	CollectedAt string                 `json:"collectedAt"`
	Selection   dashboard.SelectionDTO `json:"selection"`
	Page        *dashboardPageMeta     `json:"page,omitempty"`
}

type dashboardPageMeta struct {
	Limit       int    `json:"limit"`
	Next        string `json:"next"`
	Complete    bool   `json:"complete"`
	Truncated   bool   `json:"truncated"`
	FilterScope string `json:"filterScope"`
}

func (handler *Dashboard) writeResult(w http.ResponseWriter, r *http.Request, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, value any, page *dashboardPageMeta) {
	current, _ := handler.selection.Snapshot()
	if !sameSelectionBinding(current, binding) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed during collection.", nil, nil))
		return
	}
	if err := totalDashboardFailure(value); err != nil {
		handler.writeIfCurrent(w, r, binding, func() { api.WriteError(w, r, err) })
		return
	}
	collectedAt := handler.now().UTC()
	selection := dashboard.Selection{
		Generation: binding.Generation, Context: binding.Context, Cluster: binding.Cluster,
		Scope: resolution.ScopeName, Namespaces: append([]string(nil), resolution.Namespaces...),
	}
	if selection.Scope == "" {
		selection.Scope = resolution.ScopeSource
	}
	envelope := dashboardEnvelope{Data: value, Meta: dashboardMeta{
		RequestID: api.RequestIDFromContext(r.Context()), Generation: binding.Generation,
		CollectedAt: collectedAt.Format(time.RFC3339Nano), Selection: selection.DTO(collectedAt), Page: page,
	}}
	payload, err := json.Marshal(envelope)
	if err != nil {
		handler.writeIfCurrent(w, r, binding, func() {
			api.WriteError(w, r, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "The dashboard response could not be encoded.", nil, err))
		})
		return
	}
	write := func() {
		noStore(w)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}
	handler.writeIfCurrent(w, r, binding, write)
}

func (handler *Dashboard) writeIfCurrent(w http.ResponseWriter, r *http.Request, binding namespaces.SelectionBinding, write func()) {
	if fenced, ok := handler.selection.(interface {
		IfCurrent(namespaces.SelectionBinding, func()) bool
	}); ok {
		if !fenced.IfCurrent(binding, write) {
			api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed before the dashboard response was published.", nil, nil))
		}
		return
	}
	current, _ := handler.selection.Snapshot()
	if !sameSelectionBinding(current, binding) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed before the dashboard response was published.", nil, nil))
		return
	}
	write()
}

func totalDashboardFailure(value any) error {
	var errorsList []dashboard.PartialError
	var coverage *dashboard.CoverageDTO
	hasValue := false
	switch block := value.(type) {
	case dashboard.DashboardBlockDTO[dashboard.SummaryDTO]:
		errorsList, coverage = block.Errors, block.Coverage
		hasValue = summaryHasValue(block.Value)
	case dashboard.DashboardBlockDTO[[]dashboard.NamespaceHealthDTO]:
		errorsList, coverage, hasValue = block.Errors, block.Coverage, len(block.Value) > 0
	case dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO]:
		errorsList, coverage, hasValue = block.Errors, block.Coverage, len(block.Value) > 0
	case dashboard.DashboardBlockDTO[[]dashboard.RestartDTO]:
		errorsList, coverage, hasValue = block.Errors, block.Coverage, len(block.Value) > 0
	case dashboard.DashboardBlockDTO[[]dashboard.EventDTO]:
		errorsList, coverage, hasValue = block.Errors, block.Coverage, len(block.Value) > 0
	case dashboard.DashboardBlockDTO[[]dashboard.LogMatchDTO]:
		errorsList, coverage, hasValue = block.Errors, block.Coverage, len(block.Value) > 0
	case dashboard.DashboardBlockDTO[dashboard.MetricsDTO]:
		errorsList, coverage, hasValue = block.Errors, block.Coverage, len(block.Value.Pods) > 0
	default:
		return api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "The dashboard response is invalid.", nil, nil)
	}
	if len(errorsList) == 0 || hasValue || coverage != nil && coverage.CompletedNamespaces > 0 {
		return nil
	}
	allForbidden := true
	codes := make(map[string]struct{}, len(errorsList))
	for _, item := range errorsList {
		codes[item.Code] = struct{}{}
		allForbidden = allForbidden && item.Code == dashboard.CodeForbidden
	}
	if allForbidden {
		return api.NewHTTPError(http.StatusForbidden, api.CodeForbidden, "Kubernetes denied the requested dashboard collection.", nil, nil)
	}
	if _, ok := codes[dashboard.CodeUpstreamTimeout]; ok {
		return api.NewHTTPError(http.StatusGatewayTimeout, dashboard.CodeUpstreamTimeout, "Dashboard collection timed out.", nil, nil)
	}
	if _, ok := codes[dashboard.CodeFeatureUnavailable]; ok {
		return api.NewHTTPError(http.StatusServiceUnavailable, dashboard.CodeFeatureUnavailable, "The optional Kubernetes feature is unavailable.", nil, nil)
	}
	if _, ok := codes[dashboard.CodeAuthorizationUnavailable]; ok {
		return api.NewHTTPError(http.StatusServiceUnavailable, api.CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil, nil)
	}
	if _, ok := codes[dashboard.CodeAuthenticationUnavailable]; ok {
		return api.NewHTTPError(http.StatusServiceUnavailable, api.CodeAuthenticationUnavailable, "Kubernetes authentication is unavailable.", nil, nil)
	}
	if _, ok := codes[dashboard.CodeValidationFailed]; ok {
		return validationHTTPError("The dashboard request is invalid.", nil)
	}
	return api.NewHTTPError(http.StatusServiceUnavailable, api.CodeClusterUnavailable, "The Kubernetes API is temporarily unavailable.", nil, nil)
}

func summaryHasValue(value dashboard.SummaryDTO) bool {
	for _, counter := range []dashboard.CounterDTO{
		value.Namespaces, value.PodsTotal, value.PodsHealthy, value.PodsProblematic,
		value.WorkloadsDegraded, value.Restarts, value.WarningEvents, value.PossibleLogMatches,
	} {
		if counter.Value != nil {
			return true
		}
	}
	return false
}

func decodeLogScanRequest(w http.ResponseWriter, r *http.Request) (dashboard.LogScanRequest, error) {
	limited := http.MaxBytesReader(w, r.Body, maximumDashboardBodyBytes)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	var request dashboard.LogScanRequest
	if err := decoder.Decode(&request); err != nil {
		var tooLarge *http.MaxBytesError
		var typeError *json.UnmarshalTypeError
		switch {
		case errors.As(err, &tooLarge):
			return dashboard.LogScanRequest{}, api.NewHTTPError(http.StatusRequestEntityTooLarge, api.CodeBodyTooLarge, "The request body is too large.", nil, nil)
		case isUnknownJSONField(err):
			return dashboard.LogScanRequest{}, api.NewHTTPError(http.StatusBadRequest, api.CodeUnknownField, "The JSON body contains an unknown field.", nil, nil)
		case errors.As(err, &typeError):
			return dashboard.LogScanRequest{}, validationHTTPError("The log scan request contains an invalid value type.", nil)
		default:
			return dashboard.LogScanRequest{}, api.NewHTTPError(http.StatusBadRequest, api.CodeInvalidJSON, "The JSON body is not valid.", nil, nil)
		}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return dashboard.LogScanRequest{}, api.NewHTTPError(http.StatusBadRequest, api.CodeInvalidJSON, "The JSON body contains trailing content.", nil, nil)
	}
	if _, err := dashboard.ResolveLogScanRequest(request); err != nil {
		return dashboard.LogScanRequest{}, validationHTTPError("The log scan request is outside the supported limits.", nil)
	}
	return request, nil
}

func rejectDashboardQuery(r *http.Request) error {
	if r.URL.RawQuery != "" {
		return validationHTTPError("This dashboard route does not accept query parameters.", nil)
	}
	return nil
}

type dashboardQueryKind int

const (
	dashboardQueryProblems dashboardQueryKind = iota
	dashboardQueryRestarts
	dashboardQueryEvents
	dashboardQueryMetrics
)

type dashboardListQuery struct {
	limit      int
	namespaces []string
	search     string
	statuses   map[string]struct{}
	sort       string
	order      string
	continueAt string
	kind       dashboardQueryKind
}

func decodeDashboardListQuery(r *http.Request, kind dashboardQueryKind) (dashboardListQuery, error) {
	result := dashboardListQuery{limit: 100, statuses: make(map[string]struct{}), kind: kind}
	if kind == dashboardQueryRestarts {
		result.limit = dashboard.DefaultRestartLimit
	}
	query := r.URL.Query()
	for key, values := range query {
		if !dashboardQueryFieldAllowed(kind, key) {
			return dashboardListQuery{}, validationHTTPError("The dashboard query contains an unknown field.", nil)
		}
		if len(values) == 0 {
			return dashboardListQuery{}, validationHTTPError("Dashboard query values must be non-empty.", nil)
		}
		if key != "namespace" && key != "status" && len(values) != 1 {
			return dashboardListQuery{}, validationHTTPError("Dashboard query values must be unique.", nil)
		}
		for _, value := range values {
			if value == "" {
				return dashboardListQuery{}, validationHTTPError("Dashboard query values must be non-empty.", nil)
			}
		}
	}
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		maximum := 500
		if kind == dashboardQueryRestarts {
			maximum = dashboard.MaximumRestartLimit
		}
		if err != nil || value < 1 || value > maximum {
			return dashboardListQuery{}, validationHTTPError("limit is outside the supported range.", nil)
		}
		result.limit = value
	}
	result.namespaces = append([]string(nil), query["namespace"]...)
	result.continueAt = query.Get("continue")
	if len(result.namespaces) > 100 {
		return dashboardListQuery{}, validationHTTPError("namespace accepts at most 100 values.", nil)
	}
	result.search = query.Get("search")
	if len(result.search) > 256 {
		return dashboardListQuery{}, validationHTTPError("search must be at most 256 bytes.", nil)
	}
	for _, status := range query["status"] {
		if !dashboardStatusAllowed(kind, status) {
			return dashboardListQuery{}, validationHTTPError("status is not supported by this dashboard route.", nil)
		}
		result.statuses[status] = struct{}{}
	}
	if kind == dashboardQueryEvents && len(result.statuses) == 0 {
		result.statuses["Warning"] = struct{}{}
	}
	result.sort = query.Get("sort")
	result.order = query.Get("order")
	if result.sort == "" {
		switch kind {
		case dashboardQueryProblems:
			result.sort, result.order = "severity", "desc"
		case dashboardQueryEvents:
			result.sort, result.order = "timestamp", "desc"
		case dashboardQueryMetrics:
			result.sort, result.order = "cpu", "desc"
		}
	}
	if result.order == "" {
		result.order = "asc"
	}
	if result.order != "asc" && result.order != "desc" {
		return dashboardListQuery{}, validationHTTPError("order must be asc or desc.", nil)
	}
	if !dashboardSortAllowed(kind, result.sort) {
		return dashboardListQuery{}, validationHTTPError("sort is not supported by this dashboard route.", nil)
	}
	return result, nil
}

func dashboardQueryFieldAllowed(kind dashboardQueryKind, field string) bool {
	if field == "limit" || field == "namespace" {
		return true
	}
	if kind == dashboardQueryRestarts {
		return false
	}
	return field == "search" || field == "status" || field == "sort" || field == "order" || field == "continue"
}

func dashboardStatusAllowed(kind dashboardQueryKind, value string) bool {
	switch kind {
	case dashboardQueryProblems:
		return value == "info" || value == "warning" || value == "critical"
	case dashboardQueryEvents:
		return value == "Normal" || value == "Warning" || value == "Unknown"
	default:
		return false
	}
}

func dashboardSortAllowed(kind dashboardQueryKind, value string) bool {
	switch kind {
	case dashboardQueryProblems:
		return value == "severity" || value == "age" || value == "identity"
	case dashboardQueryEvents:
		return value == "timestamp" || value == "count" || value == "identity"
	case dashboardQueryMetrics:
		return value == "cpu" || value == "memory" || value == "identity"
	case dashboardQueryRestarts:
		return value == ""
	default:
		return false
	}
}

func narrowDashboardNamespaces(resolution *namespaces.ScopeResolution, requested []string) error {
	if len(requested) == 0 {
		return nil
	}
	active := make(map[string]struct{}, len(resolution.Namespaces))
	for _, namespace := range resolution.Namespaces {
		active[namespace] = struct{}{}
	}
	seen := make(map[string]struct{}, len(requested))
	values := make([]string, 0, len(requested))
	for _, namespace := range requested {
		if !namespaces.ValidNamespaceName(namespace) {
			return validationHTTPError("namespace contains an invalid Kubernetes namespace.", nil)
		}
		if _, ok := active[namespace]; !ok {
			return validationHTTPError("namespace must belong to the active scope.", nil)
		}
		if _, duplicate := seen[namespace]; duplicate {
			return validationHTTPError("namespace values must be distinct.", nil)
		}
		seen[namespace] = struct{}{}
		values = append(values, namespace)
	}
	resolution.Namespaces = values
	return nil
}

func filterProblems(block *dashboard.DashboardBlockDTO[[]dashboard.ProblemPodDTO], query dashboardListQuery) {
	values := make([]dashboard.ProblemPodDTO, 0, len(block.Value))
	for _, item := range block.Value {
		if len(query.statuses) > 0 {
			if _, ok := query.statuses[string(item.Severity)]; !ok {
				continue
			}
		}
		if query.search != "" && !containsFolded(query.search, item.Pod, optionalString(item.Container), optionalString(item.Reason), optionalString(item.Message)) {
			continue
		}
		values = append(values, item)
	}
	block.Value = values
	sort.SliceStable(block.Value, func(left, right int) bool {
		return compareProblems(block.Value[left], block.Value[right], query.sort, query.order) < 0
	})
}

func compareProblems(left, right dashboard.ProblemPodDTO, field, order string) int {
	comparison := 0
	switch field {
	case "severity":
		leftRank, rightRank := problemSeverityRank(left.Severity), problemSeverityRank(right.Severity)
		if leftRank != rightRank {
			comparison = leftRank - rightRank
		}
	case "age":
		if left.AgeSeconds != right.AgeSeconds {
			if left.AgeSeconds < right.AgeSeconds {
				comparison = -1
			} else {
				comparison = 1
			}
		}
	}
	if comparison != 0 {
		if order == "desc" {
			return -comparison
		}
		return comparison
	}
	comparison = strings.Compare(left.Namespace+"\x00"+left.Pod+"\x00"+optionalString(left.Container), right.Namespace+"\x00"+right.Pod+"\x00"+optionalString(right.Container))
	if field == "identity" && order == "desc" {
		return -comparison
	}
	return comparison
}

func filterEvents(block *dashboard.DashboardBlockDTO[[]dashboard.EventDTO], query dashboardListQuery) {
	values := make([]dashboard.EventDTO, 0, len(block.Value))
	for _, item := range block.Value {
		if len(query.statuses) > 0 {
			if _, ok := query.statuses[item.Type]; !ok {
				continue
			}
		}
		if query.search != "" && !containsFolded(query.search, item.ObjectKind, item.ObjectName, item.Reason, item.Message) {
			continue
		}
		values = append(values, item)
	}
	block.Value = values
	sort.SliceStable(block.Value, func(left, right int) bool {
		return compareEvents(block.Value[left], block.Value[right], query.sort, query.order) < 0
	})
}

func compareEvents(left, right dashboard.EventDTO, field, order string) int {
	comparison := 0
	switch field {
	case "timestamp":
		comparison = strings.Compare(optionalString(left.Timestamp), optionalString(right.Timestamp))
	case "count":
		if left.Count != right.Count {
			if left.Count < right.Count {
				comparison = -1
			} else {
				comparison = 1
			}
		}
	}
	if comparison != 0 {
		if order == "desc" {
			return -comparison
		}
		return comparison
	}
	comparison = compareCanonicalEvents(left, right)
	if field == "identity" && order == "desc" {
		return -comparison
	}
	return comparison
}

func compareCanonicalEvents(left, right dashboard.EventDTO) int {
	if comparison := strings.Compare(optionalString(left.Timestamp), optionalString(right.Timestamp)); comparison != 0 {
		return -comparison
	}
	if comparison := strings.Compare(left.Namespace, right.Namespace); comparison != 0 {
		return comparison
	}
	return strings.Compare(dashboard.EventCursorIdentity(left), dashboard.EventCursorIdentity(right))
}

func filterMetrics(block *dashboard.DashboardBlockDTO[dashboard.MetricsDTO], query dashboardListQuery) {
	values := make([]dashboard.PodMetricDTO, 0, len(block.Value.Pods))
	for _, item := range block.Value.Pods {
		if query.search != "" && !containsFolded(query.search, item.Pod) {
			matched := false
			for _, container := range item.Containers {
				matched = matched || containsFolded(query.search, container.Name)
			}
			if !matched {
				continue
			}
		}
		values = append(values, item)
	}
	block.Value.Pods = values
	sort.SliceStable(block.Value.Pods, func(left, right int) bool {
		return compareMetrics(block.Value.Pods[left], block.Value.Pods[right], query.sort, query.order) < 0
	})
}

func compareMetrics(left, right dashboard.PodMetricDTO, field, order string) int {
	var leftValue, rightValue int64
	switch field {
	case "cpu":
		leftValue, rightValue = left.CPUMillicores, right.CPUMillicores
	case "memory":
		leftValue, rightValue = left.MemoryBytes, right.MemoryBytes
	}
	if leftValue != rightValue {
		comparison := 1
		if leftValue < rightValue {
			comparison = -1
		}
		if order == "desc" {
			return -comparison
		}
		return comparison
	}
	comparison := strings.Compare(left.Namespace+"\x00"+left.Pod, right.Namespace+"\x00"+right.Pod)
	if field == "identity" && order == "desc" {
		return -comparison
	}
	return comparison
}

func rebuildMetricRanks(value *dashboard.MetricsDTO) {
	ranks := make([]dashboard.MetricRankDTO, 0, len(value.Pods))
	for _, pod := range value.Pods {
		ranks = append(ranks, dashboard.MetricRankDTO{Namespace: pod.Namespace, Pod: pod.Pod, CPUMillicores: pod.CPUMillicores, MemoryBytes: pod.MemoryBytes})
	}
	value.TopCPU = append([]dashboard.MetricRankDTO(nil), ranks...)
	sort.Slice(value.TopCPU, func(left, right int) bool {
		if value.TopCPU[left].CPUMillicores != value.TopCPU[right].CPUMillicores {
			return value.TopCPU[left].CPUMillicores > value.TopCPU[right].CPUMillicores
		}
		return value.TopCPU[left].Namespace+"\x00"+value.TopCPU[left].Pod < value.TopCPU[right].Namespace+"\x00"+value.TopCPU[right].Pod
	})
	value.TopMemory = append([]dashboard.MetricRankDTO(nil), ranks...)
	sort.Slice(value.TopMemory, func(left, right int) bool {
		if value.TopMemory[left].MemoryBytes != value.TopMemory[right].MemoryBytes {
			return value.TopMemory[left].MemoryBytes > value.TopMemory[right].MemoryBytes
		}
		return value.TopMemory[left].Namespace+"\x00"+value.TopMemory[left].Pod < value.TopMemory[right].Namespace+"\x00"+value.TopMemory[right].Pod
	})
	if len(value.TopCPU) > 10 {
		value.TopCPU = value.TopCPU[:10]
	}
	if len(value.TopMemory) > 10 {
		value.TopMemory = value.TopMemory[:10]
	}
}

func containsFolded(search string, values ...string) bool {
	foldedSearch := simpleFoldString(search)
	for _, value := range values {
		if strings.Contains(simpleFoldString(value), foldedSearch) {
			return true
		}
	}
	return false
}

func simpleFoldString(value string) string {
	return strings.Map(func(current rune) rune {
		minimum := current
		for next := unicode.SimpleFold(current); next != current; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		return minimum
	}, value)
}

type dashboardCursorState struct {
	Anchor string `json:"anchor"`
}

func (handler *Dashboard) decodeDashboardCursor(query dashboardListQuery, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) (dashboardCursorState, error) {
	state := dashboardCursorState{}
	if query.continueAt == "" {
		return state, nil
	}
	if handler == nil || handler.cursors == nil {
		return state, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Dashboard pagination is temporarily unavailable.", nil, nil)
	}
	if err := handler.cursors.Decode(query.continueAt, dashboardCursorBinding(query, binding, resolution), &state); err != nil {
		return dashboardCursorState{}, err
	}
	if state.Anchor == "" {
		return dashboardCursorState{}, api.NewHTTPError(http.StatusBadRequest, api.CodeCursorInvalid, "The cursor is invalid.", nil, nil)
	}
	return state, nil
}

func paginateDashboardBlock[T any](
	handler *Dashboard,
	block *dashboard.DashboardBlockDTO[[]T],
	query dashboardListQuery,
	binding namespaces.SelectionBinding,
	resolution namespaces.ScopeResolution,
	state dashboardCursorState,
	identity func(T) string,
) (*dashboardPageMeta, error) {
	if handler == nil || handler.cursors == nil || identity == nil {
		return nil, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Dashboard pagination is temporarily unavailable.", nil, nil)
	}
	cursorBinding := dashboardCursorBinding(query, binding, resolution)
	start := 0
	if query.continueAt != "" {
		found := false
		for position, item := range block.Value {
			if identity(item) == state.Anchor {
				start = position + 1
				found = true
				break
			}
		}
		if !found {
			return nil, api.NewHTTPError(http.StatusBadRequest, api.CodeCursorMismatch, "The cursor boundary no longer exists in the current collection.", nil, nil)
		}
	}
	originalComplete := block.Complete
	originalTruncated := block.Truncated
	end := start + query.limit
	if end > len(block.Value) {
		end = len(block.Value)
	}
	next := ""
	if end < len(block.Value) {
		var err error
		next, err = handler.cursors.Encode(cursorBinding, dashboardCursorState{Anchor: identity(block.Value[end-1])})
		if err != nil {
			return nil, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Dashboard pagination is temporarily unavailable.", nil, err)
		}
	}
	paged := make([]T, 0, end-start)
	paged = append(paged, block.Value[start:end]...)
	block.Value = paged
	pageTruncated := originalTruncated || query.continueAt != "" || next != ""
	pageComplete := originalComplete && next == ""
	if query.continueAt != "" || next != "" {
		block.Complete = false
		block.Truncated = true
	}
	return &dashboardPageMeta{
		Limit: query.limit, Next: next, Complete: pageComplete,
		Truncated: pageTruncated, FilterScope: "page",
	}, nil
}

func problemCursorIdentity(value dashboard.ProblemPodDTO) string {
	return strings.Join([]string{value.Namespace, value.Pod, optionalString(value.Container), string(value.Source)}, "\x00")
}

func eventCursorIdentity(value dashboard.EventDTO) string {
	return dashboard.EventCursorIdentity(value)
}

func metricCursorIdentity(value dashboard.PodMetricDTO) string {
	return value.Namespace + "\x00" + value.Pod
}

func dashboardCursorBinding(query dashboardListQuery, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) api.CursorBinding {
	scope := resolution.ScopeName
	if scope == "" {
		scope = resolution.ScopeSource
	}
	return api.CursorBinding{
		QueryHash: api.HashCursorQuery(canonicalDashboardQuery(query)),
		Context:   binding.Context, Scope: scope, Generation: binding.Generation,
	}
}

func canonicalDashboardQuery(query dashboardListQuery) string {
	namespaceValues := append([]string(nil), query.namespaces...)
	statusValues := make([]string, 0, len(query.statuses))
	for value := range query.statuses {
		statusValues = append(statusValues, value)
	}
	sort.Strings(namespaceValues)
	sort.Strings(statusValues)
	return strings.Join([]string{
		strconv.Itoa(int(query.kind)), strconv.Itoa(query.limit), strings.Join(namespaceValues, ","),
		query.search, strings.Join(statusValues, ","), query.sort, query.order,
	}, "\x1f")
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func problemSeverityRank(value dashboard.ProblemSeverity) int {
	if value == dashboard.ProblemCritical {
		return 2
	}
	if value == dashboard.ProblemWarning {
		return 1
	}
	return 0
}
