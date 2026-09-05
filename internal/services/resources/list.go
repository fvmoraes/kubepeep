package resources

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

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
}

type originOutcome[T ListItem] struct {
	page       OriginPage[T]
	capability authorization.Capability
	err        error
	queried    bool
}

// Collect executes one bounded fan-out window. It performs authorization
// before each real LIST, keeps allowed results when another namespace is
// denied/unavailable, and discards the whole window on ResourceExpired.
func Collect[T ListItem](ctx context.Context, request CollectionRequest[T]) (ListResult[T], error) {
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
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	pages := make([]OriginPage[T], 0, len(origins))
	outcomes := collectOrigins(requestContext, request, cursor)
	allowed := 0
	known := 0
	unknown := 0
	authoritativeSuccesses := 0
	var firstReadFailure *PartialErrorDTO
	completedNamespaces := make(map[string]struct{})
	deniedNamespaces := make(map[string]struct{})
	for _, outcome := range outcomes {
		namespace := outcome.page.Origin.Namespace
		switch outcome.capability.Decision {
		case authorization.DecisionDenied:
			known++
			deniedNamespaces[namespace] = struct{}{}
			result.Coverage.Failed = append(result.Coverage.Failed, PartialErrorDTO{Namespace: namespace, Code: CodeForbidden, Message: "Access to this resource was denied."})
			continue
		case authorization.DecisionUnknown:
			unknown++
			result.Coverage.Failed = append(result.Coverage.Failed, PartialErrorDTO{Namespace: namespace, Code: CodeAuthorizationUnavailable, Message: "Authorization could not be confirmed."})
			continue
		case authorization.DecisionAllowed:
			known++
			allowed++
		}
		if errors.Is(outcome.err, ErrResourceExpired) {
			return result, domainError(CodeCursorExpired, "The Kubernetes list snapshot expired; start a new list.", ErrResourceExpired)
		}
		if outcome.err != nil {
			code, message := classifyReadError(outcome.err)
			failure := PartialErrorDTO{Namespace: namespace, Code: code, Message: message}
			result.Coverage.Failed = append(result.Coverage.Failed, failure)
			if firstReadFailure == nil {
				copy := failure
				firstReadFailure = &copy
			}
			continue
		}
		authoritativeSuccesses++
		if outcome.queried {
			pages = append(pages, outcome.page)
		}
		completedNamespaces[namespace] = struct{}{}
	}
	if allowed == 0 {
		if known > 0 && len(deniedNamespaces) > 0 && unknown == 0 {
			return result, domainError(CodeForbidden, "Access to this resource was denied.", nil)
		}
		return result, domainError(CodeAuthorizationUnavailable, "Authorization could not be confirmed.", nil)
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
	items, next, err := MergeOriginPages(cursor, pages, request.Options.Limit, request.Less)
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
	result.CollectedAt = time.Now().UTC()
	return result, nil
}

func collectOrigins[T ListItem](ctx context.Context, request CollectionRequest[T], cursor CompositeCursor[T]) []originOutcome[T] {
	outcomes := make([]originOutcome[T], len(cursor.Origins))
	semaphore := make(chan struct{}, MaximumFanout)
	var wait sync.WaitGroup
	for index := range cursor.Origins {
		state := cursor.Origins[index]
		outcomes[index].page.Origin = state.Origin
		wait.Add(1)
		go func(index int, state OriginCursor[T]) {
			defer wait.Done()
			key := authorization.Key{
				Generation: request.Selection.Generation,
				Namespace:  state.Origin.Namespace,
				APIGroup:   state.Origin.APIGroup,
				Resource:   state.Origin.Resource,
				Verb:       "list",
			}
			capability := request.Authorizer.Check(ctx, key)
			outcomes[index].capability = capability
			if capability.Decision != authorization.DecisionAllowed {
				return
			}
			if state.Exhausted || len(state.Buffered) > 0 {
				return
			}
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				outcomes[index].err = ctx.Err()
				return
			}
			page, err := request.Lister.ListPage(ctx, PageRequest{Origin: state.Origin, Limit: int64(request.Options.Limit), Continue: state.Continue})
			if page.Origin.Key() == "///" {
				page.Origin = state.Origin
			}
			outcomes[index].page = page
			outcomes[index].err = err
			outcomes[index].queried = true
		}(index, state)
	}
	wait.Wait()
	return outcomes
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
