package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fvmoraes/ginger/pkg/sse"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
)

const defaultResourceReplayRetention = 30 * time.Second
const defaultStreamReauthorizationInterval = 60 * time.Second

type resourceStreamSource struct {
	topic        resourcecore.Topic
	key          string
	subscription *resourcecore.Subscription
}

type resourceStreamIncoming struct {
	source resourceStreamSource
	event  resourcecore.StreamEvent
	err    error
}

type resourceStreamSession struct {
	handler    *ResourceStreams
	service    ResourceStreamService
	binding    namespaces.SelectionBinding
	resolution namespaces.ScopeResolution
	topics     []resourcecore.Topic
	streamID   string
	ring       *resourcecore.ReplayRing
	sources    []resourceStreamSource

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once

	mu          sync.Mutex
	clients     map[*resourceReplayClient]struct{}
	terminal    *resourcecore.StreamEvent
	snapshotIDs map[resourcecore.Topic]string
	idleTimer   *time.Timer
	expired     bool
	createdAt   time.Time
}

type resourceReplayClient struct {
	mu     sync.Mutex
	signal chan struct{}
	queue  []resourceReplayDelivery
	bytes  int
	closed bool
}

type resourceReplayDelivery struct {
	entry    resourcecore.ReplayEntry
	terminal bool
	bytes    int
}

func (handler *ResourceStreams) createStreamSession(binding namespaces.SelectionBinding, resolution namespaces.ScopeResolution, topics []resourcecore.Topic) (*resourceStreamSession, error) {
	ring, err := resourcecore.NewReplayRing(handler.instance, binding.Generation, topics)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	session := &resourceStreamSession{
		handler: handler, service: handler.service, binding: binding, resolution: resolution,
		topics: append([]resourcecore.Topic(nil), topics...), streamID: randomOpaque("str_"), ring: ring,
		ctx: ctx, cancel: cancel, clients: map[*resourceReplayClient]struct{}{}, snapshotIDs: map[resourcecore.Topic]string{}, createdAt: handler.now().UTC(),
	}
	for _, topic := range topics {
		session.snapshotIDs[topic] = randomOpaque("snap_")
		for _, gvr := range resourcecore.TopicGVRs(topic) {
			for _, namespace := range resolution.Namespaces {
				subscription, subscribeErr := handler.service.Subscribe(ctx, binding, resolution, topic, gvr, namespace)
				if subscribeErr != nil {
					session.stop()
					return nil, subscribeErr
				}
				session.sources = append(session.sources, resourceStreamSource{topic: topic, key: streamSourceKey(topic, gvr, namespace), subscription: subscription})
			}
		}
	}
	if err = handler.registerStreamSession(session); err != nil {
		session.stop()
		return nil, err
	}
	return session, nil
}

func (handler *ResourceStreams) registerStreamSession(session *resourceStreamSession) error {
	var evicted *resourceStreamSession
	handler.streamMu.Lock()
	if handler.streamSessions == nil {
		handler.streamSessions = map[string]*resourceStreamSession{}
	}
	if len(handler.streamSessions) >= resourcecore.MaximumStreams {
		for _, candidate := range handler.streamSessions {
			candidate.mu.Lock()
			idle := len(candidate.clients) == 0
			candidate.mu.Unlock()
			if idle && (evicted == nil || candidate.createdAt.Before(evicted.createdAt)) {
				evicted = candidate
			}
		}
		if evicted == nil {
			handler.streamMu.Unlock()
			return &resourcecore.DomainError{Code: resourcecore.CodeLimitExceeded, Message: "The stream replay limit was reached."}
		}
		delete(handler.streamSessions, evicted.ring.Epoch())
	}
	handler.streamSessions[session.ring.Epoch()] = session
	handler.streamMu.Unlock()
	if evicted != nil {
		evicted.expire()
	}
	return nil
}

func streamSourceKey(topic resourcecore.Topic, gvr schema.GroupVersionResource, namespace string) string {
	return string(topic) + "\x00" + gvr.String() + "\x00" + namespace
}

func (handler *ResourceStreams) resumableSession(lastID string) *resourceStreamSession {
	parts := strings.Split(lastID, ".")
	if len(parts) != 3 {
		return nil
	}
	handler.streamMu.Lock()
	session := handler.streamSessions[parts[1]]
	handler.streamMu.Unlock()
	return session
}

func (handler *ResourceStreams) removeStreamSession(epoch string, expected *resourceStreamSession) {
	handler.streamMu.Lock()
	if handler.streamSessions[epoch] == expected {
		delete(handler.streamSessions, epoch)
	}
	handler.streamMu.Unlock()
}

func (session *resourceStreamSession) start() { go session.run() }

func (session *resourceStreamSession) attach(lastID, expectedBinding string) (*resourceReplayClient, []resourcecore.ReplayEntry, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.expired {
		return nil, nil, resourcecore.ErrResumeUnavailable
	}
	var replay []resourcecore.ReplayEntry
	var err error
	if lastID != "" {
		replay, err = session.ring.ReplayEntries(lastID, expectedBinding)
		if err != nil {
			return nil, nil, err
		}
	}
	if session.idleTimer != nil {
		session.idleTimer.Stop()
		session.idleTimer = nil
	}
	client := newResourceReplayClient()
	session.clients[client] = struct{}{}
	if session.terminal != nil {
		client.forceTerminal(*session.terminal)
	}
	return client, replay, nil
}

func (session *resourceStreamSession) detach(client *resourceReplayClient) {
	if client == nil {
		return
	}
	client.close()
	session.mu.Lock()
	delete(session.clients, client)
	if len(session.clients) == 0 && !session.expired {
		retention := session.handler.replayRetention
		if retention <= 0 {
			retention = defaultResourceReplayRetention
		}
		if session.idleTimer != nil {
			session.idleTimer.Stop()
		}
		session.idleTimer = time.AfterFunc(retention, session.expire)
	}
	session.mu.Unlock()
}

func (session *resourceStreamSession) expire() {
	session.mu.Lock()
	if len(session.clients) != 0 || session.expired {
		session.mu.Unlock()
		return
	}
	session.expired = true
	session.mu.Unlock()
	session.stop()
	session.handler.removeStreamSession(session.ring.Epoch(), session)
}

func (session *resourceStreamSession) stop() {
	session.once.Do(func() {
		session.cancel()
		for _, source := range session.sources {
			if source.subscription != nil {
				source.subscription.Close()
			}
		}
	})
}

func (session *resourceStreamSession) publish(event resourcecore.StreamEvent) bool {
	if event.Generation == "" {
		event.Generation = session.binding.Generation
	}
	if event.ObservedAt == "" && event.Event != "reset" && event.Event != "error" {
		event.ObservedAt = session.handler.now().UTC().Format(time.RFC3339Nano)
	}
	if event.Event == "reset" || event.Event == "error" {
		session.publishTerminal(event)
		return false
	}
	session.mu.Lock()
	if session.expired || session.terminal != nil {
		session.mu.Unlock()
		return false
	}
	id, err := session.ring.Append(event)
	if err != nil {
		session.mu.Unlock()
		session.publishTerminal(resourcecore.StreamEvent{Event: "reset", Topic: event.Topic, Generation: session.binding.Generation, Reason: "event_too_large", RefetchRequired: true})
		return false
	}
	entry := resourcecore.ReplayEntry{ID: id, Event: event}
	for client := range session.clients {
		if !client.push(entry) {
			client.forceTerminal(resourcecore.StreamEvent{Event: "reset", Topic: event.Topic, Generation: session.binding.Generation, Reason: "slow_consumer", RefetchRequired: true})
			delete(session.clients, client)
		}
	}
	session.mu.Unlock()
	return true
}

func (session *resourceStreamSession) publishTerminal(event resourcecore.StreamEvent) {
	if event.Generation == "" {
		event.Generation = session.binding.Generation
	}
	session.mu.Lock()
	if session.expired || session.terminal != nil {
		session.mu.Unlock()
		return
	}
	terminal := event
	session.terminal = &terminal
	for client := range session.clients {
		client.pushTerminal(event)
	}
	session.mu.Unlock()
	session.stop()
}

func (session *resourceStreamSession) run() {
	incoming := make(chan resourceStreamIncoming, min(resourcecore.MaximumStreamQueueEvents, max(1, len(session.sources)*8)))
	for _, source := range session.sources {
		source := source
		go func() {
			for {
				event, err := source.subscription.Next(session.ctx)
				select {
				case incoming <- resourceStreamIncoming{source: source, event: event, err: err}:
				case <-session.ctx.Done():
					return
				}
				if err != nil {
					return
				}
			}
		}()
	}
	interval := session.handler.reauthorizeEvery
	if interval <= 0 {
		interval = defaultStreamReauthorizationInterval
	}
	reauthorize := time.NewTicker(interval)
	defer reauthorize.Stop()
	expected := map[resourcecore.Topic]int{}
	for _, source := range session.sources {
		expected[source.topic]++
	}
	states := map[resourcecore.Topic]*resourceTopicSnapshot{}
	for _, topic := range session.topics {
		states[topic] = &resourceTopicSnapshot{expected: expected[topic], items: []resourcecore.TopicObject{}, finalSources: map[string]struct{}{}, buffered: []resourcecore.StreamEvent{}}
	}
	for {
		select {
		case <-session.ctx.Done():
			return
		case <-reauthorize.C:
			if err := session.service.ReauthorizeTopics(session.ctx, session.binding, session.resolution, session.topics); err != nil {
				session.publishTerminal(resourcecore.StreamEvent{Event: "error", Topic: session.topics[0], Generation: session.binding.Generation, Reason: string(resourcecore.ErrorCodeOf(err)), RefetchRequired: true})
				return
			}
		case item := <-incoming:
			if item.err != nil {
				if errors.Is(item.err, context.Canceled) || errors.Is(item.err, io.EOF) && session.ctx.Err() != nil {
					return
				}
				session.publishTerminal(resourcecore.StreamEvent{Event: "error", Topic: item.source.topic, Generation: session.binding.Generation, Reason: string(resourcecore.CodeClusterUnavailable), RefetchRequired: true})
				return
			}
			event := item.event
			if event.Event == "reset" || event.Event == "error" {
				session.publishTerminal(event)
				return
			}
			state := states[item.source.topic]
			if state == nil {
				session.publishTerminal(resourcecore.StreamEvent{Event: "reset", Topic: item.source.topic, Generation: session.binding.Generation, Reason: "event_too_large", RefetchRequired: true})
				return
			}
			if !state.complete {
				if event.Event == "snapshot" {
					if _, alreadyFinal := state.finalSources[item.source.key]; alreadyFinal {
						session.publishTerminal(resourcecore.StreamEvent{Event: "reset", Topic: item.source.topic, Generation: session.binding.Generation, Reason: "snapshot_too_large", RefetchRequired: true})
						return
					}
					if !state.addItems(event.Items) {
						session.publishTerminal(resourcecore.StreamEvent{Event: "reset", Topic: item.source.topic, Generation: session.binding.Generation, Reason: "snapshot_too_large", RefetchRequired: true})
						return
					}
					if state.expected == 1 {
						state.resourceVersion = event.ResourceVersion
					}
					if event.Final {
						state.finalSources[item.source.key] = struct{}{}
					}
					if len(state.finalSources) == state.expected {
						chunks, err := resourcecore.SnapshotEvents(session.binding.Generation, item.source.topic, resourcecore.WatchSnapshot{ResourceVersion: state.resourceVersion, Items: state.items})
						if err != nil {
							session.publishTerminal(resourcecore.StreamEvent{Event: "reset", Topic: item.source.topic, Generation: session.binding.Generation, Reason: "snapshot_too_large", RefetchRequired: true})
							return
						}
						state.complete = true
						for _, chunk := range chunks {
							if !session.publish(chunk) {
								return
							}
						}
						for _, buffered := range state.buffered {
							if !session.publish(buffered) {
								return
							}
						}
						state.buffered = nil
					}
					continue
				}
				if !state.buffer(event) {
					session.publishTerminal(resourcecore.StreamEvent{Event: "reset", Topic: item.source.topic, Generation: session.binding.Generation, Reason: "slow_consumer", RefetchRequired: true})
					return
				}
				continue
			}
			if !session.publish(event) {
				return
			}
		}
	}
}

type resourceTopicSnapshot struct {
	expected        int
	finalSources    map[string]struct{}
	items           []resourcecore.TopicObject
	resourceVersion string
	complete        bool
	buffered        []resourcecore.StreamEvent
	bufferedBytes   int
	itemBytes       int
}

func (state *resourceTopicSnapshot) addItems(items []resourcecore.TopicObject) bool {
	if len(state.items)+len(items) > resourcecore.MaximumSnapshotItems {
		return false
	}
	for _, item := range items {
		encoded, err := json.Marshal(item)
		if err != nil || state.itemBytes+len(encoded)+1 > resourcecore.MaximumSnapshotBytes {
			return false
		}
		state.itemBytes += len(encoded) + 1
	}
	state.items = append(state.items, items...)
	return true
}

func (state *resourceTopicSnapshot) buffer(event resourcecore.StreamEvent) bool {
	encoded, _ := json.Marshal(event)
	wireBytes := len(encoded) + 64
	if len(state.buffered) >= resourcecore.MaximumStreamQueueEvents || state.bufferedBytes+wireBytes > resourcecore.MaximumStreamQueueBytes {
		return false
	}
	state.buffered = append(state.buffered, event)
	state.bufferedBytes += wireBytes
	return true
}

func newResourceReplayClient() *resourceReplayClient {
	return &resourceReplayClient{signal: make(chan struct{}, 1), queue: []resourceReplayDelivery{}}
}

func (client *resourceReplayClient) push(entry resourcecore.ReplayEntry) bool {
	encoded, _ := json.Marshal(entry.Event)
	wireBytes := len(encoded) + len(entry.ID) + 64
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed || len(client.queue) >= resourcecore.MaximumStreamQueueEvents || client.bytes+wireBytes > resourcecore.MaximumStreamQueueBytes {
		return false
	}
	client.queue = append(client.queue, resourceReplayDelivery{entry: entry, bytes: wireBytes})
	client.bytes += wireBytes
	select {
	case client.signal <- struct{}{}:
	default:
	}
	return true
}

func (client *resourceReplayClient) forceTerminal(event resourcecore.StreamEvent) {
	encoded, _ := json.Marshal(event)
	delivery := resourceReplayDelivery{entry: resourcecore.ReplayEntry{Event: event}, terminal: true, bytes: len(encoded) + 64}
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		return
	}
	client.queue = []resourceReplayDelivery{delivery}
	client.bytes = delivery.bytes
	client.closed = true
	select {
	case client.signal <- struct{}{}:
	default:
	}
	client.mu.Unlock()
}

func (client *resourceReplayClient) pushTerminal(event resourcecore.StreamEvent) {
	encoded, _ := json.Marshal(event)
	delivery := resourceReplayDelivery{entry: resourcecore.ReplayEntry{Event: event}, terminal: true, bytes: len(encoded) + 64}
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		return
	}
	if len(client.queue) >= resourcecore.MaximumStreamQueueEvents || client.bytes+delivery.bytes > resourcecore.MaximumStreamQueueBytes {
		client.queue = []resourceReplayDelivery{delivery}
		client.bytes = delivery.bytes
	} else {
		client.queue = append(client.queue, delivery)
		client.bytes += delivery.bytes
	}
	client.closed = true
	select {
	case client.signal <- struct{}{}:
	default:
	}
	client.mu.Unlock()
}

func (client *resourceReplayClient) next(ctx context.Context) (resourceReplayDelivery, error) {
	for {
		client.mu.Lock()
		if len(client.queue) > 0 {
			delivery := client.queue[0]
			client.queue = client.queue[1:]
			client.bytes -= delivery.bytes
			client.mu.Unlock()
			return delivery, nil
		}
		closed := client.closed
		client.mu.Unlock()
		if closed {
			return resourceReplayDelivery{}, io.EOF
		}
		select {
		case <-ctx.Done():
			return resourceReplayDelivery{}, ctx.Err()
		case <-client.signal:
		}
	}
}

func (client *resourceReplayClient) close() {
	client.mu.Lock()
	if !client.closed {
		client.closed = true
		select {
		case client.signal <- struct{}{}:
		default:
		}
	}
	client.mu.Unlock()
}

func (session *resourceStreamSession) wireEvent(entry resourcecore.ReplayEntry, requestID string) sse.Event {
	event := entry.Event
	if event.Event == "reset" {
		return sse.Event{Type: "reset", Data: map[string]any{"streamId": session.streamID, "topic": event.Topic, "generation": session.binding.Generation, "reason": event.Reason, "message": "State continuity was lost.", "refetchRequired": true}}
	}
	if event.Event == "error" {
		payload := streamErrorPayload(&resourcecore.DomainError{Code: resourcecore.ErrorCode(event.Reason), Message: "The resource stream failed."}, session.binding.Generation, requestID)
		if event.Topic != "" {
			payload["topic"] = event.Topic
		}
		payload["streamId"] = session.streamID
		return sse.Event{Type: "error", Data: payload}
	}
	sequence := replaySequence(entry.ID)
	payload := map[string]any{"streamId": session.streamID, "topic": event.Topic, "generation": session.binding.Generation, "sequence": sequence, "observedAt": event.ObservedAt, "resourceVersion": event.ResourceVersion}
	if event.Event == "snapshot" {
		payload["snapshotId"], payload["chunk"], payload["final"], payload["items"] = session.snapshotIDs[event.Topic], event.Chunk, event.Final, event.Items
	} else if event.Event == "deleted" {
		payload["object"] = event.Deleted
	} else {
		payload["object"] = event.Object
	}
	return sse.Event{ID: entry.ID, Type: event.Event, Data: payload}
}

func replaySequence(id string) uint64 {
	parts := strings.Split(id, ".")
	if len(parts) != 3 {
		return 0
	}
	value, _ := strconv.ParseUint(parts[2], 36, 64)
	return value
}
