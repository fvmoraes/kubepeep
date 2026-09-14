package resources

import (
	"container/heap"
	"sort"
)

// Origin identifies exactly one Kubernetes LIST source. Resource is the
// plural GVR resource and Namespace may be empty only for a cluster-wide list.
type Origin struct {
	Namespace string `json:"namespace"`
	APIGroup  string `json:"apiGroup"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
}

func (origin Origin) Key() string {
	return origin.APIGroup + "/" + origin.Version + "/" + origin.Resource + "/" + origin.Namespace
}

type OriginPage[T ListItem] struct {
	Origin          Origin
	Items           []T
	Continue        string
	ResourceVersion string
}

// OriginCursor retains the native Kubernetes token and any already collected
// DTOs that were not emitted. It intentionally never models page/per-page.
type OriginCursor[T ListItem] struct {
	Origin          Origin `json:"origin"`
	Continue        string `json:"continue,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
	Exhausted       bool   `json:"exhausted"`
	Buffered        []T    `json:"buffered,omitempty"`
}

// CompositeCursor is encoded inside internal/api.CursorCodec, which supplies
// the HMAC, generation/query binding and fixed five-minute TTL.
type CompositeCursor[T ListItem] struct {
	Version int               `json:"version"`
	Origins []OriginCursor[T] `json:"origins"`
}

func NewCompositeCursor[T ListItem](origins []Origin) CompositeCursor[T] {
	canonical := append([]Origin(nil), origins...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Key() < canonical[j].Key() })
	state := CompositeCursor[T]{Version: 1, Origins: make([]OriginCursor[T], len(canonical))}
	for index := range canonical {
		state.Origins[index] = OriginCursor[T]{Origin: canonical[index], Buffered: []T{}}
	}
	return state
}

func (cursor CompositeCursor[T]) Validate(expected []Origin) error {
	if cursor.Version != 1 {
		return validationError("cursor state version is not supported")
	}
	if len(cursor.Origins) == 0 || len(cursor.Origins) != len(expected) {
		return validationError("cursor origins do not match this collection")
	}
	canonical := append([]Origin(nil), expected...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Key() < canonical[j].Key() })
	seen := make(map[string]struct{}, len(cursor.Origins))
	for index, state := range cursor.Origins {
		key := state.Origin.Key()
		if state.Origin.Version == "" || state.Origin.Resource == "" {
			return validationError("cursor origin is incomplete")
		}
		if _, duplicate := seen[key]; duplicate || key != canonical[index].Key() {
			return validationError("cursor origins do not match this collection")
		}
		seen[key] = struct{}{}
		if len(state.Continue) > MaximumCursorBytes {
			return validationError("upstream continue token is too large")
		}
		if state.Exhausted && state.Continue != "" {
			return validationError("exhausted cursor origin cannot have a continue token")
		}
	}
	return nil
}

// MergeOriginPages performs a deterministic k-way merge. It returns a new
// state containing the un-emitted DTOs and the native token for each source.
// Callers must supply pages sorted by less using the same identity contract.
func MergeOriginPages[T ListItem](current CompositeCursor[T], pages []OriginPage[T], limit int, less func(T, T) bool) ([]T, CompositeCursor[T], error) {
	return mergeAuthorizedOriginPages(current, pages, limit, less, nil)
}

func mergeAuthorizedOriginPages[T ListItem](current CompositeCursor[T], pages []OriginPage[T], limit int, less func(T, T) bool, allowed map[string]bool) ([]T, CompositeCursor[T], error) {
	if limit < 1 || limit > MaximumListLimit || less == nil {
		return nil, CompositeCursor[T]{}, validationError("merge arguments are invalid")
	}
	byOrigin := make(map[string]OriginPage[T], len(pages))
	for _, page := range pages {
		key := page.Origin.Key()
		if _, duplicate := byOrigin[key]; duplicate {
			return nil, CompositeCursor[T]{}, validationError("duplicate origin page")
		}
		byOrigin[key] = page
	}
	next := CompositeCursor[T]{Version: current.Version, Origins: make([]OriginCursor[T], len(current.Origins))}
	for index, state := range current.Origins {
		buffered := append([]T(nil), state.Buffered...)
		if page, ok := byOrigin[state.Origin.Key()]; ok {
			buffered = append(buffered, page.Items...)
			state.Continue = page.Continue
			state.ResourceVersion = page.ResourceVersion
			state.Exhausted = page.Continue == ""
		}
		sort.SliceStable(buffered, func(i, j int) bool { return less(buffered[i], buffered[j]) })
		state.Buffered = buffered
		next.Origins[index] = state
	}
	items := make([]T, 0, limit)
	candidates := &originMergeHeap[T]{cursor: &next, less: less}
	for index := range next.Origins {
		if len(next.Origins[index].Buffered) == 0 || allowed != nil && !allowed[next.Origins[index].Origin.Key()] {
			continue
		}
		heap.Push(candidates, index)
	}
	for len(items) < limit && candidates.Len() > 0 {
		selected := heap.Pop(candidates).(int)
		items = append(items, next.Origins[selected].Buffered[0])
		var zero T
		next.Origins[selected].Buffered[0] = zero
		next.Origins[selected].Buffered = next.Origins[selected].Buffered[1:]
		if len(next.Origins[selected].Buffered) > 0 {
			heap.Push(candidates, selected)
		}
	}
	return items, next, nil
}

type originMergeHeap[T ListItem] struct {
	cursor  *CompositeCursor[T]
	less    func(T, T) bool
	origins []int
}

func (value originMergeHeap[T]) Len() int { return len(value.origins) }
func (value originMergeHeap[T]) Less(left, right int) bool {
	l, r := value.origins[left], value.origins[right]
	lItem, rItem := value.cursor.Origins[l].Buffered[0], value.cursor.Origins[r].Buffered[0]
	if value.less(lItem, rItem) {
		return true
	}
	if value.less(rItem, lItem) {
		return false
	}
	return l < r
}
func (value originMergeHeap[T]) Swap(left, right int) {
	value.origins[left], value.origins[right] = value.origins[right], value.origins[left]
}
func (value *originMergeHeap[T]) Push(item any) { value.origins = append(value.origins, item.(int)) }
func (value *originMergeHeap[T]) Pop() any {
	last := len(value.origins) - 1
	item := value.origins[last]
	value.origins = value.origins[:last]
	return item
}

func (cursor CompositeCursor[T]) Complete() bool {
	for _, origin := range cursor.Origins {
		if !origin.Exhausted || len(origin.Buffered) > 0 {
			return false
		}
	}
	return true
}
