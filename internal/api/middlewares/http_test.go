package middlewares

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gingermiddleware "github.com/fvmoraes/ginger/pkg/middleware"

	"github.com/fvmoraes/kubepeep/internal/api"
)

type fixedGeneration string

func (g fixedGeneration) Current() string { return string(g) }

func TestBrowserAPIRejectsForeignRequestsAndAcceptsValidMutation(t *testing.T) {
	sessions, err := api.NewSessionStore(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	generation := fixedGeneration("gen_test")
	session, err := sessions.Current("http://127.0.0.1:2748", generation.Current())
	if err != nil {
		t.Fatal(err)
	}
	config := SecurityConfig{
		Host:       "127.0.0.1:2748",
		Origin:     "http://127.0.0.1:2748",
		Sessions:   sessions,
		Generation: generation,
	}
	handler := gingermiddleware.Chain(RequestID(), BrowserAPI(config))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name        string
		host        string
		origin      string
		token       string
		contentType string
		want        int
	}{
		{name: "host", host: "localhost:2748", origin: config.Origin, token: session.CSRFToken, contentType: "application/json", want: http.StatusForbidden},
		{name: "origin", host: config.Host, origin: "http://evil.invalid", token: session.CSRFToken, contentType: "application/json", want: http.StatusForbidden},
		{name: "token", host: config.Host, origin: config.Origin, token: "wrong", contentType: "application/json", want: http.StatusForbidden},
		{name: "content-type", host: config.Host, origin: config.Origin, token: session.CSRFToken, contentType: "text/plain", want: http.StatusUnsupportedMediaType},
		{name: "valid", host: config.Host, origin: config.Origin, token: session.CSRFToken, contentType: "application/json; charset=utf-8", want: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, config.Origin+"/api/v1/future", bytes.NewBufferString(`{}`))
			r.Host = test.host
			r.Header.Set("Origin", test.origin)
			r.Header.Set("X-KubePeep-CSRF", test.token)
			r.Header.Set("Content-Type", test.contentType)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, test.want, w.Body.String())
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("CORS unexpectedly enabled: %q", got)
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
				t.Fatalf("required response headers missing: %#v", w.Header())
			}
		})
	}
}

func TestRawChainPreservesStreamingInterfacesAndLongDeadline(t *testing.T) {
	config := SecurityConfig{Host: "127.0.0.1:2748", Origin: "http://127.0.0.1:2748"}
	var remaining time.Duration
	handler := RawChain(nil, config, 16*time.Second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := RequireStreamingInterfaces(w); err != nil {
			t.Error(err)
		}
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Error("raw route has no route-owned deadline")
			return
		}
		remaining = time.Until(deadline)
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, config.Origin+"/api/v1/stream", nil)
	r.Host = config.Host
	r.Header.Set("Origin", config.Origin)
	w := newFeatureWriter()
	handler.ServeHTTP(w, r)
	if w.status != http.StatusNoContent {
		t.Fatalf("status = %d", w.status)
	}
	if remaining <= 15*time.Second {
		t.Fatalf("raw deadline = %s, want greater than 15s", remaining)
	}
}

func TestRawChainRejectsInvalidHostAndOrigin(t *testing.T) {
	config := SecurityConfig{Host: "127.0.0.1:2748", Origin: "http://127.0.0.1:2748"}
	tests := []struct {
		name   string
		host   string
		origin string
	}{
		{name: "host", host: "localhost:2748", origin: config.Origin},
		{name: "origin", host: config.Host, origin: "http://evil.invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invoked := false
			handler := RawChain(nil, config, 16*time.Second, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				invoked = true
			}))
			r := httptest.NewRequest(http.MethodGet, config.Origin+"/api/v1/stream", nil)
			r.Host = test.host
			r.Header.Set("Origin", test.origin)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if invoked {
				t.Fatal("raw handler was invoked for a rejected request")
			}
			if w.Code != http.StatusForbidden || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
				t.Fatalf("unexpected rejection: status=%d headers=%#v", w.Code, w.Header())
			}
			var body struct {
				Code      string `json:"code"`
				RequestID string `json:"requestId"`
			}
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Code != api.CodeCSRFRejected || body.RequestID != w.Header().Get("X-Request-ID") {
				t.Fatalf("unexpected rejection body: %#v", body)
			}
		})
	}
}

type featureWriter struct {
	header http.Header
	status int
}

func newFeatureWriter() *featureWriter       { return &featureWriter{header: make(http.Header)} }
func (w *featureWriter) Header() http.Header { return w.header }
func (w *featureWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return len(body), nil
}
func (w *featureWriter) WriteHeader(status int)                       { w.status = status }
func (w *featureWriter) Flush()                                       {}
func (w *featureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, nil }

func TestRequestIDReplacesUnsafeCallerValue(t *testing.T) {
	var requestID string
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID = api.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Request-ID", "line\nbreak")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if requestID == "line\nbreak" || requestID == "" || w.Header().Get("X-Request-ID") != requestID {
		t.Fatalf("unsafe request ID was not replaced: %q", requestID)
	}
}

var _ http.Flusher = (*featureWriter)(nil)
var _ http.Hijacker = (*featureWriter)(nil)
