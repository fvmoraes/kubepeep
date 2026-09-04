// Package desktop exposes the thin desktop shell layer: a binding bridge over
// the composed HTTP application and the internal loopback listener used only
// for streaming transports (SSE and WebSocket) that Wails cannot carry. It
// contains no Wails dependency; the Wails glue lives in internal/desktop/wails.
package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/fvmoraes/kubepeep/internal/buildinfo"
)

// PlatformInfoDTO is the sanitized environment surface exposed to the React
// frontend. It deliberately carries no credentials or paths.
type PlatformInfoDTO struct {
	Mode       string `json:"mode"`
	StreamBase string `json:"streamBase"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	BuildDate  string `json:"buildDate"`
}

// InvokeResult mirrors the minimal HTTP response contract consumed by the
// frontend client without involving any network listener. The json tags are
// load-bearing: the Wails bridge serializes this struct with encoding/json,
// and without lowercase tags the frontend would receive Go field names
// ("Status"/"Headers"/"Body") instead of the contract keys.
type InvokeResult struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body"`
}

// Bridge is the Wails binding layer. Invoke forwards allowlisted API calls to
// the in-process HTTP application, keeping every validation, pagination,
// authorization and serialization rule in the existing handlers.
type Bridge struct {
	handler      http.Handler
	host         string
	origin       string
	streamBase   string
	platformInfo PlatformInfoDTO
}

func NewBridge(handler http.Handler, origin string, streamBase string) *Bridge {
	return &Bridge{
		handler:    handler,
		host:       strings.TrimPrefix(origin, "http://"),
		origin:     origin,
		streamBase: streamBase,
		platformInfo: PlatformInfoDTO{
			Mode:       "desktop",
			StreamBase: streamBase,
			Version:    buildinfo.Version,
			Commit:     buildinfo.Commit,
			BuildDate:  buildinfo.BuildDate,
		},
	}
}

// PlatformInfo returns build metadata and the loopback base used only by
// streaming transports.
func (bridge *Bridge) PlatformInfo() PlatformInfoDTO {
	if bridge == nil {
		return PlatformInfoDTO{}
	}
	return bridge.platformInfo
}

// Invoke runs one allowlisted API request in-process. Streaming endpoints are
// excluded because Wails bindings cannot carry them; they use the loopback
// base returned by PlatformInfo.
func (bridge *Bridge) Invoke(method string, path string, headers map[string]string, body string) (InvokeResult, error) {
	if bridge == nil || bridge.handler == nil {
		return InvokeResult{}, fmt.Errorf("desktop: bridge is unavailable")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return InvokeResult{}, fmt.Errorf("desktop: unsupported method")
	}
	if !invokePathAllowed(path) {
		return InvokeResult{}, fmt.Errorf("desktop: path is not allowed through bindings")
	}
	request, err := http.NewRequestWithContext(context.Background(), method, "http://"+bridge.host+path, strings.NewReader(body))
	if err != nil {
		return InvokeResult{}, fmt.Errorf("desktop: build request: %w", err)
	}
	request.Host = bridge.host
	request.Header.Set("Origin", bridge.origin)
	request.Header.Set("Accept", "application/json")
	for name, value := range headers {
		if headerAllowed(name) {
			request.Header.Set(name, value)
		}
	}
	recorder := httptest.NewRecorder()
	bridge.handler.ServeHTTP(recorder, request)
	// The desktop client looks headers up by lowercase name; Go canonicalizes
	// them (Content-Type), so normalize here to keep the contract consistent.
	normalized := make(map[string][]string, len(recorder.Header()))
	for name, values := range recorder.Header() {
		normalized[strings.ToLower(name)] = values
	}
	return InvokeResult{
		Status:  recorder.Code,
		Headers: normalized,
		Body:    recorder.Body.String(),
	}, nil
}

func invokePathAllowed(path string) bool {
	if path == "" || strings.Contains(path, "..") || strings.Contains(path, "\\") {
		return false
	}
	if !strings.HasPrefix(path, "/api/v1/") {
		return false
	}
	if strings.Contains(path, "/stream") || strings.Contains(path, "/exec/") {
		return false
	}
	return true
}

func headerAllowed(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "X-Kubepeep-Csrf", "Idempotency-Key", "Content-Type", "X-Request-Id":
		return true
	default:
		return false
	}
}
