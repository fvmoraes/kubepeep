package resources

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/fvmoraes/kubepeep/internal/observability"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

// Collection window budgets. The per-call Kubernetes deadline is enforced by
// the client factory; this budget bounds one whole fan-out window so queued
// origins always finish inside a defined limit. DefaultListWindowTimeout used
// to be a fixed 10s that starved large scopes (fan-out 4); it is now only the
// fallback when wiring does not supply a configured budget.
const (
	DefaultListWindowTimeout = 30 * time.Second
	MaximumListWindowTimeout = 300 * time.Second
)

// Origin chunk budgets separate the UI page size from the per-origin LIST
// size. Fan-out windows fetch small chunks so a page request no longer pulls
// pageLimit items from every namespace; the un-emitted remainder waits in the
// server-side cursor state instead of growing the HTTP token. A single origin
// (global LIST or cluster-scoped collection) keeps the full page limit: there
// is no over-fetch and the native continue token resumes exactly.
const (
	DefaultOriginChunkSize = 10
	MaxOriginChunkSize     = 50
)

// NormalizeListWindowTimeout clamps a configured collection budget into the
// supported range, keeping zero/negative values at the default.
func NormalizeListWindowTimeout(value time.Duration) time.Duration {
	if value <= 0 {
		return DefaultListWindowTimeout
	}
	if value > MaximumListWindowTimeout {
		return MaximumListWindowTimeout
	}
	return value
}

type CollectionRequest[T ListItem] struct {
	Selection           Selection
	Options             ListOptions
	Origins             []Origin
	Cursor              *CompositeCursor[T]
	APIGroup            string
	Resource            string
	Lister              OriginLister[T]
	Authorizer          AuthorizationChecker
	Less                func(T, T) bool
	Timeout             time.Duration
	RequestedNamespaces int
	Fanout              int
	Retry               RetryPolicy
	// PreferredNamespace is collected before the other authorized origins.
	// Cursor origins retain their canonical identity and authorization boundary.
	PreferredNamespace string
	// NativeIdentityOrder is set only by adapters whose LIST continuation is
	// monotonic for the same namespace/name comparator used by Less.
	NativeIdentityOrder bool
	// A cluster-wide list grant is stronger than each namespace grant. Enable
	// only for real Kubernetes authorizers; a denied/unknown probe falls back
	// to the ordinary per-namespace matrix.
	GlobalGrantFastPath bool
	// ReadThroughUnknown is enabled only for real Kubernetes listers. An
	// inconclusive SSAR then requires a successful bounded LIST probe before
	// either fresh or previously buffered DTOs may be returned.
	ReadThroughUnknown bool
	globalListGrant    *authorization.Capability
	pressure           *apiPressure
}

type originOutcome[T ListItem] struct {
	page          OriginPage[T]
	capability    authorization.Capability
	err           error
	queried       bool
	authoritative bool
}

// Collect executes one bounded fan-out window. It performs authorization
// before each real LIST, keeps allowed results when another namespace is
// denied/unavailable, and discards the whole window on ResourceExpired.
func Collect[T ListItem](ctx context.Context, request CollectionRequest[T]) (_ ListResult[T], resultErr error) {
	result := ListResult[T]{
		Items:    []T{},
		Page:     PageDTO{Limit: request.Options.Limit, FilterScope: FilterScopePage},
		Coverage: CoverageDTO{DeniedNamespaces: []string{}, Failed: []PartialErrorDTO{}},
	}
	if request.Lister == nil || request.Authorizer == nil || request.Less == nil {
		return result, domainError(CodeFeatureUnavailable, "The resource reader is unavailable.", nil)
	}
	if request.Selection.Generation == "" || request.Selection.Context == "" || request.Selection.Scope == "" {
		return result, validationError("selection binding is incomplete")
	}
	if request.Options.Limit < 1 || request.Options.Limit > MaximumListLimit {
		return result, validationError("list options must be normalized before collection")
	}
	origins := canonicalOrigins(request.Origins)
	if request.GlobalGrantFastPath && len(origins) > 1 && singleGVR(origins) {
		capability := request.Authorizer.Check(ctx, authorization.Key{
			Generation: request.Selection.Generation,
			APIGroup:   origins[0].APIGroup,
			Resource:   origins[0].Resource,
			Verb:       "list",
		})
		if capability.Decision == authorization.DecisionAllowed {
			request.globalListGrant = &capability
		}
	}
	pagination := selectPaginationStrategy(request, origins)
	strategy := pagination.Name()
	spanName := "resources.list.fanout"
	if globalOrigins(origins) {
		spanName = "resources.list.global"
	}
	ctx, end := observability.StartSpanWithAttributes(ctx, spanName, observability.SafeSpanAttributes{
		Strategy:        strategy,
		NamespaceCount:  countNamespaces(origins),
		PageSize:        request.Options.Limit,
		OriginChunkSize: int(originChunkLimit(len(origins), request.Options.Limit)),
		Fanout:          min(len(origins), NormalizeFanout(request.Fanout)),
	})
	defer func() { end(resultErr) }()
	if len(origins) == 0 {
		result.Page.Complete = true
		result.CollectedAt = time.Now().UTC()
		return result, nil
	}
	result.Coverage.RequestedNamespaces = countNamespaces(origins)
	if request.RequestedNamespaces > 0 {
		result.Coverage.RequestedNamespaces = request.RequestedNamespaces
	}
	cursor := NewCompositeCursor[T](origins)
	if request.Cursor != nil {
		cursor = *request.Cursor
		if err := cursor.Validate(origins); err != nil {
			return result, err
		}
	}
	timeout := NormalizeListWindowTimeout(request.Timeout)
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if request.pressure == nil {
		request.pressure = &apiPressure{}
	}
	page, paginationState, err := pagination.Next(requestContext, PaginationState[T]{Request: request, Cursor: cursor})
	if err != nil {
		return result, err
	}
	cursor = paginationState.Cursor
	outcomes := page.outcomes
	if request.Cursor != nil {
		// The final window may contain only failed origins, after authorized
		// origins were exhausted on earlier pages. Recheck those completed
		// origins before classifying this as a collection-wide authorization
		// failure. This never reads more objects or trusts an old grant.
		visited := make(map[string]bool, len(outcomes))
		hasAllowed := false
		for _, outcome := range outcomes {
			visited[outcome.page.Origin.Key()] = true
			hasAllowed = hasAllowed || outcome.capability.Decision == authorization.DecisionAllowed
		}
		if !hasAllowed {
			completed := make([]int, 0, len(cursor.Origins))
			for index, state := range cursor.Origins {
				if state.Exhausted && len(state.Buffered) == 0 && !visited[state.Origin.Key()] {
					completed = append(completed, index)
				}
			}
			outcomes = append(outcomes, authorizeBatch(requestContext, request, cursor, completed)...)
		}
	}
	type aggregate struct {
		origin        Origin
		capability    authorization.Capability
		authoritative bool
		failure       *PartialErrorDTO
	}
	aggregates := make(map[string]*aggregate, len(outcomes))
	var firstReadFailure *PartialErrorDTO
	completedNamespaces := make(map[string]struct{})
	deniedNamespaces := make(map[string]struct{})
	firstDenialMessage := ""
	for _, outcome := range outcomes {
		key := outcome.page.Origin.Key()
		current := aggregates[key]
		if current == nil {
			current = &aggregate{origin: outcome.page.Origin}
			aggregates[key] = current
		}
		current.capability = outcome.capability
		if errors.Is(outcome.err, ErrResourceExpired) {
			return result, domainError(CodeCursorExpired, "The Kubernetes list snapshot expired; start a new list.", ErrResourceExpired)
		}
		if outcome.err != nil && current.failure == nil {
			code, message := classifyReadError(outcome.err)
			current.failure = &PartialErrorDTO{Namespace: outcome.page.Origin.Namespace, Code: code, Message: message}
		}
		if outcome.err == nil && outcome.authoritative {
			current.authoritative = true
		}
	}
	allowed := 0
	known := 0
	unknown := 0
	authoritativeSuccesses := 0
	allowedOrigins := make(map[string]bool, len(aggregates))
	for _, origin := range origins {
		current := aggregates[origin.Key()]
		if current == nil {
			continue
		}
		namespace := current.origin.Namespace
		switch current.capability.Decision {
		case authorization.DecisionDenied:
			known++
			deniedNamespaces[namespace] = struct{}{}
			message := fmt.Sprintf("Kubernetes permission review denied LIST on %s in namespace %q. Check the current identity's Role or ClusterRole bindings.", current.origin.Resource, namespace)
			if namespace == "" {
				message = fmt.Sprintf("Kubernetes permission review denied cluster-wide LIST on %s. Check the current identity's ClusterRole bindings.", current.origin.Resource)
			}
			if current.failure != nil {
				message = current.failure.Message
			}
			if firstDenialMessage == "" {
				firstDenialMessage = message
			}
			result.Coverage.Failed = append(result.Coverage.Failed, PartialErrorDTO{Namespace: namespace, Code: CodeForbidden, Message: message})
			continue
		case authorization.DecisionUnknown:
			unknown++
			failure := current.failure
			if failure == nil {
				review := authorization.ReviewFailure(current.capability)
				failure = &PartialErrorDTO{Namespace: namespace, Code: ErrorCode(review.Code), Message: review.Message}
			}
			result.Coverage.Failed = append(result.Coverage.Failed, *failure)
			if firstReadFailure == nil {
				firstReadFailure = failure
			}
			continue
		case authorization.DecisionAllowed:
			known++
			allowed++
			// A real 403/401 overrides a cached allowed review. Do not leak a
			// cursor's buffered rows after Kubernetes rejects the live request.
			allowedOrigins[origin.Key()] = current.failure == nil || current.failure.Code != CodeForbidden && current.failure.Code != CodeAuthenticationUnavailable
		}
		if current.failure != nil {
			failure := *current.failure
			result.Coverage.Failed = append(result.Coverage.Failed, failure)
			if firstReadFailure == nil {
				copy := failure
				firstReadFailure = &copy
			}
		}
		if current.authoritative {
			authoritativeSuccesses++
			completedNamespaces[namespace] = struct{}{}
		}
	}
	if allowed == 0 {
		if known > 0 && len(deniedNamespaces) > 0 && unknown == 0 {
			return result, domainError(CodeForbidden, firstDenialMessage, nil)
		}
		if firstReadFailure != nil {
			return result, domainError(firstReadFailure.Code, firstReadFailure.Message, nil)
		}
		return result, domainError(CodeAuthorizationUnavailable, authorization.ReviewFailure(authorization.Capability{}).Message, nil)
	}
	if authoritativeSuccesses == 0 {
		if firstReadFailure != nil {
			return result, domainError(firstReadFailure.Code, firstReadFailure.Message, nil)
		}
		return result, domainError(CodeClusterUnavailable, "The Kubernetes API could not complete the request.", nil)
	}
	for namespace := range deniedNamespaces {
		result.Coverage.DeniedNamespaces = append(result.Coverage.DeniedNamespaces, namespace)
	}
	sort.Strings(result.Coverage.DeniedNamespaces)
	result.Coverage.CompletedNamespaces = len(completedNamespaces)
	if globalOrigins(origins) && authoritativeSuccesses > 0 && request.RequestedNamespaces > 0 {
		result.Coverage.CompletedNamespaces = request.RequestedNamespaces
	}
	_, endMerge := observability.StartSpan(ctx, "resources.merge")
	items, next, err := mergePreferredNamespace(cursor, request, allowedOrigins)
	endMerge(err)
	if err != nil {
		return result, err
	}
	if err := next.Validate(origins); err != nil {
		return result, err
	}
	result.Items = items
	result.Cursor = &next
	result.Page.Complete = next.Complete() && len(result.Coverage.Failed) == 0
	result.Page.Truncated = !next.Complete() || len(result.Coverage.Failed) > 0
	if !next.Complete() {
		// Do not advertise another page when only known failures remain.
		// Keep the coverage incomplete, and retry those origins on a fresh
		// collection instead of turning its last partial page into a 403/503.
		result.Cursor = nil
		for _, state := range next.Origins {
			if state.Exhausted && len(state.Buffered) == 0 {
				continue
			}
			current := aggregates[state.Origin.Key()]
			if current == nil || current.capability.Decision == authorization.DecisionAllowed && (current.failure == nil || len(state.Buffered) > 0) {
				result.Cursor = &next
				break
			}
		}
	}
	result.CollectedAt = time.Now().UTC()
	return result, nil
}

func mergePreferredNamespace[T ListItem](cursor CompositeCursor[T], request CollectionRequest[T], allowed map[string]bool) ([]T, CompositeCursor[T], error) {
	if request.PreferredNamespace == "" {
		return mergeAuthorizedOriginPages(cursor, nil, request.Options.Limit, request.Less, allowed)
	}
	preferred := make(map[string]bool)
	for _, state := range cursor.Origins {
		if state.Origin.Namespace == request.PreferredNamespace && allowed[state.Origin.Key()] {
			preferred[state.Origin.Key()] = true
		}
	}
	items, next, err := mergeAuthorizedOriginPages(cursor, nil, request.Options.Limit, request.Less, preferred)
	if err != nil || len(items) == request.Options.Limit {
		return items, next, err
	}
	rest, next, err := mergeAuthorizedOriginPages(next, nil, request.Options.Limit-len(items), request.Less, allowed)
	return append(items, rest...), next, err
}

// originChunkLimit bounds the per-origin page size for one collection window.
func originChunkLimit(origins, pageLimit int) int64 {
	if origins <= 1 {
		return int64(pageLimit)
	}
	chunk := DefaultOriginChunkSize
	if chunk > MaxOriginChunkSize {
		chunk = MaxOriginChunkSize
	}
	if chunk > pageLimit {
		chunk = pageLimit
	}
	return int64(chunk)
}

func canonicalOrigins(origins []Origin) []Origin {
	result := append([]Origin(nil), origins...)
	sort.Slice(result, func(i, j int) bool { return result[i].Key() < result[j].Key() })
	return result
}

func countNamespaces(origins []Origin) int {
	seen := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		seen[origin.Namespace] = struct{}{}
	}
	return len(seen)
}

func globalOrigins(origins []Origin) bool {
	if len(origins) == 0 {
		return false
	}
	for _, origin := range origins {
		if origin.Namespace != "" {
			return false
		}
	}
	return true
}

func classifyReadError(err error) (ErrorCode, string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return CodeUpstreamTimeout, "Collection timed out."
	}
	if errors.Is(err, context.Canceled) {
		return CodeGenerationChanged, "The active selection changed."
	}
	var domain *DomainError
	if errors.As(err, &domain) {
		return domain.Code, domain.Message
	}
	return CodeClusterUnavailable, "The Kubernetes API could not complete the request."
}
