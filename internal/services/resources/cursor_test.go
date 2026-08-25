package resources

import (
	"strings"
	"testing"
)

type testListItem string

func (testListItem) resourceListItem() {}
func joinTestItems(values []testListItem) string {
	result := make([]string, len(values))
	for index := range values {
		result[index] = string(values[index])
	}
	return strings.Join(result, ",")
}

func testOrigins() []Origin {
	return []Origin{{Namespace: "b", Version: "v1", Resource: "pods"}, {Namespace: "a", Version: "v1", Resource: "pods"}}
}

func TestCompositeCursorCanonicalizesOriginsAndPreservesNativeContinuations(t *testing.T) {
	cursor := NewCompositeCursor[testListItem](testOrigins())
	if cursor.Origins[0].Origin.Namespace != "a" {
		t.Fatalf("origins not canonical: %#v", cursor.Origins)
	}
	items, next, err := MergeOriginPages(cursor, []OriginPage[testListItem]{{Origin: cursor.Origins[0].Origin, Items: []testListItem{"a1", "a3"}, Continue: "native-a", ResourceVersion: "10"}, {Origin: cursor.Origins[1].Origin, Items: []testListItem{"a2", "a4"}, Continue: "native-b", ResourceVersion: "20"}}, 3, func(a, b testListItem) bool { return a < b })
	if err != nil {
		t.Fatal(err)
	}
	if got := joinTestItems(items); got != "a1,a2,a3" {
		t.Fatalf("items = %q", got)
	}
	if next.Origins[0].Continue != "native-a" || next.Origins[1].Continue != "native-b" {
		t.Fatalf("native tokens lost: %#v", next.Origins)
	}
	if len(next.Origins[1].Buffered) != 1 || next.Origins[1].Buffered[0] != "a4" {
		t.Fatalf("buffer = %#v", next.Origins[1].Buffered)
	}
	if next.Complete() {
		t.Fatal("cursor with native continuations must not be complete")
	}
}

func TestCompositeCursorRejectsMismatchImpossibleStateAndOversize(t *testing.T) {
	cursor := NewCompositeCursor[testListItem](testOrigins())
	cursor.Origins[0].Exhausted = true
	cursor.Origins[0].Continue = "invalid"
	if err := cursor.Validate(testOrigins()); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("impossible state: %v", err)
	}
	cursor = NewCompositeCursor[testListItem]([]Origin{{Namespace: "a", Version: "v1", Resource: "pods"}})
	cursor.Origins[0].Buffered = []testListItem{testListItem(strings.Repeat("x", 13<<10))}
	if err := cursor.Validate([]Origin{{Namespace: "a", Version: "v1", Resource: "pods"}}); ErrorCodeOf(err) != CodeLimitExceeded {
		t.Fatalf("oversize state: %v", err)
	}
	if err := NewCompositeCursor[testListItem](testOrigins()).Validate([]Origin{{Namespace: "other", Version: "v1", Resource: "pods"}}); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("mismatch: %v", err)
	}
}

func TestOriginsForWorkloadsBuildsNamespaceKindCartesianProduct(t *testing.T) {
	origins, err := OriginsFor(CollectionWorkloads, []string{"b", "a"}, []WorkloadKind{WorkloadJobs, WorkloadDeployments})
	if err != nil {
		t.Fatal(err)
	}
	if len(origins) != 4 {
		t.Fatalf("origin count = %d", len(origins))
	}
	for index := 1; index < len(origins); index++ {
		if origins[index-1].Key() > origins[index].Key() {
			t.Fatalf("origins are not stable: %#v", origins)
		}
	}
}
