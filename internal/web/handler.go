package web

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
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
	if requestPath == "index.html" {
		h.serveIndex(w, r)
		return
	}
	if requestPath != "." && fs.ValidPath(requestPath) {
		if info, err := fs.Stat(h.frontend, requestPath); err == nil && !info.IsDir() {
			setAssetCache(w.Header(), requestPath)
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
