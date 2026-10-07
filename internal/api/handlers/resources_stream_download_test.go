package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
)

func TestLogDownloadUsesProtectedStreamWithBackpressure(t *testing.T) {
	for _, test := range []struct {
		name, query     string
		badCSRF, denied bool
		status          int
	}{
		{"current", "container=api", false, false, 200},
		{"previous", "container=api&previous=true", false, false, 200},
		{"tail rejected", "container=api&tailLines=10", false, false, 400},
		{"since rejected", "container=api&since=1m", false, false, 400},
		{"unknown field", "container=api&all=true", false, false, 400},
		{"CSRF", "container=api", true, false, 403},
		{"RBAC", "container=api", false, true, 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			origin := "http://127.0.0.1:2748"
			sessions, err := api.NewSessionStore(0)
			if err != nil {
				t.Fatal(err)
			}
			session, err := sessions.Current(origin, "gen")
			if err != nil {
				t.Fatal(err)
			}
			service := &resourceStreamServiceStub{}
			if test.denied {
				service.logAuthErr = &resourcecore.DomainError{Code: resourcecore.CodeForbidden, Message: "Denied"}
			}
			selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}}
			handler := NewResourceStreams(service, selection, sessions, origin)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/pods/default/api/logs/download/stream?"+test.query, nil)
			request.SetPathValue("namespace", "default")
			request.SetPathValue("name", "api")
			request.Header.Set("Origin", origin)
			request.Header.Set("X-KubePeep-CSRF", session.CSRFToken)
			if test.badCSRF {
				request.Header.Set("X-KubePeep-CSRF", "wrong")
			}
			response := httptest.NewRecorder()
			handler.LogDownload(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.status != 200 {
				if service.downloaded != 0 {
					t.Fatal("download bypassed a guard")
				}
				return
			}
			body := response.Body.String()
			if strings.Count(body, "event: line") != 100 || !strings.Contains(body, `"reason":"completed"`) || strings.Contains(body, "event: error") {
				t.Fatalf("incomplete export: %s", body)
			}
			if response.Header().Get("Cache-Control") != "no-store" || service.downloaded != 1 || service.followed != 0 {
				t.Fatal("download used an unexpected transport")
			}
		})
	}
}
