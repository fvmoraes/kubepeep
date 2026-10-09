package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/api/middlewares"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type helmServiceStub struct {
	ResourceService
	calls                           int
	driver, namespace, name, format string
	mutation                        resources.HelmMutationRequest
	err                             error
	after                           func()
}

func (s *helmServiceStub) ListHelmReleases(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, resources.ListOptions, *resources.CompositeCursor[resources.HelmReleaseDTO]) (resources.ListResult[resources.HelmReleaseDTO], error) {
	s.calls++
	return resources.ListResult[resources.HelmReleaseDTO]{Items: []resources.HelmReleaseDTO{}, Page: resources.PageDTO{Complete: true, Limit: 100}}, s.err
}
func (s *helmServiceStub) GetHelmRelease(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, driver, namespace, name string) (resources.HelmReleaseDTO, error) {
	s.calls++
	s.driver, s.namespace, s.name = driver, namespace, name
	if s.after != nil {
		s.after()
	}
	return resources.HelmReleaseDTO{Name: name, Namespace: namespace, Driver: driver, Revision: 2}, s.err
}
func (s *helmServiceStub) HelmDocument(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, driver, namespace, name, format string) (resources.HelmDocumentDTO, error) {
	s.calls++
	s.driver, s.namespace, s.name, s.format = driver, namespace, name, format
	return resources.HelmDocumentDTO{Document: "key: explicit-value", Format: format, Revision: 2}, s.err
}
func (s *helmServiceStub) MutateHelmRelease(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, driver, namespace, name string, command resources.HelmMutationRequest) (resources.HelmMutationResult, error) {
	s.calls++
	s.driver, s.namespace, s.name = driver, namespace, name
	s.mutation = command
	return resources.HelmMutationResult{Accepted: true, Revision: 3, Status: "deployed"}, s.err
}
func helmHandlerFixture(t *testing.T) (*Resources, *helmServiceStub, *resourceSelectionStub) {
	t.Helper()
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	service := &helmServiceStub{}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"ns"}}}
	return NewResources(service, nil, selection, codec), service, selection
}
func helmRequest(method, format, body string) *http.Request {
	request := httptest.NewRequest(method, "/api/v1/helm/releases/secrets/ns/demo", strings.NewReader(body))
	request.SetPathValue("driver", "secrets")
	request.SetPathValue("namespace", "ns")
	request.SetPathValue("name", "demo")
	if format != "" {
		request.SetPathValue("format", format)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}
func TestHelmEndpointsUseExactTargetAndNoStore(t *testing.T) {
	handler, service, _ := helmHandlerFixture(t)
	for _, format := range []string{"", "values", "manifest"} {
		request := helmRequest(http.MethodGet, format, "")
		response := httptest.NewRecorder()
		if format == "" {
			handler.HelmDetail(response, request)
		} else {
			handler.HelmDocument(response, request)
		}
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || service.driver != "secrets" || service.namespace != "ns" || service.name != "demo" {
			t.Fatalf("wrong target/headers: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestHelmListUsesBoundedCollectionHandler(t *testing.T) {
	handler, service, _ := helmHandlerFixture(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/helm/releases/secrets?namespace=ns&limit=25", nil)
	request.SetPathValue("driver", "secrets")
	response := httptest.NewRecorder()
	handler.HelmList(response, request)
	if response.Code != http.StatusOK || service.calls != 1 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Helm list failed: %d %s", response.Code, response.Body.String())
	}
}

type helmGeneration string

func (g helmGeneration) Current() string { return string(g) }
func TestHelmMutationRequiresCurrentCSRFSession(t *testing.T) {
	handler, service, _ := helmHandlerFixture(t)
	sessions, err := api.NewSessionStore(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://127.0.0.1:2748"
	session, err := sessions.Current(origin, "gen")
	if err != nil {
		t.Fatal(err)
	}
	secured := middlewares.BrowserAPI(middlewares.SecurityConfig{Host: "127.0.0.1:2748", Origin: origin, Sessions: sessions, Generation: helmGeneration("gen")})(http.HandlerFunc(handler.HelmApply))
	command := `{"expectedGeneration":"gen","expectedRevision":2,"expectedUid":"uid","expectedResourceVersion":"17","confirmed":true,"action":"rollback","revision":1}`
	for _, token := range []string{"", "wrong", session.CSRFToken} {
		request := helmRequest(http.MethodPost, "", command)
		request.Host = "127.0.0.1:2748"
		request.Header.Set("Origin", origin)
		request.Header.Set("X-KubePeep-CSRF", token)
		response := httptest.NewRecorder()
		secured.ServeHTTP(response, request)
		if token == session.CSRFToken {
			if response.Code != 200 || service.calls != 1 {
				t.Fatalf("valid mutation blocked: %d %s", response.Code, response.Body.String())
			}
		} else if response.Code != 403 || service.calls != 0 {
			t.Fatalf("CSRF bypass: %d calls=%d", response.Code, service.calls)
		}
	}
}
func TestHelmHandlerRejectsUnsupportedDriverFormatAndNamespace(t *testing.T) {
	for _, invalid := range []string{"driver", "format", "namespace", "query"} {
		t.Run(invalid, func(t *testing.T) {
			handler, service, _ := helmHandlerFixture(t)
			request := helmRequest(http.MethodGet, "values", "")
			switch invalid {
			case "driver":
				request.SetPathValue("driver", "sql")
			case "format":
				request.SetPathValue("format", "secret")
			case "namespace":
				request.SetPathValue("namespace", "outside")
			case "query":
				request.URL.RawQuery = "unknown=true"
			}
			response := httptest.NewRecorder()
			handler.HelmDocument(response, request)
			if response.Code < 400 || service.calls != 0 {
				t.Fatalf("invalid target reached service: %d calls=%d", response.Code, service.calls)
			}
		})
	}
}
func TestHelmHandlerRejectsUnknownMutationFieldsAndPreservesConflict(t *testing.T) {
	handler, service, _ := helmHandlerFixture(t)
	command := `{"expectedGeneration":"gen","expectedRevision":2,"expectedUid":"uid","expectedResourceVersion":"17","confirmed":true,"action":"values","values":"replicas: 3"}`
	response := httptest.NewRecorder()
	handler.HelmApply(response, helmRequest(http.MethodPost, "", strings.TrimSuffix(command, "}")+`,"unsafe":true}`))
	if response.Code != 400 || service.calls != 0 {
		t.Fatalf("unknown field accepted: %d %s", response.Code, response.Body.String())
	}
	service.err = apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, "sensitive", errors.New("private-values"))
	response = httptest.NewRecorder()
	handler.HelmApply(response, helmRequest(http.MethodPost, "", command))
	if response.Code != 409 || strings.Contains(response.Body.String(), "private-values") || strings.Contains(response.Body.String(), "draft was not applied") || service.mutation.ExpectedResourceVersion != "17" {
		t.Fatalf("unsafe conflict response: %d %s", response.Code, response.Body.String())
	}
}
func TestHelmHandlerFencesGenerationChanges(t *testing.T) {
	handler, service, selection := helmHandlerFixture(t)
	service.after = func() { selection.binding.Generation = "next" }
	response := httptest.NewRecorder()
	handler.HelmDetail(response, helmRequest(http.MethodGet, "", ""))
	if response.Code != 409 || !strings.Contains(response.Body.String(), "GENERATION_CHANGED") {
		t.Fatalf("stale release details published: %d %s", response.Code, response.Body.String())
	}
}
