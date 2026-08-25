package resources

import (
	"errors"
	"fmt"
)

// ErrorCode is a stable, public classification. Detail from Kubernetes is
// intentionally not carried by DomainError.
type ErrorCode string

const (
	CodeValidationFailed          ErrorCode = "VALIDATION_FAILED"
	CodeForbidden                 ErrorCode = "FORBIDDEN"
	CodeAuthorizationUnavailable  ErrorCode = "AUTHORIZATION_UNAVAILABLE"
	CodeAuthenticationUnavailable ErrorCode = "AUTHENTICATION_UNAVAILABLE"
	CodeFeatureUnavailable        ErrorCode = "FEATURE_UNAVAILABLE"
	CodeClusterUnavailable        ErrorCode = "CLUSTER_UNAVAILABLE"
	CodeUpstreamTimeout           ErrorCode = "UPSTREAM_TIMEOUT"
	CodeNotFound                  ErrorCode = "NOT_FOUND"
	CodeCursorExpired             ErrorCode = "CURSOR_EXPIRED"
	CodeGenerationChanged         ErrorCode = "GENERATION_CHANGED"
	CodeLimitExceeded             ErrorCode = "LIMIT_EXCEEDED"
	CodePreferenceSensitive       ErrorCode = "PREFERENCE_SENSITIVE_VALUE"
)

var (
	ErrResourceExpired = errors.New("resources: upstream resource version expired")
	ErrSecretYAML      = errors.New("resources: Secret YAML is prohibited")
)

// DomainError contains only an allowlisted code and message suitable for a
// response. Cause remains available to errors.Is/As but must not be rendered.
type DomainError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *DomainError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func domainError(code ErrorCode, message string, cause error) error {
	return &DomainError{Code: code, Message: message, Cause: cause}
}

// ErrorCodeOf extracts a stable code without revealing an upstream error.
func ErrorCodeOf(err error) ErrorCode {
	var domain *DomainError
	if errors.As(err, &domain) {
		return domain.Code
	}
	return CodeClusterUnavailable
}

// PublicMessage returns only the domain-owned public text.
func PublicMessage(err error) string {
	var domain *DomainError
	if errors.As(err, &domain) {
		return domain.Message
	}
	return "The Kubernetes API could not complete the request."
}
