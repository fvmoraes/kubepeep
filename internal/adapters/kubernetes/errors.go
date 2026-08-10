package kubernetes

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrorCode is a stable, credential-free classification suitable for logs and
// HTTP error translation.
type ErrorCode string

const (
	CodeKubeconfigNotFound        ErrorCode = "KUBECONFIG_NOT_FOUND"
	CodeKubeconfigInvalid         ErrorCode = "KUBECONFIG_INVALID"
	CodeContextNotFound           ErrorCode = "CONTEXT_NOT_FOUND"
	CodeContextRequired           ErrorCode = "CONTEXT_REQUIRED"
	CodeAuthenticationUnavailable ErrorCode = "AUTHENTICATION_UNAVAILABLE"
	CodeClusterUnavailable        ErrorCode = "CLUSTER_UNAVAILABLE"
	CodeRequestCanceled           ErrorCode = "REQUEST_CANCELED"
	CodeRequestTimeout            ErrorCode = "REQUEST_TIMEOUT"
	CodeGenerationChanged         ErrorCode = "GENERATION_CHANGED"
	CodeClientUnavailable         ErrorCode = "KUBERNETES_CLIENT_UNAVAILABLE"
)

// SafeError deliberately carries no wrapped cause. This prevents a parser,
// kubeconfig path, token, certificate, or exec-plugin output from escaping via
// Error, Unwrap, JSON, or structured logs.
type SafeError struct {
	Code      ErrorCode
	Message   string
	Retryable bool
}

func (e *SafeError) Error() string {
	if e == nil {
		return ""
	}
	return string(e.Code) + ": " + e.Message
}

func safeError(code ErrorCode, message string, retryable bool) error {
	return &SafeError{Code: code, Message: message, Retryable: retryable}
}

// ErrorDetails extracts only the stable public classification.
func ErrorDetails(err error) (ErrorCode, string, bool) {
	var safe *SafeError
	if errors.As(err, &safe) {
		return safe.Code, safe.Message, safe.Retryable
	}
	return CodeClientUnavailable, "The Kubernetes client is unavailable.", true
}

// SanitizeError classifies an external Kubernetes or plugin error without
// retaining its potentially sensitive text.
func SanitizeError(err error) error {
	if err == nil {
		return nil
	}
	var alreadySafe *SafeError
	if errors.As(err, &alreadySafe) {
		return alreadySafe
	}
	if errors.Is(err, context.Canceled) {
		return safeError(CodeRequestCanceled, "The Kubernetes request was canceled.", true)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return safeError(CodeRequestTimeout, "The Kubernetes request timed out.", true)
	}
	if apierrors.IsUnauthorized(err) {
		return safeError(CodeAuthenticationUnavailable, "Kubernetes authentication is unavailable.", true)
	}
	var exitError *exec.ExitError
	var executableError *exec.Error
	if errors.As(err, &exitError) || errors.As(err, &executableError) {
		return safeError(CodeAuthenticationUnavailable, "The kubeconfig authentication plugin is unavailable.", true)
	}
	// client-go intentionally converts exec.Cmd failures into an unwrapped
	// credential-safe diagnostic. Match only its fixed categories and never
	// propagate the source text, which can also contain an install hint.
	if isExecCredentialError(err) {
		return safeError(CodeAuthenticationUnavailable, "The kubeconfig authentication plugin is unavailable.", true)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return safeError(CodeClusterUnavailable, "The Kubernetes cluster is temporarily unavailable.", true)
	}
	return safeError(CodeClientUnavailable, "The Kubernetes client is unavailable.", true)
}

// IsRebuildableAuthenticationError identifies errors which must invalidate a
// cached transport so client-go can reload credentials or rerun an exec plugin.
func IsRebuildableAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsUnauthorized(err) {
		return true
	}
	var exitError *exec.ExitError
	var executableError *exec.Error
	return errors.As(err, &exitError) || errors.As(err, &executableError) || isExecCredentialError(err)
}

func isExecCredentialError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "exec: executable") ||
		strings.Contains(message, "exec plugin") ||
		strings.Contains(message, "decoding stdout") ||
		strings.Contains(message, "ExecCredential")
}
