package handlers

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/fvmoraes/ginger/pkg/response"
	"github.com/fvmoraes/ginger/pkg/router"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type NamespaceScopeService interface {
	List(context.Context, int64, string) ([]namespaces.Scope, error)
	Get(context.Context, int64) (namespaces.Scope, error)
	Validate(context.Context, namespaces.ScopeWriteRequest, bool) (namespaces.ValidationReport, error)
	Create(context.Context, namespaces.ScopeWriteRequest) (namespaces.Scope, error)
	Update(context.Context, int64, namespaces.ScopeWriteRequest) (namespaces.Scope, namespaces.SelectionResult, error)
	Delete(context.Context, int64, namespaces.ScopeDeleteRequest) (namespaces.SelectionResult, error)
	Select(context.Context, int64, namespaces.ScopeSelectRequest) (namespaces.ScopeResolution, namespaces.SelectionResult, error)
}

type NamespaceCatalog interface {
	List(context.Context, namespaces.SelectionBinding) ([]string, error)
}

type NamespaceScopes struct {
	service   NamespaceScopeService
	selection SelectionReader
	catalog   NamespaceCatalog
	snapshots api.SnapshotProvider
	cursors   *api.CursorCodec
}

type namespaceDTO struct {
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Selected bool   `json:"selected"`
}

type selectionDTO struct {
	ClusterProfileID int64                  `json:"clusterProfileId"`
	Context          string                 `json:"context"`
	Cluster          string                 `json:"cluster"`
	ScopeID          *int64                 `json:"scopeId"`
	ScopeName        *string                `json:"scopeName"`
	ScopeMode        *string                `json:"scopeMode"`
	ScopeSource      string                 `json:"scopeSource"`
	DefaultNamespace *string                `json:"defaultNamespace"`
	NamespaceCount   int                    `json:"namespaceCount"`
	Generation       string                 `json:"generation"`
	Components       selectionDTOComponents `json:"components"`
}

type selectionDTOComponents struct {
	Cluster api.ComponentState `json:"cluster"`
}

func NewNamespaceScopes(service NamespaceScopeService, selection SelectionReader, catalog NamespaceCatalog, snapshots ...api.SnapshotProvider) *NamespaceScopes {
	var provider api.SnapshotProvider
	if len(snapshots) > 0 {
		provider = snapshots[0]
	}
	return &NamespaceScopes{service: service, selection: selection, catalog: catalog, snapshots: provider}
}

func (h *NamespaceScopes) WithCursors(cursors *api.CursorCodec) *NamespaceScopes {
	h.cursors = cursors
	return h
}

type namespaceCollectionEnvelope struct {
	Data any                     `json:"data"`
	Meta namespaceCollectionMeta `json:"meta"`
}

type namespaceCollectionMeta struct {
	RequestID  string             `json:"requestId"`
	Generation string             `json:"generation"`
	Page       *dashboardPageMeta `json:"page,omitempty"`
}

type namespaceCursorState struct {
	Continue string `json:"continue,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

type scopeCursorState struct {
	SortValue string `json:"sortValue"`
	Name      string `json:"name"`
	ID        int64  `json:"id"`
}

type namespaceListQuery struct {
	limit      int
	continueAt string
	search     string
	namespaces []string
	statuses   []string
	sort       string
	order      string
}

type scopeListQuery struct {
	limit      int
	continueAt string
	search     string
	sort       string
	order      string
}

func (h *NamespaceScopes) ListNamespaces(w http.ResponseWriter, r *http.Request) {
	query, err := decodeNamespaceListQuery(r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := h.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err := validateNamespaceSelectionFilter(query, resolution); err != nil {
		api.WriteError(w, r, err)
		return
	}
	if h.catalog == nil {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeClusterUnavailable, "The namespace list is temporarily unavailable.", nil, nil))
		return
	}
	selected := make(map[string]struct{}, len(resolution.Namespaces))
	for _, namespace := range resolution.Namespaces {
		selected[namespace] = struct{}{}
	}
	cursorBinding := api.CursorBinding{
		QueryHash: api.HashCursorQuery(canonicalNamespaceQuery(query)), Context: binding.Context,
		Scope: namespaceScopeBinding(binding, resolution), Generation: binding.Generation,
	}
	state := namespaceCursorState{}
	if query.continueAt != "" {
		if h.cursors == nil {
			api.WriteError(w, r, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Namespace pagination is temporarily unavailable.", nil, nil))
			return
		}
		if err := h.cursors.Decode(query.continueAt, cursorBinding, &state); err != nil {
			api.WriteError(w, r, err)
			return
		}
	}

	records, nextState, err := h.namespacePage(r.Context(), binding, query, state)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	for _, namespace := range records {
		if !namespaces.ValidNamespaceName(namespace.Name) || !validNamespacePhase(namespace.Phase) {
			api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeClusterUnavailable, "The namespace list is temporarily unavailable.", nil, nil))
			return
		}
	}
	filterNamespaceRecords(&records, query)
	items := make([]namespaceDTO, 0, len(records))
	for _, namespace := range records {
		_, isSelected := selected[namespace.Name]
		items = append(items, namespaceDTO{Name: namespace.Name, Phase: namespace.Phase, Selected: isSelected})
	}
	next, err := h.encodeNamespaceCursor(cursorBinding, nextState)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	page := &dashboardPageMeta{Limit: query.limit, Next: next, Complete: next == "", Truncated: query.continueAt != "" || next != "", FilterScope: "page"}
	if !h.selectionMatches(binding) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed during namespace collection.", nil, nil))
		return
	}
	noStore(w)
	router.JSON(w, http.StatusOK, namespaceCollectionEnvelope{Data: items, Meta: namespaceCollectionMeta{
		RequestID: api.RequestIDFromContext(r.Context()), Generation: binding.Generation, Page: page,
	}})
}

func (h *NamespaceScopes) List(w http.ResponseWriter, r *http.Request) {
	binding, _, err := h.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	query, err := decodeScopeListQuery(r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	scopes, err := h.service.List(r.Context(), binding.ClusterProfileID, binding.Context)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	filtered := make([]namespaces.Scope, 0, len(scopes))
	for _, scope := range scopes {
		if query.search != "" && !containsFolded(query.search, scope.Name, scope.Context) {
			continue
		}
		filtered = append(filtered, scope)
	}
	sortScopes(filtered, query)
	cursorBinding := api.CursorBinding{
		QueryHash: api.HashCursorQuery(canonicalScopeQuery(query)), Context: binding.Context,
		Scope: strconv.FormatInt(binding.ActiveScopeID, 10), Generation: binding.Generation,
	}
	state := scopeCursorState{}
	if query.continueAt != "" {
		if h.cursors == nil {
			api.WriteError(w, r, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Scope pagination is temporarily unavailable.", nil, nil))
			return
		}
		if err := h.cursors.Decode(query.continueAt, cursorBinding, &state); err != nil {
			api.WriteError(w, r, err)
			return
		}
	}
	start, err := scopePageStart(filtered, query, state, query.continueAt != "")
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	end := min(start+query.limit, len(filtered))
	items := make([]namespaces.ScopeDTO, 0, end-start)
	for _, scope := range filtered[start:end] {
		items = append(items, namespaces.NewScopeDTO(scope))
	}
	next := ""
	if end < len(filtered) {
		if h.cursors == nil {
			api.WriteError(w, r, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Scope pagination is temporarily unavailable.", nil, nil))
			return
		}
		next, err = h.cursors.Encode(cursorBinding, newScopeCursorState(filtered[end-1], query))
		if err != nil {
			api.WriteError(w, r, api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Scope pagination is temporarily unavailable.", nil, err))
			return
		}
	}
	page := &dashboardPageMeta{Limit: query.limit, Next: next, Complete: next == "", Truncated: query.continueAt != "" || next != "", FilterScope: "collection"}
	if !h.selectionMatches(binding) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed during scope collection.", nil, nil))
		return
	}
	noStore(w)
	router.JSON(w, http.StatusOK, namespaceCollectionEnvelope{Data: items, Meta: namespaceCollectionMeta{
		RequestID: api.RequestIDFromContext(r.Context()), Generation: binding.Generation, Page: page,
	}})
}

func (h *NamespaceScopes) Get(w http.ResponseWriter, r *http.Request) {
	id, err := scopeID(r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	scope, err := h.service.Get(r.Context(), id)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	noStore(w)
	response.OK(w, namespaces.NewScopeDTO(scope))
}

func (h *NamespaceScopes) Validate(w http.ResponseWriter, r *http.Request) {
	request, err := namespaces.DecodeScopeWriteRequest(r.Body)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	binding, _, err := h.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	request, err = bindScopeWriteRequest(request, binding)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	report, err := h.service.Validate(r.Context(), request, true)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	noStore(w)
	response.OK(w, report)
}

func (h *NamespaceScopes) Create(w http.ResponseWriter, r *http.Request) {
	request, err := namespaces.DecodeScopeWriteRequest(r.Body)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	binding, _, err := h.activeSelection()
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	request, err = bindScopeWriteRequest(request, binding)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	scope, err := h.service.Create(r.Context(), request)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	noStore(w)
	response.Created(w, namespaces.NewScopeDTO(scope))
}

func (h *NamespaceScopes) Update(w http.ResponseWriter, r *http.Request) {
	id, err := scopeID(r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	request, err := namespaces.DecodeScopeWriteRequest(r.Body)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	scope, result, err := h.service.Update(r.Context(), id, request)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	if result.Changed {
		_, summary := h.selectionResult(r.Context(), result)
		if !h.publishSelection(result.Binding, summary) {
			api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed after this scope update.", nil, nil))
			return
		}
	}
	noStore(w)
	router.JSON(w, http.StatusOK, namespaceCollectionEnvelope{Data: namespaces.NewScopeDTO(scope), Meta: namespaceCollectionMeta{
		RequestID: api.RequestIDFromContext(r.Context()), Generation: result.Generation,
	}})
}

func (h *NamespaceScopes) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := scopeID(r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	request, err := namespaces.DecodeScopeDeleteRequest(r.Body)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	result, err := h.service.Delete(r.Context(), id, request)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	if !result.Changed {
		response.NoContent(w)
		return
	}
	dto, summary := h.selectionResult(r.Context(), result)
	if !h.publishSelection(result.Binding, summary) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed after this scope deletion.", nil, nil))
		return
	}
	noStore(w)
	response.OK(w, dto)
}

func (h *NamespaceScopes) Select(w http.ResponseWriter, r *http.Request) {
	id, err := scopeID(r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	request, err := namespaces.DecodeScopeSelectRequest(r.Body)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	_, result, err := h.service.Select(r.Context(), id, request)
	if err != nil {
		api.WriteError(w, r, namespaceHTTPError(err))
		return
	}
	dto, summary := h.selectionResult(r.Context(), result)
	if !h.publishSelection(result.Binding, summary) {
		api.WriteError(w, r, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed after this scope selection.", nil, nil))
		return
	}
	noStore(w)
	response.OK(w, dto)
}

func (h *NamespaceScopes) activeSelection() (namespaces.SelectionBinding, namespaces.ScopeResolution, error) {
	binding, resolution := h.selection.Snapshot()
	if binding.ClusterProfileID <= 0 || binding.Context == "" || binding.Generation == "" {
		return binding, resolution, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "No active Kubernetes selection is available.", nil, nil)
	}
	return binding, resolution, nil
}

func (h *NamespaceScopes) selectionResult(ctx context.Context, result namespaces.SelectionResult) (selectionDTO, api.SelectionSummary) {
	binding, resolution := result.Binding, result.Resolution
	if binding.Generation == "" {
		binding.Generation = result.Generation
	}
	var scopeID *int64
	if resolution.ScopeID > 0 {
		value := resolution.ScopeID
		scopeID = &value
	}
	var scopeName *string
	if resolution.ScopeName != "" {
		value := resolution.ScopeName
		scopeName = &value
	}
	var scopeMode *string
	if resolution.ScopeMode != "" {
		value := string(resolution.ScopeMode)
		scopeMode = &value
	}
	clusterState := api.ComponentState{Status: api.StatusUnknown, Code: "NOT_CHECKED", Message: "The cluster has not been checked."}
	if h.snapshots != nil {
		if snapshot, err := h.snapshots.Snapshot(ctx); err == nil {
			clusterState = snapshot.Components.Cluster
		}
	}
	source := resolution.ScopeSource
	if source == "" {
		source = "none"
	}
	summary := api.SelectionSummary{
		ClusterProfileID: binding.ClusterProfileID, Context: binding.Context, Cluster: binding.Cluster,
		ScopeID: scopeID, ScopeName: scopeName, ScopeMode: scopeMode, ScopeSource: source,
		DefaultNamespace: resolution.DefaultNamespace, NamespaceCount: len(resolution.Namespaces), Generation: binding.Generation,
	}
	return selectionDTO{
		ClusterProfileID: binding.ClusterProfileID, Context: binding.Context, Cluster: binding.Cluster,
		ScopeID: scopeID, ScopeName: scopeName, ScopeMode: scopeMode, ScopeSource: source,
		DefaultNamespace: resolution.DefaultNamespace, NamespaceCount: len(resolution.Namespaces), Generation: binding.Generation,
		Components: selectionDTOComponents{Cluster: clusterState},
	}, summary
}

func (h *NamespaceScopes) publishSelection(binding namespaces.SelectionBinding, summary api.SelectionSummary) bool {
	writer, ok := h.snapshots.(interface{ SetSelection(*api.SelectionSummary) })
	if !ok {
		return true
	}
	publish := func() { writer.SetSelection(&summary) }
	if current, ok := h.selection.(interface {
		IfCurrent(namespaces.SelectionBinding, func()) bool
	}); ok {
		return current.IfCurrent(binding, publish)
	}
	actual, _ := h.selection.Snapshot()
	if !sameSelectionBinding(actual, binding) {
		return false
	}
	publish()
	return true
}

func (h *NamespaceScopes) selectionMatches(binding namespaces.SelectionBinding) bool {
	actual, _ := h.selection.Snapshot()
	return sameSelectionBinding(actual, binding)
}

func scopeID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, validationHTTPError("Scope id must be a positive integer.", nil)
	}
	return id, nil
}

func (h *NamespaceScopes) namespacePage(ctx context.Context, binding namespaces.SelectionBinding, query namespaceListQuery, state namespaceCursorState) ([]namespaces.NamespaceRecord, namespaceCursorState, error) {
	if pageCatalog, ok := h.catalog.(namespaces.NamespacePageCatalog); ok {
		if state.Offset != 0 {
			return nil, namespaceCursorState{}, api.NewHTTPError(http.StatusBadRequest, api.CodeCursorMismatch, "The cursor does not match the namespace collection.", nil, nil)
		}
		page, err := pageCatalog.ListPage(ctx, binding, namespaces.NamespacePageRequest{Limit: int64(query.limit), Continue: state.Continue})
		if err != nil {
			return nil, namespaceCursorState{}, err
		}
		return page.Items, namespaceCursorState{Continue: page.Continue}, nil
	}
	if state.Continue != "" {
		return nil, namespaceCursorState{}, api.NewHTTPError(http.StatusBadRequest, api.CodeCursorMismatch, "The cursor does not match the namespace collection.", nil, nil)
	}
	listed, err := h.catalog.List(ctx, binding)
	if err != nil {
		return nil, namespaceCursorState{}, err
	}
	if state.Offset < 0 || state.Offset > len(listed) {
		return nil, namespaceCursorState{}, api.NewHTTPError(http.StatusBadRequest, api.CodeCursorMismatch, "The cursor does not match the namespace collection.", nil, nil)
	}
	end := min(state.Offset+query.limit, len(listed))
	items := make([]namespaces.NamespaceRecord, 0, end-state.Offset)
	for _, name := range listed[state.Offset:end] {
		items = append(items, namespaces.NamespaceRecord{Name: name, UID: name, Phase: "Active"})
	}
	next := namespaceCursorState{}
	if end < len(listed) {
		next.Offset = end
	}
	return items, next, nil
}

func (h *NamespaceScopes) encodeNamespaceCursor(binding api.CursorBinding, state namespaceCursorState) (string, error) {
	if state.Continue == "" && state.Offset == 0 {
		return "", nil
	}
	if h.cursors == nil {
		return "", api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Pagination is temporarily unavailable.", nil, nil)
	}
	token, err := h.cursors.Encode(binding, state)
	if err != nil {
		return "", api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Pagination is temporarily unavailable.", nil, err)
	}
	return token, nil
}

func filterNamespaceRecords(records *[]namespaces.NamespaceRecord, query namespaceListQuery) {
	nameSet := make(map[string]struct{}, len(query.namespaces))
	for _, name := range query.namespaces {
		nameSet[name] = struct{}{}
	}
	statusSet := make(map[string]struct{}, len(query.statuses))
	for _, status := range query.statuses {
		statusSet[status] = struct{}{}
	}
	filtered := make([]namespaces.NamespaceRecord, 0, len(*records))
	for _, record := range *records {
		if len(nameSet) > 0 {
			if _, ok := nameSet[record.Name]; !ok {
				continue
			}
		}
		if len(statusSet) > 0 {
			if _, ok := statusSet[record.Phase]; !ok {
				continue
			}
		}
		if query.search != "" && !containsFolded(query.search, record.Name) {
			continue
		}
		filtered = append(filtered, record)
	}
	sort.SliceStable(filtered, func(left, right int) bool {
		comparison := strings.Compare(filtered[left].Name+"\x00"+filtered[left].UID, filtered[right].Name+"\x00"+filtered[right].UID)
		if query.order == "desc" {
			return comparison > 0
		}
		return comparison < 0
	})
	*records = filtered
}

func validNamespacePhase(phase string) bool {
	return phase == "Active" || phase == "Terminating" || phase == "Unknown"
}

func validateNamespaceSelectionFilter(query namespaceListQuery, resolution namespaces.ScopeResolution) error {
	if len(query.namespaces) == 0 {
		return nil
	}
	active := make(map[string]struct{}, len(resolution.Namespaces))
	for _, namespace := range resolution.Namespaces {
		active[namespace] = struct{}{}
	}
	for _, namespace := range query.namespaces {
		if _, ok := active[namespace]; !ok {
			return validationHTTPError("namespace must stay within the active scope.", nil)
		}
	}
	return nil
}

func decodeNamespaceListQuery(r *http.Request) (namespaceListQuery, error) {
	result := namespaceListQuery{limit: 100, sort: "name", order: "asc"}
	query := r.URL.Query()
	allowed := map[string]bool{"limit": false, "continue": false, "search": false, "namespace": true, "status": true, "sort": false, "order": false}
	for key, values := range query {
		repeatable, ok := allowed[key]
		if !ok {
			return namespaceListQuery{}, validationHTTPError("The namespace query contains an unknown field.", nil)
		}
		if len(values) == 0 || (!repeatable && len(values) != 1) {
			return namespaceListQuery{}, validationHTTPError("Namespace query values must be non-empty and unique.", nil)
		}
		for _, value := range values {
			if value == "" {
				return namespaceListQuery{}, validationHTTPError("Namespace query values must be non-empty and unique.", nil)
			}
		}
	}
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			return namespaceListQuery{}, validationHTTPError("limit must be between 1 and 500.", nil)
		}
		result.limit = parsed
	}
	result.continueAt = query.Get("continue")
	if len(result.continueAt) > api.MaxCursorTokenBytes {
		return namespaceListQuery{}, validationHTTPError("continue must be at most 16 KiB.", nil)
	}
	result.search = strings.ToLower(query.Get("search"))
	if len(result.search) > 256 || !utf8.ValidString(result.search) {
		return namespaceListQuery{}, validationHTTPError("search must be at most 256 UTF-8 bytes.", nil)
	}
	result.namespaces = append([]string(nil), query["namespace"]...)
	if len(result.namespaces) > 100 || !uniqueValidNamespaces(result.namespaces) {
		return namespaceListQuery{}, validationHTTPError("namespace must contain at most 100 distinct Kubernetes namespace names.", nil)
	}
	result.statuses = append([]string(nil), query["status"]...)
	if !uniqueAllowed(result.statuses, map[string]struct{}{"Active": {}, "Terminating": {}, "Unknown": {}}) {
		return namespaceListQuery{}, validationHTTPError("status must contain distinct Active, Terminating, or Unknown values.", nil)
	}
	if raw := query.Get("sort"); raw != "" {
		if raw != "name" {
			return namespaceListQuery{}, validationHTTPError("sort must be name.", nil)
		}
		result.sort = raw
	}
	if raw := query.Get("order"); raw != "" {
		if raw != "asc" && raw != "desc" {
			return namespaceListQuery{}, validationHTTPError("order must be asc or desc.", nil)
		}
		result.order = raw
	}
	return result, nil
}

func decodeScopeListQuery(r *http.Request) (scopeListQuery, error) {
	result := scopeListQuery{limit: 100, sort: "name", order: "asc"}
	query := r.URL.Query()
	for key, values := range query {
		if key != "limit" && key != "continue" && key != "search" && key != "sort" && key != "order" {
			return scopeListQuery{}, validationHTTPError("The scope query contains an unknown field.", nil)
		}
		if len(values) != 1 || values[0] == "" {
			return scopeListQuery{}, validationHTTPError("Scope query values must be non-empty and unique.", nil)
		}
	}
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			return scopeListQuery{}, validationHTTPError("limit must be between 1 and 500.", nil)
		}
		result.limit = parsed
	}
	result.continueAt = query.Get("continue")
	if len(result.continueAt) > api.MaxCursorTokenBytes {
		return scopeListQuery{}, validationHTTPError("continue must be at most 16 KiB.", nil)
	}
	result.search = strings.ToLower(query.Get("search"))
	if len(result.search) > 256 || !utf8.ValidString(result.search) {
		return scopeListQuery{}, validationHTTPError("search must be at most 256 UTF-8 bytes.", nil)
	}
	if raw := query.Get("sort"); raw != "" {
		if raw != "name" && raw != "updatedAt" {
			return scopeListQuery{}, validationHTTPError("sort must be name or updatedAt.", nil)
		}
		result.sort = raw
	}
	if raw := query.Get("order"); raw != "" {
		if raw != "asc" && raw != "desc" {
			return scopeListQuery{}, validationHTTPError("order must be asc or desc.", nil)
		}
		result.order = raw
	}
	return result, nil
}

func sortScopes(scopes []namespaces.Scope, query scopeListQuery) {
	sort.SliceStable(scopes, func(left, right int) bool {
		comparison := compareScopeKeys(newScopeCursorState(scopes[left], query), newScopeCursorState(scopes[right], query))
		if query.order == "desc" {
			return comparison > 0
		}
		return comparison < 0
	})
}

func scopePageStart(scopes []namespaces.Scope, query scopeListQuery, cursor scopeCursorState, continued bool) (int, error) {
	if !continued {
		return 0, nil
	}
	if cursor.ID <= 0 || cursor.SortValue == "" || cursor.Name == "" {
		return 0, api.NewHTTPError(http.StatusBadRequest, api.CodeCursorInvalid, "The cursor is invalid.", nil, nil)
	}
	for position, scope := range scopes {
		comparison := compareScopeKeys(newScopeCursorState(scope, query), cursor)
		if query.order == "desc" {
			if comparison < 0 {
				return position, nil
			}
			continue
		}
		if comparison > 0 {
			return position, nil
		}
	}
	return len(scopes), nil
}

func newScopeCursorState(scope namespaces.Scope, query scopeListQuery) scopeCursorState {
	sortValue := scope.Name
		if query.sort == "updatedAt" {
			sortValue = scope.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		}
	return scopeCursorState{SortValue: sortValue, Name: scope.Name, ID: scope.ID}
}

func compareScopeKeys(left, right scopeCursorState) int {
	if comparison := strings.Compare(left.SortValue, right.SortValue); comparison != 0 {
		return comparison
	}
	if comparison := strings.Compare(left.Name, right.Name); comparison != 0 {
		return comparison
	}
	if left.ID < right.ID {
		return -1
	}
	if left.ID > right.ID {
		return 1
	}
	return 0
}

func canonicalNamespaceQuery(query namespaceListQuery) string {
	namespaceValues := append([]string(nil), query.namespaces...)
	statusValues := append([]string(nil), query.statuses...)
	sort.Strings(namespaceValues)
	sort.Strings(statusValues)
	return strings.Join([]string{strconv.Itoa(query.limit), query.search, strings.Join(namespaceValues, ","), strings.Join(statusValues, ","), query.sort, query.order}, "\x1f")
}

func canonicalScopeQuery(query scopeListQuery) string {
	return strings.Join([]string{strconv.Itoa(query.limit), query.search, query.sort, query.order}, "\x1f")
}

func namespaceScopeBinding(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) string {
	return strings.Join([]string{strconv.FormatInt(binding.ClusterProfileID, 10), strconv.FormatInt(binding.ActiveScopeID, 10), resolution.ScopeSource}, ":")
}

func bindScopeWriteRequest(request namespaces.ScopeWriteRequest, binding namespaces.SelectionBinding) (namespaces.ScopeWriteRequest, error) {
	if request.ExpectedGeneration != "" && request.ExpectedGeneration != binding.Generation {
		return namespaces.ScopeWriteRequest{}, namespaces.ErrGenerationChanged
	}
	if request.ClusterProfileID != binding.ClusterProfileID || request.Context != binding.Context {
		return namespaces.ScopeWriteRequest{}, namespaces.ErrSelectionMismatch
	}
	request.ExpectedGeneration = binding.Generation
	return request, nil
}

func uniqueValidNamespaces(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !namespaces.ValidNamespaceName(value) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func uniqueAllowed(values []string, allowed map[string]struct{}) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func sameSelectionBinding(left, right namespaces.SelectionBinding) bool {
	return left.ClusterProfileID == right.ClusterProfileID && left.Context == right.Context && left.Cluster == right.Cluster && left.ActiveScopeID == right.ActiveScopeID && left.Generation == right.Generation
}

func namespaceHTTPError(err error) error {
	var httpError *api.HTTPError
	if errors.As(err, &httpError) {
		return httpError
	}
	switch {
	case errors.Is(err, namespaces.ErrBodyTooLarge):
		return api.NewHTTPError(http.StatusRequestEntityTooLarge, api.CodeBodyTooLarge, "The request body is too large.", nil, err)
	case errors.Is(err, namespaces.ErrUnknownField):
		return api.NewHTTPError(http.StatusBadRequest, api.CodeUnknownField, "The JSON body contains an unknown field.", nil, err)
	case errors.Is(err, namespaces.ErrInvalidJSON):
		return api.NewHTTPError(http.StatusBadRequest, api.CodeInvalidJSON, "The JSON body is not valid.", nil, err)
	case errors.Is(err, namespaces.ErrNotFound):
		return api.NewHTTPError(http.StatusNotFound, api.CodeNotFound, "The namespace scope was not found.", nil, err)
	case errors.Is(err, namespaces.ErrConflict):
		return api.NewHTTPError(http.StatusConflict, api.CodeConflict, "The namespace scope changed before this operation.", nil, err)
	case errors.Is(err, namespaces.ErrGenerationChanged):
		return api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "The active selection changed before this operation.", nil, err)
	case errors.Is(err, namespaces.ErrSelectionMismatch):
		return api.NewHTTPError(http.StatusConflict, api.CodeSelectionMismatch, "The namespace scope belongs to another cluster selection.", nil, err)
	case errors.Is(err, namespaces.ErrNamespaceListForbidden):
		return api.NewHTTPError(http.StatusForbidden, api.CodeForbidden, "Kubernetes denied namespace listing.", nil, err)
	case errors.Is(err, namespaces.ErrNamespaceListUnavailable):
		return api.NewHTTPError(http.StatusServiceUnavailable, api.CodeClusterUnavailable, "The namespace list is temporarily unavailable.", nil, err)
	case errors.Is(err, namespaces.ErrNamespacePageExpired):
		return api.NewHTTPError(http.StatusGone, api.CodeCursorExpired, "The Kubernetes namespace page expired; start a new list.", nil, err)
	case errors.Is(err, namespaces.ErrValidation), errors.Is(err, namespaces.ErrInvalidNamespaceInput), errors.Is(err, namespaces.ErrNamespaceLimit):
		details := any(nil)
		var field *namespaces.FieldError
		var report *namespaces.ReportError
		if errors.As(err, &field) {
			details = map[string]string{"field": field.Field, "message": field.Message}
		} else if errors.As(err, &report) {
			details = report.Report
		}
		return validationHTTPError("The namespace scope is invalid.", details)
	default:
		return api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Namespace scopes are temporarily unavailable.", nil, err)
	}
}

func validationHTTPError(message string, details any) error {
	return api.NewHTTPError(http.StatusBadRequest, api.CodeValidationFailed, message, details, nil)
}
