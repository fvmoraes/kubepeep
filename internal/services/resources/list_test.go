package resources

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

type fakeAuthorization struct {
	mu        sync.Mutex
	decisions map[string]authorization.Decision
	keys      []authorization.Key
}

func (fake *fakeAuthorization) Check(_ context.Context, key authorization.Key) authorization.Capability {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.keys = append(fake.keys, key)
	decision := fake.decisions[key.Namespace]
	if decision == "" {
		decision = authorization.DecisionAllowed
	}
	return authorization.Capability{Decision: decision}
}

type fakeStringLister struct {
	mu      sync.Mutex
	pages   map[string]OriginPage[testListItem]
	errs    map[string]error
	calls   []PageRequest
	active  atomic.Int32
	maximum atomic.Int32
	delay   time.Duration
}

func (fake *fakeStringLister) ListPage(_ context.Context, request PageRequest) (OriginPage[testListItem], error) {
	active := fake.active.Add(1)
	defer fake.active.Add(-1)
	for {
		current := fake.maximum.Load()
		if active <= current || fake.maximum.CompareAndSwap(current, active) {
			break
		}
	}
	if fake.delay > 0 {
		time.Sleep(fake.delay)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.calls = append(fake.calls, request)
	page := fake.pages[request.Origin.Namespace]
	if page.Origin.Resource == "" {
		page.Origin = request.Origin
	}
	return page, fake.errs[request.Origin.Namespace]
}

func collectRequest(authorizer AuthorizationChecker, lister OriginLister[testListItem]) CollectionRequest[testListItem] {
	origins, _ := OriginsFor(CollectionPods, []string{"allowed", "denied"}, nil)
	return CollectionRequest[testListItem]{Selection: Selection{Generation: "gen-1", Context: "ctx", Scope: "scope", Namespaces: []string{"allowed", "denied"}}, Options: ListOptions{Limit: 10}, Origins: origins, Lister: lister, Authorizer: authorizer, Less: func(a, b testListItem) bool { return a < b }}
}

func TestCollectKeepsAllowedNamespacesAndNeverCallsDeniedOrigin(t *testing.T) {
	auth := &fakeAuthorization{decisions: map[string]authorization.Decision{"denied": authorization.DecisionDenied}}
	lister := &fakeStringLister{pages: map[string]OriginPage[testListItem]{"allowed": {Items: []testListItem{"b", "a"}}}, errs: map[string]error{}}
	result, err := Collect(context.Background(), collectRequest(auth, lister))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Items[0] != "a" || result.Items[1] != "b" {
		t.Fatalf("items = %#v", result.Items)
	}
	if result.Coverage.CompletedNamespaces != 1 || len(result.Coverage.DeniedNamespaces) != 1 || result.Coverage.DeniedNamespaces[0] != "denied" {
		t.Fatalf("coverage = %#v", result.Coverage)
	}
	if !result.Page.Truncated || result.Page.Complete {
		t.Fatalf("page = %#v", result.Page)
	}
	if len(lister.calls) != 1 || lister.calls[0].Origin.Namespace != "allowed" {
		t.Fatalf("calls = %#v", lister.calls)
	}
}

func TestCollectFailsClosedWhenNoAuthorizationIsKnown(t *testing.T) {
	auth := &fakeAuthorization{decisions: map[string]authorization.Decision{"allowed": authorization.DecisionUnknown, "denied": authorization.DecisionUnknown}}
	_, err := Collect(context.Background(), collectRequest(auth, &fakeStringLister{pages: map[string]OriginPage[testListItem]{}, errs: map[string]error{}}))
	if ErrorCodeOf(err) != CodeAuthorizationUnavailable {
		t.Fatalf("error = %v", err)
	}
}

func TestCollectDoesNotMisreportMixedDeniedAndUnknownAsTotalDenial(t *testing.T) {
	auth := &fakeAuthorization{decisions: map[string]authorization.Decision{"allowed": authorization.DecisionUnknown, "denied": authorization.DecisionDenied}}
	_, err := Collect(context.Background(), collectRequest(auth, &fakeStringLister{pages: map[string]OriginPage[testListItem]{}, errs: map[string]error{}}))
	if ErrorCodeOf(err) != CodeAuthorizationUnavailable {
		t.Fatalf("mixed decision = %v", err)
	}
}

func TestCollectDiscardsWholeWindowOnResourceExpired(t *testing.T) {
	auth := &fakeAuthorization{decisions: map[string]authorization.Decision{}}
	lister := &fakeStringLister{pages: map[string]OriginPage[testListItem]{"allowed": {Items: []testListItem{"must-not-leak"}}}, errs: map[string]error{"denied": ErrResourceExpired}}
	result, err := Collect(context.Background(), collectRequest(auth, lister))
	if ErrorCodeOf(err) != CodeCursorExpired {
		t.Fatalf("error = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("mixed snapshot leaked: %#v", result.Items)
	}
}

func TestCollectUsesAuthoritativeHTTPErrorWhenEveryAllowedOriginFails(t *testing.T) {
	auth := &fakeAuthorization{decisions: map[string]authorization.Decision{}}
	lister := &fakeStringLister{pages: map[string]OriginPage[testListItem]{}, errs: map[string]error{"allowed": context.DeadlineExceeded, "denied": context.DeadlineExceeded}}
	result, err := Collect(context.Background(), collectRequest(auth, lister))
	if ErrorCodeOf(err) != CodeUpstreamTimeout {
		t.Fatalf("error = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("failed total returned items: %#v", result.Items)
	}
}

func TestCollectReauthorizesBufferedCursorBeforeReturningIt(t *testing.T) {
	origin := Origin{Namespace: "denied", Version: "v1", Resource: "pods"}
	cursor := NewCompositeCursor[testListItem]([]Origin{origin})
	cursor.Origins[0].Buffered = []testListItem{"client-carried"}
	request := CollectionRequest[testListItem]{Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"}, Options: ListOptions{Limit: 1}, Origins: []Origin{origin}, Cursor: &cursor, Lister: &fakeStringLister{pages: map[string]OriginPage[testListItem]{}, errs: map[string]error{}}, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{"denied": authorization.DecisionDenied}}, Less: func(a, b testListItem) bool { return a < b }}
	result, err := Collect(context.Background(), request)
	if ErrorCodeOf(err) != CodeForbidden {
		t.Fatalf("error = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatal("buffered cursor bypassed current RBAC")
	}
}

func TestNormalizeListWindowTimeout(t *testing.T) {
	cases := map[string]struct {
		input    time.Duration
		expected time.Duration
	}{
		"zero falls back to default":      {input: 0, expected: DefaultListWindowTimeout},
		"negative falls back to default":  {input: -time.Second, expected: DefaultListWindowTimeout},
		"configured value is preserved":   {input: 45 * time.Second, expected: 45 * time.Second},
		"above ceiling is clamped":        {input: MaximumListWindowTimeout + time.Minute, expected: MaximumListWindowTimeout},
		"below minimum is still accepted": {input: time.Second, expected: time.Second},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if got := NormalizeListWindowTimeout(testCase.input); got != testCase.expected {
				t.Fatalf("NormalizeListWindowTimeout(%v) = %v, want %v", testCase.input, got, testCase.expected)
			}
		})
	}
}

// A slow namespace must not invalidate the healthy set: the window budget
// covers the whole fan-out while the per-origin call keeps its own deadline.
func TestCollectPreservesHealthyOriginsWhenOneOriginIsSlow(t *testing.T) {
	names := []string{"healthy-1", "healthy-2", "healthy-3", "healthy-4", "slow"}
	origins, _ := OriginsFor(CollectionPods, names, nil)
	lister := &fakeStringLister{pages: map[string]OriginPage[testListItem]{}, errs: map[string]error{}, delay: 0}
	for _, name := range names {
		if name == "slow" {
			lister.errs[name] = context.DeadlineExceeded
			continue
		}
		lister.pages[name] = OriginPage[testListItem]{Items: []testListItem{testListItem(name)}}
	}
	request := CollectionRequest[testListItem]{
		Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"},
		Options:   ListOptions{Limit: 20}, Origins: origins, Lister: lister,
		Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}},
		Less:       func(a, b testListItem) bool { return a < b },
		Timeout:    30 * time.Second,
	}
	result, err := Collect(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 4 {
		t.Fatalf("healthy items = %d, want 4", len(result.Items))
	}
	found := false
	for _, failure := range result.Coverage.Failed {
		if failure.Namespace == "slow" && failure.Code == CodeUpstreamTimeout {
			found = true
		}
	}
	if !found {
		t.Fatalf("slow namespace missing from coverage failures: %+v", result.Coverage.Failed)
	}
}

func TestCollectCapsFanoutConcurrencyAtFour(t *testing.T) {
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	origins, _ := OriginsFor(CollectionPods, names, nil)
	lister := &fakeStringLister{pages: map[string]OriginPage[testListItem]{}, errs: map[string]error{}, delay: 10 * time.Millisecond}
	for _, name := range names {
		lister.pages[name] = OriginPage[testListItem]{Items: []testListItem{testListItem(name)}}
	}
	request := CollectionRequest[testListItem]{Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"}, Options: ListOptions{Limit: 20}, Origins: origins, Lister: lister, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}}, Less: func(a, b testListItem) bool { return a < b }}
	if _, err := Collect(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if got := lister.maximum.Load(); got > MaximumFanout || got < 2 {
		t.Fatalf("maximum concurrency = %d", got)
	}
}

func TestOriginChunkLimitSeparatesPageSizeFromOriginChunk(t *testing.T) {
	cases := []struct {
		name      string
		origins   int
		pageLimit int
		expected  int64
	}{
		{"single origin keeps the page limit", 1, 100, 100},
		{"single origin with small page", 1, 5, 5},
		{"fan-out uses the default chunk", 100, 100, DefaultOriginChunkSize},
		{"fan-out respects a page smaller than the chunk", 50, 5, 5},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := originChunkLimit(testCase.origins, testCase.pageLimit); got != testCase.expected {
				t.Fatalf("originChunkLimit(%d, %d) = %d, want %d", testCase.origins, testCase.pageLimit, got, testCase.expected)
			}
		})
	}
}

func TestCollectFetchesOriginChunksInsteadOfFullPages(t *testing.T) {
	names := []string{"ns-a", "ns-b", "ns-c", "ns-d"}
	origins, _ := OriginsFor(CollectionPods, names, nil)
	lister := &fakeStringLister{pages: map[string]OriginPage[testListItem]{}, errs: map[string]error{}}
	for _, name := range names {
		lister.pages[name] = OriginPage[testListItem]{Items: []testListItem{testListItem(name)}}
	}
	request := CollectionRequest[testListItem]{Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"}, Options: ListOptions{Limit: 100}, Origins: origins, Lister: lister, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}}, Less: func(a, b testListItem) bool { return a < b }}
	if _, err := Collect(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(lister.calls) != len(names) {
		t.Fatalf("calls = %d, want %d", len(lister.calls), len(names))
	}
	for _, call := range lister.calls {
		if call.Limit != int64(DefaultOriginChunkSize) {
			t.Fatalf("origin %s fetched limit %d, want chunk %d", call.Origin.Namespace, call.Limit, DefaultOriginChunkSize)
		}
	}
}

func TestCollectSingleOriginFetchesTheFullPageLimit(t *testing.T) {
	origins, _ := GlobalOriginsFor(CollectionPods, nil)
	lister := &fakeStringLister{pages: map[string]OriginPage[testListItem]{"": {Items: []testListItem{"a", "b"}}}, errs: map[string]error{}}
	request := CollectionRequest[testListItem]{Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"}, Options: ListOptions{Limit: 100}, Origins: origins, Lister: lister, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}}, Less: func(a, b testListItem) bool { return a < b }}
	if _, err := Collect(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(lister.calls) != 1 || lister.calls[0].Limit != 100 {
		t.Fatalf("global origin calls = %#v", lister.calls)
	}
}

// A zero timeout must fall back to the configured 30s window budget, not the
// retired 10s constant, so wiring gaps cannot starve large scopes.
func TestCollectNormalizesZeroTimeoutToWindowBudget(t *testing.T) {
	origins, _ := OriginsFor(CollectionPods, []string{"allowed"}, nil)
	var observed time.Duration
	lister := listerFuncPage(func(ctx context.Context) {
		if deadline, ok := ctx.Deadline(); ok {
			observed = time.Until(deadline)
		}
	}, testListItem("a"))
	request := CollectionRequest[testListItem]{Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"}, Options: ListOptions{Limit: 10}, Origins: origins, Lister: lister, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}}, Less: func(a, b testListItem) bool { return a < b }, Timeout: 0}
	if _, err := Collect(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if observed < 29*time.Second {
		t.Fatalf("window deadline = %v, want the %v budget", observed, DefaultListWindowTimeout)
	}
}

// testOriginLister adapts a function to the OriginLister port.
type testOriginLister func(ctx context.Context, request PageRequest) (OriginPage[testListItem], error)

func (fn testOriginLister) ListPage(ctx context.Context, request PageRequest) (OriginPage[testListItem], error) {
	return fn(ctx, request)
}

// listerFuncPage returns an OriginLister that observes the request context and
// yields a single-item page.
func listerFuncPage(hook func(context.Context), item testListItem) OriginLister[testListItem] {
	return testOriginLister(func(ctx context.Context, request PageRequest) (OriginPage[testListItem], error) {
		hook(ctx)
		return OriginPage[testListItem]{Origin: request.Origin, Items: []testListItem{item}}, nil
	})
}

// pagingOriginLister serves deterministic chunks per origin, emulating the
// native Kubernetes limit/continue contract across collection windows.
type pagingOriginLister struct {
	perOrigin int
	mu        sync.Mutex
	served    map[string]int
}

func (lister *pagingOriginLister) ListPage(_ context.Context, request PageRequest) (OriginPage[testListItem], error) {
	lister.mu.Lock()
	defer lister.mu.Unlock()
	served := lister.served[request.Origin.Namespace]
	remaining := lister.perOrigin - served
	take := min(int(request.Limit), remaining)
	items := make([]testListItem, 0, take)
	for index := 0; index < take; index++ {
		items = append(items, testListItem(fmt.Sprintf("%s-%04d", request.Origin.Namespace, served+index)))
	}
	lister.served[request.Origin.Namespace] = served + take
	continueToken := ""
	if remaining > take {
		continueToken = "native-" + request.Origin.Namespace
	}
	return OriginPage[testListItem]{Origin: request.Origin, Items: items, Continue: continueToken}, nil
}

// The Logs catalog paginates the Pods collection with limit=500 across every
// scoped namespace. Pagination must converge with no gaps and no duplicates
// regardless of the window sizes the backend chooses per origin.
func TestCollectPaginatesLargeFanoutWithoutGapsOrDuplicates(t *testing.T) {
	names := make([]string, 40)
	for index := range names {
		names[index] = fmt.Sprintf("ns-%02d", index)
	}
	origins, _ := OriginsFor(CollectionPods, names, nil)
	lister := &pagingOriginLister{perOrigin: 37, served: map[string]int{}}
	var cursor *CompositeCursor[testListItem]
	collected := make(map[testListItem]int)
	windowItems := []int{}
	for window := 0; window < 100; window++ {
		request := CollectionRequest[testListItem]{
			Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"},
			Options:   ListOptions{Limit: 500}, Origins: origins, Cursor: cursor, Lister: lister,
			Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}},
			Less:       func(a, b testListItem) bool { return a < b },
		}
		result, err := Collect(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		windowItems = append(windowItems, len(result.Items))
		for _, item := range result.Items {
			collected[item]++
		}
		if result.Cursor.Complete() {
			if !result.Page.Complete {
				t.Fatalf("complete cursor reported an incomplete page: %#v", result.Page)
			}
			break
		}
		cursor = result.Cursor
	}
	if len(collected) != 40*37 {
		t.Fatalf("collected %d distinct items, want %d", len(collected), 40*37)
	}
	for item, count := range collected {
		if count != 1 {
			t.Fatalf("item %q appeared %d times", item, count)
		}
	}
	for namespace := range lister.served {
		if lister.served[namespace] != 37 {
			t.Fatalf("origin %s served %d items, want 37", namespace, lister.served[namespace])
		}
	}
	total := 0
	for _, count := range windowItems {
		total += count
	}
	if total != 40*37 {
		t.Fatalf("window sizes %v sum to %d, want %d", windowItems, total, 40*37)
	}
}

type fakeGetter struct {
	called bool
	value  PodDetailDTO
	err    error
}

func (fake *fakeGetter) Get(context.Context, Origin, string) (PodDetailDTO, error) {
	fake.called = true
	return fake.value, fake.err
}
func TestGetAuthorizedChecksResourceNameBeforeCallingGetter(t *testing.T) {
	getter := &fakeGetter{value: PodDetailDTO{Metadata: ResourceMetadataDTO{Name: "ok"}}}
	auth := &fakeAuthorization{decisions: map[string]authorization.Decision{"ns": authorization.DecisionDenied}}
	_, err := GetAuthorized(context.Background(), GetRequest[PodDetailDTO]{Selection: Selection{Generation: "gen"}, Origin: Origin{Namespace: "ns", Version: "v1", Resource: "pods"}, Name: "pod", Getter: getter, Authorizer: auth})
	if ErrorCodeOf(err) != CodeForbidden || getter.called {
		t.Fatalf("err=%v called=%v", err, getter.called)
	}
	auth.decisions["ns"] = authorization.DecisionAllowed
	got, err := GetAuthorized(context.Background(), GetRequest[PodDetailDTO]{Selection: Selection{Generation: "gen"}, Origin: Origin{Namespace: "ns", Version: "v1", Resource: "pods"}, Name: "pod", Getter: getter, Authorizer: auth})
	if err != nil || got.Metadata.Name != "ok" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	last := auth.keys[len(auth.keys)-1]
	if last.ResourceName != "pod" || last.Verb != "get" {
		t.Fatalf("authorization key = %#v", last)
	}
}

func TestReadErrorsAreSanitized(t *testing.T) {
	code, message := classifyReadError(errors.New("token=secret upstream exploded"))
	if code != CodeClusterUnavailable || message == "" || message == "token=secret upstream exploded" {
		t.Fatalf("classification leaked: %s %q", code, message)
	}
}
