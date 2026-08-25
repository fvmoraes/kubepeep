package resources

import (
	"context"
	"errors"
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
