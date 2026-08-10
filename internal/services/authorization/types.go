// Package authorization implements the Kubernetes RBAC decision boundary used
// by UI capability discovery and by backend operation guards.
package authorization

import (
	"context"
	"time"
)

const (
	// DefaultTTL is deliberately short because Kubernetes RBAC may change while
	// the local process is running.
	DefaultTTL = 45 * time.Second
	MinTTL     = 30 * time.Second
	MaxTTL     = 60 * time.Second
)

// Decision is a tri-state result. Unknown is deliberately distinct from a
// denial: an unavailable review is not evidence that Kubernetes denied access.
type Decision string

const (
	DecisionAllowed Decision = "allowed"
	DecisionDenied  Decision = "denied"
	DecisionUnknown Decision = "unknown"
)

// ReasonCode is a public, allowlisted explanation. Kubernetes-provided reason
// and evaluationError strings are never copied into this field.
type ReasonCode string

const (
	ReasonSARAllowed                   ReasonCode = "SAR_ALLOWED"
	ReasonSARDenied                    ReasonCode = "SAR_DENIED"
	ReasonSARIncomplete                ReasonCode = "SAR_INCOMPLETE"
	ReasonSARUnavailable               ReasonCode = "SAR_UNAVAILABLE"
	ReasonSARTimeout                   ReasonCode = "SAR_TIMEOUT"
	ReasonSARAuthenticationUnavailable ReasonCode = "SAR_AUTHENTICATION_UNAVAILABLE"
	ReasonRequestCanceled              ReasonCode = "REQUEST_CANCELED"
	ReasonOperationAllowed             ReasonCode = "OPERATION_ALLOWED"
	ReasonOperationDenied              ReasonCode = "OPERATION_DENIED"
	ReasonOperationUnavailable         ReasonCode = "OPERATION_UNAVAILABLE"
	ReasonSSRRSummaryAvailable         ReasonCode = "SSRR_SUMMARY_AVAILABLE"
	ReasonSSRRIncomplete               ReasonCode = "SSRR_INCOMPLETE"
	ReasonSSRRUnavailable              ReasonCode = "SSRR_UNAVAILABLE"
)

// Key is the complete identity of one authorization decision. Generation is
// part of the key, so a decision can never cross a selection boundary.
type Key struct {
	Generation   string
	Namespace    string
	APIGroup     string
	Resource     string
	Subresource  string
	Verb         string
	ResourceName string
}

// Capability is the public DTO returned by the service. CapabilityID is set
// only by allowlist expansion; direct backend checks may leave it empty.
type Capability struct {
	CapabilityID string     `json:"capabilityId"`
	Namespace    string     `json:"namespace"`
	APIGroup     string     `json:"apiGroup"`
	Resource     string     `json:"resource"`
	Subresource  string     `json:"subresource"`
	Verb         string     `json:"verb"`
	ResourceName string     `json:"resourceName"`
	Decision     Decision   `json:"decision"`
	ReasonCode   ReasonCode `json:"reasonCode"`
	ExpiresAt    time.Time  `json:"expiresAt"`
}

// AccessReviewResult is the credential-free result produced by the narrow
// SSAR port. Complete is false for evaluation errors and no-opinion results.
type AccessReviewResult struct {
	Allowed  bool
	Denied   bool
	Complete bool
}

// AccessReviewer is the only capability the service needs from Kubernetes.
// Implementations must use SelfSubjectAccessReview and must not impersonate.
type AccessReviewer interface {
	ReviewAccess(context.Context, Key) (AccessReviewResult, error)
}

// ResourceRuleHint is an optional, non-authoritative SSRR hint. It is never
// consulted by Check, Refresh, Revalidate, Guard, or Matrix.
type ResourceRuleHint struct {
	Verbs         []string `json:"verbs"`
	APIGroups     []string `json:"apiGroups"`
	Resources     []string `json:"resources"`
	ResourceNames []string `json:"resourceNames"`
}

// RulesReviewResult is returned by the optional SSRR port.
type RulesReviewResult struct {
	Complete bool
	Rules    []ResourceRuleHint
}

// RulesReviewer is optional. SSRR may summarize UI hints but cannot grant an
// operation and does not populate the SSAR cache.
type RulesReviewer interface {
	ReviewRules(context.Context, string) (RulesReviewResult, error)
}

// RulesSummary is a bounded and sanitized SSRR response.
type RulesSummary struct {
	Namespace  string             `json:"namespace"`
	Complete   bool               `json:"complete"`
	ReasonCode ReasonCode         `json:"reasonCode"`
	Rules      []ResourceRuleHint `json:"rules"`
}

// OperationKind controls fail-closed behavior in Guard.
type OperationKind string

const (
	OperationRead     OperationKind = "read"
	OperationMutation OperationKind = "mutation"
	OperationUpgrade  OperationKind = "upgrade"
)

// Operation is the actual Kubernetes call. Its result remains authoritative
// even after an allowed SSAR decision.
type Operation func(context.Context) error

// GuardResult reports both the authorization result and whether the real
// operation was invoked.
type GuardResult struct {
	Capability Capability
	Executed   bool
}

// AuthorizationService is the narrow port consumed by handlers and other
// application services. There is intentionally no credential or impersonation
// input anywhere in this contract.
type AuthorizationService interface {
	Check(context.Context, Key) Capability
	Refresh(context.Context, Key) Capability
	Revalidate(context.Context, Key, OperationKind) (Capability, error)
	Guard(context.Context, Key, OperationKind, Operation) (GuardResult, error)
	InvalidateGeneration(string)
	InvalidateAll()
}
