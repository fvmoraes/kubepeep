package spike

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// embeddedAssets proves that frontend assets and migrations can coexist in one
// compiled binary.
//
//go:embed assets/dist/* assets/migrations/*
var embeddedAssets embed.FS

func FrontendFS() (fs.FS, error) {
	return fs.Sub(embeddedAssets, "assets/dist")
}

func MigrationFS() (fs.FS, error) {
	return fs.Sub(embeddedAssets, "assets/migrations")
}

func SPAHandler(frontend fs.FS) http.Handler {
	files := http.FileServer(http.FS(frontend))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/health" ||
			strings.HasPrefix(r.URL.Path, "/health/") ||
			r.URL.Path == "/api" ||
			strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		requestPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requestPath != "." {
			if file, err := frontend.Open(requestPath); err == nil {
				_ = file.Close()
				files.ServeHTTP(w, r)
				return
			}
		}

		if !strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.NotFound(w, r)
			return
		}

		index, err := fs.ReadFile(frontend, "index.html")
		if err != nil {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(index)
	})
}
