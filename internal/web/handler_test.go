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
		"index.html":                &fstest.MapFile{Data: []byte("<html>kubePeep shell</html>"), Mode: fs.FileMode(0o444)},
		"assets/app-AbCd1234.js":    &fstest.MapFile{Data: []byte("export {};"), Mode: fs.FileMode(0o444)},
		"assets/app-AbCd1234.js.br": &fstest.MapFile{Data: []byte("brotli-bytes"), Mode: fs.FileMode(0o444)},
		"assets/app-AbCd1234.js.gz": &fstest.MapFile{Data: []byte("gzip-bytes"), Mode: fs.FileMode(0o444)},
		"assets/logo.svg":           &fstest.MapFile{Data: []byte("<svg></svg>"), Mode: fs.FileMode(0o444)},
	}
	handler, err := New(frontend, 2748)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestAssetEncodingNegotiationPrefersBrotliAndFallsBackHonestly(t *testing.T) {
	handler := testHandler(t)
	for _, test := range []struct {
		name     string
		accept   string
		encoding string
		body     string
	}{
		{name: "both offered prefers brotli", accept: "gzip, deflate, br", encoding: "br", body: "brotli-bytes"},
		{name: "only gzip offered", accept: "gzip, deflate", encoding: "gzip", body: "gzip-bytes"},
		{name: "no accept-encoding stays identity", accept: "", encoding: "", body: "export {};"},
		{name: "brotli refused by qualifier", accept: "br;q=0, gzip", encoding: "gzip", body: "gzip-bytes"},
		{name: "every encoding refused stays identity", accept: "br;q=0, gzip;q=0", encoding: "", body: "export {};"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/assets/app-AbCd1234.js", nil)
			if test.accept != "" {
				r.Header.Set("Accept-Encoding", test.accept)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d", w.Code)
			}
			if got := w.Header().Get("Content-Encoding"); got != test.encoding {
				t.Fatalf("content-encoding=%q, want %q", got, test.encoding)
			}
			if got := w.Body.String(); got != test.body {
				t.Fatalf("body=%q, want %q", got, test.body)
			}
			// A cache that ignores Accept-Encoding would hand a brotli body
			// to a client that cannot read it.
			if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Fatalf("vary=%q, want Accept-Encoding", got)
			}
			// The sibling is named app-AbCd1234.js.br, which does not match
			// the versioned-asset pattern. Deriving the policy from the file
			// written instead of the file requested would silently drop a
			// year of caching.
			if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
				t.Fatalf("cache=%q, want the immutable policy of the requested asset", got)
			}
			if got := w.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
				t.Fatalf("content-type=%q, want the asset's type and not the sibling's", got)
			}
		})
	}
}

func TestAssetWithoutEncodedSiblingStillServesIdentity(t *testing.T) {
	handler := testHandler(t)
	r := httptest.NewRequest(http.MethodGet, "/assets/logo.svg", nil)
	r.Header.Set("Accept-Encoding", "br, gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "<svg></svg>" {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("content-encoding=%q, want identity when the build emitted no sibling", got)
	}
	if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("vary=%q, want Accept-Encoding even when serving identity", got)
	}
}

// The encoded siblings exist only to answer negotiation. Addressing one
// directly would hand compressed bytes labelled as the sibling's own type,
// with no Content-Encoding for the client to act on.
func TestEncodedSiblingIsNotAddressableOnItsOwn(t *testing.T) {
	handler := testHandler(t)
	for _, path := range []string{"/assets/app-AbCd1234.js.br", "/assets/app-AbCd1234.js.gz"} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.Header.Set("Accept-Encoding", "br, gzip")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status=%d, want 404 for a sibling addressed directly", w.Code)
			}
		})
	}
}

// A Kubernetes object name is a DNS subdomain and may legally end in ".gz" or
// ".br". The sibling guard must reject real sibling files, not every client
// route that happens to share the suffix.
func TestClientRouteEndingInAnEncodingSuffixStillReachesTheShell(t *testing.T) {
	handler := testHandler(t)
	for _, route := range []string{"/config/web/archive.tar.gz", "/pods/web/backup.br"} {
		t.Run(route, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, route, nil)
			r.Header.Set("Accept", "text/html")
			r.Header.Set("Accept-Encoding", "br, gzip")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "kubePeep shell") {
				t.Fatalf("deep link was captured by the sibling guard: status=%d body=%q", w.Code, w.Body.String())
			}
		})
	}
}

// http.ServeContent declines to set Content-Length once Content-Encoding is
// present, and a HEAD writes no body for net/http to measure, so the header
// would vanish for exactly the method that only wants it.
func TestHeadOnNegotiatedAssetReportsTheEncodedLength(t *testing.T) {
	handler := testHandler(t)
	for _, test := range []struct {
		name     string
		accept   string
		encoding string
		length   string
	}{
		{name: "encoded", accept: "br", encoding: "br", length: "12"},
		{name: "identity", accept: "", encoding: "", length: "10"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodHead, "/assets/app-AbCd1234.js", nil)
			if test.accept != "" {
				r.Header.Set("Accept-Encoding", test.accept)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d", w.Code)
			}
			if got := w.Header().Get("Content-Encoding"); got != test.encoding {
				t.Fatalf("content-encoding=%q, want %q", got, test.encoding)
			}
			if got := w.Header().Get("Content-Length"); got != test.length {
				t.Fatalf("content-length=%q, want %q", got, test.length)
			}
			if w.Body.Len() != 0 {
				t.Fatalf("HEAD wrote a body of %d bytes", w.Body.Len())
			}
		})
	}
}

// A client offering the wildcard accepts any coding the server has, so it must
// not silently fall back to identity.
func TestWildcardAcceptEncodingNegotiatesTheBestAvailableCoding(t *testing.T) {
	handler := testHandler(t)
	r := httptest.NewRequest(http.MethodGet, "/assets/app-AbCd1234.js", nil)
	r.Header.Set("Accept-Encoding", "*")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if got := w.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("content-encoding=%q, want br for a wildcard offer", got)
	}
	if got := w.Body.String(); got != "brotli-bytes" {
		t.Fatalf("body=%q", got)
	}
}

func TestRootServesIndexRegardlessOfAccept(t *testing.T) {
	handler := testHandler(t)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("root status=%d — plain HTTP health probes must read the SPA index, not a 404", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("root cache=%q", got)
	}
}

// Range applies to the selected representation, so a ranged request over a
// negotiated asset addresses the compressed bytes. httptest.NewRecorder does
// not exercise the transport, so this runs against a real server: that is how
// the missing Content-Length on HEAD escaped the first round of tests.
func TestRangeOverNegotiatedAssetAddressesTheEncodedRepresentation(t *testing.T) {
	server := httptest.NewServer(testHandler(t))
	t.Cleanup(server.Close)
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	ranged := func(acceptEncoding string) *http.Response {
		request, err := http.NewRequest(http.MethodGet, server.URL+"/assets/app-AbCd1234.js", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Range", "bytes=0-3")
		if acceptEncoding != "" {
			request.Header.Set("Accept-Encoding", acceptEncoding)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}

	encoded := ranged("br")
	identity := ranged("")

	for name, response := range map[string]*http.Response{"encoded": encoded, "identity": identity} {
		if response.StatusCode != http.StatusPartialContent {
			t.Fatalf("%s range status=%d, want 206", name, response.StatusCode)
		}
		if response.ContentLength != 4 {
			t.Fatalf("%s range Content-Length=%d, want the four requested bytes", name, response.ContentLength)
		}
		// Without Vary a shared cache could answer a ranged identity request
		// with bytes taken from the compressed representation.
		if got := response.Header.Get("Vary"); got != "Accept-Encoding" {
			t.Fatalf("%s range vary=%q, want Accept-Encoding", name, got)
		}
	}
	if got := encoded.Header.Get("Content-Encoding"); got != "br" {
		t.Fatalf("encoded range lost its encoding: %q", got)
	}
	if got := identity.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("identity range advertised an encoding: %q", got)
	}
	// The two representations have different sizes, so their ranges must be
	// reported against different totals.
	if encoded.Header.Get("Content-Range") == identity.Header.Get("Content-Range") {
		t.Fatalf("both ranges reported the same total: %q", encoded.Header.Get("Content-Range"))
	}
}
