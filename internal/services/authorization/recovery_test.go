package authorization

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestInconclusiveReviewExpiresBeforeNextRefresh(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	recovered := false
	reviewer := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
		if recovered {
			return AccessReviewResult{Allowed: true, Complete: true}, nil
		}
		return AccessReviewResult{}, context.DeadlineExceeded
	}}
	service := newTestService(t, reviewer, Options{Now: clock.Now})
	key := testKey()
	if capability := service.Check(t.Context(), key); capability.Decision != DecisionUnknown || capability.ExpiresAt.Sub(clock.Now()) != UnknownTTL {
		t.Fatalf("unknown TTL: %+v", capability)
	}
	recovered = true
	clock.Advance(10 * time.Second)
	if capability := service.Check(t.Context(), key); capability.Decision != DecisionAllowed || reviewer.callCount(key) != 2 {
		t.Fatalf("recovery pinned in cache: %+v", capability)
	}
}

func TestForbiddenReviewDoesNotImplyForbiddenResource(t *testing.T) {
	reviewer := &fakeAccessReviewer{fn: func(context.Context, Key) (AccessReviewResult, error) {
		return AccessReviewResult{}, apierrors.NewForbidden(schema.GroupResource{Resource: "selfsubjectaccessreviews"}, "", errors.New("private detail"))
	}}
	service := newTestService(t, reviewer, Options{})
	key := testKey()
	key.Verb = "get"
	capability := service.Check(t.Context(), key)
	if capability.Decision != DecisionUnknown || capability.ReasonCode != ReasonSARForbidden {
		t.Fatalf("review denial became resource denial: %+v", capability)
	}
	result, err := service.Guard(t.Context(), key, OperationRead, func(context.Context) error { return nil })
	if err != nil || !result.Executed || result.Capability.Decision != DecisionAllowed {
		t.Fatalf("actual read blocked: %+v %v", result, err)
	}
	result, err = service.Guard(t.Context(), key, OperationMutation, func(context.Context) error { t.Fatal("unknown review allowed mutation"); return nil })
	if err == nil || result.Executed || !strings.Contains(err.Error(), "SelfSubjectAccessReview") {
		t.Fatalf("mutation must remain closed with review explanation: %+v %v", result, err)
	}
}
