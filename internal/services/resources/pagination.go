package resources

import (
	"context"
	"sort"
	"sync"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

// PaginationState is the server-side state consumed and returned by a
// pagination strategy. It is never serialized directly to the frontend.
type PaginationState[T ListItem] struct {
	Request CollectionRequest[T]
	Cursor  CompositeCursor[T]
}

// PaginationPage describes the bounded work performed by one strategy step.
// The outcomes remain package-private because authorization and upstream
// failures are translated into the public coverage contract by Collect.
type PaginationPage[T ListItem] struct {
	outcomes []originOutcome[T]
}

// PaginationStrategy separates source scheduling from authorization,
// coverage, cursor encoding and the public list result contract.
type PaginationStrategy[T ListItem] interface {
	Name() string
	Next(context.Context, PaginationState[T]) (PaginationPage[T], PaginationState[T], error)
}

// GlobalNative preserves Kubernetes native continuation for a cluster-wide or
// cluster-scoped LIST. It does not claim that Kubernetes sorted arbitrary
// user-selected fields globally.
type GlobalNative[T ListItem] struct{}

func (GlobalNative[T]) Name() string { return "global-native" }

func (GlobalNative[T]) Next(ctx context.Context, state PaginationState[T]) (PaginationPage[T], PaginationState[T], error) {
	if len(state.Cursor.Origins) != 1 {
		return PaginationPage[T]{}, state, validationError("global pagination requires exactly one origin")
	}
	outcome := authorizeOrigin(ctx, state.Request, state.Cursor.Origins[0].Origin)
	page := PaginationPage[T]{outcomes: []originOutcome[T]{outcome}}
	if outcome.capability.Decision != authorization.DecisionAllowed || state.Cursor.Origins[0].Exhausted || len(state.Cursor.Origins[0].Buffered) >= state.Request.Options.Limit {
		page.outcomes[0].authoritative = state.Cursor.Origins[0].Exhausted || len(state.Cursor.Origins[0].Buffered) > 0
		return page, state, nil
	}
	fetched := fetchOrigin(ctx, state.Request, state.Cursor.Origins[0], int64(state.Request.Options.Limit), outcome.capability)
	page.outcomes = append(page.outcomes, fetched)
	if fetched.err == nil {
		if err := applyOriginPage(&state.Cursor.Origins[0], fetched.page, state.Request.Less); err != nil {
			return PaginationPage[T]{}, state, err
		}
	}
	return page, state, nil
}

// NamespaceSequential follows the canonical namespace sequence and exhausts
// only as many origins as needed to fill the current page. It is selected only
// for ascending identity order over a single GVR, where namespace + native
// name order is monotonic. Descending and non-identity sorts use LazyMerge and
// keep the honest page-local ordering contract.
type NamespaceSequential[T ListItem] struct{}

func (NamespaceSequential[T]) Name() string { return "namespace-sequential" }

func (NamespaceSequential[T]) Next(ctx context.Context, state PaginationState[T]) (PaginationPage[T], PaginationState[T], error) {
	result := PaginationPage[T]{outcomes: []originOutcome[T]{}}
	available := 0
	prefetched := make(map[int]originOutcome[T])
	for index := range state.Cursor.Origins {
		if err := ctx.Err(); err != nil {
			return result, state, err
		}
		originState := &state.Cursor.Origins[index]
		if originState.Exhausted && len(originState.Buffered) == 0 {
			continue
		}
		// Probe the first origin alone: a dense namespace can fill the page
		// without touching any other namespace. Once it is exhausted, fetch a
		// bounded window of untouched origins concurrently while consuming their
		// buffers strictly in namespace order.
		if index > 0 && len(originState.Buffered) == 0 && originState.Continue == "" {
			if _, ok := prefetched[index]; !ok {
				indices := make([]int, 0, NormalizeFanout(state.Request.Fanout))
				for next := index; next < len(state.Cursor.Origins) && len(indices) < cap(indices); next++ {
					candidate := state.Cursor.Origins[next]
					if len(candidate.Buffered) > 0 || candidate.Continue != "" {
						break
					}
					if !candidate.Exhausted {
						indices = append(indices, next)
					}
				}
				for _, fetched := range fetchBatch(ctx, state.Request, state.Cursor, indices, nil) {
					result.outcomes = append(result.outcomes, fetched.outcome)
					prefetched[fetched.index] = fetched.outcome
					if fetched.outcome.err == nil && fetched.outcome.queried {
						if err := applyOriginPage(&state.Cursor.Origins[fetched.index], fetched.outcome.page, state.Request.Less); err != nil {
							return PaginationPage[T]{}, state, err
						}
					}
				}
			}
		}
		authorized, wasPrefetched := prefetched[index]
		if !wasPrefetched {
			authorized = authorizeOrigin(ctx, state.Request, originState.Origin)
			authorized.authoritative = originState.Exhausted || len(originState.Buffered) > 0
			result.outcomes = append(result.outcomes, authorized)
		}
		if authorized.capability.Decision != authorization.DecisionAllowed {
			continue
		}
		if wasPrefetched && authorized.err != nil {
			continue
		}
		available += len(originState.Buffered)
		for available < state.Request.Options.Limit && !originState.Exhausted {
			fetched := fetchOrigin(ctx, state.Request, *originState, originChunkLimit(len(state.Cursor.Origins), state.Request.Options.Limit), authorized.capability)
			result.outcomes = append(result.outcomes, fetched)
			if fetched.err != nil {
				break
			}
			before := len(originState.Buffered)
			if err := applyOriginPage(originState, fetched.page, state.Request.Less); err != nil {
				return PaginationPage[T]{}, state, err
			}
			available += len(originState.Buffered) - before
		}
		if available >= state.Request.Options.Limit {
			break
		}
	}
	return result, state, nil
}

// LazyMerge bounds the active origin window. It reads at most one small chunk
// per origin in a round, stopping as soon as the current page can be filled.
// The resulting heap merge is deliberately page-local unless a later complete
// snapshot proves collection-wide ordering.
type LazyMerge[T ListItem] struct{}

func (LazyMerge[T]) Name() string { return "lazy-merge" }

func (LazyMerge[T]) Next(ctx context.Context, state PaginationState[T]) (PaginationPage[T], PaginationState[T], error) {
	result := PaginationPage[T]{outcomes: []originOutcome[T]{}}
	allowed := make(map[string]bool, len(state.Cursor.Origins))

	// Buffered DTOs are reauthorized before they become heap candidates.
	buffered := make([]int, 0, len(state.Cursor.Origins))
	for index := range state.Cursor.Origins {
		if len(state.Cursor.Origins[index].Buffered) > 0 {
			buffered = append(buffered, index)
		}
	}
	for _, outcome := range authorizeBatch(ctx, state.Request, state.Cursor, buffered) {
		result.outcomes = append(result.outcomes, outcome)
		allowed[outcome.page.Origin.Key()] = outcome.capability.Decision == authorization.DecisionAllowed
	}
	if authorizedBufferedCount(state.Cursor, allowed) >= state.Request.Options.Limit {
		return result, state, nil
	}

	// Prefer untouched origins before refilling origins already represented in
	// the limited window. A new round enables refills only when every remaining
	// origin has contributed once and the page is still short.
	for {
		progress := false
		for _, preferEmpty := range []bool{true, false} {
			indices := make([]int, 0, len(state.Cursor.Origins))
			for index := range state.Cursor.Origins {
				originState := state.Cursor.Origins[index]
				if originState.Exhausted || (len(originState.Buffered) == 0) != preferEmpty {
					continue
				}
				indices = append(indices, index)
			}
			for start := 0; start < len(indices); start += NormalizeFanout(state.Request.Fanout) {
				if err := ctx.Err(); err != nil {
					return result, state, err
				}
				end := min(start+NormalizeFanout(state.Request.Fanout), len(indices))
				outcomes := fetchBatch(ctx, state.Request, state.Cursor, indices[start:end], allowed)
				for _, indexed := range outcomes {
					result.outcomes = append(result.outcomes, indexed.outcome)
					key := indexed.outcome.page.Origin.Key()
					allowed[key] = indexed.outcome.capability.Decision == authorization.DecisionAllowed
					if indexed.outcome.err == nil && indexed.outcome.queried {
						if err := applyOriginPage(&state.Cursor.Origins[indexed.index], indexed.outcome.page, state.Request.Less); err != nil {
							return PaginationPage[T]{}, state, err
						}
						progress = true
					}
				}
				if authorizedBufferedCount(state.Cursor, allowed) >= state.Request.Options.Limit {
					return result, state, nil
				}
			}
		}
		if !progress || cursorSourcesExhausted(state.Cursor) {
			break
		}
	}
	return result, state, nil
}

func selectPaginationStrategy[T ListItem](request CollectionRequest[T], origins []Origin) PaginationStrategy[T] {
	if globalOrigins(origins) {
		return GlobalNative[T]{}
	}
	if request.NativeIdentityOrder && request.Options.Sort == "identity" && request.Options.Order == OrderAscending && singleGVR(origins) {
		return NamespaceSequential[T]{}
	}
	return LazyMerge[T]{}
}

func singleGVR(origins []Origin) bool {
	if len(origins) == 0 {
		return false
	}
	first := origins[0]
	for _, origin := range origins[1:] {
		if origin.APIGroup != first.APIGroup || origin.Version != first.Version || origin.Resource != first.Resource {
			return false
		}
	}
	return true
}

func authorizeOrigin[T ListItem](ctx context.Context, request CollectionRequest[T], origin Origin) originOutcome[T] {
	if request.globalListGrant != nil {
		return originOutcome[T]{page: OriginPage[T]{Origin: origin}, capability: *request.globalListGrant}
	}
	capability := request.Authorizer.Check(ctx, authorization.Key{
		Generation: request.Selection.Generation,
		Namespace:  origin.Namespace,
		APIGroup:   origin.APIGroup,
		Resource:   origin.Resource,
		Verb:       "list",
	})
	return originOutcome[T]{page: OriginPage[T]{Origin: origin}, capability: capability}
}

func fetchOrigin[T ListItem](ctx context.Context, request CollectionRequest[T], state OriginCursor[T], limit int64, capability authorization.Capability) originOutcome[T] {
	page, err := retryListPage(ctx, request.Lister, PageRequest{
		Origin:        state.Origin,
		Limit:         limit,
		Continue:      state.Continue,
		LabelSelector: request.Options.LabelSelector,
		FieldSelector: request.Options.FieldSelector,
	}, request.Retry, request.pressure)
	if page.Origin.Key() == "///" {
		page.Origin = state.Origin
	}
	return originOutcome[T]{page: page, capability: capability, err: err, queried: true, authoritative: err == nil}
}

type indexedOutcome[T ListItem] struct {
	index   int
	outcome originOutcome[T]
}

func authorizeBatch[T ListItem](ctx context.Context, request CollectionRequest[T], cursor CompositeCursor[T], indices []int) []originOutcome[T] {
	indexed := runOriginWorkers(ctx, request, cursor, indices, false, nil)
	result := make([]originOutcome[T], 0, len(indexed))
	for _, item := range indexed {
		result = append(result, item.outcome)
	}
	return result
}

func fetchBatch[T ListItem](ctx context.Context, request CollectionRequest[T], cursor CompositeCursor[T], indices []int, known map[string]bool) []indexedOutcome[T] {
	return runOriginWorkers(ctx, request, cursor, indices, true, known)
}

// runOriginWorkers creates a fixed-size worker pool; neither authorization
// reviews nor Kubernetes LIST calls create one goroutine per namespace.
func runOriginWorkers[T ListItem](ctx context.Context, request CollectionRequest[T], cursor CompositeCursor[T], indices []int, fetch bool, known map[string]bool) []indexedOutcome[T] {
	if len(indices) == 0 {
		return nil
	}
	jobs := make(chan int)
	results := make(chan indexedOutcome[T], len(indices))
	workers := min(request.pressure.fanout(request.Fanout), len(indices))
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			for index := range jobs {
				state := cursor.Origins[index]
				outcome := authorizeOrigin(ctx, request, state.Origin)
				outcome.authoritative = state.Exhausted || len(state.Buffered) > 0
				if allowed, ok := known[state.Origin.Key()]; ok {
					if allowed {
						outcome.capability.Decision = authorization.DecisionAllowed
					} else {
						results <- indexedOutcome[T]{index: index, outcome: outcome}
						continue
					}
				}
				if fetch && outcome.capability.Decision == authorization.DecisionAllowed {
					outcome = fetchOrigin(ctx, request, state, originChunkLimit(len(cursor.Origins), request.Options.Limit), outcome.capability)
				}
				results <- indexedOutcome[T]{index: index, outcome: outcome}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, index := range indices {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	wait.Wait()
	close(results)
	byIndex := make(map[int]indexedOutcome[T], len(indices))
	for result := range results {
		byIndex[result.index] = result
	}
	ordered := make([]indexedOutcome[T], 0, len(byIndex))
	for _, index := range indices {
		if result, ok := byIndex[index]; ok {
			ordered = append(ordered, result)
		}
	}
	return ordered
}

func applyOriginPage[T ListItem](state *OriginCursor[T], page OriginPage[T], less func(T, T) bool) error {
	if state.Origin.Key() != page.Origin.Key() {
		return validationError("origin page does not match cursor state")
	}
	if page.Continue != "" && page.Continue == state.Continue {
		return validationError("upstream pagination made no progress")
	}
	state.Buffered = append(state.Buffered, page.Items...)
	state.Continue = page.Continue
	state.ResourceVersion = page.ResourceVersion
	state.Exhausted = page.Continue == ""
	if less == nil {
		return validationError("pagination comparator is unavailable")
	}
	sortListItems(state.Buffered, less)
	return nil
}

func authorizedBufferedCount[T ListItem](cursor CompositeCursor[T], allowed map[string]bool) int {
	total := 0
	for _, state := range cursor.Origins {
		if allowed[state.Origin.Key()] {
			total += len(state.Buffered)
		}
	}
	return total
}

func cursorSourcesExhausted[T ListItem](cursor CompositeCursor[T]) bool {
	for _, state := range cursor.Origins {
		if !state.Exhausted {
			return false
		}
	}
	return true
}

// NormalizeFanout applies the internal 4/6/8 tuning range while keeping four
// as the safe default until benchmarks justify a different production value.
func NormalizeFanout(value int) int {
	if value <= 0 {
		return DefaultFanout
	}
	if value > MaximumConfigurableFanout {
		return MaximumConfigurableFanout
	}
	return value
}

func sortListItems[T ListItem](items []T, less func(T, T) bool) {
	// Kept as a small helper so strategies cannot accidentally use a different
	// comparator from the heap merge.
	sort.SliceStable(items, func(i, j int) bool { return less(items[i], items[j]) })
}
