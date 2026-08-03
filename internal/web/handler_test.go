package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAFallbackNeverCapturesReservedRoutes(t *testing.T) {
	handler := testHandler(t)
	for _, route := range []string{
		"/api/v1/missing",
		"/health/missing",
		"/_kubepeep/control/v1/missing",
	} {
		t.Run(route, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, route, nil)
			r.Header.Set("Accept", "text/html")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "kubePeep shell") {
				t.Fatalf("reserved route captured: status=%d body=%q", w.Code, w.Body.String())
			}
		})
	}
}

func TestSPAHeadersAndAssetCaching(t *testing.T) {
	handler := testHandler(t)
	tests := []struct {
		path   string
		accept string
		cache  string
	}{
		{path: "/workloads", accept: "text/html", cache: "no-store"},
		{path: "/assets/app-AbCd1234.js", accept: "*/*", cache: "public, max-age=31536000, immutable"},
		{path: "/assets/logo.svg", accept: "image/svg+xml", cache: "no-cache"},
		{path: "/index.html", accept: "text/html", cache: "no-store"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, test.path, nil)
			r.Header.Set("Accept", test.accept)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Cache-Control"); got != test.cache {
				t.Fatalf("cache=%q, want %q", got, test.cache)
			}
			csp := w.Header().Get("Content-Security-Policy")
			if !strings.Contains(csp, "connect-src 'self' ws://127.0.0.1:2748") || strings.Contains(csp, "localhost") {
				t.Fatalf("unexpected CSP: %q", csp)
			}
		})
	}
}

func TestSPAFallbackRequiresHTMLAccept(t *testing.T) {
	handler := testHandler(t)
	r := httptest.NewRequest(http.MethodGet, "/missing", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestFrontendDistributionIsEmbedded(t *testing.T) {
	frontend, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(frontend, "index.html"); err != nil {
		t.Fatalf("embedded index.html is unavailable: %v", err)
	}
}

func testHandler(t *testing.T) *Handler {
	t.Helper()
	frontend := fstest.MapFS{
		"index.html":             &fstest.MapFile{Data: []byte("<html>kubePeep shell</html>"), Mode: fs.FileMode(0o444)},
		"assets/app-AbCd1234.js": &fstest.MapFile{Data: []byte("export {};"), Mode: fs.FileMode(0o444)},
		"assets/logo.svg":        &fstest.MapFile{Data: []byte("<svg></svg>"), Mode: fs.FileMode(0o444)},
	}
	handler, err := New(frontend, 2748)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
