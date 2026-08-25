package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fvmoraes/ginger/pkg/sse"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/lifecycle"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
)

const streamHeartbeatInterval = 15 * time.Second

type ResourceStreamService interface {
	AuthorizeLogs(context.Context, namespaces.SelectionBinding, string, string) error
	ReauthorizeLogs(context.Context, namespaces.SelectionBinding, string, string) error
	FollowLogs(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, string, string, resourcecore.LogQuery, func(resourcecore.LogLineDTO) error) (resourcecore.FollowTerminal, error)
	AuthorizeTopics(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, []resourcecore.Topic) (namespaces.ScopeResolution, error)
	ReauthorizeTopics(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, []resourcecore.Topic) error
	Subscribe(context.Context, namespaces.SelectionBinding, namespaces.ScopeResolution, resourcecore.Topic, schema.GroupVersionResource, string) (*resourcecore.Subscription, error)
}

type ResourceStreams struct {
	service   ResourceStreamService
	selection SelectionReader
	sessions  *api.SessionStore
	origin    string
	registry  resourcecore.StreamRegistry
	now       func() time.Time

	streamMu         sync.Mutex
	streamSessions   map[string]*resourceStreamSession
	instance         string
	replayRetention  time.Duration
	reauthorizeEvery time.Duration
}

func NewResourceStreams(service ResourceStreamService, selection SelectionReader, sessions *api.SessionStore, origin string) *ResourceStreams {
	return &ResourceStreams{service: service, selection: selection, sessions: sessions, origin: origin, now: time.Now, streamSessions: map[string]*resourceStreamSession{}, instance: randomOpaque("ins_"), replayRetention: defaultResourceReplayRetention, reauthorizeEvery: defaultStreamReauthorizationInterval}
}

func (handler *ResourceStreams) LogFollow(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Last-Event-ID") != "" {
		api.WriteError(w, r, validationHTTPError("Log follow does not support Last-Event-ID.", nil))
		return
	}
	query, err := decodeLogQuery(r, true)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, resolution, err := handler.preflight(w, r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	if err = validateResourcePath(r, resolution); err != nil {
		api.WriteError(w, r, err)
		return
	}
	release, err := handler.registry.Acquire()
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	defer release()
	namespace, pod := r.PathValue("namespace"), r.PathValue("name")
	if err = handler.service.AuthorizeLogs(r.Context(), binding, namespace, pod); err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	stream, err := newResourceSSE(w)
	if err != nil {
		return
	}
	started := handler.now().UTC()
	if err = stream.Send(sse.Event{Type: "meta", Data: map[string]any{"requestId": api.RequestIDFromContext(r.Context()), "generation": binding.Generation, "container": query.Container, "startedAt": started.Format(time.RFC3339Nano)}}); err != nil {
		return
	}
	streamContext, cancelStream := context.WithCancel(r.Context())
	defer cancelStream()
	lines := make(chan resourcecore.LogLineDTO, 15)
	type followResult struct {
		terminal resourcecore.FollowTerminal
		err      error
	}
	done := make(chan followResult, 1)
	go func() {
		terminal, followErr := handler.service.FollowLogs(streamContext, binding, resolution, namespace, pod, query, func(line resourcecore.LogLineDTO) error {
			select {
			case lines <- line:
				return nil
			default:
				return resourcecore.ErrSlowConsumer
			}
		})
		done <- followResult{terminal: terminal, err: followErr}
	}()
	heartbeat := time.NewTicker(streamHeartbeatInterval)
	defer heartbeat.Stop()
	reauthorize := time.NewTicker(handler.reauthorizationInterval())
	defer reauthorize.Stop()
	for {
		select {
		case <-r.Context().Done():
			if errors.Is(context.Cause(r.Context()), lifecycle.ErrServerShutdown) {
				_ = stream.Send(sse.Event{Type: "end", Data: resourcecore.FollowTerminal{Reason: "server_shutdown", Generation: binding.Generation}})
			}
			return
		case line := <-lines:
			if err := stream.Send(sse.Event{Type: "line", Data: line}); err != nil {
				return
			}
		case result := <-done:
			if r.Context().Err() != nil {
				if errors.Is(context.Cause(r.Context()), lifecycle.ErrServerShutdown) {
					_ = stream.Send(sse.Event{Type: "end", Data: resourcecore.FollowTerminal{Reason: "server_shutdown", Generation: binding.Generation}})
				}
				return
			}
			// FollowLogs has returned before publishing done, so no producer can
			// enqueue another line. Drain everything already accepted before the
			// terminal event; select does not preserve ordering across channels.
			for draining := true; draining; {
				select {
				case line := <-lines:
					if err := stream.Send(sse.Event{Type: "line", Data: line}); err != nil {
						return
					}
				default:
					draining = false
				}
			}
			if result.err != nil {
				_ = stream.Send(sse.Event{Type: "error", Data: streamErrorPayload(result.err, binding.Generation, api.RequestIDFromContext(r.Context()))})
				return
			}
			_ = stream.Send(sse.Event{Type: "end", Data: result.terminal})
			return
		case sentAt := <-heartbeat.C:
			if err := stream.Send(sse.Event{Type: "heartbeat", Data: map[string]any{"generation": binding.Generation, "sentAt": sentAt.UTC().Format(time.RFC3339Nano)}}); err != nil {
				return
			}
		case <-reauthorize.C:
			if reauthorizeErr := handler.service.ReauthorizeLogs(streamContext, binding, namespace, pod); reauthorizeErr != nil {
				cancelStream()
				_ = stream.Send(sse.Event{Type: "error", Data: streamErrorPayload(reauthorizeErr, binding.Generation, api.RequestIDFromContext(r.Context()))})
				return
			}
		}
	}
}

func (handler *ResourceStreams) reauthorizationInterval() time.Duration {
	if handler.reauthorizeEvery > 0 {
		return handler.reauthorizeEvery
	}
	return defaultStreamReauthorizationInterval
}

func (handler *ResourceStreams) Resources(w http.ResponseWriter, r *http.Request) {
	topics, err := decodeTopics(r.URL.RawQuery)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	lastID := r.Header.Get("Last-Event-ID")
	if lastID != "" && !validStreamEventID(lastID) {
		api.WriteError(w, r, validationHTTPError("Last-Event-ID is malformed.", nil))
		return
	}
	binding, resolution, err := handler.preflight(w, r)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	streamResolution := resolution
	var session *resourceStreamSession
	if lastID == "" {
		streamResolution, err = handler.service.AuthorizeTopics(r.Context(), binding, resolution, topics)
	} else {
		// A reconnect may arrive while the ordinary read-capability cache still
		// contains a now-revoked grant. Resume only after a fresh SAR matrix.
		session = handler.resumableSession(lastID)
		if session != nil {
			streamResolution = session.resolution
		}
		err = handler.service.ReauthorizeTopics(r.Context(), binding, streamResolution, topics)
	}
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	release, err := handler.registry.Acquire()
	if err != nil {
		api.WriteError(w, r, resourceHTTPError(err))
		return
	}
	defer release()
	expectedBinding, bindErr := resourcecore.ReplayBinding(handler.instance, binding.Generation, topics)
	if bindErr != nil {
		api.WriteError(w, r, resourceHTTPError(bindErr))
		return
	}
	var client *resourceReplayClient
	var replay []resourcecore.ReplayEntry
	if lastID != "" {
		if session != nil {
			client, replay, err = session.attach(lastID, expectedBinding)
		}
		if session == nil || err != nil {
			handler.sendResumeUnavailable(w, binding.Generation, topics[0])
			return
		}
	} else {
		session, err = handler.createStreamSession(binding, streamResolution, topics)
		if err != nil {
			api.WriteError(w, r, resourceHTTPError(err))
			return
		}
		client, replay, err = session.attach("", expectedBinding)
		if err != nil {
			session.expire()
			api.WriteError(w, r, resourceHTTPError(err))
			return
		}
		session.start()
	}
	defer session.detach(client)
	stream, err := newResourceSSE(w)
	if err != nil {
		return
	}
	requestID := api.RequestIDFromContext(r.Context())
	for _, entry := range replay {
		if err = stream.Send(session.wireEvent(entry, requestID)); err != nil {
			return
		}
	}
	type deliveryResult struct {
		delivery resourceReplayDelivery
		err      error
	}
	deliveries := make(chan deliveryResult, 1)
	go func() {
		for {
			delivery, nextErr := client.next(r.Context())
			select {
			case deliveries <- deliveryResult{delivery: delivery, err: nextErr}:
			case <-r.Context().Done():
				return
			}
			if nextErr != nil {
				return
			}
		}
	}()
	heartbeat := time.NewTicker(streamHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case sentAt := <-heartbeat.C:
			if err := stream.Send(sse.Event{Type: "heartbeat", Data: map[string]any{"streamId": session.streamID, "generation": binding.Generation, "sentAt": sentAt.UTC().Format(time.RFC3339Nano)}}); err != nil {
				return
			}
		case result := <-deliveries:
			if result.err != nil {
				return
			}
			if err := stream.Send(session.wireEvent(result.delivery.entry, requestID)); err != nil {
				return
			}
			if result.delivery.terminal {
				return
			}
		}
	}
}

func (handler *ResourceStreams) sendResumeUnavailable(w http.ResponseWriter, generation string, topic resourcecore.Topic) {
	stream, err := newResourceSSE(w)
	if err == nil {
		_ = stream.Send(sse.Event{Type: "reset", Data: map[string]any{"streamId": randomOpaque("str_"), "topic": topic, "generation": generation, "reason": "resume_unavailable", "message": "State continuity was lost.", "refetchRequired": true}})
	}
}

func (handler *ResourceStreams) preflight(w http.ResponseWriter, r *http.Request) (namespaces.SelectionBinding, namespaces.ScopeResolution, error) {
	if handler == nil || handler.service == nil || handler.selection == nil || handler.sessions == nil {
		return namespaces.SelectionBinding{}, namespaces.ScopeResolution{}, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeFeatureUnavailable, "Streaming is unavailable.", nil, nil)
	}
	binding, resolution := handler.selection.Snapshot()
	if binding.ClusterProfileID <= 0 || binding.Context == "" || binding.Generation == "" || len(resolution.Namespaces) == 0 {
		return binding, resolution, api.NewHTTPError(http.StatusConflict, api.CodeGenerationChanged, "No active Kubernetes resource scope is available.", nil, nil)
	}
	if r.Header.Get("Origin") != handler.origin || !handler.sessions.Validate(r.Header.Get("X-KubePeep-CSRF"), binding.Generation) {
		return binding, resolution, api.NewHTTPError(http.StatusForbidden, api.CodeCSRFRejected, "The request did not pass local browser security checks.", nil, nil)
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return binding, resolution, api.NewHTTPError(http.StatusForbidden, api.CodeCSRFRejected, "The request did not pass local browser security checks.", nil, nil)
	}
	return binding, resolution, nil
}

func decodeTopics(raw string) ([]resourcecore.Topic, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, validationHTTPError("The stream query is invalid.", nil)
	}
	if len(values) != 1 {
		return nil, validationHTTPError("The stream query accepts only topic.", nil)
	}
	rawTopics, ok := values["topic"]
	if !ok {
		return nil, validationHTTPError("At least one stream topic is required.", nil)
	}
	topics := make([]resourcecore.Topic, len(rawTopics))
	for index, value := range rawTopics {
		if value == "" {
			return nil, validationHTTPError("Stream topics must be non-empty.", nil)
		}
		topics[index] = resourcecore.Topic(value)
	}
	canonical, err := resourcecore.ValidateTopics(topics)
	if err != nil {
		return nil, resourceHTTPError(err)
	}
	return canonical, nil
}
func validStreamEventID(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "kpse1" {
		return false
	}
	epoch, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(epoch) != 16 {
		return false
	}
	sequence, err := strconv.ParseUint(parts[2], 36, 64)
	return err == nil && sequence > 0
}
func randomOpaque(prefix string) string {
	var value [16]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return prefix + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value[:])
}
func streamErrorPayload(err error, generation, requestID string) map[string]any {
	code := resourcecore.ErrorCodeOf(err)
	retryable := code == resourcecore.CodeAuthorizationUnavailable || code == resourcecore.CodeClusterUnavailable || code == resourcecore.CodeUpstreamTimeout
	payload := map[string]any{"generation": generation, "requestId": requestID, "code": code, "message": resourcecore.PublicMessage(err), "retryable": retryable}
	if retryable {
		payload["retryAfterMs"] = 500
	}
	return payload
}

type resourceSSEWriter struct{ http.ResponseWriter }

func (writer *resourceSSEWriter) WriteHeader(status int) {
	noStore(writer.ResponseWriter)
	writer.ResponseWriter.Header().Set("X-Content-Type-Options", "nosniff")
	writer.ResponseWriter.WriteHeader(status)
}
func (writer *resourceSSEWriter) Flush() { writer.ResponseWriter.(http.Flusher).Flush() }

func newResourceSSE(w http.ResponseWriter) (*sse.Stream, error) {
	if _, ok := w.(http.Flusher); !ok {
		return nil, errors.New("resource stream: response writer cannot flush")
	}
	return sse.New(&resourceSSEWriter{ResponseWriter: w})
}
