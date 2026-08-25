package dashboard

import (
	"context"
	"errors"
)

const (
	CodeValidationFailed          = "VALIDATION_FAILED"
	CodeForbidden                 = "FORBIDDEN"
	CodeFeatureUnavailable        = "FEATURE_UNAVAILABLE"
	CodeAuthorizationUnavailable  = "AUTHORIZATION_UNAVAILABLE"
	CodeAuthenticationUnavailable = "AUTHENTICATION_UNAVAILABLE"
	CodeUpstreamTimeout           = "UPSTREAM_TIMEOUT"
	CodeClusterUnavailable        = "CLUSTER_UNAVAILABLE"
	// CodeUpstreamUnavailable remains a Go-level compatibility alias. The
	// public value is the closed API code CLUSTER_UNAVAILABLE.
	CodeUpstreamUnavailable = CodeClusterUnavailable
	CodeClientCanceled      = "CLIENT_CANCELED"
)

type PublicError struct {
	code    string
	message string
	denied  bool
}

func (e *PublicError) Error() string         { return e.message }
func (e *PublicError) Code() string          { return e.code }
func (e *PublicError) PublicMessage() string { return e.message }
func (e *PublicError) Denied() bool          { return e.denied }

func validationError(message string) error {
	return &PublicError{code: CodeValidationFailed, message: message}
}

func NewDeniedError() error {
	return &PublicError{code: CodeForbidden, message: "Access to the requested resource was denied.", denied: true}
}

func NewFeatureUnavailableError() error {
	return &PublicError{code: CodeFeatureUnavailable, message: "The optional feature is unavailable."}
}

func NewAuthorizationUnavailableError() error {
	return &PublicError{code: CodeAuthorizationUnavailable, message: "Authorization could not be confirmed."}
}

func NewAuthenticationUnavailableError() error {
	return &PublicError{code: CodeAuthenticationUnavailable, message: "Kubernetes authentication is unavailable."}
}

type publicError interface {
	Code() string
	PublicMessage() string
	Denied() bool
}

func classifyPartialError(namespace string, err error) (PartialError, bool) {
	var safe publicError
	if errors.As(err, &safe) {
		return PartialError{Namespace: namespace, Code: safe.Code(), Message: safe.PublicMessage()}, safe.Denied()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return PartialError{Namespace: namespace, Code: CodeUpstreamTimeout, Message: "Collection timed out."}, false
	}
	if errors.Is(err, context.Canceled) {
		return PartialError{Namespace: namespace, Code: CodeClientCanceled, Message: "Collection was canceled."}, false
	}
	return PartialError{Namespace: namespace, Code: CodeClusterUnavailable, Message: "The Kubernetes API is temporarily unavailable."}, false
}

func emptyCoverage(requested int) *CoverageDTO {
	return &CoverageDTO{
		RequestedNamespaces: requested,
		DeniedNamespaces:    make([]string, 0),
		Failed:              make([]PartialError, 0),
	}
}

func blockWithValue[T any](value T, coverage *CoverageDTO) DashboardBlockDTO[T] {
	return DashboardBlockDTO[T]{
		Value:    value,
		Complete: true,
		Coverage: coverage,
		Errors:   make([]PartialError, 0),
	}
}

func addBlockError[T any](block *DashboardBlockDTO[T], namespace string, err error) {
	partial, denied := classifyPartialError(namespace, err)
	block.Complete = false
	block.Errors = append(block.Errors, partial)
	if block.Coverage == nil {
		return
	}
	if denied {
		block.Coverage.DeniedNamespaces = appendUnique(block.Coverage.DeniedNamespaces, namespace)
		return
	}
	block.Coverage.Failed = append(block.Coverage.Failed, partial)
}
