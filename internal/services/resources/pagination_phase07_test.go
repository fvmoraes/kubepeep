package resources

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

type phase07ListItem struct {
	Namespace string
	Name      string
	Age       int
}

func (phase07ListItem) resourceListItem() {}

type phase07Lister struct {
	revision int
	items    map[string][]phase07ListItem
}

func (lister *phase07Lister) ListPage(_ context.Context, request PageRequest) (OriginPage[phase07ListItem], error) {
	offset := 0
	if request.Continue != "" {
		parts := strings.Split(request.Continue, ":")
		if len(parts) != 2 || parts[0] != fmt.Sprintf("rv-%d", lister.revision) {
			return OriginPage[phase07ListItem]{Origin: request.Origin}, ErrResourceExpired
		}
		parsed, err := strconv.Atoi(parts[1])
		if err != nil || parsed < 0 {
			return OriginPage[phase07ListItem]{Origin: request.Origin}, ErrResourceExpired
		}
		offset = parsed
	}
	items := lister.items[request.Origin.Namespace]
	end := min(offset+int(request.Limit), len(items))
	page := append([]phase07ListItem(nil), items[offset:end]...)
	next := ""
	if end < len(items) {
		next = fmt.Sprintf("rv-%d:%d", lister.revision, end)
	}
	return OriginPage[phase07ListItem]{
		Origin: request.Origin, Items: page, Continue: next,
		ResourceVersion: fmt.Sprintf("rv-%d", lister.revision),
	}, nil
}

func newPhase07Lister() *phase07Lister {
	items := make(map[string][]phase07ListItem, 10)
	for namespaceIndex := 0; namespaceIndex < 10; namespaceIndex++ {
		namespace := fmt.Sprintf("ns-%02d", namespaceIndex)
		items[namespace] = make([]phase07ListItem, 100)
		for itemIndex := range items[namespace] {
			items[namespace][itemIndex] = phase07ListItem{
				Namespace: namespace,
				Name:      fmt.Sprintf("pod-%03d", itemIndex),
				Age:       namespaceIndex*100 + (99 - itemIndex),
			}
		}
	}
	return &phase07Lister{revision: 1, items: items}
}

func phase07IdentityLess(left, right phase07ListItem) bool {
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	return left.Name < right.Name
}

func phase07PageLess(sortBy string, order SortOrder) func(phase07ListItem, phase07ListItem) bool {
	return func(left, right phase07ListItem) bool {
		comparison := 0
		if sortBy == "age" && left.Age != right.Age {
			if left.Age < right.Age {
				comparison = -1
			} else {
				comparison = 1
			}
		} else if phase07IdentityLess(left, right) {
			comparison = -1
		} else if phase07IdentityLess(right, left) {
			comparison = 1
		}
		if order == OrderDescending {
			return comparison > 0
		}
		return comparison < 0
	}
}

func TestPhase07PaginationLimit37AcrossOneThousandObjects(t *testing.T) {
	tests := []struct {
		name                   string
		sort                   string
		order                  SortOrder
		nativeIdentityOrdering bool
		wantCollectionOrder    bool
	}{
		{name: "identity ascending", sort: "identity", order: OrderAscending, nativeIdentityOrdering: true, wantCollectionOrder: true},
		{name: "identity descending is page local", sort: "identity", order: OrderDescending},
		{name: "age ascending is page local", sort: "age", order: OrderAscending},
		{name: "age descending is page local", sort: "age", order: OrderDescending},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			lister := newPhase07Lister()
			namespaces := make([]string, 10)
			for index := range namespaces {
				namespaces[index] = fmt.Sprintf("ns-%02d", index)
			}
			origins, err := OriginsFor(CollectionPods, namespaces, nil)
			if err != nil {
				t.Fatal(err)
			}
			var cursor *CompositeCursor[phase07ListItem]
			seen := make(map[string]struct{}, 1_000)
			all := make([]phase07ListItem, 0, 1_000)
			pageLess := phase07PageLess(testCase.sort, testCase.order)
			for pageNumber := 0; pageNumber < 100; pageNumber++ {
				result, collectErr := Collect(t.Context(), CollectionRequest[phase07ListItem]{
					Selection:           Selection{Generation: "phase07", Context: "ctx", Scope: "scope"},
					Options:             ListOptions{Limit: 37, Sort: testCase.sort, Order: testCase.order},
					NativeIdentityOrder: testCase.nativeIdentityOrdering,
					Origins:             origins, Cursor: cursor, Lister: lister,
					Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}},
					Less:       phase07IdentityLess,
				})
				if collectErr != nil {
					t.Fatal(collectErr)
				}
				if len(result.Items) == 0 || len(result.Items) > 37 {
					t.Fatalf("page %d returned %d items", pageNumber, len(result.Items))
				}
				sort.SliceStable(result.Items, func(left, right int) bool { return pageLess(result.Items[left], result.Items[right]) })
				if !sort.SliceIsSorted(result.Items, func(left, right int) bool { return pageLess(result.Items[left], result.Items[right]) }) {
					t.Fatalf("page %d is not sorted by %s/%s", pageNumber, testCase.sort, testCase.order)
				}
				for _, item := range result.Items {
					key := item.Namespace + "/" + item.Name
					if _, duplicate := seen[key]; duplicate {
						t.Fatalf("duplicate item %s on page %d", key, pageNumber)
					}
					seen[key] = struct{}{}
					all = append(all, item)
				}
				if result.Page.FilterScope != FilterScopePage {
					t.Fatalf("page %d claimed unsupported collection ordering: %+v", pageNumber, result.Page)
				}
				if result.Cursor == nil {
					t.Fatal("pagination omitted its cursor state")
				}
				if result.Cursor.Complete() {
					break
				}
				cursor = result.Cursor
			}
			if len(seen) != 1_000 || len(all) != 1_000 {
				t.Fatalf("collected %d distinct/%d total items, want 1000", len(seen), len(all))
			}
			if testCase.wantCollectionOrder && !sort.SliceIsSorted(all, func(left, right int) bool { return phase07IdentityLess(all[left], all[right]) }) {
				t.Fatal("demonstrated native identity ordering did not remain monotonic across pages")
			}
		})
	}
}

func TestPhase07MutationExpiresOldSnapshotBeforeMixingRevisions(t *testing.T) {
	lister := newPhase07Lister()
	origin := Origin{Namespace: "ns-00", Version: "v1", Resource: "pods"}
	cursor := NewCompositeCursor[phase07ListItem]([]Origin{origin})
	cursor.Origins[0].Continue = "rv-1:37"
	cursor.Origins[0].ResourceVersion = "rv-1"

	lister.revision = 2
	lister.items["ns-00"] = append([]phase07ListItem{{Namespace: "ns-00", Name: "aaa-before", Age: 1_000}}, lister.items["ns-00"]...)
	result, err := Collect(t.Context(), CollectionRequest[phase07ListItem]{
		Selection: Selection{Generation: "phase07-next", Context: "ctx", Scope: "scope"},
		Options:   ListOptions{Limit: 37, Sort: "identity", Order: OrderAscending},
		Origins:   []Origin{origin}, Cursor: &cursor, Lister: lister,
		Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}},
		Less:       phase07IdentityLess,
	})
	if ErrorCodeOf(err) != CodeCursorExpired {
		t.Fatalf("mutated snapshot error = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expired snapshot mixed revisions: %#v", result.Items)
	}

	restarted, err := Collect(t.Context(), CollectionRequest[phase07ListItem]{
		Selection:           Selection{Generation: "phase07-next", Context: "ctx", Scope: "scope"},
		Options:             ListOptions{Limit: 37, Sort: "identity", Order: OrderAscending},
		NativeIdentityOrder: true, Origins: []Origin{origin}, Lister: lister,
		Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}},
		Less:       phase07IdentityLess,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.Items) != 37 || restarted.Items[0].Name != "aaa-before" || restarted.Cursor.Origins[0].ResourceVersion != "rv-2" {
		t.Fatalf("fresh snapshot did not restart at revision 2: %+v", restarted)
	}
}

func TestPhase07CancellationReturnsWorkersToBaseline(t *testing.T) {
	baseline := runtime.NumGoroutine()
	origins, err := OriginsFor(CollectionPods, []string{"a", "b", "c", "d", "e", "f", "g", "h"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lister := &cancelingLister{started: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, collectErr := Collect(ctx, CollectionRequest[phase07ListItem]{
			Selection: Selection{Generation: "phase07", Context: "ctx", Scope: "scope"},
			Options:   ListOptions{Limit: 37, Sort: "age", Order: OrderDescending}, Origins: origins,
			Lister: phase07CancelLister{delegate: lister}, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}},
			Less: phase07IdentityLess,
		})
		done <- collectErr
	}()
	<-lister.started
	cancel()
	if collectErr := <-done; collectErr != context.Canceled {
		t.Fatalf("cancellation = %v", collectErr)
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if current := runtime.NumGoroutine(); current > baseline {
		t.Fatalf("goroutines did not return to baseline: before=%d after=%d", baseline, current)
	}
}

type phase07CancelLister struct{ delegate *cancelingLister }

func (lister phase07CancelLister) ListPage(ctx context.Context, request PageRequest) (OriginPage[phase07ListItem], error) {
	_, err := lister.delegate.ListPage(ctx, request)
	return OriginPage[phase07ListItem]{Origin: request.Origin}, err
}
