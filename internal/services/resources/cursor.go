package resources

import (
	"encoding/json"
	"fmt"
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
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return fmt.Errorf("resources: marshal cursor state: %w", err)
	}
	// Leave room for the authenticated envelope and base64 expansion.
	if len(encoded) > 12<<10 {
		return domainError(CodeLimitExceeded, "The composed cursor exceeded its safe size.", nil)
	}
	return nil
}

// MergeOriginPages performs a deterministic k-way merge. It returns a new
// state containing the un-emitted DTOs and the native token for each source.
// Callers must supply pages sorted by less using the same identity contract.
func MergeOriginPages[T ListItem](current CompositeCursor[T], pages []OriginPage[T], limit int, less func(T, T) bool) ([]T, CompositeCursor[T], error) {
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
	for len(items) < limit {
		selected := -1
		for index := range next.Origins {
			if len(next.Origins[index].Buffered) == 0 {
				continue
			}
			if selected < 0 || less(next.Origins[index].Buffered[0], next.Origins[selected].Buffered[0]) {
				selected = index
			}
		}
		if selected < 0 {
			break
		}
		items = append(items, next.Origins[selected].Buffered[0])
		next.Origins[selected].Buffered = next.Origins[selected].Buffered[1:]
	}
	return items, next, nil
}

func (cursor CompositeCursor[T]) Complete() bool {
	for _, origin := range cursor.Origins {
		if !origin.Exhausted || len(origin.Buffered) > 0 {
			return false
		}
	}
	return true
}
