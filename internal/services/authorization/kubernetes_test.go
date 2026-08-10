package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestKubernetesReviewerBuildsExactSelfSubjectAccessReview(t *testing.T) {
	client := fake.NewSimpleClientset()
	var captured *authorizationv1.SelfSubjectAccessReview
	client.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction, ok := action.(k8stesting.CreateAction)
		if !ok {
			t.Fatalf("action type = %T, want CreateAction", action)
		}
		captured = createAction.GetObject().(*authorizationv1.SelfSubjectAccessReview).DeepCopy()
		return true, &authorizationv1.SelfSubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{
				Allowed: true,
				Reason:  "raw server reason must be discarded",
			},
		}, nil
	})
	reviewer, err := NewKubernetesReviewer(client.AuthorizationV1())
	if err != nil {
		t.Fatal(err)
	}
	key := testKey()
	result, err := reviewer.ReviewAccess(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed || result.Denied || !result.Complete {
		t.Fatalf("result = %+v", result)
	}
	if captured == nil || captured.Spec.ResourceAttributes == nil {
		t.Fatal("SelfSubjectAccessReview was not captured")
	}
	want := authorizationv1.ResourceAttributes{
		Namespace:   key.Namespace,
		Verb:        key.Verb,
		Group:       key.APIGroup,
		Resource:    key.Resource,
		Subresource: key.Subresource,
		Name:        key.ResourceName,
	}
	if got := *captured.Spec.ResourceAttributes; got != want {
		t.Fatalf("resource attributes = %+v, want %+v", got, want)
	}
	if captured.Spec.NonResourceAttributes != nil {
		t.Fatalf("unexpected non-resource attributes: %+v", captured.Spec.NonResourceAttributes)
	}
	encoded, err := json.Marshal(captured)
	if err != nil {
		t.Fatal(err)
	}
	if contains(string(encoded), key.Generation) || contains(string(encoded), "token") || contains(string(encoded), "credential") {
		t.Fatalf("SSAR contains local generation or credential-like data: %s", encoded)
	}
}

func TestKubernetesReviewerTreatsEvaluationErrorAndNoOpinionAsIncomplete(t *testing.T) {
	statuses := []authorizationv1.SubjectAccessReviewStatus{
		{EvaluationError: "plugin printed Bearer private-token"},
		{},
		{Allowed: true, Denied: true},
	}
	for index, status := range statuses {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			client := fake.NewSimpleClientset()
			client.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, &authorizationv1.SelfSubjectAccessReview{Status: status}, nil
			})
			reviewer, err := NewKubernetesReviewer(client.AuthorizationV1())
			if err != nil {
				t.Fatal(err)
			}
			result, err := reviewer.ReviewAccess(context.Background(), testKey())
			if err != nil {
				t.Fatal(err)
			}
			if result.Complete {
				t.Fatalf("result = %+v, want incomplete", result)
			}
		})
	}
}

func TestSelfSubjectRulesReviewIsSummaryOnly(t *testing.T) {
	client := fake.NewSimpleClientset()
	var rulesCalls int
	client.PrependReactor("create", "selfsubjectrulesreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		rulesCalls++
		createAction := action.(k8stesting.CreateAction)
		review := createAction.GetObject().(*authorizationv1.SelfSubjectRulesReview)
		if review.Spec.Namespace != "payments" {
			t.Fatalf("summary namespace = %q", review.Spec.Namespace)
		}
		return true, &authorizationv1.SelfSubjectRulesReview{
			Status: authorizationv1.SubjectRulesReviewStatus{
				ResourceRules: []authorizationv1.ResourceRule{{
					Verbs:     []string{"*"},
					APIGroups: []string{"*"},
					Resources: []string{"*"},
				}},
			},
		}, nil
	})
	client.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SelfSubjectAccessReview{
			Status: authorizationv1.SubjectAccessReviewStatus{Denied: true},
		}, nil
	})
	reviewer, err := NewKubernetesReviewer(client.AuthorizationV1())
	if err != nil {
		t.Fatal(err)
	}
	service := newTestService(t, reviewer, Options{})
	summary := service.Summary(context.Background(), "payments")
	if !summary.Complete || summary.ReasonCode != ReasonSSRRSummaryAvailable || len(summary.Rules) != 1 || rulesCalls != 1 {
		t.Fatalf("summary = %+v calls=%d", summary, rulesCalls)
	}
	capability := service.Check(context.Background(), testKey())
	if capability.Decision != DecisionDenied || capability.ReasonCode != ReasonSARDenied {
		t.Fatalf("SSRR incorrectly granted access: %+v", capability)
	}
}

type combinedReviewer struct {
	*fakeAccessReviewer
	rules RulesReviewResult
	err   error
}

func (reviewer *combinedReviewer) ReviewRules(context.Context, string) (RulesReviewResult, error) {
	return reviewer.rules, reviewer.err
}

func TestSummaryIsBoundedSanitizedAndNeverPopulatesAccessCache(t *testing.T) {
	rules := make([]ResourceRuleHint, maxRules+20)
	for index := range rules {
		rules[index] = ResourceRuleHint{
			Verbs:     []string{"get", " bad", "Bearer\nsecret"},
			APIGroups: []string{"apps"},
			Resources: []string{"deployments"},
		}
	}
	access := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
		return AccessReviewResult{Denied: true, Complete: true}, nil
	}}
	reviewer := &combinedReviewer{fakeAccessReviewer: access, rules: RulesReviewResult{Complete: true, Rules: rules}}
	service := newTestService(t, reviewer, Options{})
	summary := service.Summary(context.Background(), "payments")
	if len(summary.Rules) != maxRules {
		t.Fatalf("summary rules = %d, want bounded %d", len(summary.Rules), maxRules)
	}
	if got := summary.Rules[0].Verbs; len(got) != 1 || got[0] != "get" {
		t.Fatalf("sanitized verbs = %#v", got)
	}
	if access.totalCalls() != 0 {
		t.Fatalf("SSRR summary unexpectedly performed access review")
	}
	if got := service.Check(context.Background(), testKey()).Decision; got != DecisionDenied {
		t.Fatalf("access decision = %q, want denied", got)
	}
}

func TestRulesReviewFailureIsSanitized(t *testing.T) {
	access := &fakeAccessReviewer{fn: allowedReview}
	reviewer := &combinedReviewer{fakeAccessReviewer: access, err: errors.New("password=secret")}
	service := newTestService(t, reviewer, Options{})
	summary := service.Summary(context.Background(), "payments")
	if summary.Complete || summary.ReasonCode != ReasonSSRRUnavailable || len(summary.Rules) != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if contains(string(encoded), "secret") {
		t.Fatalf("summary leaked raw failure: %s", encoded)
	}
}

func TestKubernetesReviewerPropagatesOnlyErrorsToServiceSanitizer(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("exec plugin token=private")
	})
	reviewer, err := NewKubernetesReviewer(client.AuthorizationV1())
	if err != nil {
		t.Fatal(err)
	}
	service := newTestService(t, reviewer, Options{})
	capability := service.Check(context.Background(), testKey())
	if capability.Decision != DecisionUnknown || capability.ReasonCode != ReasonSARUnavailable {
		t.Fatalf("capability = %+v", capability)
	}
}

func TestConcurrentSummaryDoesNotRaceWithAccessChecks(t *testing.T) {
	access := &fakeAccessReviewer{fn: allowedReview}
	reviewer := &combinedReviewer{
		fakeAccessReviewer: access,
		rules:              RulesReviewResult{Complete: true, Rules: []ResourceRuleHint{{Verbs: []string{"get"}, Resources: []string{"pods"}}}},
	}
	service := newTestService(t, reviewer, Options{})
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(2)
		go func() {
			defer wait.Done()
			service.Summary(context.Background(), "payments")
		}()
		go func() {
			defer wait.Done()
			service.Check(context.Background(), testKey())
		}()
	}
	wait.Wait()
}

func TestNewKubernetesReviewerRejectsNilClient(t *testing.T) {
	if _, err := NewKubernetesReviewer(nil); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("code = %q", ErrorCodeOf(err))
	}
}

func TestRulesReviewIncompleteFlagIsPreserved(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectrulesreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authorizationv1.SelfSubjectRulesReview{
			ObjectMeta: metav1.ObjectMeta{Name: "ignored"},
			Status: authorizationv1.SubjectRulesReviewStatus{
				Incomplete:      true,
				EvaluationError: "private upstream detail",
			},
		}, nil
	})
	reviewer, err := NewKubernetesReviewer(client.AuthorizationV1())
	if err != nil {
		t.Fatal(err)
	}
	result, err := reviewer.ReviewRules(context.Background(), "payments")
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete {
		t.Fatalf("rules result = %+v, want incomplete", result)
	}
}
