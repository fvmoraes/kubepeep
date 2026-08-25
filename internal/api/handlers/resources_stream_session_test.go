package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/lifecycle"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
)

type handlerWatchStream struct {
	changes chan resourcecore.WatchChange
	once    sync.Once
}

func (stream *handlerWatchStream) ResultChan() <-chan resourcecore.WatchChange { return stream.changes }
func (stream *handlerWatchStream) Stop()                                       { stream.once.Do(func() {}) }

type handlerWatchPort struct {
	mu                sync.Mutex
	streams           map[string]*handlerWatchStream
	created           chan string
	itemsPerNamespace int
}

func newHandlerWatchPort() *handlerWatchPort {
	return &handlerWatchPort{streams: map[string]*handlerWatchStream{}, created: make(chan string, 16)}
}

func (port *handlerWatchPort) List(_ context.Context, key resourcecore.WatchKey) (resourcecore.WatchSnapshot, error) {
	count := port.itemsPerNamespace
	if count <= 0 {
		count = 1
	}
	items := make([]resourcecore.TopicObject, 0, count)
	for index := 0; index < count; index++ {
		namespace := key.Namespace
		if namespace == "" {
			namespace = "cluster-result"
		}
		name := "pod-" + key.Namespace
		if count > 1 {
			name += "-" + strconv.Itoa(index) + "-" + strings.Repeat("x", 1000)
		}
		items = append(items, resourcecore.PodDTO{Namespace: namespace, Name: name, Status: "Running"})
	}
	return resourcecore.WatchSnapshot{ResourceVersion: "rv-" + key.Namespace, Items: items}, nil
}

func (port *handlerWatchPort) Watch(_ context.Context, key resourcecore.WatchKey, _ string, _ int64, _ bool) (resourcecore.WatchStream, error) {
	stream := &handlerWatchStream{changes: make(chan resourcecore.WatchChange, 16)}
	port.mu.Lock()
	port.streams[key.Namespace] = stream
	port.mu.Unlock()
	select {
	case port.created <- key.Namespace:
	default:
	}
	return stream, nil
}

func (port *handlerWatchPort) stream(namespace string) *handlerWatchStream {
	port.mu.Lock()
	defer port.mu.Unlock()
	return port.streams[namespace]
}

type liveResourceStreamService struct {
	manager          *resourcecore.WatchManager
	topicReauthErr   error
	logReauthErr     error
	topicRevalidates atomic.Int32
	logRevalidates   atomic.Int32
	logCanceled      chan struct{}
	authorizedScope  *namespaces.ScopeResolution
}

func (service *liveResourceStreamService) AuthorizeTopics(_ context.Context, _ namespaces.SelectionBinding, resolution namespaces.ScopeResolution, _ []resourcecore.Topic) (namespaces.ScopeResolution, error) {
	if service.authorizedScope != nil {
		return *service.authorizedScope, nil
	}
	return resolution, nil
}
func (service *liveResourceStreamService) ReauthorizeTopics(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, []resourcecore.Topic) error {
	service.topicRevalidates.Add(1)
	return service.topicReauthErr
}
func (service *liveResourceStreamService) Subscribe(ctx context.Context, binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, topic resourcecore.Topic, gvr schema.GroupVersionResource, namespace string) (*resourcecore.Subscription, error) {
	return service.manager.Subscribe(ctx, resourcecore.WatchKey{Generation: binding.Generation, Context: binding.Context, Scope: resolution.ScopeName, Topic: topic, GVR: gvr, Namespace: namespace})
}
func (service *liveResourceStreamService) AuthorizeLogs(context.Context, namespaces.SelectionBinding, string, string) error {
	return nil
}
func (service *liveResourceStreamService) ReauthorizeLogs(context.Context, namespaces.SelectionBinding, string, string) error {
	service.logRevalidates.Add(1)
	return service.logReauthErr
}
func (service *liveResourceStreamService) FollowLogs(ctx context.Context, binding namespaces.SelectionBinding, _ namespaces.ScopeResolution, _, _ string, _ resourcecore.LogQuery, _ func(resourcecore.LogLineDTO) error) (resourcecore.FollowTerminal, error) {
	<-ctx.Done()
	if service.logCanceled != nil {
		close(service.logCanceled)
	}
	reason := "generation_changed"
	if errors.Is(context.Cause(ctx), lifecycle.ErrServerShutdown) {
		reason = "server_shutdown"
	}
	return resourcecore.FollowTerminal{Reason: reason, Generation: binding.Generation}, nil
}

type liveResponseRecorder struct {
	mu     sync.Mutex
	header http.Header
	status int
	body   bytes.Buffer
	notify chan struct{}
}

func newLiveResponseRecorder() *liveResponseRecorder {
	return &liveResponseRecorder{header: make(http.Header), notify: make(chan struct{}, 1)}
}
func (recorder *liveResponseRecorder) Header() http.Header { return recorder.header }
func (recorder *liveResponseRecorder) WriteHeader(status int) {
	recorder.mu.Lock()
	if recorder.status == 0 {
		recorder.status = status
	}
	recorder.mu.Unlock()
}
func (recorder *liveResponseRecorder) Write(value []byte) (int, error) {
	recorder.mu.Lock()
	if recorder.status == 0 {
		recorder.status = http.StatusOK
	}
	written, err := recorder.body.Write(value)
	recorder.mu.Unlock()
	select {
	case recorder.notify <- struct{}{}:
	default:
	}
	return written, err
}
func (recorder *liveResponseRecorder) Flush() {}
func (recorder *liveResponseRecorder) String() string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.body.String()
}
func (recorder *liveResponseRecorder) waitContains(t *testing.T, markers ...string) string {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		body := recorder.String()
		matched := true
		for _, marker := range markers {
			matched = matched && strings.Contains(body, marker)
		}
		if matched {
			return body
		}
		select {
		case <-recorder.notify:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %v in %s", markers, body)
		}
	}
}

func streamHandlerFixture(t *testing.T, service ResourceStreamService, namespacesList []string) (*ResourceStreams, *api.SessionStore, string, string) {
	t.Helper()
	origin := "http://127.0.0.1:2748"
	sessions, err := api.NewSessionStore(0)
	if err != nil {
		t.Fatal(err)
	}
	csrf, err := sessions.Current(origin, "gen")
	if err != nil {
		t.Fatal(err)
	}
	selection := &resourceSelectionStub{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "ctx", Generation: "gen"}, resolution: namespaces.ScopeResolution{ScopeName: "scope", Namespaces: namespacesList}}
	handler := NewResourceStreams(service, selection, sessions, origin)
	handler.reauthorizeEvery = time.Hour
	handler.replayRetention = 2 * time.Second
	return handler, sessions, origin, csrf.CSRFToken
}

func newAuthorizedStreamRequest(ctx context.Context, origin, csrf, lastID string) *http.Request {
	request := httptestNewRequestWithContext(ctx, http.MethodGet, "/api/v1/stream?topic=pods")
	request.Header.Set("Origin", origin)
	request.Header.Set("X-KubePeep-CSRF", csrf)
	if lastID != "" {
		request.Header.Set("Last-Event-ID", lastID)
	}
	return request
}

// Keep request construction isolated so all tests exercise request.Context
// cancellation without depending on a real TCP listener.
func httptestNewRequestWithContext(ctx context.Context, method, target string) *http.Request {
	request, _ := http.NewRequestWithContext(ctx, method, target, nil)
	return request
}

func TestResourceStreamPublishesOneTransactionalSnapshotPerTopic(t *testing.T) {
	port := newHandlerWatchPort()
	manager := resourcecore.NewWatchManager(port)
	defer manager.Close()
	service := &liveResourceStreamService{manager: manager}
	handler, _, origin, csrf := streamHandlerFixture(t, service, []string{"alpha", "beta"})
	ctx, cancel := context.WithCancel(context.Background())
	request := newAuthorizedStreamRequest(ctx, origin, csrf, "")
	recorder := newLiveResponseRecorder()
	done := make(chan struct{})
	go func() { handler.Resources(recorder, request); close(done) }()
	body := recorder.waitContains(t, "event: snapshot", `"namespace":"alpha"`, `"namespace":"beta"`, `"final":true`)
	if count := strings.Count(body, "event: snapshot"); count != 1 {
		t.Fatalf("snapshot was published per origin instead of per topic: count=%d body=%s", count, body)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after request cancellation")
	}
}

func TestResourceStreamUsesEffectiveGlobalScopeReturnedByAuthorization(t *testing.T) {
	port := newHandlerWatchPort()
	manager := resourcecore.NewWatchManager(port)
	defer manager.Close()
	effective := namespaces.ScopeResolution{ScopeName: "scope", Namespaces: []string{""}, PreferGlobal: true}
	service := &liveResourceStreamService{manager: manager, authorizedScope: &effective}
	handler, _, origin, csrf := streamHandlerFixture(t, service, []string{"alpha", "beta"})
	ctx, cancel := context.WithCancel(context.Background())
	request := newAuthorizedStreamRequest(ctx, origin, csrf, "")
	recorder := newLiveResponseRecorder()
	done := make(chan struct{})
	go func() { handler.Resources(recorder, request); close(done) }()
	recorder.waitContains(t, "event: snapshot", `"namespace":"cluster-result"`)
	select {
	case namespace := <-port.created:
		if namespace != "" {
			t.Fatalf("watch namespace=%q, want cluster-wide", namespace)
		}
	default:
		t.Fatal("global watch was not created")
	}
	if manager.SharedWatchCount() != 1 || len(port.created) != 0 {
		t.Fatalf("shared=%d queued watch creations=%d", manager.SharedWatchCount(), len(port.created))
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("global resource stream did not close")
	}
}

func TestResourceStreamSnapshotChunksShareOneTransactionIDAndFinalOnlyLast(t *testing.T) {
	port := newHandlerWatchPort()
	port.itemsPerNamespace = 100
	manager := resourcecore.NewWatchManager(port)
	defer manager.Close()
	service := &liveResourceStreamService{manager: manager}
	handler, _, origin, csrf := streamHandlerFixture(t, service, []string{"alpha", "beta"})
	ctx, cancel := context.WithCancel(context.Background())
	recorder := newLiveResponseRecorder()
	done := make(chan struct{})
	go func() { handler.Resources(recorder, newAuthorizedStreamRequest(ctx, origin, csrf, "")); close(done) }()
	body := recorder.waitContains(t, `"final":true`)
	ids := regexp.MustCompile(`"snapshotId":"([^"]+)"`).FindAllStringSubmatch(body, -1)
	if len(ids) < 2 {
		t.Fatalf("expected chunked transactional snapshot: %s", body)
	}
	for _, id := range ids[1:] {
		if id[1] != ids[0][1] {
			t.Fatalf("snapshot chunks used different transaction IDs: %#v", ids)
		}
	}
	if strings.Count(body, `"final":true`) != 1 || !strings.Contains(body[strings.LastIndex(body, "event: snapshot"):], `"final":true`) {
		t.Fatalf("final was not exclusive to the last chunk: %s", body)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("chunked stream did not stop")
	}
}

func TestResourceStreamReplaysMissedEventsWithinLiveRingWithoutSnapshot(t *testing.T) {
	port := newHandlerWatchPort()
	manager := resourcecore.NewWatchManager(port)
	defer manager.Close()
	service := &liveResourceStreamService{manager: manager}
	handler, _, origin, csrf := streamHandlerFixture(t, service, []string{"default"})
	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstRequest := newAuthorizedStreamRequest(firstCtx, origin, csrf, "")
	firstRecorder := newLiveResponseRecorder()
	firstDone := make(chan struct{})
	go func() { handler.Resources(firstRecorder, firstRequest); close(firstDone) }()
	firstBody := firstRecorder.waitContains(t, "event: snapshot", `"final":true`)
	idPattern := regexp.MustCompile(`(?m)^id: ([^\r\n]+)`)
	match := idPattern.FindStringSubmatch(firstBody)
	if len(match) != 2 {
		t.Fatalf("snapshot ID missing: %s", firstBody)
	}
	lastID := match[1]
	firstCancel()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first stream did not disconnect")
	}
	deadline := time.Now().Add(time.Second)
	for port.stream("default") == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	watch := port.stream("default")
	if watch == nil {
		t.Fatal("watch was not established")
	}
	watch.changes <- resourcecore.WatchChange{Type: "MODIFIED", ResourceVersion: "rv-2", Object: resourcecore.PodDTO{Namespace: "default", Name: "pod-default", Status: "Running"}}
	session := handler.resumableSession(lastID)
	if session == nil {
		t.Fatal("live replay session disappeared")
	}
	expectedBinding, _ := resourcecore.ReplayBinding(handler.instance, "gen", []resourcecore.Topic{resourcecore.TopicPods})
	replayDeadline := time.Now().Add(time.Second)
	replayReady := false
	replayedID := ""
	for time.Now().Before(replayDeadline) {
		entries, replayErr := session.ring.ReplayEntries(lastID, expectedBinding)
		if replayErr == nil && len(entries) == 1 {
			replayReady = true
			replayedID = entries[0].ID
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !replayReady {
		t.Fatal("missed watch event was not retained in the replay ring")
	}
	secondCtx, secondCancel := context.WithCancel(context.Background())
	secondRequest := newAuthorizedStreamRequest(secondCtx, origin, csrf, lastID)
	secondRecorder := newLiveResponseRecorder()
	secondDone := make(chan struct{})
	go func() { handler.Resources(secondRecorder, secondRequest); close(secondDone) }()
	secondBody := secondRecorder.waitContains(t, "event: modified", `"resourceVersion":"rv-2"`)
	if strings.Contains(secondBody, "event: snapshot") || strings.Contains(secondBody, "resume_unavailable") {
		t.Fatalf("resume unexpectedly snapshotted/reset: %s", secondBody)
	}
	if !strings.Contains(secondBody, "id: "+replayedID) {
		t.Fatalf("resume changed the opaque event ID: want=%s body=%s", replayedID, secondBody)
	}
	if service.topicRevalidates.Load() == 0 {
		t.Fatal("resume did not force fresh topic authorization")
	}
	secondCancel()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("resumed stream did not disconnect")
	}
	session.expire()
}

func TestResourceStreamPeriodicReauthorizationTerminatesAfterRevocation(t *testing.T) {
	port := newHandlerWatchPort()
	manager := resourcecore.NewWatchManager(port)
	defer manager.Close()
	service := &liveResourceStreamService{manager: manager, topicReauthErr: &resourcecore.DomainError{Code: resourcecore.CodeForbidden, Message: "Access was revoked."}}
	handler, _, origin, csrf := streamHandlerFixture(t, service, []string{"default"})
	handler.reauthorizeEvery = 5 * time.Millisecond
	request := newAuthorizedStreamRequest(context.Background(), origin, csrf, "")
	recorder := newLiveResponseRecorder()
	done := make(chan struct{})
	go func() { handler.Resources(recorder, request); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("revoked resource stream did not terminate")
	}
	body := recorder.String()
	if !strings.Contains(body, "event: error") || !strings.Contains(body, string(resourcecore.CodeForbidden)) || strings.Contains(body, "event: reset") {
		t.Fatalf("revocation terminal=%s", body)
	}
	if service.topicRevalidates.Load() == 0 {
		t.Fatal("topic authorization was not revalidated")
	}
}

func TestLogFollowPeriodicReauthorizationCancelsUpstreamAfterRevocation(t *testing.T) {
	port := newHandlerWatchPort()
	manager := resourcecore.NewWatchManager(port)
	defer manager.Close()
	canceled := make(chan struct{})
	service := &liveResourceStreamService{manager: manager, logReauthErr: &resourcecore.DomainError{Code: resourcecore.CodeForbidden, Message: "Log access was revoked."}, logCanceled: canceled}
	handler, _, origin, csrf := streamHandlerFixture(t, service, []string{"default"})
	handler.reauthorizeEvery = 5 * time.Millisecond
	request := httptestNewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/pods/default/api/logs/stream?container=api")
	request.SetPathValue("namespace", "default")
	request.SetPathValue("name", "api")
	request.Header.Set("Origin", origin)
	request.Header.Set("X-KubePeep-CSRF", csrf)
	recorder := newLiveResponseRecorder()
	handler.LogFollow(recorder, request)
	body := recorder.String()
	if !strings.Contains(body, "event: meta") || !strings.Contains(body, "event: error") || !strings.Contains(body, string(resourcecore.CodeForbidden)) {
		t.Fatalf("log revocation terminal=%s", body)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("log upstream was not canceled after revocation")
	}
	if service.logRevalidates.Load() == 0 {
		t.Fatal("log authorization was not revalidated")
	}
}

func TestLogFollowEmitsServerShutdownTerminalOnlyForShutdownCause(t *testing.T) {
	port := newHandlerWatchPort()
	manager := resourcecore.NewWatchManager(port)
	defer manager.Close()
	service := &liveResourceStreamService{manager: manager}
	handler, _, origin, csrf := streamHandlerFixture(t, service, []string{"default"})
	requestContext, cancelRequest := context.WithCancelCause(context.Background())
	request := httptestNewRequestWithContext(requestContext, http.MethodGet, "/api/v1/pods/default/api/logs/stream?container=api")
	request.SetPathValue("namespace", "default")
	request.SetPathValue("name", "api")
	request.Header.Set("Origin", origin)
	request.Header.Set("X-KubePeep-CSRF", csrf)
	recorder := newLiveResponseRecorder()
	done := make(chan struct{})
	go func() { handler.LogFollow(recorder, request); close(done) }()
	recorder.waitContains(t, "event: meta")
	cancelRequest(lifecycle.ErrServerShutdown)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("log follow did not stop during server shutdown")
	}
	body := recorder.String()
	if !strings.Contains(body, "event: end") || !strings.Contains(body, `"reason":"server_shutdown"`) || strings.Contains(body, "event: error") {
		t.Fatalf("shutdown terminal=%s", body)
	}
}

func TestReplayClientQueueIsBoundedAndSlowConsumerGetsTerminalReset(t *testing.T) {
	client := newResourceReplayClient()
	large := resourcecore.StreamEvent{Event: "modified", Generation: "gen", Object: resourcecore.PodDTO{Name: strings.Repeat("x", 60000)}}
	accepted := 0
	for client.push(resourcecore.ReplayEntry{ID: "kpse1.MDEyMzQ1Njc4OWFiY2RlZg.1", Event: large}) {
		accepted++
	}
	if accepted < 1 || accepted >= resourcecore.MaximumStreamQueueEvents {
		t.Fatalf("unexpected byte-bounded queue size %d", accepted)
	}
	client.forceTerminal(resourcecore.StreamEvent{Event: "reset", Generation: "gen", Reason: "slow_consumer"})
	delivery, err := client.next(context.Background())
	if err != nil || !delivery.terminal || delivery.entry.Event.Reason != "slow_consumer" {
		t.Fatalf("delivery=%#v err=%v", delivery, err)
	}
	if _, err = client.next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("terminal queue did not close: %v", err)
	}
}

func TestTransactionalSnapshotAccumulatorEnforcesCombinedByteBudget(t *testing.T) {
	state := &resourceTopicSnapshot{items: []resourcecore.TopicObject{}}
	item := resourcecore.PodDTO{Namespace: "default", Name: strings.Repeat("x", 60000), Status: "Running"}
	accepted := 0
	for state.addItems([]resourcecore.TopicObject{item}) {
		accepted++
	}
	if accepted < 1 || accepted >= resourcecore.MaximumSnapshotItems || state.itemBytes > resourcecore.MaximumSnapshotBytes {
		t.Fatalf("accepted=%d bytes=%d", accepted, state.itemBytes)
	}
}

func TestReplayRegistryKeepsAtMostEightBoundedSessions(t *testing.T) {
	handler := &ResourceStreams{streamSessions: map[string]*resourceStreamSession{}, replayRetention: time.Minute, now: time.Now}
	created := make([]*resourceStreamSession, 0, resourcecore.MaximumStreams+1)
	for index := 0; index <= resourcecore.MaximumStreams; index++ {
		ring, err := resourcecore.NewReplayRing("instance", "gen", []resourcecore.Topic{resourcecore.TopicPods})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		session := &resourceStreamSession{handler: handler, ring: ring, ctx: ctx, cancel: cancel, clients: map[*resourceReplayClient]struct{}{}, createdAt: time.Unix(int64(index), 0)}
		if err = handler.registerStreamSession(session); err != nil {
			t.Fatal(err)
		}
		created = append(created, session)
	}
	if len(handler.streamSessions) != resourcecore.MaximumStreams || !created[0].expired {
		t.Fatalf("sessions=%d oldestExpired=%v", len(handler.streamSessions), created[0].expired)
	}
	for _, session := range created[1:] {
		session.expire()
	}
}
