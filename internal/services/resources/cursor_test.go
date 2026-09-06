package resources

import (
	"fmt"
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

func TestCompositeCursorRejectsMismatchAndImpossibleState(t *testing.T) {
	cursor := NewCompositeCursor[testListItem](testOrigins())
	cursor.Origins[0].Exhausted = true
	cursor.Origins[0].Continue = "invalid"
	if err := cursor.Validate(testOrigins()); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("impossible state: %v", err)
	}
	if err := NewCompositeCursor[testListItem](testOrigins()).Validate([]Origin{{Namespace: "other", Version: "v1", Resource: "pods"}}); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("mismatch: %v", err)
	}
	// Buffered DTOs no longer travel inside the token, so their size cannot
	// invalidate Validate. The equivalent oversize guard lives in the
	// server-side CursorStore entry cap.
	big := NewCompositeCursor[testListItem]([]Origin{{Namespace: "a", Version: "v1", Resource: "pods"}})
	big.Origins[0].Buffered = []testListItem{testListItem(strings.Repeat("x", 13<<10))}
	if err := big.Validate([]Origin{{Namespace: "a", Version: "v1", Resource: "pods"}}); err != nil {
		t.Fatalf("server-side buffer size must not fail validation: %v", err)
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

// BenchmarkMergeOriginPages measures the deterministic k-way merge across
// fan-out widths and chunk sizes. Baseline for the lazy-paginator work.
func BenchmarkMergeOriginPages(b *testing.B) {
	less := func(a, b testListItem) bool { return a < b }
	for _, origins := range []int{10, 50, 100, 200} {
		for _, chunk := range []int{10, 50} {
			b.Run(fmt.Sprintf("origins=%d/chunk=%d", origins, chunk), func(b *testing.B) {
				cursorOrigins := make([]Origin, origins)
				for index := range cursorOrigins {
					cursorOrigins[index] = Origin{Namespace: fmt.Sprintf("ns-%04d", index), Version: "v1", Resource: "pods"}
				}
				cursor := NewCompositeCursor[testListItem](cursorOrigins)
				pages := make([]OriginPage[testListItem], origins)
				for index := range cursor.Origins {
					origin := cursor.Origins[index].Origin
					items := make([]testListItem, chunk)
					for item := range items {
						items[item] = testListItem(fmt.Sprintf("%04d-%04d", index, item))
					}
					pages[index] = OriginPage[testListItem]{Origin: origin, Items: items, Continue: fmt.Sprintf("native-%d", index)}
				}
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					if _, _, err := MergeOriginPages(cursor, pages, DefaultListLimit, less); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
