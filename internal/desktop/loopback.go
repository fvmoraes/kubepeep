package desktop

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
)

// DesktopExtraHosts lists the WebView authorities used by Wails when the
// embedded page is served from the AssetServer (wails://wails on Linux/macOS,
// http://wails.localhost on Windows). They are only accepted by the desktop
// security profile.
func DesktopExtraHosts() []string {
	return []string{"wails", "wails.localhost"}
}

// DesktopExtraOrigins lists the page origins a Wails WebView can present for
// cross-origin streaming requests: the native wails scheme, the Windows host
// and the opaque "null" origin reported by browsers for non-http schemes.
func DesktopExtraOrigins() []string {
	return []string{"null", "wails://wails", "http://wails.localhost", "https://wails.localhost"}
}

// Loopback is the internal 127.0.0.1 listener reserved for streaming
// transports (SSE follow/resources and the exec WebSocket), which the Wails
// AssetServer cannot carry. It is never advertised beyond loopback.
type Loopback struct {
	server   *http.Server
	listener closeListener
	base     string
	port     int
}

type closeListener interface {
	Close() error
}

// NewLoopback binds an internal loopback listener for the composed handler.
// Streaming routes receive a strict CORS policy restricted to the desktop
// WebView origins.
func NewLoopback(handler http.Handler, allowedOrigins []string) (*Loopback, error) {
	listener, port, err := localruntime.BindLoopback(nil)
	if err != nil {
		return nil, fmt.Errorf("desktop: bind internal loopback: %w", err)
	}
	return NewLoopbackFromListener(listener, port, handler, allowedOrigins)
}

// NewLoopbackFromListener serves an already-bound loopback listener so the
// application composition and the streaming transport share one port.
func NewLoopbackFromListener(listener net.Listener, port int, handler http.Handler, allowedOrigins []string) (*Loopback, error) {
	if listener == nil {
		return nil, fmt.Errorf("desktop: loopback listener is required")
	}
	if handler == nil {
		return nil, fmt.Errorf("desktop: loopback handler is required")
	}
	server := &http.Server{
		Handler:           corsForStreams(allowedOrigins)(handler),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		_ = server.Serve(listener)
	}()
	return &Loopback{
		server:   server,
		listener: listener,
		base:     fmt.Sprintf("http://127.0.0.1:%d", port),
		port:     port,
	}, nil
}

// Base returns the loopback URL used by the frontend for streaming transports.
func (loopback *Loopback) Base() string {
	if loopback == nil {
		return ""
	}
	return loopback.base
}

// Port returns the acquired loopback port.
func (loopback *Loopback) Port() int {
	if loopback == nil {
		return 0
	}
	return loopback.port
}

// Close shuts the internal listener down gracefully.
func (loopback *Loopback) Close(ctx context.Context) error {
	if loopback == nil {
		return nil
	}
	if err := loopback.server.Shutdown(ctx); err != nil {
		_ = loopback.server.Close()
		return err
	}
	_ = loopback.listener.Close()
	return nil
}

// corsForStreams applies a desktop-only CORS policy to streaming routes. It
// reflects an allowed WebView origin and answers the required OPTIONS
// preflight for the CSRF header. Non-stream routes are untouched, so the web
// security posture of the shared handler never changes.
func corsForStreams(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			_, ok := allowed[origin]
			if ok && isStreamRoute(r.URL.Path) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "X-KubePeep-CSRF, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions && ok {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isStreamRoute(path string) bool {
	return strings.HasSuffix(path, "/stream")
}
