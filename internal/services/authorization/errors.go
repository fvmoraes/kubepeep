package authorization

import (
	"context"
	"crypto/x509"
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
	CodeForbidden:                 "Kubernetes denied this operation (HTTP 403). The current identity lacks permission for this resource and operation; check its Role or ClusterRole bindings.",
	CodeAuthorizationUnavailable:  "The Kubernetes permission review did not return a decision. This does not establish that resource access was denied.",
	CodeAuthenticationUnavailable: "Kubernetes rejected the current credentials (HTTP 401). Refresh the login or credential provider for this context.",
	CodeClusterUnavailable:        "The Kubernetes API could not complete the request. Check cluster availability and retry.",
	CodeUpstreamTimeout:           "The Kubernetes API did not respond before the request deadline. Check cluster connectivity; the request can be retried.",
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

// ReviewFailure describes the review mechanism's failure, never a denial of
// the target operation. Only an explicit denied decision or real 403 does that.
func ReviewFailure(capability Capability) *PublicError {
	result := authorizationUnavailableError(nil)
	switch capability.ReasonCode {
	case ReasonSARTimeout:
		result.Code = CodeUpstreamTimeout
		result.HTTPStatus = http.StatusGatewayTimeout
		result.Message = "The Kubernetes permission review timed out. Resource access has not been denied; the review can be retried."
	case ReasonSARAuthenticationUnavailable:
		result.Code = CodeAuthenticationUnavailable
		result.Message = publicMessages[CodeAuthenticationUnavailable]
	case ReasonSARForbidden:
		result.Message = "Kubernetes denied the permission-review request (SelfSubjectAccessReview). This does not mean the resource itself is forbidden; reads are checked by the actual Kubernetes request."
	case ReasonSARIncomplete:
		result.Message = "Kubernetes returned an incomplete permission review without an allow or deny decision. The review will be checked again."
	case ReasonRequestCanceled:
		result.Message = "The Kubernetes permission review was canceled before it returned a decision. The review can be retried."
	case ReasonSARUnavailable:
		result.Message = "The Kubernetes permission-review endpoint could not be reached or returned an error. Resource access has not been denied; the review can be retried."
	}
	return result
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
		result := newPublicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
		result.Message = "The Kubernetes API is busy or temporarily unavailable (HTTP 429/5xx). The request can be retried."
		return result
	}
	var certificateError x509.UnknownAuthorityError
	var invalidCertificate x509.CertificateInvalidError
	var hostnameError x509.HostnameError
	if errors.As(err, &certificateError) || errors.As(err, &invalidCertificate) || errors.As(err, &hostnameError) {
		result := newPublicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
		result.Message = "The Kubernetes API TLS certificate could not be verified. Check the certificate authority and server name in this context's kubeconfig."
		return result
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) && !dnsError.Timeout() {
		result := newPublicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
		result.Message = "The Kubernetes API host could not be resolved by DNS. Check the network or VPN and the server address in this context."
		return result
	}

	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return newPublicError(CodeUpstreamTimeout, http.StatusGatewayTimeout, true, err)
		}
		result := newPublicError(CodeClusterUnavailable, http.StatusServiceUnavailable, true, err)
		result.Message = "A network connection to the Kubernetes API could not be established or was interrupted. Check the network or VPN; the request can be retried."
		return result
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
