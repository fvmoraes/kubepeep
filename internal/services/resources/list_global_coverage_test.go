package resources

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

// A cluster-wide LIST has one global origin per GVR, all carrying the empty
// namespace, so the shared fakes keyed by namespace cannot tell them apart.
// These are keyed by resource instead.
type resourceKeyedAuthorization struct {
	mu        sync.Mutex
	decisions map[string]authorization.Decision
}

func (fake *resourceKeyedAuthorization) Check(_ context.Context, key authorization.Key) authorization.Capability {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	decision := fake.decisions[key.Resource]
	if decision == "" {
		decision = authorization.DecisionAllowed
	}
	return authorization.Capability{Decision: decision}
}

type resourceKeyedLister struct {
	mu    sync.Mutex
	pages map[string]OriginPage[testListItem]
	errs  map[string]error
}

func (fake *resourceKeyedLister) ListPage(_ context.Context, request PageRequest) (OriginPage[testListItem], error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	page := fake.pages[request.Origin.Resource]
	page.Origin = request.Origin
	return page, fake.errs[request.Origin.Resource]
}

func globalCollectRequest(authorizer AuthorizationChecker, lister OriginLister[testListItem]) CollectionRequest[testListItem] {
	origins, _ := GlobalOriginsFor(CollectionWorkloads, nil)
	selection := Selection{Generation: "gen-1", Context: "ctx", Scope: "scope", Namespaces: []string{""}}
	return CollectionRequest[testListItem]{
		ReadThroughUnknown: true,
		Selection:          selection,
		Options:            ListOptions{Limit: 10},
		Origins:            origins,
		Lister:             lister,
		Authorizer:         authorizer,
		Less:               func(a, b testListItem) bool { return a < b },
		// The caller declares how many namespaces the scope asked for; a
		// cluster-wide read enumerates none of them individually.
		RequestedNamespaces: 7,
	}
}

// A cluster-wide read may report the requested namespace count instead of the
// single empty-namespace origin it enumerated, but only when nothing failed.
// With one global origin per GVR, a success on one kind must not vouch for the
// whole scope while another kind was denied or unavailable.
func TestGlobalCoverageDoesNotClaimTheScopeCompleteWhenAKindFails(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		decisions   map[string]authorization.Decision
		errs        map[string]error
		wantPartial bool
	}{
		{
			name:        "every kind succeeds",
			wantPartial: false,
		},
		{
			name:        "one kind denied by review",
			decisions:   map[string]authorization.Decision{"jobs": authorization.DecisionDenied},
			wantPartial: true,
		},
		{
			name:        "one kind fails the live read",
			errs:        map[string]error{"jobs": errors.New("upstream is unavailable")},
			wantPartial: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decisions := test.decisions
			if decisions == nil {
				decisions = map[string]authorization.Decision{}
			}
			errs := test.errs
			if errs == nil {
				errs = map[string]error{}
			}
			lister := &resourceKeyedLister{
				pages: map[string]OriginPage[testListItem]{"deployments": {Items: []testListItem{"api"}}},
				errs:  errs,
			}
			result, err := Collect(context.Background(), globalCollectRequest(&resourceKeyedAuthorization{decisions: decisions}, lister))
			if err != nil {
				t.Fatal(err)
			}
			if result.Coverage.RequestedNamespaces != 7 {
				t.Fatalf("requested namespaces = %d, want the scope's count", result.Coverage.RequestedNamespaces)
			}
			if test.wantPartial {
				if result.Coverage.CompletedNamespaces == result.Coverage.RequestedNamespaces {
					t.Fatalf("coverage claims the whole scope complete while a kind failed: %#v", result.Coverage)
				}
				if len(result.Coverage.Failed) == 0 {
					t.Fatalf("a failing kind must appear in the failed list: %#v", result.Coverage)
				}
				return
			}
			if result.Coverage.CompletedNamespaces != result.Coverage.RequestedNamespaces {
				t.Fatalf("a clean cluster-wide read must report every requested namespace complete: %#v", result.Coverage)
			}
		})
	}
}

// A denial on a global origin has no namespace to name. A denial covering
// every origin short-circuits to a 403 before coverage is built, so an empty
// entry could only appear beside a partial success, where it names nothing the
// operator can act on.
func TestGlobalCoverageNeverPublishesAnEmptyDeniedNamespace(t *testing.T) {
	t.Parallel()
	lister := &resourceKeyedLister{
		pages: map[string]OriginPage[testListItem]{"deployments": {Items: []testListItem{"api"}}},
		errs:  map[string]error{},
	}
	authorizer := &resourceKeyedAuthorization{decisions: map[string]authorization.Decision{"jobs": authorization.DecisionDenied}}
	result, err := Collect(context.Background(), globalCollectRequest(authorizer, lister))
	if err != nil {
		t.Fatal(err)
	}
	for _, namespace := range result.Coverage.DeniedNamespaces {
		if namespace == "" {
			t.Fatalf("denied namespaces published an empty entry: %#v", result.Coverage)
		}
	}
	// The denial must still be reported, just not as a nameless namespace.
	if len(result.Coverage.Failed) == 0 {
		t.Fatalf("the denied kind disappeared from coverage entirely: %#v", result.Coverage)
	}
}

// Every namespaced DTO admitted to the global-in-scope strategy must expose
// the namespace the per-namespace fan-out was keyed by. Events carry two
// namespaces and only one of them is the Event's own: swapping the accessor to
// ObjectNamespace would silently change which Events a scope admits.
func TestNamespacedListItemAccessorsReportTheRowsOwnNamespace(t *testing.T) {
	t.Parallel()
	event := EventDTO{Namespace: "where-the-event-lives", ObjectNamespace: "where-the-subject-lives"}
	if got := event.ListNamespace(); got != "where-the-event-lives" {
		t.Fatalf("EventDTO.ListNamespace = %q, want the Event's own namespace and not the subject's", got)
	}
	for name, item := range map[string]NamespacedListItem{
		"PodDTO":           PodDTO{Namespace: "ns"},
		"WorkloadDTO":      WorkloadDTO{Namespace: "ns"},
		"ServiceDTO":       ServiceDTO{Namespace: "ns"},
		"IngressDTO":       IngressDTO{Namespace: "ns"},
		"EndpointSliceDTO": EndpointSliceDTO{Namespace: "ns"},
		"ConfigMapListDTO": ConfigMapListDTO{Namespace: "ns"},
		"EventDTO":         EventDTO{Namespace: "ns"},
	} {
		if got := item.ListNamespace(); got != "ns" {
			t.Fatalf("%s.ListNamespace = %q, want ns", name, got)
		}
	}
}

// A cluster-scoped DTO must not satisfy the interface: that is what keeps the
// global-in-scope filter unavailable to collections it cannot filter.
func TestClusterScopedDTOsDoNotSatisfyNamespacedListItem(t *testing.T) {
	t.Parallel()
	for name, item := range map[string]ListItem{
		"NodeDTO":              NodeDTO{},
		"PersistentVolumeDTO":  PersistentVolumeDTO{},
		"StorageClassDTO":      StorageClassDTO{},
		"PriorityClassDTO":     PriorityClassDTO{},
		"CustomResourceDefDTO": CustomResourceDefinitionDTO{},
	} {
		if _, ok := any(item).(NamespacedListItem); ok {
			t.Fatalf("%s satisfies NamespacedListItem; a cluster-scoped row has no namespace to filter by", name)
		}
	}
}
