package api

import (
	"errors"
	"net/http"

	gingererrors "github.com/fvmoraes/ginger/pkg/errors"
	"github.com/fvmoraes/ginger/pkg/router"
)

const (
	CodeInvalidJSON              = "INVALID_JSON"
	CodeUnknownField             = "UNKNOWN_FIELD"
	CodeValidationFailed         = "VALIDATION_FAILED"
	CodeNotFound                 = "NOT_FOUND"
	CodeMethodNotAllowed         = "METHOD_NOT_ALLOWED"
	CodeCSRFRejected             = "CSRF_REJECTED"
	CodeBodyTooLarge             = "BODY_TOO_LARGE"
	CodeUnsupportedMediaType     = "UNSUPPORTED_MEDIA_TYPE"
	CodeCursorInvalid            = "CURSOR_INVALID"
	CodeCursorMismatch           = "CURSOR_MISMATCH"
	CodeCursorExpired            = "CURSOR_EXPIRED"
	CodeConflict                 = "CONFLICT"
	CodeGenerationChanged        = "GENERATION_CHANGED"
	CodeSelectionMismatch        = "SELECTION_MISMATCH"
	CodeForbidden                = "FORBIDDEN"
	CodeClusterUnavailable       = "CLUSTER_UNAVAILABLE"
	CodeAuthorizationUnavailable = "AUTHORIZATION_UNAVAILABLE"
	CodeKubeconfigInvalid        = "KUBECONFIG_INVALID"
	CodeKubeconfigNotFound       = "KUBECONFIG_NOT_FOUND"
	CodeContextNotFound          = "CONTEXT_NOT_FOUND"
	CodeInternal                 = "INTERNAL"
)

// HTTPError complements Ginger's AppError with the exact HTTP code required by
// the public contract. The wrapped cause is never serialized.
type HTTPError struct {
	AppError *gingererrors.AppError
	Status   int
	Details  any
}

func NewHTTPError(status int, code, message string, details any, cause error) *HTTPError {
	return &HTTPError{
		AppError: gingererrors.New(gingererrors.Code(code), message, cause),
		Status:   status,
		Details:  details,
	}
}

func (e *HTTPError) Error() string { return e.AppError.Error() }

func (e *HTTPError) Unwrap() error { return e.AppError }

type errorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	Details   any    `json:"details,omitempty"`
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	httpError := normalizeError(err)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	router.JSON(w, httpError.Status, errorEnvelope{
		Code:      string(httpError.AppError.Code),
		Message:   httpError.AppError.Message,
		RequestID: RequestIDFromContext(r.Context()),
		Details:   httpError.Details,
	})
}

func normalizeError(err error) *HTTPError {
	var httpError *HTTPError
	if errors.As(err, &httpError) {
		return httpError
	}
	if appError, ok := gingererrors.As(err); ok {
		return &HTTPError{AppError: appError, Status: appError.HTTPStatus()}
	}
	return NewHTTPError(
		http.StatusInternalServerError,
		CodeInternal,
		"Internal server error.",
		nil,
		err,
	)
}
