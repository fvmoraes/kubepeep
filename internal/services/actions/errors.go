package actions

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type ErrorCode string

const (
	CodeValidationFailed          ErrorCode = "VALIDATION_FAILED"
	CodeForbidden                 ErrorCode = "FORBIDDEN"
	CodeNotFound                  ErrorCode = "NOT_FOUND"
	CodeConflict                  ErrorCode = "CONFLICT"
	CodeIdempotencyConflict       ErrorCode = "IDEMPOTENCY_CONFLICT"
	CodeGenerationChanged         ErrorCode = "GENERATION_CHANGED"
	CodeSessionGone               ErrorCode = "SESSION_GONE"
	CodeLimitExceeded             ErrorCode = "LIMIT_EXCEEDED"
	CodeClusterUnavailable        ErrorCode = "CLUSTER_UNAVAILABLE"
	CodeAuthenticationUnavailable ErrorCode = "AUTHENTICATION_UNAVAILABLE"
	CodeAuthorizationUnavailable  ErrorCode = "AUTHORIZATION_UNAVAILABLE"
	CodeUpstreamTimeout           ErrorCode = "UPSTREAM_TIMEOUT"
	CodeClientCanceled            ErrorCode = "CLIENT_CANCELED"
	CodeExecIdleTimeout           ErrorCode = "EXEC_IDLE_TIMEOUT"
	CodeExecDurationLimit         ErrorCode = "EXEC_DURATION_LIMIT"
	CodeServerShutdown            ErrorCode = "SERVER_SHUTDOWN"
	CodeProtocolViolation         ErrorCode = "PROTOCOL_VIOLATION"
	CodeExecTargetGone            ErrorCode = "EXEC_TARGET_GONE"
	CodeExecUpstreamError         ErrorCode = "EXEC_UPSTREAM_ERROR"
	CodeInternal                  ErrorCode = "INTERNAL"
)

var messages = map[ErrorCode]string{
	CodeValidationFailed:          "The action request is invalid.",
	CodeForbidden:                 "Kubernetes denied this operation.",
	CodeNotFound:                  "The requested target was not found.",
	CodeConflict:                  "The target changed before the operation completed.",
	CodeIdempotencyConflict:       "The idempotency key was already used for a different request.",
	CodeGenerationChanged:         "The active selection changed.",
	CodeSessionGone:               "The session is no longer available.",
	CodeLimitExceeded:             "The action limit was exceeded.",
	CodeClusterUnavailable:        "The Kubernetes API is unavailable.",
	CodeAuthenticationUnavailable: "Kubernetes authentication is unavailable.",
	CodeAuthorizationUnavailable:  "Authorization could not be confirmed.",
	CodeUpstreamTimeout:           "The Kubernetes API request timed out.",
	CodeClientCanceled:            "The request was canceled.",
	CodeExecIdleTimeout:           "The exec session was idle for too long.",
	CodeExecDurationLimit:         "The exec session reached its duration limit.",
	CodeServerShutdown:            "The local server is shutting down.",
	CodeProtocolViolation:         "The exec protocol was violated.",
	CodeExecTargetGone:            "The exec target is no longer available.",
	CodeExecUpstreamError:         "The remote exec stream failed.",
	CodeInternal:                  "The action failed internally.",
}

type FieldViolation struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

// Error exposes only allowlisted public data. cause is intentionally private.
type Error struct {
	Code       ErrorCode        `json:"code"`
	Message    string           `json:"message"`
	HTTPStatus int              `json:"-"`
	Retryable  bool             `json:"-"`
	Details    []FieldViolation `json:"details,omitempty"`
	cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func publicError(code ErrorCode, status int, retryable bool, cause error) *Error {
	return &Error{Code: code, Message: messages[code], HTTPStatus: status, Retryable: retryable, cause: cause}
}

func validationError(violations ...FieldViolation) *Error {
	err := publicError(CodeValidationFailed, http.StatusBadRequest, false, nil)
	err.Details = append([]FieldViolation(nil), violations...)
	return err
}

func ErrorCodeOf(err error) ErrorCode {
	var actionError *Error
	if errors.As(err, &actionError) && actionError != nil {
		return actionError.Code
	}
	return ""
}

func translateError(err error) *Error {
	if err == nil {
		return nil
	}
	var actionError *Error
	if errors.As(err, &actionError) && actionError != nil {
		return actionError
	}
	var authorizationError *authorization.PublicError
	if errors.As(err, &authorizationError) && authorizationError != nil {
		code := ErrorCode(authorizationError.Code)
		return publicError(code, authorizationError.HTTPStatus, authorizationError.Retryable, err)
	}
	switch {
	case errors.Is(err, context.Canceled):
		return publicError(CodeClientCanceled, 0, false, err)
	case errors.Is(err, context.DeadlineExceeded), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return publicError(CodeUpstreamTimeout, http.StatusGatewayTimeout, true, err)
	case apierrors.IsForbidden(err):
		return publicError(CodeForbidden, http.StatusForbidden, false, err)
	case apierrors.IsUnauthorized(err):
		return publicError(CodeAuthenticationUnavailable, http.StatusServiceUnavailable, true, err)
	case apierrors.IsNotFound(err):
		return publicError(CodeNotFound, http.StatusNotFound, false, err)
	case apierrors.IsConflict(err), apierrors.IsAlreadyExists(err), apierrors.IsGone(err):
		return publicError(CodeConflict, http.StatusConflict, false, err)
	case apierrors.IsServiceUnavailable(err), apierrors.IsTooManyRequests(err), apierrors.IsInternalError(err):
		return publicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return publicError(CodeUpstreamTimeout, http.StatusGatewayTimeout, true, err)
		}
		return publicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
	}
	return publicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
}
