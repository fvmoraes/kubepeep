package spike

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fvmoraes/ginger/pkg/logger"
	"github.com/fvmoraes/ginger/pkg/middleware"
)

func RawMiddleware(log *logger.Logger, next http.Handler) http.Handler {
	secured := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}

		started := time.Now()
		next.ServeHTTP(w, r)
		log.Info(
			"raw_request_finished",
			slog.String("operation", "http.raw"),
			slog.String("request_id", middleware.RequestIDFromContext(r.Context())),
			slog.Duration("duration", time.Since(started)),
		)
	})

	return middleware.Chain(
		middleware.RequestID(),
		middleware.Recover(log),
	)(secured)
}

func isLoopbackHost(hostport string) bool {
	host := hostport
	if parsed, _, err := net.SplitHostPort(hostport); err == nil {
		host = parsed
	} else {
		host = strings.Trim(host, "[]")
	}

	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host) &&
		(parsed.Scheme == "http" || parsed.Scheme == "https")
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
