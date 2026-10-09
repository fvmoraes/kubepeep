package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

func TestUnknownReviewRequiresLiveListBeforeReturningBufferedRows(t *testing.T) {
	for _, strategy := range []string{"global", "sequential", "merge"} {
		for _, test := range []struct {
			name    string
			failure error
			code    ErrorCode
		}{
			{name: "actual allow"},
			{name: "actual denial", failure: domainError(CodeForbidden, "Kubernetes rejected LIST (403).", nil), code: CodeForbidden},
			{name: "credential failure", failure: domainError(CodeAuthenticationUnavailable, "Credentials rejected (401).", nil), code: CodeAuthenticationUnavailable},
			{name: "timeout", failure: domainError(CodeUpstreamTimeout, "LIST timed out.", nil), code: CodeUpstreamTimeout},
		} {
			t.Run(strategy+"/"+test.name, func(t *testing.T) {
				namespace := "ns"
				if strategy == "global" {
					namespace = ""
				}
				origin := Origin{Namespace: namespace, Version: "v1", Resource: "pods"}
				lister := &fakeStringLister{errs: map[string]error{namespace: test.failure}}
				cursor := NewCompositeCursor[testListItem]([]Origin{origin})
				cursor.Origins[0].Buffered = []testListItem{"private-row"}
				cursor.Origins[0].Exhausted = true
				request := CollectionRequest[testListItem]{Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"}, Options: ListOptions{Limit: 1, Sort: "identity", Order: OrderAscending}, Origins: []Origin{origin}, Cursor: &cursor, Lister: lister, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{namespace: authorization.DecisionUnknown}}, Less: func(a, b testListItem) bool { return a < b }, ReadThroughUnknown: true, NativeIdentityOrder: strategy == "sequential", Retry: RetryPolicy{Attempts: 1}}
				result, err := Collect(t.Context(), request)
				if len(lister.calls) != 1 || lister.calls[0].Limit != 1 || lister.calls[0].Continue != "" {
					t.Fatalf("authorization probe not bounded: %+v", lister.calls)
				}
				if test.failure == nil {
					if err != nil || len(result.Items) != 1 {
						t.Fatalf("live allow did not release row: %+v %v", result, err)
					}
				} else if ErrorCodeOf(err) != test.code || len(result.Items) != 0 {
					t.Fatalf("live failure lost or buffered rows leaked: %+v %v", result, err)
				}
			})
		}
	}
}

func TestReadThroughUnknownDoesNotCallExplicitlyDeniedOrigin(t *testing.T) {
	lister := &fakeStringLister{}
	request := collectRequest(&fakeAuthorization{decisions: map[string]authorization.Decision{"allowed": authorization.DecisionDenied, "denied": authorization.DecisionDenied}}, lister)
	request.ReadThroughUnknown = true
	_, err := Collect(t.Context(), request)
	if ErrorCodeOf(err) != CodeForbidden || len(lister.calls) != 0 {
		t.Fatalf("explicit denial bypassed: %v %+v", err, lister.calls)
	}
}

func TestRealDenialOverridesAllowedReviewAndBufferedRows(t *testing.T) {
	origin := Origin{Namespace: "ns", Version: "v1", Resource: "pods"}
	cursor := NewCompositeCursor[testListItem]([]Origin{origin})
	cursor.Origins[0].Buffered = []testListItem{"must-not-leak"}
	cursor.Origins[0].Continue = "previous-page"
	lister := &fakeStringLister{errs: map[string]error{"ns": domainError(CodeForbidden, "Live LIST denied (403).", nil)}}
	result, err := Collect(t.Context(), CollectionRequest[testListItem]{Selection: Selection{Generation: "gen", Context: "ctx", Scope: "scope"}, Options: ListOptions{Limit: 2}, Origins: []Origin{origin}, Cursor: &cursor, Lister: lister, Authorizer: &fakeAuthorization{}, Less: func(a, b testListItem) bool { return a < b }, ReadThroughUnknown: true})
	if ErrorCodeOf(err) != CodeForbidden || len(result.Items) != 0 || len(lister.calls) != 1 {
		t.Fatalf("real denial lost to earlier review or buffer: %+v %v calls=%d", result, err, len(lister.calls))
	}
}

type recoveringLister struct {
	calls   int
	failure error
}

func (lister *recoveringLister) ListPage(_ context.Context, request PageRequest) (OriginPage[testListItem], error) {
	lister.calls++
	if lister.calls < 3 {
		return OriginPage[testListItem]{Origin: request.Origin}, lister.failure
	}
	return OriginPage[testListItem]{Origin: request.Origin, Items: []testListItem{"recovered"}}, nil
}

func TestReadRetriesTransientFailuresButNeverDenial(t *testing.T) {
	for _, code := range []ErrorCode{CodeClusterUnavailable, CodeUpstreamTimeout, CodeAuthenticationUnavailable, CodeForbidden, CodeNotFound} {
		t.Run(string(code), func(t *testing.T) {
			lister := &recoveringLister{failure: domainError(code, "safe detail", errors.New("private credential"))}
			waits := 0
			page, err := retryListPage(t.Context(), lister, PageRequest{Origin: Origin{Version: "v1", Resource: "pods"}}, RetryPolicy{Wait: func(context.Context, time.Duration) error { waits++; return nil }}, nil)
			if code == CodeForbidden || code == CodeNotFound {
				if err == nil || lister.calls != 1 || waits != 0 {
					t.Fatalf("permanent failure retried: %v %d", err, lister.calls)
				}
			} else if err != nil || lister.calls != 3 || waits != 2 || len(page.Items) != 1 {
				t.Fatalf("transient failure not recovered: %+v %v %d", page, err, lister.calls)
			}
		})
	}
}

func TestUnknownDetailReviewUsesActualRead(t *testing.T) {
	for _, denied := range []bool{false, true} {
		getter := &fakeGetter{value: PodDetailDTO{Metadata: ResourceMetadataDTO{Name: "live"}}}
		if denied {
			getter.err = domainError(CodeForbidden, "Actual GET denied.", nil)
		}
		value, err := GetAuthorized(t.Context(), GetRequest[PodDetailDTO]{Selection: Selection{Generation: "gen"}, Origin: Origin{Namespace: "ns", Version: "v1", Resource: "pods"}, Name: "live", Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{"ns": authorization.DecisionUnknown}}, Getter: getter, ReadThroughUnknown: true})
		if !getter.called || denied && (ErrorCodeOf(err) != CodeForbidden || value.Metadata.Name != "") || !denied && (err != nil || value.Metadata.Name != "live") {
			t.Fatalf("actual authority lost: %+v %v", value, err)
		}
	}
}
