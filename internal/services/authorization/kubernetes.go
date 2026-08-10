package authorization

import (
	"context"
	"errors"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// KubernetesReviewer adapts the official authorization/v1 client to the
// credential-free service ports. It uses only self-subject review resources;
// callers cannot provide a user, group, extra field, token, or impersonation.
type KubernetesReviewer struct {
	client authorizationclient.AuthorizationV1Interface
}

// NewKubernetesReviewer accepts only the narrow authorization/v1 client, not a
// full clientset or rest.Config.
func NewKubernetesReviewer(client authorizationclient.AuthorizationV1Interface) (*KubernetesReviewer, error) {
	if client == nil {
		return nil, validationError()
	}
	return &KubernetesReviewer{client: client}, nil
}

// ReviewAccess creates an official SelfSubjectAccessReview. The Kubernetes
// reason and evaluationError strings are deliberately discarded.
func (reviewer *KubernetesReviewer) ReviewAccess(ctx context.Context, key Key) (AccessReviewResult, error) {
	if reviewer == nil || reviewer.client == nil {
		return AccessReviewResult{}, errors.New("authorization reviewer is unavailable")
	}
	if err := ValidateKey(key); err != nil {
		return AccessReviewResult{}, err
	}
	review := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace:   key.Namespace,
				Verb:        key.Verb,
				Group:       key.APIGroup,
				Resource:    key.Resource,
				Subresource: key.Subresource,
				Name:        key.ResourceName,
			},
		},
	}
	response, err := reviewer.client.SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return AccessReviewResult{}, err
	}
	if response == nil {
		return AccessReviewResult{}, errors.New("authorization review returned no response")
	}
	complete := response.Status.EvaluationError == "" && response.Status.Allowed != response.Status.Denied
	return AccessReviewResult{
		Allowed:  response.Status.Allowed,
		Denied:   response.Status.Denied,
		Complete: complete,
	}, nil
}

// ReviewRules retrieves optional SelfSubjectRulesReview hints. These hints are
// not authoritative and are never fed into ReviewAccess.
func (reviewer *KubernetesReviewer) ReviewRules(ctx context.Context, namespace string) (RulesReviewResult, error) {
	if reviewer == nil || reviewer.client == nil {
		return RulesReviewResult{}, errors.New("authorization reviewer is unavailable")
	}
	if namespace != "" {
		if err := ValidateKey(Key{Generation: "summary", Namespace: namespace, Resource: "pods", Verb: "list"}); err != nil {
			return RulesReviewResult{}, err
		}
	}
	review := &authorizationv1.SelfSubjectRulesReview{
		Spec: authorizationv1.SelfSubjectRulesReviewSpec{Namespace: namespace},
	}
	response, err := reviewer.client.SelfSubjectRulesReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return RulesReviewResult{}, err
	}
	if response == nil {
		return RulesReviewResult{}, errors.New("authorization rules review returned no response")
	}
	rules := make([]ResourceRuleHint, 0, len(response.Status.ResourceRules))
	for _, rule := range response.Status.ResourceRules {
		rules = append(rules, ResourceRuleHint{
			Verbs:         append([]string(nil), rule.Verbs...),
			APIGroups:     append([]string(nil), rule.APIGroups...),
			Resources:     append([]string(nil), rule.Resources...),
			ResourceNames: append([]string(nil), rule.ResourceNames...),
		})
	}
	return RulesReviewResult{
		Complete: !response.Status.Incomplete && response.Status.EvaluationError == "",
		Rules:    rules,
	}, nil
}
