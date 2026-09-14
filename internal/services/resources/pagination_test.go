package resources

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

func TestSelectPaginationStrategyPreservesOrderingContract(t *testing.T) {
	origins, err := OriginsFor(CollectionPods, []string{"a", "b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name                   string
		options                ListOptions
		nativeIdentityOrdering bool
		want                   string
	}{
		{name: "ascending identity is monotonic", options: ListOptions{Sort: "identity", Order: OrderAscending}, nativeIdentityOrdering: true, want: "namespace-sequential"},
		{name: "unproven ascending identity remains page local", options: ListOptions{Sort: "identity", Order: OrderAscending}, want: "lazy-merge"},
		{name: "descending identity remains page local", options: ListOptions{Sort: "identity", Order: OrderDescending}, want: "lazy-merge"},
		{name: "age remains page local", options: ListOptions{Sort: "age", Order: OrderDescending}, want: "lazy-merge"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := CollectionRequest[testListItem]{Options: testCase.options, NativeIdentityOrder: testCase.nativeIdentityOrdering}
			if got := selectPaginationStrategy(request, origins).Name(); got != testCase.want {
				t.Fatalf("strategy = %q, want %q", got, testCase.want)
			}
		})
	}
	global, err := GlobalOriginsFor(CollectionPods, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := selectPaginationStrategy(CollectionRequest[testListItem]{}, global).Name(); got != "global-native" {
		t.Fatalf("global strategy = %q", got)
	}
}

func TestNamespaceSequentialStopsAfterFillingPage(t *testing.T) {
	names := make([]string, 50)
	for index := range names {
		names[index] = fmt.Sprintf("ns-%02d", index)
	}
	origins, err := OriginsFor(CollectionPods, names, nil)
	if err != nil {
		t.Fatal(err)
	}
	lister := &pagingOriginLister{perOrigin: 60, served: map[string]int{}}
	result, err := Collect(t.Context(), CollectionRequest[testListItem]{
		Selection:           Selection{Generation: "gen", Context: "ctx", Scope: "scope"},
		Options:             ListOptions{Limit: 50, Sort: "identity", Order: OrderAscending},
		NativeIdentityOrder: true,
		Origins:             origins,
		Lister:              lister,
		Authorizer:          &fakeAuthorization{decisions: map[string]authorization.Decision{}},
		Less:                func(left, right testListItem) bool { return left < right },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 50 || len(lister.served) != 1 || lister.served["ns-00"] != 50 {
		t.Fatalf("items=%d served=%v", len(result.Items), lister.served)
	}
	if result.Page.FilterScope != FilterScopePage || result.Page.Complete {
		t.Fatalf("page contract = %+v", result.Page)
	}
}

func TestLazyMergeLimitsOriginWindow(t *testing.T) {
	names := make([]string, 100)
	pages := make(map[string]OriginPage[testListItem], len(names))
	for index := range names {
		names[index] = fmt.Sprintf("ns-%03d", index)
		pages[names[index]] = OriginPage[testListItem]{Items: []testListItem{testListItem(names[index])}}
	}
	origins, err := OriginsFor(CollectionPods, names, nil)
	if err != nil {
		t.Fatal(err)
	}
	lister := &fakeStringLister{pages: pages, errs: map[string]error{}}
	result, err := Collect(t.Context(), CollectionRequest[testListItem]{
		Selection:  Selection{Generation: "gen", Context: "ctx", Scope: "scope"},
		Options:    ListOptions{Limit: 50, Sort: "age", Order: OrderDescending},
		Origins:    origins,
		Lister:     lister,
		Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}},
		Less:       func(left, right testListItem) bool { return left < right },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 50 {
		t.Fatalf("items = %d", len(result.Items))
	}
	lister.mu.Lock()
	calls := len(lister.calls)
	lister.mu.Unlock()
	if calls >= len(names) || calls > 52 {
		t.Fatalf("lazy window queried %d of %d origins", calls, len(names))
	}
	if result.Page.FilterScope != FilterScopePage || result.Page.Complete {
		t.Fatalf("page contract = %+v", result.Page)
	}
}

type cancelingLister struct {
	started chan struct{}
	once    sync.Once
}

func (lister *cancelingLister) ListPage(ctx context.Context, request PageRequest) (OriginPage[testListItem], error) {
	lister.once.Do(func() { close(lister.started) })
	<-ctx.Done()
	return OriginPage[testListItem]{Origin: request.Origin}, ctx.Err()
}

func TestPaginationCancellationStopsWorkers(t *testing.T) {
	origins, err := OriginsFor(CollectionPods, []string{"a", "b", "c", "d", "e"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lister := &cancelingLister{started: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, collectErr := Collect(ctx, CollectionRequest[testListItem]{
			Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"}, Options: ListOptions{Limit: 10, Sort: "age", Order: OrderDescending},
			Origins: origins, Lister: lister, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}}, Less: func(left, right testListItem) bool { return left < right },
		})
		done <- collectErr
	}()
	<-lister.started
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("cancellation = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pagination workers did not stop")
	}
}

func TestCollectPropagatesSelectorsToLister(t *testing.T) {
	options, err := NormalizeListOptions(CollectionPods, ListOptions{Limit: 10, Node: "worker-1", LabelSelector: "app=api"})
	if err != nil {
		t.Fatal(err)
	}
	origins, _ := OriginsFor(CollectionPods, []string{"default"}, nil)
	var observed PageRequest
	lister := testOriginLister(func(_ context.Context, request PageRequest) (OriginPage[testListItem], error) {
		observed = request
		return OriginPage[testListItem]{Origin: request.Origin, Items: []testListItem{"pod"}}, nil
	})
	_, err = Collect(t.Context(), CollectionRequest[testListItem]{
		Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"}, Options: options, Origins: origins,
		Lister: lister, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}}, Less: func(left, right testListItem) bool { return left < right },
	})
	if err != nil {
		t.Fatal(err)
	}
	if observed.LabelSelector != "app=api" || observed.FieldSelector != "spec.nodeName=worker-1" {
		t.Fatalf("selectors = label:%q field:%q", observed.LabelSelector, observed.FieldSelector)
	}
}

type benchmarkLatencyLister struct{ delay time.Duration }

func (lister benchmarkLatencyLister) ListPage(ctx context.Context, request PageRequest) (OriginPage[testListItem], error) {
	timer := time.NewTimer(lister.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return OriginPage[testListItem]{Origin: request.Origin}, ctx.Err()
	case <-timer.C:
		return OriginPage[testListItem]{Origin: request.Origin, Items: []testListItem{testListItem(request.Origin.Namespace)}}, nil
	}
}

// BenchmarkCollectWorkerPoolFanout keeps the 4/6/8 tuning decision
// reproducible without changing the safe production default of four.
func BenchmarkCollectWorkerPoolFanout(b *testing.B) {
	names := make([]string, 32)
	for index := range names {
		names[index] = fmt.Sprintf("ns-%02d", index)
	}
	origins, err := OriginsFor(CollectionPods, names, nil)
	if err != nil {
		b.Fatal(err)
	}
	for _, fanout := range []int{4, 6, 8} {
		b.Run(fmt.Sprintf("fanout-%d", fanout), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, collectErr := Collect(b.Context(), CollectionRequest[testListItem]{
					Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"},
					Options:   ListOptions{Limit: 32, Sort: "age", Order: OrderDescending},
					Origins:   origins, Lister: benchmarkLatencyLister{delay: time.Millisecond}, Fanout: fanout,
					Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}},
					Less:       func(left, right testListItem) bool { return left < right },
				})
				if collectErr != nil {
					b.Fatal(collectErr)
				}
			}
		})
	}
}
