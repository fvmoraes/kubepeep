package middlewares

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/fvmoraes/ginger/pkg/logger"
	gingermiddleware "github.com/fvmoraes/ginger/pkg/middleware"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/logging"
)

const requestIDBytes = 18

type SecurityConfig struct {
	Host       string
	Origin     string
	Sessions   *api.SessionStore
	Generation api.GenerationSource
	// ExtraHosts and ExtraOrigins widen the loopback-only policy for embedded
	// desktop shells (Wails). They are never set by the web runtime, which
	// keeps exact Host/Origin enforcement.
	ExtraHosts   []string
	ExtraOrigins []string
}

// RequestID sanitizes the optional caller-supplied ID before Ginger sees it,
// then propagates the same value through the response and application context.
func RequestID() gingermiddleware.Func {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if !validRequestID(requestID) {
				requestID = generateRequestID()
			}
			r.Header.Set("X-Request-ID", requestID)
			w.Header().Set("X-Request-ID", requestID)
			ctx := api.WithRequestID(r.Context(), requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Recovery returns the contract error envelope and never serializes the panic
// value. It does not wrap ResponseWriter, preserving streaming interfaces.
// Recovered panics are rate-limited (O-03): a broken route hammered by a
// poller emits the first three panics and then at most one per minute.
func Recovery(log *logger.Logger) gingermiddleware.Func {
	panicSampler := logging.NewSampler(3, time.Minute)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recover() == nil {
					return
				}
				if log != nil && panicSampler.Allow() {
					log.Error(
						"panic_recovered",
						"operation", "http.request",
						"request_id", api.RequestIDFromContext(r.Context()),
						"error_code", api.CodeInternal,
					)
				}
				api.WriteError(w, r, api.NewHTTPError(
					http.StatusInternalServerError,
					api.CodeInternal,
					"Internal server error.",
					nil,
					nil,
				))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Host requires the published authority, preventing DNS rebinding and
// accidental alias expansion. Desktop shells may allow additional hosts.
func Host(expected string, extras ...string) gingermiddleware.Func {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hostAllowed(r.Host, expected, extras) {
				api.WriteError(w, r, csrfRejected())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BrowserAPI applies no-store, same-origin and mutation CSRF policy. It emits
// no Access-Control-* headers, which keeps CORS disabled.
func BrowserAPI(config SecurityConfig) gingermiddleware.Func {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setAPIHeaders(w.Header())
			if !hostAllowed(r.Host, config.Host, config.ExtraHosts) || !sameOriginIfPresent(r, config.Origin, config.ExtraOrigins) {
				api.WriteError(w, r, csrfRejected())
				return
			}
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				api.WriteError(w, r, csrfRejected())
				return
			}
			if isMutable(r.Method) {
				if !originAllowed(r.Header.Get("Origin"), config.Origin, config.ExtraOrigins) || config.Sessions == nil || config.Generation == nil {
					api.WriteError(w, r, csrfRejected())
					return
				}
				if !config.Sessions.Validate(r.Header.Get("X-KubePeep-CSRF"), config.Generation.Current()) {
					api.WriteError(w, r, csrfRejected())
					return
				}
				if !isJSONContentType(r.Header.Get("Content-Type")) {
					api.WriteError(w, r, api.NewHTTPError(
						http.StatusUnsupportedMediaType,
						api.CodeUnsupportedMediaType,
						"Content-Type must be application/json.",
						nil,
						nil,
					))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SameOriginRead protects browser-readable raw routes while preserving the
// original ResponseWriter.
func SameOriginRead(config SecurityConfig) gingermiddleware.Func {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hostAllowed(r.Host, config.Host, config.ExtraHosts) || !sameOriginIfPresent(r, config.Origin, config.ExtraOrigins) {
				api.WriteError(w, r, csrfRejected())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WithDeadline adds a route-owned deadline without wrapping ResponseWriter.
// Streaming callers choose a duration compatible with their own wire contract.
func WithDeadline(timeout time.Duration) gingermiddleware.Func {
	return func(next http.Handler) http.Handler {
		if timeout <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RawChain is the reusable chain for future SSE/WebSocket endpoints. None of
// its middlewares wraps the writer, so Flusher and Hijacker remain available.
func RawChain(log *logger.Logger, config SecurityConfig, timeout time.Duration, next http.Handler) http.Handler {
	return gingermiddleware.Chain(
		RequestID(),
		Recovery(log),
		SameOriginRead(config),
		WithDeadline(timeout),
	)(next)
}

func RequireStreamingInterfaces(w http.ResponseWriter) error {
	if _, ok := w.(http.Flusher); !ok {
		return fmt.Errorf("response writer does not implement http.Flusher")
	}
	if _, ok := w.(http.Hijacker); !ok {
		return fmt.Errorf("response writer does not implement http.Hijacker")
	}
	return nil
}

func setAPIHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Pragma", "no-cache")
	header.Set("X-Content-Type-Options", "nosniff")
}

func sameOriginIfPresent(r *http.Request, expected string, extras []string) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || originAllowed(origin, expected, extras)
}

func originAllowed(origin, expected string, extras []string) bool {
	if origin == expected {
		return true
	}
	for _, extra := range extras {
		if origin == extra {
			return true
		}
	}
	return false
}

func hostAllowed(host, expected string, extras []string) bool {
	if host == expected {
		return true
	}
	for _, extra := range extras {
		if host == extra {
			return true
		}
	}
	return false
}

func csrfRejected() error {
	return api.NewHTTPError(
		http.StatusForbidden,
		api.CodeCSRFRejected,
		"The request did not pass local browser security checks.",
		nil,
		nil,
	)
}

func isMutable(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func generateRequestID() string {
	bytes := make([]byte, requestIDBytes)
	if _, err := rand.Read(bytes); err != nil {
		// The timestamp is not secret and is used only as an extremely unlikely
		// entropy-source fallback; it is never accepted as authorization.
		return fmt.Sprintf("req_%x", time.Now().UnixNano())
	}
	return "req_" + base64.RawURLEncoding.EncodeToString(bytes)
}
