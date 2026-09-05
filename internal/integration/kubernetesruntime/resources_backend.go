package kubernetesruntime

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

// ResourceBackendOptions carries production wiring for the resource backend.
type ResourceBackendOptions struct {
	// ListWindowTimeout bounds one collection fan-out window. Zero falls back
	// to the resources package default; values are clamped to the supported
	// ceiling.
	ListWindowTimeout time.Duration
}

// ResourceBackend is the Phase 6 application-facing adapter. It owns no
// credentials: every operation obtains a generation-bound lease from Runtime
// and exposes only resource DTOs or a bounded YAML document.
type ResourceBackend struct {
	runtime    *Runtime
	clients    resourceClientProvider
	authorizer resources.AuthorizationChecker
	redactor   resources.TextRedactor
	now        func() time.Time

	listWindowTimeout time.Duration

	watchMu         sync.Mutex
	watchManager    *resources.WatchManager
	watchGeneration string
	watchBindings   map[string]namespaces.SelectionBinding
}

func NewResourceBackend(runtime *Runtime, authorizer resources.AuthorizationChecker, redactor resources.TextRedactor) (*ResourceBackend, error) {
	return NewResourceBackendWithOptions(runtime, authorizer, redactor, ResourceBackendOptions{})
}

func NewResourceBackendWithOptions(runtime *Runtime, authorizer resources.AuthorizationChecker, redactor resources.TextRedactor, options ResourceBackendOptions) (*ResourceBackend, error) {
	if runtime == nil || authorizer == nil {
		return nil, errors.New("resource backend: runtime and authorizer are required")
	}
	backend := &ResourceBackend{
		runtime: runtime, clients: runtimeResourceClientProvider{runtime: runtime}, authorizer: authorizer, redactor: redactor, now: time.Now,
		listWindowTimeout: resources.NormalizeListWindowTimeout(options.ListWindowTimeout),
		watchBindings:     make(map[string]namespaces.SelectionBinding),
	}
	backend.watchManager = resources.NewWatchManager(&resourceWatchPort{backend: backend})
	return backend, nil
}

func (backend *ResourceBackend) OnGeneration(next string) {
	backend.watchMu.Lock()
	previous := backend.watchGeneration
	backend.watchGeneration = next
	if previous != "" && previous != next {
		delete(backend.watchBindings, previous)
	}
	manager := backend.watchManager
	backend.watchMu.Unlock()
	if manager != nil && previous != "" && previous != next {
		manager.CancelGeneration(previous)
	}
}

func (backend *ResourceBackend) Close() {
	backend.watchMu.Lock()
	manager := backend.watchManager
	backend.watchManager = nil
	backend.watchMu.Unlock()
	if manager != nil {
		manager.Close()
	}
}

func resourceSelection(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution) resources.Selection {
	scope := resolution.ScopeName
	if scope == "" {
		scope = resolution.ScopeSource
	}
	return resources.Selection{Generation: binding.Generation, Context: binding.Context, Scope: scope, Namespaces: append([]string(nil), resolution.Namespaces...)}
}

type originListerFunc[T resources.ListItem] func(context.Context, resources.PageRequest) (resources.OriginPage[T], error)

func (function originListerFunc[T]) ListPage(ctx context.Context, request resources.PageRequest) (resources.OriginPage[T], error) {
	return function(ctx, request)
}

func collectFilteredResource[T resources.ListItem](
	ctx context.Context,
	backend *ResourceBackend,
	binding namespaces.SelectionBinding,
	resolution namespaces.ScopeResolution,
	collection resources.Collection,
	options resources.ListOptions,
	cursor *resources.CompositeCursor[T],
	less func(T, T) bool,
	list originListerFunc[T],
	filterSort func([]T, resources.ListOptions) []T,
) (resources.ListResult[T], error) {
	normalized, err := resources.NormalizeListOptions(collection, options)
	if err != nil {
		return resources.ListResult[T]{}, err
	}
	result, err := collectResource(ctx, backend, binding, resolution, collection, normalized, cursor, less, list)
	if err == nil {
		result.Items = filterSort(result.Items, normalized)
	}
	return result, err
}

func collectResource[T resources.ListItem](
	ctx context.Context,
	backend *ResourceBackend,
	binding namespaces.SelectionBinding,
	resolution namespaces.ScopeResolution,
	collection resources.Collection,
	options resources.ListOptions,
	cursor *resources.CompositeCursor[T],
	less func(T, T) bool,
	list originListerFunc[T],
) (resources.ListResult[T], error) {
	cursorGlobal, cursorNamespaced, err := listCursorMode(cursor)
	if err != nil {
		return resources.ListResult[T]{}, err
	}
	globalCandidate := resolution.PreferGlobal && len(options.Namespaces) == 0 && !cursorNamespaced
	if globalCandidate {
		origins, originsErr := resources.GlobalOriginsFor(collection, options.Kinds)
		if originsErr != nil {
			return resources.ListResult[T]{}, originsErr
		}
		decision := globalListDecision(ctx, backend.authorizer, binding.Generation, origins)
		if decision == authorization.DecisionAllowed {
			selection := resourceSelection(binding, resolution)
			selection.Namespaces = []string{""}
			return resources.Collect(ctx, resources.CollectionRequest[T]{
				Selection: selection, Options: options, Origins: origins, Cursor: cursor,
				Lister: list, Authorizer: backend.authorizer, Less: less,
				Timeout:             backend.listWindowTimeout,
				RequestedNamespaces: len(resolution.Namespaces),
			})
		}
		if cursorGlobal {
			if decision == authorization.DecisionDenied {
				return resources.ListResult[T]{}, resourceDomain(resources.CodeForbidden, "Access to this resource was denied.", nil)
			}
			return resources.ListResult[T]{}, resourceDomain(resources.CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
		}
	}
	if len(options.Namespaces) == 0 && len(resolution.Namespaces) > resources.MaximumNamespaces {
		return resources.ListResult[T]{}, resourceDomain(resources.CodeLimitExceeded, "The selected scope is too large for namespace list fan-out; narrow the namespace filter.", nil)
	}
	names, err := resources.ResolveNamespaces(resolution.Namespaces, options.Namespaces)
	if err != nil {
		return resources.ListResult[T]{}, err
	}
	selection := resourceSelection(binding, resolution)
	selection.Namespaces = names
	origins, err := resources.OriginsFor(collection, names, options.Kinds)
	if err != nil {
		return resources.ListResult[T]{}, err
	}
	return resources.Collect(ctx, resources.CollectionRequest[T]{
		Selection: selection, Options: options, Origins: origins, Cursor: cursor,
		Lister: list, Authorizer: backend.authorizer, Less: less,
		Timeout:             backend.listWindowTimeout,
		RequestedNamespaces: len(names),
	})
}

func listCursorMode[T resources.ListItem](cursor *resources.CompositeCursor[T]) (global, namespaced bool, err error) {
	if cursor == nil {
		return false, false, nil
	}
	for _, state := range cursor.Origins {
		if state.Origin.Namespace == "" {
			global = true
		} else {
			namespaced = true
		}
	}
	if global && namespaced {
		return false, false, resourceDomain(resources.CodeValidationFailed, "The cursor mixes global and namespace list strategies.", nil)
	}
	return global, namespaced, nil
}

func globalListDecision(ctx context.Context, checker resources.AuthorizationChecker, generation string, origins []resources.Origin) authorization.Decision {
	if checker == nil {
		return authorization.DecisionUnknown
	}
	decision := authorization.DecisionAllowed
	for _, origin := range origins {
		capability := checker.Check(ctx, authorization.Key{Generation: generation, Namespace: "", APIGroup: origin.APIGroup, Resource: origin.Resource, Verb: "list"})
		switch capability.Decision {
		case authorization.DecisionAllowed:
		case authorization.DecisionDenied:
			decision = authorization.DecisionDenied
		case authorization.DecisionUnknown:
			return authorization.DecisionUnknown
		default:
			return authorization.DecisionUnknown
		}
	}
	return decision
}

func (backend *ResourceBackend) ListWorkloads(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, options resources.ListOptions, cursor *resources.CompositeCursor[resources.WorkloadDTO]) (resources.ListResult[resources.WorkloadDTO], error) {
	return collectFilteredResource(ctx, backend, binding, resolution, resources.CollectionWorkloads, options, cursor, workloadIdentityLess, func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.WorkloadDTO], error) {
		return backend.listWorkloadPage(ctx, binding, page)
	}, filterSortWorkloads)
}

func (backend *ResourceBackend) ListPods(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, options resources.ListOptions, cursor *resources.CompositeCursor[resources.PodDTO]) (resources.ListResult[resources.PodDTO], error) {
	return collectFilteredResource(ctx, backend, binding, resolution, resources.CollectionPods, options, cursor, podIdentityLess, func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.PodDTO], error) {
		return backend.listPodPage(ctx, binding, page)
	}, filterSortPods)
}

func (backend *ResourceBackend) ListEvents(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, options resources.ListOptions, cursor *resources.CompositeCursor[resources.EventDTO]) (resources.ListResult[resources.EventDTO], error) {
	return collectFilteredResource(ctx, backend, binding, resolution, resources.CollectionEvents, options, cursor, eventIdentityLess, func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.EventDTO], error) {
		return backend.listEventPage(ctx, binding, page)
	}, filterSortEvents)
}

func (backend *ResourceBackend) ListServices(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, options resources.ListOptions, cursor *resources.CompositeCursor[resources.ServiceDTO]) (resources.ListResult[resources.ServiceDTO], error) {
	return collectFilteredResource(ctx, backend, binding, resolution, resources.CollectionServices, options, cursor, serviceIdentityLess, func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.ServiceDTO], error) {
		return backend.listServicePage(ctx, binding, page)
	}, filterSortServices)
}

func (backend *ResourceBackend) ListIngresses(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, options resources.ListOptions, cursor *resources.CompositeCursor[resources.IngressDTO]) (resources.ListResult[resources.IngressDTO], error) {
	return collectFilteredResource(ctx, backend, binding, resolution, resources.CollectionIngresses, options, cursor, ingressIdentityLess, func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.IngressDTO], error) {
		return backend.listIngressPage(ctx, binding, page)
	}, filterSortIngresses)
}

func (backend *ResourceBackend) ListEndpointSlices(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, options resources.ListOptions, cursor *resources.CompositeCursor[resources.EndpointSliceDTO]) (resources.ListResult[resources.EndpointSliceDTO], error) {
	return collectFilteredResource(ctx, backend, binding, resolution, resources.CollectionEndpointSlices, options, cursor, endpointSliceIdentityLess, func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.EndpointSliceDTO], error) {
		return backend.listEndpointSlicePage(ctx, binding, page)
	}, filterSortEndpointSlices)
}

func (backend *ResourceBackend) ListConfigMaps(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, options resources.ListOptions, cursor *resources.CompositeCursor[resources.ConfigMapListDTO]) (resources.ListResult[resources.ConfigMapListDTO], error) {
	return collectFilteredResource(ctx, backend, binding, resolution, resources.CollectionConfigMaps, options, cursor, configMapIdentityLess, func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.ConfigMapListDTO], error) {
		return backend.listConfigMapPage(ctx, binding, page)
	}, filterSortConfigMaps)
}

func (backend *ResourceBackend) ListSecrets(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, options resources.ListOptions, cursor *resources.CompositeCursor[resources.SecretMetadataDTO]) (resources.ListResult[resources.SecretMetadataDTO], error) {
	return collectFilteredResource(ctx, backend, binding, resolution, resources.CollectionSecrets, options, cursor, secretIdentityLess, func(ctx context.Context, page resources.PageRequest) (resources.OriginPage[resources.SecretMetadataDTO], error) {
		return backend.listSecretPage(ctx, binding, page)
	}, filterSortSecrets)
}

func workloadIdentityLess(left, right resources.WorkloadDTO) bool {
	if left.Kind != right.Kind {
		return workloadKindRank(left.Kind) < workloadKindRank(right.Kind)
	}
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	return left.Name < right.Name
}
func podIdentityLess(left, right resources.PodDTO) bool {
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	return left.Name < right.Name
}
func eventIdentityLess(left, right resources.EventDTO) bool {
	l, r := pointerString(left.Timestamp), pointerString(right.Timestamp)
	if l != r {
		return l > r
	}
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	if left.ObjectKind != right.ObjectKind {
		return left.ObjectKind < right.ObjectKind
	}
	if left.ObjectName != right.ObjectName {
		return left.ObjectName < right.ObjectName
	}
	return left.Reason < right.Reason
}
func serviceIdentityLess(left, right resources.ServiceDTO) bool {
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	return left.Name < right.Name
}
func ingressIdentityLess(left, right resources.IngressDTO) bool {
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	return left.Name < right.Name
}
func endpointSliceIdentityLess(left, right resources.EndpointSliceDTO) bool {
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	return left.Name < right.Name
}
func configMapIdentityLess(left, right resources.ConfigMapListDTO) bool {
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return left.UID < right.UID
}
func secretIdentityLess(left, right resources.SecretMetadataDTO) bool {
	if left.Metadata.Namespace != right.Metadata.Namespace {
		return left.Metadata.Namespace < right.Metadata.Namespace
	}
	if left.Metadata.Name != right.Metadata.Name {
		return left.Metadata.Name < right.Metadata.Name
	}
	return left.Metadata.UID < right.Metadata.UID
}

func filterSortWorkloads(items []resources.WorkloadDTO, options resources.ListOptions) []resources.WorkloadDTO {
	result := items[:0]
	statuses := stringSet(options.Statuses)
	for _, item := range items {
		if len(statuses) > 0 && !statuses[string(item.Status)] {
			continue
		}
		if options.Search != "" && !matchesSearch(options, item.Namespace, item.Name, item.Kind) {
			continue
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		canonical := workloadPageCanonicalCompare(left, right)
		primary := workloadPageIdentityCompare(left, right)
		switch options.Sort {
		case "name":
			primary = naturalTextCompare(left.Name, right.Name)
		case "age":
			primary = int64Compare(left.AgeSeconds, right.AgeSeconds)
		case "status":
			primary = naturalTextCompare(string(left.Status), string(right.Status))
		}
		return pageSortLess(primary, canonical, options.Order == resources.OrderDescending)
	})
	return result
}

func filterSortPods(items []resources.PodDTO, options resources.ListOptions) []resources.PodDTO {
	result := items[:0]
	statuses := stringSet(options.Statuses)
	for _, item := range items {
		if len(statuses) > 0 && !statuses[item.Status] {
			continue
		}
		if options.Node != "" && pointerString(item.Node) != options.Node {
			continue
		}
		if options.Workload != "" && (item.Owner == nil || item.Owner.Name != options.Workload) {
			continue
		}
		if options.Problematic != nil && item.Problematic != *options.Problematic {
			continue
		}
		if !restartMatches(item.Restarts, options.Restarts) {
			continue
		}
		owner := ""
		if item.Owner != nil {
			owner = item.Owner.Name
		}
		if options.Search != "" && !matchesSearch(options, item.Namespace, item.Name, pointerString(item.Node), owner) {
			continue
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		canonical := podPageCanonicalCompare(left, right)
		primary := podPageIdentityCompare(left, right)
		switch options.Sort {
		case "name":
			primary = naturalTextCompare(left.Name, right.Name)
		case "age":
			primary = int64Compare(left.AgeSeconds, right.AgeSeconds)
		case "restarts":
			primary = int64Compare(left.Restarts, right.Restarts)
		case "status":
			primary = naturalTextCompare(left.Status, right.Status)
		}
		return pageSortLess(primary, canonical, options.Order == resources.OrderDescending)
	})
	return result
}

func filterSortEvents(items []resources.EventDTO, options resources.ListOptions) []resources.EventDTO {
	result := items[:0]
	statuses := stringSet(options.Statuses)
	for _, item := range items {
		if len(statuses) > 0 && !statuses[item.Type] {
			continue
		}
		if options.ObjectKind != "" && item.ObjectKind != options.ObjectKind {
			continue
		}
		if options.Reason != "" && item.Reason != options.Reason {
			continue
		}
		if options.Search != "" && !matchesSearch(options, item.Namespace, item.ObjectKind, item.ObjectName, item.Reason, item.Message) {
			continue
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		canonical := eventPageCanonicalCompare(left, right)
		primary := strings.Compare(pointerString(left.Timestamp), pointerString(right.Timestamp))
		switch options.Sort {
		case "count":
			primary = int64Compare(left.Count, right.Count)
		case "identity":
			primary = eventPageIdentityCompare(left, right)
		}
		return pageSortLess(primary, canonical, options.Order == resources.OrderDescending)
	})
	return result
}

func filterSortServices(items []resources.ServiceDTO, options resources.ListOptions) []resources.ServiceDTO {
	result := items[:0]
	for _, item := range items {
		ips := strings.Join(item.ClusterIPs, " ")
		if options.Search == "" || matchesSearch(options, item.Namespace, item.Name, item.Type, ips) {
			result = append(result, item)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		l, r := result[i], result[j]
		canonical := servicePageCanonicalCompare(l, r)
		primary := servicePageIdentityCompare(l, r)
		switch options.Sort {
		case "name":
			primary = naturalTextCompare(l.Name, r.Name)
		case "type":
			primary = naturalTextCompare(l.Type, r.Type)
		}
		return pageSortLess(primary, canonical, options.Order == resources.OrderDescending)
	})
	return result
}
func filterSortIngresses(items []resources.IngressDTO, options resources.ListOptions) []resources.IngressDTO {
	result := items[:0]
	for _, item := range items {
		if options.Search == "" || matchesSearch(options, item.Namespace, item.Name, pointerString(item.ClassName), strings.Join(item.Hosts, " ")) {
			result = append(result, item)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		l, r := result[i], result[j]
		canonical := ingressPageCanonicalCompare(l, r)
		primary := ingressPageIdentityCompare(l, r)
		if options.Sort == "name" {
			primary = naturalTextCompare(l.Name, r.Name)
		}
		return pageSortLess(primary, canonical, options.Order == resources.OrderDescending)
	})
	return result
}
func filterSortEndpointSlices(items []resources.EndpointSliceDTO, options resources.ListOptions) []resources.EndpointSliceDTO {
	result := items[:0]
	for _, item := range items {
		if options.AddressType != "" && item.AddressType != options.AddressType {
			continue
		}
		addresses := []string{}
		for _, endpoint := range item.Endpoints {
			addresses = append(addresses, endpoint.Addresses...)
		}
		if options.Search == "" || matchesSearch(options, item.Namespace, item.Name, strings.Join(addresses, " ")) {
			result = append(result, item)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		l, r := result[i], result[j]
		canonical := endpointSlicePageCanonicalCompare(l, r)
		primary := endpointSlicePageIdentityCompare(l, r)
		switch options.Sort {
		case "name":
			primary = naturalTextCompare(l.Name, r.Name)
		case "addressType":
			primary = naturalTextCompare(l.AddressType, r.AddressType)
		}
		return pageSortLess(primary, canonical, options.Order == resources.OrderDescending)
	})
	return result
}
func filterSortConfigMaps(items []resources.ConfigMapListDTO, options resources.ListOptions) []resources.ConfigMapListDTO {
	result := items[:0]
	for _, item := range items {
		if options.Search == "" || matchesSearch(options, item.Namespace, item.Name) {
			result = append(result, item)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		l, r := result[i], result[j]
		canonical := configMapPageCanonicalCompare(l, r)
		primary := configMapPageIdentityCompare(l, r)
		switch options.Sort {
		case "name":
			primary = naturalTextCompare(l.Name, r.Name)
		case "createdAt":
			primary = strings.Compare(l.CreationTimestamp, r.CreationTimestamp)
		}
		return pageSortLess(primary, canonical, options.Order == resources.OrderDescending)
	})
	return result
}
func filterSortSecrets(items []resources.SecretMetadataDTO, options resources.ListOptions) []resources.SecretMetadataDTO {
	result := items[:0]
	for _, item := range items {
		if options.Search == "" || matchesSearch(options, item.Metadata.Namespace, item.Metadata.Name) {
			result = append(result, item)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		l, r := result[i], result[j]
		canonical := secretPageCanonicalCompare(l, r)
		primary := secretPageIdentityCompare(l, r)
		switch options.Sort {
		case "name":
			primary = naturalTextCompare(l.Metadata.Name, r.Metadata.Name)
		case "createdAt":
			primary = strings.Compare(l.Metadata.CreationTimestamp, r.Metadata.CreationTimestamp)
		}
		return pageSortLess(primary, canonical, options.Order == resources.OrderDescending)
	})
	return result
}

func matchesSearch(options resources.ListOptions, values ...string) bool {
	query := options.SearchQuery
	if query.IsEmpty() && options.Search != "" {
		query = resources.ParseSearch(options.Search)
	}
	return query.Matches(values...)
}
func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
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
func restartMatches(value int64, filter resources.RestartFilter) bool {
	switch filter {
	case "", resources.RestartAny:
		return true
	case resources.RestartGT0:
		return value > 0
	case resources.RestartGTE3:
		return value >= 3
	case resources.RestartGTE10:
		return value >= 10
	default:
		return false
	}
}
