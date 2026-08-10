package authorization

import (
	"context"
	"errors"
	"net"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrorCode is an allowlisted public failure code.
type ErrorCode string

const (
	CodeValidationFailed          ErrorCode = "VALIDATION_FAILED"
	CodeForbidden                 ErrorCode = "FORBIDDEN"
	CodeAuthorizationUnavailable  ErrorCode = "AUTHORIZATION_UNAVAILABLE"
	CodeAuthenticationUnavailable ErrorCode = "AUTHENTICATION_UNAVAILABLE"
	CodeClusterUnavailable        ErrorCode = "CLUSTER_UNAVAILABLE"
	CodeUpstreamTimeout           ErrorCode = "UPSTREAM_TIMEOUT"
	CodeClientCanceled            ErrorCode = "CLIENT_CANCELED"
)

var publicMessages = map[ErrorCode]string{
	CodeValidationFailed:          "The authorization request is invalid.",
	CodeForbidden:                 "Kubernetes denied this operation.",
	CodeAuthorizationUnavailable:  "Authorization could not be confirmed.",
	CodeAuthenticationUnavailable: "Kubernetes authentication is unavailable.",
	CodeClusterUnavailable:        "The Kubernetes API is unavailable.",
	CodeUpstreamTimeout:           "The Kubernetes API request timed out.",
	CodeClientCanceled:            "The request was canceled.",
}

// PublicError carries only stable response data. The original cause remains
// unexported and is excluded from JSON serialization.
type PublicError struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	HTTPStatus int       `json:"-"`
	Retryable  bool      `json:"-"`
	cause      error
}

func (e *PublicError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *PublicError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newPublicError(code ErrorCode, status int, retryable bool, cause error) *PublicError {
	return &PublicError{
		Code:       code,
		Message:    publicMessages[code],
		HTTPStatus: status,
		Retryable:  retryable,
		cause:      cause,
	}
}

func validationError() *PublicError {
	return newPublicError(CodeValidationFailed, http.StatusBadRequest, false, nil)
}

func forbiddenError(cause error) *PublicError {
	return newPublicError(CodeForbidden, http.StatusForbidden, false, cause)
}

func authorizationUnavailableError(cause error) *PublicError {
	return newPublicError(CodeAuthorizationUnavailable, http.StatusServiceUnavailable, true, cause)
}

// TranslateOperationError maps Kubernetes StatusError, timeout, cancellation,
// authentication and offline failures to stable public codes. It never embeds
// an upstream message in the public error.
func TranslateOperationError(err error) *PublicError {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled):
		return newPublicError(CodeClientCanceled, 0, false, err)
	case errors.Is(err, context.DeadlineExceeded), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return newPublicError(CodeUpstreamTimeout, http.StatusGatewayTimeout, true, err)
	case apierrors.IsForbidden(err):
		return forbiddenError(err)
	case apierrors.IsUnauthorized(err):
		return newPublicError(CodeAuthenticationUnavailable, http.StatusServiceUnavailable, true, err)
	case apierrors.IsServiceUnavailable(err), apierrors.IsTooManyRequests(err), apierrors.IsInternalError(err):
		return newPublicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
	}

	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return newPublicError(CodeUpstreamTimeout, http.StatusGatewayTimeout, true, err)
		}
		return newPublicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
	}
	return newPublicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
}

// ErrorCodeOf extracts a stable code without exposing the wrapped cause.
func ErrorCodeOf(err error) ErrorCode {
	var publicError *PublicError
	if errors.As(err, &publicError) && publicError != nil {
		return publicError.Code
	}
	return ""
}
