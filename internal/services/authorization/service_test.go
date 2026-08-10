package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeAccessReviewer struct {
	mu    sync.Mutex
	calls map[Key]int
	fn    func(context.Context, Key) (AccessReviewResult, error)
}

func (reviewer *fakeAccessReviewer) ReviewAccess(ctx context.Context, key Key) (AccessReviewResult, error) {
	reviewer.mu.Lock()
	if reviewer.calls == nil {
		reviewer.calls = make(map[Key]int)
	}
	reviewer.calls[key]++
	reviewer.mu.Unlock()
	return reviewer.fn(ctx, key)
}

func (reviewer *fakeAccessReviewer) callCount(key Key) int {
	reviewer.mu.Lock()
	defer reviewer.mu.Unlock()
	return reviewer.calls[key]
}

func (reviewer *fakeAccessReviewer) totalCalls() int {
	reviewer.mu.Lock()
	defer reviewer.mu.Unlock()
	total := 0
	for _, count := range reviewer.calls {
		total += count
	}
	return total
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *fakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *fakeClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(duration)
}

func allowedReview(context.Context, Key) (AccessReviewResult, error) {
	return AccessReviewResult{Allowed: true, Complete: true}, nil
}

func testKey() Key {
	return Key{
		Generation:   "gen_42",
		Namespace:    "payments",
		APIGroup:     "apps",
		Resource:     "deployments",
		Subresource:  "scale",
		Verb:         "update",
		ResourceName: "api",
	}
}

func newTestService(t *testing.T, reviewer AccessReviewer, options Options) *Service {
	t.Helper()
	service, err := New(reviewer, options)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

func TestTTLDefaultsBoundsCachingAndExpiry(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}
	reviewer := &fakeAccessReviewer{fn: allowedReview}
	service := newTestService(t, reviewer, Options{Now: clock.Now})
	if service.TTL() != DefaultTTL {
		t.Fatalf("TTL() = %s, want %s", service.TTL(), DefaultTTL)
	}

	key := testKey()
	first := service.Check(context.Background(), key)
	second := service.Check(context.Background(), key)
	if first.Decision != DecisionAllowed || second.Decision != DecisionAllowed {
		t.Fatalf("decisions = %q, %q", first.Decision, second.Decision)
	}
	if got := reviewer.callCount(key); got != 1 {
		t.Fatalf("review calls before expiry = %d, want 1", got)
	}
	if want := clock.Now().Add(DefaultTTL); !first.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %s, want %s", first.ExpiresAt, want)
	}

	clock.Advance(DefaultTTL - time.Nanosecond)
	service.Check(context.Background(), key)
	if got := reviewer.callCount(key); got != 1 {
		t.Fatalf("review calls inside TTL = %d, want 1", got)
	}
	clock.Advance(time.Nanosecond)
	service.Check(context.Background(), key)
	if got := reviewer.callCount(key); got != 2 {
		t.Fatalf("review calls at expiry = %d, want 2", got)
	}

	for _, ttl := range []time.Duration{MinTTL - time.Nanosecond, MaxTTL + time.Nanosecond} {
		if _, err := New(reviewer, Options{TTL: ttl}); ErrorCodeOf(err) != CodeValidationFailed {
			t.Fatalf("New(TTL=%s) code = %q, want %q", ttl, ErrorCodeOf(err), CodeValidationFailed)
		}
	}
	for _, ttl := range []time.Duration{MinTTL, MaxTTL} {
		if _, err := New(reviewer, Options{TTL: ttl}); err != nil {
			t.Fatalf("New(TTL=%s) error = %v", ttl, err)
		}
	}
}

func TestCompleteKeySeparatesEveryCacheDimension(t *testing.T) {
	reviewer := &fakeAccessReviewer{fn: allowedReview}
	service := newTestService(t, reviewer, Options{})
	base := testKey()
	keys := []Key{
		base,
		withKey(base, func(key *Key) { key.Generation = "gen_43" }),
		withKey(base, func(key *Key) { key.Namespace = "billing" }),
		withKey(base, func(key *Key) { key.APIGroup = "autoscaling" }),
		withKey(base, func(key *Key) { key.Resource = "statefulsets" }),
		withKey(base, func(key *Key) { key.Subresource = "status" }),
		withKey(base, func(key *Key) { key.Verb = "patch" }),
		withKey(base, func(key *Key) { key.ResourceName = "worker" }),
	}
	for _, key := range keys {
		if capability := service.Check(context.Background(), key); capability.Decision != DecisionAllowed {
			t.Fatalf("Check(%+v) = %q", key, capability.Decision)
		}
	}
	if got := reviewer.totalCalls(); got != len(keys) {
		t.Fatalf("review calls = %d, want %d", got, len(keys))
	}
}

func withKey(key Key, mutate func(*Key)) Key {
	mutate(&key)
	return key
}

func TestReviewTriStateAndSafeReasonCodes(t *testing.T) {
	reviewer := &fakeAccessReviewer{fn: func(_ context.Context, key Key) (AccessReviewResult, error) {
		switch key.ResourceName {
		case "allowed":
			return AccessReviewResult{Allowed: true, Complete: true}, nil
		case "denied":
			return AccessReviewResult{Denied: true, Complete: true}, nil
		case "incomplete":
			return AccessReviewResult{}, nil
		case "conflict":
			return AccessReviewResult{Allowed: true, Denied: true, Complete: true}, nil
		case "timeout":
			return AccessReviewResult{}, context.DeadlineExceeded
		default:
			return AccessReviewResult{}, errors.New("Bearer secret-token must not escape")
		}
	}}
	service := newTestService(t, reviewer, Options{})
	tests := []struct {
		name     string
		decision Decision
		reason   ReasonCode
	}{
		{name: "allowed", decision: DecisionAllowed, reason: ReasonSARAllowed},
		{name: "denied", decision: DecisionDenied, reason: ReasonSARDenied},
		{name: "incomplete", decision: DecisionUnknown, reason: ReasonSARIncomplete},
		{name: "conflict", decision: DecisionUnknown, reason: ReasonSARIncomplete},
		{name: "timeout", decision: DecisionUnknown, reason: ReasonSARTimeout},
		{name: "remote-error", decision: DecisionUnknown, reason: ReasonSARUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key := testKey()
			key.ResourceName = test.name
			capability := service.Check(context.Background(), key)
			if capability.Decision != test.decision || capability.ReasonCode != test.reason {
				t.Fatalf("decision/reason = %q/%q, want %q/%q", capability.Decision, capability.ReasonCode, test.decision, test.reason)
			}
			encoded, err := json.Marshal(capability)
			if err != nil {
				t.Fatal(err)
			}
			if contains := string(encoded); containsSecret(contains) {
				t.Fatalf("capability leaked upstream text: %s", contains)
			}
		})
	}
}

func containsSecret(value string) bool {
	return value == "Bearer secret-token must not escape" ||
		len(value) >= len("Bearer secret-token") && contains(value, "Bearer secret-token")
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

func TestConcurrentChecksAreDeduplicated(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	reviewer := &fakeAccessReviewer{fn: func(ctx context.Context, _ Key) (AccessReviewResult, error) {
		once.Do(func() { close(started) })
		select {
		case <-release:
			return AccessReviewResult{Allowed: true, Complete: true}, nil
		case <-ctx.Done():
			return AccessReviewResult{}, ctx.Err()
		}
	}}
	service := newTestService(t, reviewer, Options{})
	key := testKey()

	const goroutines = 64
	start := make(chan struct{})
	var ready atomic.Int32
	var wait sync.WaitGroup
	wait.Add(goroutines)
	results := make(chan Capability, goroutines)
	for range goroutines {
		go func() {
			defer wait.Done()
			ready.Add(1)
			<-start
			results <- service.Check(context.Background(), key)
		}()
	}
	for ready.Load() != goroutines {
		time.Sleep(time.Millisecond)
	}
	close(start)
	<-started
	time.Sleep(20 * time.Millisecond)
	if got := reviewer.callCount(key); got != 1 {
		t.Fatalf("concurrent review calls while blocked = %d, want 1", got)
	}
	close(release)
	wait.Wait()
	close(results)
	for capability := range results {
		if capability.Decision != DecisionAllowed {
			t.Fatalf("decision = %q, want allowed", capability.Decision)
		}
	}
	if got := reviewer.callCount(key); got != 1 {
		t.Fatalf("total concurrent review calls = %d, want 1", got)
	}
}

func TestConcurrentRefreshesShareOneLiveReview(t *testing.T) {
	var blocking atomic.Bool
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	reviewer := &fakeAccessReviewer{fn: func(ctx context.Context, _ Key) (AccessReviewResult, error) {
		if !blocking.Load() {
			return AccessReviewResult{Allowed: true, Complete: true}, nil
		}
		once.Do(func() { close(started) })
		select {
		case <-release:
			return AccessReviewResult{Denied: true, Complete: true}, nil
		case <-ctx.Done():
			return AccessReviewResult{}, ctx.Err()
		}
	}}
	service := newTestService(t, reviewer, Options{})
	key := testKey()
	service.Check(context.Background(), key)
	blocking.Store(true)

	const goroutines = 32
	start := make(chan struct{})
	var ready atomic.Int32
	var wait sync.WaitGroup
	wait.Add(goroutines)
	for range goroutines {
		go func() {
			defer wait.Done()
			ready.Add(1)
			<-start
			if got := service.Refresh(context.Background(), key).Decision; got != DecisionDenied {
				t.Errorf("refreshed decision = %q, want denied", got)
			}
		}()
	}
	for ready.Load() != goroutines {
		time.Sleep(time.Millisecond)
	}
	close(start)
	<-started
	time.Sleep(20 * time.Millisecond)
	if got := reviewer.callCount(key); got != 2 {
		t.Fatalf("calls while concurrent refresh is blocked = %d, want 2 including prime", got)
	}
	close(release)
	wait.Wait()
	if got := reviewer.callCount(key); got != 2 {
		t.Fatalf("total calls after concurrent refresh = %d, want 2", got)
	}
}

func TestRefreshInvalidationAndRBACChange(t *testing.T) {
	var allowed atomic.Bool
	allowed.Store(true)
	reviewer := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
		if allowed.Load() {
			return AccessReviewResult{Allowed: true, Complete: true}, nil
		}
		return AccessReviewResult{Denied: true, Complete: true}, nil
	}}
	service := newTestService(t, reviewer, Options{})
	key := testKey()
	otherGeneration := key
	otherGeneration.Generation = "gen_other"

	if got := service.Check(context.Background(), key).Decision; got != DecisionAllowed {
		t.Fatalf("initial decision = %q", got)
	}
	service.Check(context.Background(), otherGeneration)
	allowed.Store(false)
	if got := service.Check(context.Background(), key).Decision; got != DecisionAllowed {
		t.Fatalf("cached decision = %q, want allowed", got)
	}
	if got := service.Refresh(context.Background(), key).Decision; got != DecisionDenied {
		t.Fatalf("refreshed decision = %q, want denied", got)
	}
	if calls := reviewer.callCount(key); calls != 2 {
		t.Fatalf("calls after refresh = %d, want 2", calls)
	}

	service.InvalidateGeneration(key.Generation)
	service.Check(context.Background(), key)
	if calls := reviewer.callCount(key); calls != 3 {
		t.Fatalf("calls after generation invalidation = %d, want 3", calls)
	}
	service.Check(context.Background(), otherGeneration)
	if calls := reviewer.callCount(otherGeneration); calls != 1 {
		t.Fatalf("other generation was unexpectedly invalidated: calls=%d", calls)
	}
	service.InvalidateAll()
	service.Check(context.Background(), otherGeneration)
	if calls := reviewer.callCount(otherGeneration); calls != 2 {
		t.Fatalf("calls after global invalidation = %d, want 2", calls)
	}
}

func TestInvalidationFencesAnInflightResult(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	reviewer := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
		once.Do(func() { close(started) })
		<-release
		return AccessReviewResult{Allowed: true, Complete: true}, nil
	}}
	service := newTestService(t, reviewer, Options{})
	key := testKey()
	done := make(chan struct{})
	go func() {
		service.Check(context.Background(), key)
		close(done)
	}()
	<-started
	service.InvalidateGeneration(key.Generation)
	close(release)
	<-done
	service.Check(context.Background(), key)
	if got := reviewer.callCount(key); got != 2 {
		t.Fatalf("review calls = %d, want 2 because stale inflight result must not cache", got)
	}
}

func TestGuardFailClosedReadFallbackAndOperationAuthority(t *testing.T) {
	t.Run("unknown read falls through to real operation", func(t *testing.T) {
		reviewer := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
			return AccessReviewResult{}, errors.New("review unavailable")
		}}
		service := newTestService(t, reviewer, Options{})
		executions := 0
		result, err := service.Guard(context.Background(), testKey(), OperationRead, func(context.Context) error {
			executions++
			return nil
		})
		if err != nil || !result.Executed || executions != 1 || result.Capability.Decision != DecisionAllowed || result.Capability.ReasonCode != ReasonOperationAllowed {
			t.Fatalf("result=%+v executions=%d err=%v", result, executions, err)
		}
	})

	for _, kind := range []OperationKind{OperationMutation, OperationUpgrade} {
		t.Run(string(kind)+" fails closed on unknown", func(t *testing.T) {
			reviewer := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
				return AccessReviewResult{}, errors.New("review unavailable")
			}}
			service := newTestService(t, reviewer, Options{})
			executed := false
			result, err := service.Guard(context.Background(), testKey(), kind, func(context.Context) error {
				executed = true
				return nil
			})
			if ErrorCodeOf(err) != CodeAuthorizationUnavailable || executed || result.Executed {
				t.Fatalf("result=%+v executed=%v code=%q", result, executed, ErrorCodeOf(err))
			}
		})
	}

	t.Run("mutation bypasses stale allowed cache", func(t *testing.T) {
		var allowed atomic.Bool
		allowed.Store(true)
		reviewer := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
			if allowed.Load() {
				return AccessReviewResult{Allowed: true, Complete: true}, nil
			}
			return AccessReviewResult{Denied: true, Complete: true}, nil
		}}
		service := newTestService(t, reviewer, Options{})
		key := testKey()
		service.Check(context.Background(), key)
		allowed.Store(false)
		result, err := service.Guard(context.Background(), key, OperationMutation, func(context.Context) error {
			t.Fatal("denied mutation executed")
			return nil
		})
		if ErrorCodeOf(err) != CodeForbidden || result.Executed || reviewer.callCount(key) != 2 {
			t.Fatalf("result=%+v code=%q calls=%d", result, ErrorCodeOf(err), reviewer.callCount(key))
		}
	})

	t.Run("real 403 overrides SAR and invalidates", func(t *testing.T) {
		reviewer := &fakeAccessReviewer{fn: allowedReview}
		service := newTestService(t, reviewer, Options{})
		key := testKey()
		result, err := service.Guard(context.Background(), key, OperationMutation, func(context.Context) error {
			return apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "api", errors.New("RBAC changed"))
		})
		if ErrorCodeOf(err) != CodeForbidden || result.Capability.Decision != DecisionDenied || result.Capability.ReasonCode != ReasonOperationDenied {
			t.Fatalf("result=%+v code=%q", result, ErrorCodeOf(err))
		}
		service.Check(context.Background(), key)
		if got := reviewer.callCount(key); got != 2 {
			t.Fatalf("review calls after authoritative 403 = %d, want 2", got)
		}
	})
}

func TestTranslateOperationErrorUsesStableCodesAndMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code ErrorCode
	}{
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "api", errors.New("token=secret")), code: CodeForbidden},
		{name: "unauthorized", err: apierrors.NewUnauthorized("Bearer secret"), code: CodeAuthenticationUnavailable},
		{name: "status timeout", err: apierrors.NewTimeoutError("secret endpoint", 1), code: CodeUpstreamTimeout},
		{name: "deadline", err: context.DeadlineExceeded, code: CodeUpstreamTimeout},
		{name: "canceled", err: context.Canceled, code: CodeClientCanceled},
		{name: "offline", err: &net.DNSError{Err: "connection refused secret", Name: "cluster.internal"}, code: CodeClusterUnavailable},
		{name: "service unavailable", err: apierrors.NewServiceUnavailable("private upstream"), code: CodeClusterUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publicError := TranslateOperationError(test.err)
			if publicError.Code != test.code {
				t.Fatalf("code = %q, want %q", publicError.Code, test.code)
			}
			encoded, err := json.Marshal(publicError)
			if err != nil {
				t.Fatal(err)
			}
			if contains(string(encoded), "secret") || contains(publicError.Error(), "secret") {
				t.Fatalf("public error leaked raw cause: json=%s error=%q", encoded, publicError.Error())
			}
		})
	}

	status := &apierrors.StatusError{ErrStatus: metav1.Status{Status: metav1.StatusFailure, Code: 500, Reason: metav1.StatusReasonInternalError}}
	if got := TranslateOperationError(status).Code; got != CodeClusterUnavailable {
		t.Fatalf("internal StatusError code = %q", got)
	}
}

func TestValidateKeyRejectsMalformedAttributes(t *testing.T) {
	valid := testKey()
	if err := ValidateKey(valid); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	invalid := []Key{
		withKey(valid, func(key *Key) { key.Generation = "" }),
		withKey(valid, func(key *Key) { key.Generation = "gen with spaces" }),
		withKey(valid, func(key *Key) { key.Namespace = "Bad_Name" }),
		withKey(valid, func(key *Key) { key.APIGroup = "Bad Group" }),
		withKey(valid, func(key *Key) { key.Resource = "Pods/Exec" }),
		withKey(valid, func(key *Key) { key.Subresource = "bad/sub" }),
		withKey(valid, func(key *Key) { key.Verb = "" }),
		withKey(valid, func(key *Key) { key.ResourceName = "Bad_Name" }),
	}
	for _, key := range invalid {
		if ErrorCodeOf(ValidateKey(key)) != CodeValidationFailed {
			t.Fatalf("invalid key accepted: %+v", key)
		}
	}
	if _, err := New(nil, Options{}); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("New(nil) code = %q", ErrorCodeOf(err))
	}
}

func TestPublicErrorDoesNotSerializeCause(t *testing.T) {
	err := newPublicError(CodeClusterUnavailable, 503, true, fmt.Errorf("password=secret"))
	encoded, marshalError := json.Marshal(err)
	if marshalError != nil {
		t.Fatal(marshalError)
	}
	if contains(string(encoded), "secret") || contains(string(encoded), "cause") {
		t.Fatalf("serialized public error leaked cause: %s", encoded)
	}
}
