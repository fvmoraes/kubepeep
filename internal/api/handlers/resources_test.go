package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	podOptions  resourcecore.ListOptions
	podCursor   *resourcecore.CompositeCursor[resourcecore.PodDTO]
	nodeOptions resourcecore.ListOptions
	calls       int
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

func (stub *resourceServiceStub) ListNodes(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, options resourcecore.ListOptions, _ *resourcecore.CompositeCursor[resourcecore.NodeDTO]) (resourcecore.ListResult[resourcecore.NodeDTO], error) {
	return stub.listNodes(options)
}

func (stub *resourceServiceStub) GetNode(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string) (resourcecore.NodeDetailDTO, error) {
	return resourcecore.NodeDetailDTO{}, errors.New("node reader is unavailable in this stub")
}

func (stub *resourceServiceStub) ListPods(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, options resourcecore.ListOptions, cursor *resourcecore.CompositeCursor[resourcecore.PodDTO]) (resourcecore.ListResult[resourcecore.PodDTO], error) {
	stub.calls++
	stub.podOptions = options
	stub.podCursor = cursor
	origin := resourcecore.Origin{Namespace: "default", Version: "v1", Resource: "pods"}
	result := resourcecore.ListResult[resourcecore.PodDTO]{Items: []resourcecore.PodDTO{{Namespace: "default", Name: "api", Status: "Running"}}, Page: resourcecore.PageDTO{Limit: options.Limit, Truncated: true, FilterScope: resourcecore.FilterScopePage}, Coverage: resourcecore.CoverageDTO{RequestedNamespaces: 1, CompletedNamespaces: 1, DeniedNamespaces: []string{}, Failed: []resourcecore.PartialErrorDTO{}}, CollectedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	if cursor != nil {
		// Continue the stored window so a second page carries the buffered
		// remainder forward.
		next := *cursor
		next.Origins[0].Continue = "native-next"
		result.Cursor = &next
		return result, nil
	}
	fresh := resourcecore.NewCompositeCursor[resourcecore.PodDTO]([]resourcecore.Origin{origin})
	fresh.Origins[0].Buffered = []resourcecore.PodDTO{{Namespace: "default", Name: "buffered-1"}}
	result.Cursor = &fresh
	return result, nil
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

func (stub *resourceServiceStub) listNodes(options resourcecore.ListOptions) (resourcecore.ListResult[resourcecore.NodeDTO], error) {
	stub.calls++
	stub.nodeOptions = options
	origin := resourcecore.Origin{Version: "v1", Resource: "nodes"}
	cursor := resourcecore.NewCompositeCursor[resourcecore.NodeDTO]([]resourcecore.Origin{origin})
	return resourcecore.ListResult[resourcecore.NodeDTO]{Items: []resourcecore.NodeDTO{{Name: "worker-1", Status: "Ready", Ready: true}}, Cursor: &cursor, Page: resourcecore.PageDTO{Limit: options.Limit, FilterScope: resourcecore.FilterScopePage}, Coverage: resourcecore.CoverageDTO{RequestedNamespaces: 0, CompletedNamespaces: 0, DeniedNamespaces: []string{}, Failed: []resourcecore.PartialErrorDTO{}}, CollectedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
}

func TestNodeListWorksWithoutNamespaceScope(t *testing.T) {
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	service := &resourceServiceStub{}
	// No scope resolution namespaces: cluster-scoped reads must still pass.
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeSource: "none"}}
	handler := NewResources(service, nil, selection, codec)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?limit=25", nil)
	response := httptest.NewRecorder()
	handler.Nodes(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.nodeOptions.Limit != 25 {
		t.Fatalf("options=%#v", service.nodeOptions)
	}
	if strings.Contains(response.Body.String(), `"requestedNamespaces":1`) {
		t.Fatalf("cluster list must not report namespace fan-out: %s", response.Body.String())
	}
}

func TestNodeListRejectsNamespaceFilterAndMissingContext(t *testing.T) {
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	service := &resourceServiceStub{}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeSource: "none"}}
	handler := NewResources(service, nil, selection, codec)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?namespace=default", nil)
	response := httptest.NewRecorder()
	handler.Nodes(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("namespace filter status=%d body=%s", response.Code, response.Body.String())
	}
	if service.calls != 0 {
		t.Fatalf("service called=%d", service.calls)
	}
	empty := &resourceSelectionStub{binding: namespaces.SelectionBinding{}, resolution: namespaces.ScopeResolution{}}
	handler = NewResources(service, nil, empty, codec)
	noContext := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	noContextResponse := httptest.NewRecorder()
	handler.Nodes(noContextResponse, noContext)
	if noContextResponse.Code != http.StatusConflict {
		t.Fatalf("no-context status=%d body=%s", noContextResponse.Code, noContextResponse.Body.String())
	}
}

func TestNodeDetailAndYAMLRequireOnlyContext(t *testing.T) {
	service := &resourceServiceStub{}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeSource: "none"}}
	handler := NewResources(service, nil, selection, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/worker-1", nil)
	request.SetPathValue("name", "worker-1")
	response := httptest.NewRecorder()
	handler.NodeDetail(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("detail without service reader status=%d", response.Code)
	}
	invalid := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/Bad_Name", nil)
	invalid.SetPathValue("name", "Bad_Name")
	invalidResponse := httptest.NewRecorder()
	handler.NodeDetail(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid name status=%d", invalidResponse.Code)
	}
	if allow, known := allowedMethods("/api/v1/nodes/worker-1/yaml"); !known || allow != "GET, HEAD" {
		t.Fatalf("nodes yaml allow=%q known=%v", allow, known)
	}
	if allow, known := allowedMethods("/api/v1/nodes"); !known || allow != "GET, HEAD" {
		t.Fatalf("nodes allow=%q known=%v", allow, known)
	}
}

func resourceListHandler(t *testing.T, service ResourceService, store *api.CursorStore, codec *api.CursorCodec) *Resources {
	t.Helper()
	if codec == nil {
		var err error
		codec, err = api.NewCursorCodec()
		if err != nil {
			t.Fatal(err)
		}
	}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "scope", ScopeSource: "saved", Namespaces: []string{"default"}}}
	return NewResources(service, nil, selection, codec).WithCursorStore(store)
}

const (
	wideCatalogOrigins           = 40
	wideCatalogBufferedPerOrigin = 30
)

// wideCatalogServiceStub returns the cursor state a wide Logs-catalog window
// produces: one origin per namespace, each holding buffered DTOs plus a native
// continuation.
type wideCatalogServiceStub struct {
	ResourceService
	continuations     int
	continuationState *resourcecore.CompositeCursor[resourcecore.PodDTO]
}

func (stub *wideCatalogServiceStub) ListPods(_ context.Context, _ namespaces.SelectionBinding, _ namespaces.ScopeResolution, options resourcecore.ListOptions, cursor *resourcecore.CompositeCursor[resourcecore.PodDTO]) (resourcecore.ListResult[resourcecore.PodDTO], error) {
	if cursor != nil {
		stub.continuations++
		stub.continuationState = cursor
		return resourcecore.ListResult[resourcecore.PodDTO]{Items: []resourcecore.PodDTO{}, Page: resourcecore.PageDTO{Limit: options.Limit, FilterScope: resourcecore.FilterScopePage}, Coverage: resourcecore.CoverageDTO{RequestedNamespaces: wideCatalogOrigins, CompletedNamespaces: wideCatalogOrigins, DeniedNamespaces: []string{}, Failed: []resourcecore.PartialErrorDTO{}}, CollectedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
	}
	origins := make([]resourcecore.Origin, wideCatalogOrigins)
	for index := range origins {
		origins[index] = resourcecore.Origin{Namespace: fmt.Sprintf("ns-%02d", index), Version: "v1", Resource: "pods"}
	}
	state := resourcecore.NewCompositeCursor[resourcecore.PodDTO](origins)
	for index := range state.Origins {
		buffered := make([]resourcecore.PodDTO, wideCatalogBufferedPerOrigin)
		for item := range buffered {
			buffered[item] = resourcecore.PodDTO{Namespace: fmt.Sprintf("ns-%02d", index), Name: fmt.Sprintf("pod-%03d", item), Status: "Running"}
		}
		state.Origins[index].Buffered = buffered
		state.Origins[index].Continue = fmt.Sprintf("native-%02d", index)
	}
	return resourcecore.ListResult[resourcecore.PodDTO]{Items: []resourcecore.PodDTO{{Namespace: "ns-00", Name: "pod-000", Status: "Running"}}, Cursor: &state, Page: resourcecore.PageDTO{Limit: options.Limit, Truncated: true, FilterScope: resourcecore.FilterScopePage}, Coverage: resourcecore.CoverageDTO{RequestedNamespaces: wideCatalogOrigins, CompletedNamespaces: wideCatalogOrigins, DeniedNamespaces: []string{}, Failed: []resourcecore.PartialErrorDTO{}}, CollectedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
}

func podsRequest(cursor string) *http.Request {
	path := "/api/v1/pods?limit=25&status=Running&sort=name"
	if cursor != "" {
		path += "&continue=" + cursor
	}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	return request.WithContext(api.WithRequestID(request.Context(), "req_test"))
}

func requestNextToken(response *httptest.ResponseRecorder) string {
	var envelope struct {
		Meta struct {
			Page resourcecore.PageDTO `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		return ""
	}
	return envelope.Meta.Page.Next
}

func TestResourceListCursorStoreKeepsStateServerSide(t *testing.T) {
	store := api.NewCursorStore(nil)
	service := &resourceServiceStub{}
	handler := resourceListHandler(t, service, store, nil)

	first := httptest.NewRecorder()
	handler.Pods(first, podsRequest(""))
	if first.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", first.Code, first.Body.String())
	}
	token := requestNextToken(first)
	if token == "" {
		t.Fatal("first page did not return a cursor")
	}
	if strings.Contains(token, "buffered") || strings.Contains(token, "origins") || len(token) > 512 {
		t.Fatalf("token still carries state (%d bytes): %.80s", len(token), token)
	}
	if store.Len() != 1 {
		t.Fatalf("store entries=%d, want 1", store.Len())
	}

	second := httptest.NewRecorder()
	handler.Pods(second, podsRequest(token))
	if second.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", second.Code, second.Body.String())
	}
	if service.calls != 2 {
		t.Fatalf("service calls=%d, want 2", service.calls)
	}
	if service.podCursor == nil || len(service.podCursor.Origins) != 1 || len(service.podCursor.Origins[0].Buffered) != 1 {
		t.Fatalf("stored buffered state did not reach the service: %#v", service.podCursor)
	}
}

func TestResourceListCursorStoreMissingReferenceIsExpired(t *testing.T) {
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	store := api.NewCursorStore(nil)
	service := &resourceServiceStub{}
	handler := resourceListHandler(t, service, store, codec)

	first := httptest.NewRecorder()
	handler.Pods(first, podsRequest(""))
	token := requestNextToken(first)
	if token == "" {
		t.Fatal("first page did not return a cursor")
	}

	// Simulate a restart/purge: the store lost the state while the signed
	// token is still within its TTL window.
	replacement := resourceListHandler(t, &resourceServiceStub{}, api.NewCursorStore(nil), codec)
	recovery := httptest.NewRecorder()
	replacement.Pods(recovery, podsRequest(token))
	if recovery.Code != http.StatusGone || !strings.Contains(recovery.Body.String(), api.CodeCursorExpired) {
		t.Fatalf("missing reference status=%d body=%s", recovery.Code, recovery.Body.String())
	}
	if service.calls != 1 {
		t.Fatalf("expired cursor reached the service: calls=%d", service.calls)
	}
}

// The Logs page opens by paginating the Pods collection (limit=500) across
// every scoped namespace. A wide fan-out produces cursor state with thousands
// of buffered DTOs; the token must stay a small reference and the stored state
// must survive a continuation roundtrip intact. Under the inline-state cursor
// this scenario exceeded the token size limit and failed the catalog load.
func TestResourceListCursorStoreServesWideFanoutCatalogState(t *testing.T) {
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	service := &wideCatalogServiceStub{}
	handler := resourceListHandler(t, service, api.NewCursorStore(nil), codec)

	first := httptest.NewRecorder()
	handler.Pods(first, podsRequest(""))
	if first.Code != http.StatusOK {
		t.Fatalf("catalog page status=%d body=%s", first.Code, first.Body.String())
	}
	token := requestNextToken(first)
	if token == "" || len(token) > 512 || strings.Contains(token, "pod-") {
		t.Fatalf("catalog token is not a small reference (%d bytes): %.80s", len(token), token)
	}

	second := httptest.NewRecorder()
	handler.Pods(second, podsRequest(token))
	if second.Code != http.StatusOK {
		t.Fatalf("continuation status=%d body=%s", second.Code, second.Body.String())
	}
	if service.continuations != 1 {
		t.Fatalf("continuations=%d", service.continuations)
	}
	if service.continuationState == nil || len(service.continuationState.Origins) != wideCatalogOrigins {
		t.Fatalf("stored state lost origins: %#v", service.continuationState)
	}
	for _, origin := range service.continuationState.Origins {
		if len(origin.Buffered) != wideCatalogBufferedPerOrigin || origin.Continue == "" {
			t.Fatalf("stored state lost buffered items for %s: %d buffered, continue=%q", origin.Origin.Namespace, len(origin.Buffered), origin.Continue)
		}
	}
}

func TestResourceListCursorStoreStillAcceptsLegacyInlineTokens(t *testing.T) {
	codec, err := api.NewCursorCodec()
	if err != nil {
		t.Fatal(err)
	}
	// A handler without a store produces the pre-store inline-state token.
	legacyService := &resourceServiceStub{}
	legacyHandler := resourceListHandler(t, legacyService, nil, codec)
	first := httptest.NewRecorder()
	legacyHandler.Pods(first, podsRequest(""))
	token := requestNextToken(first)
	if token == "" || len(token) < 256 {
		t.Fatalf("expected a legacy inline-state token, got %.80s (%d bytes)", token, len(token))
	}

	// The store-enabled handler must still serve it until it expires.
	store := api.NewCursorStore(nil)
	service := &resourceServiceStub{}
	handler := resourceListHandler(t, service, store, codec)
	response := httptest.NewRecorder()
	handler.Pods(response, podsRequest(token))
	if response.Code != http.StatusOK {
		t.Fatalf("legacy token status=%d body=%s", response.Code, response.Body.String())
	}
	if service.podCursor == nil || len(service.podCursor.Origins[0].Buffered) != 1 {
		t.Fatalf("legacy inline state was not decoded: %#v", service.podCursor)
	}
}
