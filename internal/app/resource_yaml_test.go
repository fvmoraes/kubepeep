package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type yamlSelection struct{}

func (yamlSelection) Snapshot() (namespaces.SelectionBinding, namespaces.ScopeResolution) {
	return namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen_yaml"}, namespaces.ScopeResolution{Namespaces: []string{"payments"}}
}

type yamlActions struct {
	actions.ActionService
	writes int
	route  actions.RouteTarget
}

func (s *yamlActions) ReadResourceYAML(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, route actions.RouteTarget) (actions.ResourceYAMLDocument, error) {
	s.route = route
	return actions.ResourceYAMLDocument{YAML: "fixture-only", Kind: "Secret", Generation: "gen_yaml"}, nil
}
func (s *yamlActions) UpdateResourceYAML(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, route actions.RouteTarget, _ actions.ResourceYAMLRequest) (actions.ActionAcceptedDTO, error) {
	s.writes++
	s.route = route
	return actions.ActionAcceptedDTO{Accepted: true}, nil
}

func TestGenericYAMLRoutesKeepHTTPGuards(t *testing.T) {
	backend := &yamlActions{}
	options := testApplicationOptions(t, staticSnapshots{snapshot: healthySnapshot()})
	options.Actions, options.Selection, options.Generation = backend, yamlSelection{}, fixedGeneration("gen_yaml")
	application, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, origin, csrf string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1:2748"+path, strings.NewReader(body))
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if csrf != "" {
			req.Header.Set("X-KubePeep-CSRF", csrf)
		}
		if method == http.MethodPut {
			req.Header.Set("Content-Type", "application/json")
		}
		out := httptest.NewRecorder()
		application.Handler.ServeHTTP(out, req)
		return out
	}
	session := request("GET", "/api/v1/session", "", "", "")
	var envelope struct {
		Data api.SessionData `json:"data"`
	}
	if err := json.Unmarshal(session.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/resources/secrets/payments/sample/yaml", "/api/v1/resources/cluster-roles/system:discovery/yaml"} {
		response := request("GET", path, "", "", "")
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read route: %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(path, "cluster-roles") && (backend.route.Namespace != "" || backend.route.Name != "system:discovery") {
			t.Fatalf("wrong cluster route: %#v", backend.route)
		}
		for _, guard := range []struct {
			origin, csrf, body string
			status             int
		}{
			{"http://127.0.0.1:2748", "", "{}", 403},
			{"http://evil.invalid", envelope.Data.CSRFToken, "{}", 403},
			{"http://127.0.0.1:2748", envelope.Data.CSRFToken, `{"extra":"forbidden"}`, 400},
			{"http://127.0.0.1:2748", envelope.Data.CSRFToken, "{}", 200},
		} {
			before := backend.writes
			response = request("PUT", path, guard.body, guard.origin, guard.csrf)
			if response.Code != guard.status {
				t.Fatalf("guard status %d: %s", response.Code, response.Body.String())
			}
			if guard.status != 200 && backend.writes != before {
				t.Fatal("write bypassed HTTP guard")
			}
		}
	}
}
