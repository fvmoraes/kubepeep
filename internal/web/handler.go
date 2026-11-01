package web

import (
	"bytes"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var versionedAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{8,}\.[A-Za-z0-9]+$`)

// Handler serves an injected frontend filesystem. Production passes the
// embedded Vite dist; tests use fstest.MapFS and never depend on Node.js.
type Handler struct {
	frontend fs.FS
	files    http.Handler
	csp      string
}

func New(frontend fs.FS, publishedPort int) (*Handler, error) {
	if frontend == nil {
		return nil, fmt.Errorf("web: frontend filesystem is nil")
	}
	if publishedPort < 1024 || publishedPort > 65535 {
		return nil, fmt.Errorf("web: published port is outside 1024-65535")
	}
	return &Handler{
		frontend: frontend,
		files:    http.FileServer(http.FS(frontend)),
		csp: strings.Join([]string{
			"default-src 'self'",
			"script-src 'self'",
			"style-src 'self'",
			"img-src 'self' data:",
			"connect-src 'self' ws://127.0.0.1:" + strconv.Itoa(publishedPort),
			"object-src 'none'",
			"base-uri 'none'",
			"frame-ancestors 'none'",
		}, "; ") + ";",
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setBrowserHeaders(w.Header(), h.csp)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	if reservedPath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}

	requestPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	// The root path is the canonical SPA entry: plain HTTP health probes
	// (systemd, curl, container checks) request it without an Accept header
	// and must not read a 404 as "service dead".
	if requestPath == "" || requestPath == "index.html" {
		h.serveIndex(w, r)
		return
	}
	if requestPath != "." && fs.ValidPath(requestPath) {
		if info, err := fs.Stat(h.frontend, requestPath); err == nil && !info.IsDir() {
			// Encoded siblings exist only to answer negotiation. Serving one
			// directly would hand compressed bytes labelled as the sibling's
			// own type, with no Content-Encoding for the client to act on.
			// This check belongs here, after the path resolved to a real file:
			// a Kubernetes object name is a DNS subdomain and may legally end
			// in ".gz", so a client-side route like
			// /config/web/archive.tar.gz must still reach the SPA shell.
			if encodedSibling(requestPath) {
				http.NotFound(w, r)
				return
			}
			// Cache policy is derived from the asset the client asked for.
			// An encoded sibling is named "<asset>.br", which does not match
			// the versioned-asset pattern, so deriving it from the file that
			// is actually written would downgrade immutable assets.
			setAssetCache(w.Header(), requestPath)
			if h.serveEncoded(w, r, requestPath) {
				return
			}
			h.files.ServeHTTP(w, r)
			return
		}
	}

	if !acceptsHTML(r.Header.Get("Accept")) {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	index, err := fs.ReadFile(h.frontend, "index.html")
	if err != nil {
		http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Length", strconv.Itoa(len(index)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(index)
	}
}

// encodedSiblings are the build-time compressed variants, best first. The
// frontend build emits them next to each text asset so this handler never
// compresses per request.
var encodedSiblings = []struct {
	encoding string
	suffix   string
}{
	{encoding: "br", suffix: ".br"},
	{encoding: "gzip", suffix: ".gz"},
}

func encodedSibling(value string) bool {
	for _, sibling := range encodedSiblings {
		if strings.HasSuffix(value, sibling.suffix) {
			return true
		}
	}
	return false
}

// serveEncoded answers with a compressed sibling when the client accepts its
// encoding and the build produced one. It reports whether it wrote the
// response. Vary is set either way: the same URL legitimately has different
// bodies per Accept-Encoding, and a cache that ignores that would hand a
// brotli body to a client that cannot read it.
func (h *Handler) serveEncoded(w http.ResponseWriter, r *http.Request, assetPath string) bool {
	w.Header().Set("Vary", "Accept-Encoding")
	accept := r.Header.Get("Accept-Encoding")
	for _, sibling := range encodedSiblings {
		if !acceptsEncoding(accept, sibling.encoding) {
			continue
		}
		encoded, err := fs.ReadFile(h.frontend, assetPath+sibling.suffix)
		if err != nil {
			continue
		}
		// ServeContent falls back to sniffing the body when an extension has
		// no MIME entry, and the body here is the compressed sibling. Declare
		// the requested asset's type so the client never sees the encoding's.
		if contentType := mime.TypeByExtension(path.Ext(assetPath)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Content-Encoding", sibling.encoding)
		// ServeContent declines to set Content-Length once Content-Encoding is
		// present, and a HEAD writes no body for net/http to measure, so the
		// header would vanish for exactly the method that only wants it.
		w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
		http.ServeContent(w, r, assetPath, time.Time{}, bytes.NewReader(encoded))
		return true
	}
	return false
}

// acceptsEncoding reports whether the client allows one content coding. A
// "q=0" qualifier is an explicit refusal, so matching the token is not enough,
// and "*" offers every coding the server has.
func acceptsEncoding(header, encoding string) bool {
	for _, element := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(element), ";")
		token := strings.TrimSpace(fields[0])
		if !strings.EqualFold(token, encoding) && token != "*" {
			continue
		}
		for _, parameter := range fields[1:] {
			parameter = strings.ToLower(strings.Join(strings.Fields(parameter), ""))
			if parameter == "q=0" || strings.HasPrefix(parameter, "q=0.0") {
				return false
			}
		}
		return true
	}
	return false
}

func reservedPath(value string) bool {
	for _, prefix := range []string{"/api", "/health", "/_kubepeep/control"} {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return true
		}
	}
	return false
}

func acceptsHTML(accept string) bool {
	for _, mediaRange := range strings.Split(accept, ",") {
		mediaType := strings.TrimSpace(strings.SplitN(mediaRange, ";", 2)[0])
		if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
			return true
		}
	}
	return false
}

func setBrowserHeaders(header http.Header, csp string) {
	header.Set("Content-Security-Policy", csp)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
}

func setAssetCache(header http.Header, assetPath string) {
	if assetPath == "index.html" {
		header.Set("Cache-Control", "no-store")
		header.Set("Pragma", "no-cache")
		return
	}
	if versionedAsset.MatchString(path.Base(assetPath)) {
		header.Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	header.Set("Cache-Control", "no-cache")
}
