package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
)

type resourceSelectionStub struct {
	binding    namespaces.SelectionBinding
	resolution namespaces.ScopeResolution
}

func (stub *resourceSelectionStub) Snapshot() (namespaces.SelectionBinding, namespaces.ScopeResolution) {
	return stub.binding, stub.resolution
}
func (stub *resourceSelectionStub) IfCurrent(binding namespaces.SelectionBinding, write func()) bool {
	if !sameSelectionBinding(binding, stub.binding) {
		return false
	}
	write()
	return true
}

type resourceServiceStub struct {
	ResourceService
	podOptions resourcecore.ListOptions
	calls      int
}

type resourceStreamServiceStub struct {
	ResourceStreamService
	authorized int
	followed   int
}

func (stub *resourceStreamServiceStub) AuthorizeTopics(_ context.Context, _ namespaces.SelectionBinding, resolution namespaces.ScopeResolution, _ []resourcecore.Topic) (namespaces.ScopeResolution, error) {
	return resolution, nil
}
func (stub *resourceStreamServiceStub) ReauthorizeTopics(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, []resourcecore.Topic) error {
	return nil
}

func (stub *resourceStreamServiceStub) AuthorizeLogs(context.Context, namespaces.SelectionBinding, string, string) error {
	stub.authorized++
	return nil
}
func (stub *resourceStreamServiceStub) ReauthorizeLogs(context.Context, namespaces.SelectionBinding, string, string) error {
	return nil
}
func (stub *resourceStreamServiceStub) FollowLogs(_ context.Context, binding namespaces.SelectionBinding, _ namespaces.ScopeResolution, _ string, _ string, _ resourcecore.LogQuery, emit func(resourcecore.LogLineDTO) error) (resourcecore.FollowTerminal, error) {
	stub.followed++
	if err := emit(resourcecore.LogLineDTO{Text: "hello"}); err != nil {
		return resourcecore.FollowTerminal{}, err
	}
	return resourcecore.FollowTerminal{Reason: "upstream_eof", Generation: binding.Generation}, nil
}

func (stub *resourceServiceStub) ListPods(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, options resourcecore.ListOptions, _ *resourcecore.CompositeCursor[resourcecore.PodDTO]) (resourcecore.ListResult[resourcecore.PodDTO], error) {
	stub.calls++
	stub.podOptions = options
	origin := resourcecore.Origin{Namespace: "default", Version: "v1", Resource: "pods"}
	cursor := resourcecore.NewCompositeCursor[resourcecore.PodDTO]([]resourcecore.Origin{origin})
	cursor.Origins[0].Continue = "native-next"
	return resourcecore.ListResult[resourcecore.PodDTO]{Items: []resourcecore.PodDTO{{Namespace: "default", Name: "api", Status: "Running"}}, Cursor: &cursor, Page: resourcecore.PageDTO{Limit: options.Limit, Truncated: true, FilterScope: resourcecore.FilterScopePage}, Coverage: resourcecore.CoverageDTO{RequestedNamespaces: 1, CompletedNamespaces: 1, DeniedNamespaces: []string{}, Failed: []resourcecore.PartialErrorDTO{}}, CollectedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
}

func TestResourceListEnvelopeCursorBindingAndNoStore(t *testing.T) {
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	service := &resourceServiceStub{}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "scope", ScopeSource: "saved", Namespaces: []string{"default"}}}
	handler := NewResources(service, nil, selection, codec)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/pods?limit=25&status=Running&sort=name", nil)
	request = request.WithContext(api.WithRequestID(request.Context(), "req_test"))
	response := httptest.NewRecorder()
	handler.Pods(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control=%q", response.Header().Get("Cache-Control"))
	}
	if service.calls != 1 || service.podOptions.Limit != 25 {
		t.Fatalf("service options=%#v calls=%d", service.podOptions, service.calls)
	}
	var envelope struct {
		Data []resourcecore.PodDTO `json:"data"`
		Meta struct {
			RequestID  string               `json:"requestId"`
			Generation string               `json:"generation"`
			Page       resourcecore.PageDTO `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Meta.RequestID != "req_test" || envelope.Meta.Generation != "gen" || envelope.Meta.Page.Next == "" {
		t.Fatalf("bad envelope: %#v", envelope)
	}
	mismatch := httptest.NewRequest(http.MethodGet, "/api/v1/pods?limit=25&status=Failed&sort=name&continue="+envelope.Meta.Page.Next, nil)
	mismatchResponse := httptest.NewRecorder()
	handler.Pods(mismatchResponse, mismatch)
	if mismatchResponse.Code != http.StatusBadRequest {
		t.Fatalf("cursor mismatch status=%d body=%s", mismatchResponse.Code, mismatchResponse.Body.String())
	}
	if service.calls != 1 {
		t.Fatalf("mismatched cursor reached service: calls=%d", service.calls)
	}
}

func TestResourceQueryGrammarIsClosedAndNormalized(t *testing.T) {
	tests := []struct {
		query      string
		collection resourcecore.Collection
		valid      bool
	}{{"namespace=a&namespace=b&status=Running&status=Failed", resourcecore.CollectionPods, true}, {"kind=deployments&kind=jobs", resourcecore.CollectionWorkloads, true}, {"limit=01", resourcecore.CollectionPods, true}, {"search=", resourcecore.CollectionPods, false}, {"sort=name&sort=age", resourcecore.CollectionPods, false}, {"unknown=x", resourcecore.CollectionPods, false}, {"problematic=1", resourcecore.CollectionPods, false}, {"addressType=IPv4", resourcecore.CollectionServices, false}}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/resources?"+test.query, nil)
			_, err := decodeResourceListQuery(request, test.collection)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}

func TestStreamTopicAndEventIDGrammar(t *testing.T) {
	topics, err := decodeTopics("topic=events&topic=pods")
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 2 || topics[0] != resourcecore.TopicPods || topics[1] != resourcecore.TopicEvents {
		t.Fatalf("canonical topics=%v", topics)
	}
	for _, raw := range []string{"", "topic=pods&topic=pods", "topic=secrets", "topic=pods&extra=x"} {
		if _, err := decodeTopics(raw); err == nil {
			t.Fatalf("accepted topics %q", raw)
		}
	}
	if !validStreamEventID("kpse1.MDEyMzQ1Njc4OWFiY2RlZg.1") {
		t.Fatal("valid event id rejected")
	}
	for _, value := range []string{"kpse1.bad.1", "kpse1.MDEyMzQ1Njc4OWFiY2RlZg.0", "other.MDEyMzQ1Njc4OWFiY2RlZg.1"} {
		if validStreamEventID(value) {
			t.Fatalf("invalid event id accepted: %s", value)
		}
	}
}

func TestLogFollowRejectsResumeBeforeOpeningStream(t *testing.T) {
	handler := NewResourceStreams(nil, nil, nil, "http://127.0.0.1:2748")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/pods/default/api/logs/stream?container=api", nil)
	request.Header.Set("Last-Event-ID", "kpse1.any.1")
	response := httptest.NewRecorder()
	handler.LogFollow(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), api.CodeValidationFailed) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLogFollowEmitsMetaLineEndWithExactCSRFAndNoStore(t *testing.T) {
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
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}}
	handler := NewResourceStreams(service, selection, sessions, origin)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/pods/default/api/logs/stream?container=api", nil)
	request.SetPathValue("namespace", "default")
	request.SetPathValue("name", "api")
	request.Header.Set("Origin", origin)
	request.Header.Set("X-KubePeep-CSRF", session.CSRFToken)
	request = request.WithContext(api.WithRequestID(request.Context(), "req_stream"))
	response := httptest.NewRecorder()
	handler.LogFollow(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control=%q", response.Header().Get("Cache-Control"))
	}
	body := response.Body.String()
	for _, marker := range []string{"event: meta", "event: line", "event: end", "\"requestId\":\"req_stream\"", "\"text\":\"hello\""} {
		if !strings.Contains(body, marker) {
			t.Fatalf("missing %q in %s", marker, body)
		}
	}
	if strings.Contains(body, "id:") {
		t.Fatalf("log follow emitted resumable id: %s", body)
	}
	if service.authorized != 1 || service.followed != 1 {
		t.Fatalf("calls authorize=%d follow=%d", service.authorized, service.followed)
	}
	bad := httptest.NewRequest(http.MethodGet, "/api/v1/pods/default/api/logs/stream?container=api", nil)
	bad.SetPathValue("namespace", "default")
	bad.SetPathValue("name", "api")
	bad.Header.Set("Origin", origin)
	bad.Header.Set("X-KubePeep-CSRF", "wrong")
	badResponse := httptest.NewRecorder()
	handler.LogFollow(badResponse, bad)
	if badResponse.Code != http.StatusForbidden {
		t.Fatalf("bad CSRF status=%d body=%s", badResponse.Code, badResponse.Body.String())
	}
}

func TestResourceStreamValidUnavailableResumeUsesTerminalReset(t *testing.T) {
	origin := "http://127.0.0.1:2748"
	sessions, err := api.NewSessionStore(0)
	if err != nil {
		t.Fatal(err)
	}
	session, err := sessions.Current(origin, "gen")
	if err != nil {
		t.Fatal(err)
	}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{"default"}}}
	handler := NewResourceStreams(&resourceStreamServiceStub{}, selection, sessions, origin)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/stream?topic=pods", nil)
	request.Header.Set("Origin", origin)
	request.Header.Set("X-KubePeep-CSRF", session.CSRFToken)
	request.Header.Set("Last-Event-ID", "kpse1.MDEyMzQ1Njc4OWFiY2RlZg.1")
	response := httptest.NewRecorder()
	handler.Resources(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: reset") || !strings.Contains(response.Body.String(), "resume_unavailable") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control=%q", response.Header().Get("Cache-Control"))
	}
}

func TestResourceAllowedMethodsMergeReadAndDelete(t *testing.T) {
	allow, known := allowedMethods("/api/v1/pods/default/api")
	if !known || allow != "DELETE, GET, HEAD" {
		t.Fatalf("allow=%q known=%v", allow, known)
	}
	if allow, known = allowedMethods("/api/v1/secrets/default/name/yaml"); known || allow != "" {
		t.Fatalf("secret YAML unexpectedly reserved: allow=%q known=%v", allow, known)
	}
}
