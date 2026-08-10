package namespaces

import (
	"errors"
	"fmt"
)

var (
	ErrBodyTooLarge             = errors.New("namespace scopes: request body is too large")
	ErrInvalidJSON              = errors.New("namespace scopes: invalid JSON")
	ErrUnknownField             = errors.New("namespace scopes: unknown JSON field")
	ErrInvalidNamespaceInput    = errors.New("namespace scopes: invalid namespace input")
	ErrNamespaceLimit           = errors.New("namespace scopes: namespace entry limit exceeded")
	ErrValidation               = errors.New("namespace scopes: validation failed")
	ErrNotFound                 = errors.New("namespace scopes: scope not found")
	ErrConflict                 = errors.New("namespace scopes: conflicting scope state")
	ErrInvariant                = errors.New("namespace scopes: invariant violation")
	ErrGenerationChanged        = errors.New("namespace scopes: generation changed")
	ErrSelectionMismatch        = errors.New("namespace scopes: selection does not match scope origin")
	ErrNamespaceListForbidden   = errors.New("namespace scopes: listing namespaces is forbidden")
	ErrNamespaceListUnavailable = errors.New("namespace scopes: namespace list is unavailable")
	ErrNamespacePageExpired     = errors.New("namespace scopes: namespace page expired")
)

// FieldError identifies a public validation field without retaining the
// rejected value, which may have originated in an untrusted request.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string {
	if e == nil || e.Field == "" {
		return ErrValidation.Error()
	}
	return fmt.Sprintf("%s: %s", ErrValidation, e.Field)
}

func (e *FieldError) Unwrap() error { return ErrValidation }

// ReportError carries the safe, structured validation report to an API
// adapter. It deliberately omits the original raw input.
type ReportError struct {
	Report ValidationReport
}

func (e *ReportError) Error() string { return ErrValidation.Error() }
func (e *ReportError) Unwrap() error { return ErrValidation }

func fieldError(field, message string) error {
	return &FieldError{Field: field, Message: message}
}
