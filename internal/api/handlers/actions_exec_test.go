package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	actionservice "github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

type execTestSelection struct{ binding namespaces.SelectionBinding }

func (selection execTestSelection) Snapshot() (namespaces.SelectionBinding, namespaces.ScopeResolution) {
	return selection.binding, namespaces.ScopeResolution{}
}

type execTestWriteCloser struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	closed bool
}

func (writer *execTestWriteCloser) Write(payload []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.Write(payload)
}

func (writer *execTestWriteCloser) Close() error {
	writer.mu.Lock()
	writer.closed = true
	writer.mu.Unlock()
	return nil
}

type execTestRemote struct {
	stdin  *execTestWriteCloser
	resize chan [2]uint16
}

func (remote *execTestRemote) Stdin() io.WriteCloser { return remote.stdin }
func (remote *execTestRemote) Stdout() io.Reader     { return strings.NewReader("") }
func (remote *execTestRemote) Stderr() io.Reader     { return strings.NewReader("") }
func (remote *execTestRemote) Resize(_ context.Context, columns, rows uint16) error {
	remote.resize <- [2]uint16{columns, rows}
	return nil
}

type execTestService struct {
	mu           sync.Mutex
	remote       *execTestRemote
	terminal     chan actionservice.ExecTerminal
	authorized   bool
	authorizeErr error
	offered      []string
	abort        actionservice.ExecAbortReason
	canceled     bool
	released     bool
	touches      int
}

func newExecTestService() *execTestService {
	return &execTestService{
		remote:   &execTestRemote{stdin: &execTestWriteCloser{}, resize: make(chan [2]uint16, 2)},
		terminal: make(chan actionservice.ExecTerminal, 1),
	}
}

func (*execTestService) CreateTicket(context.Context, namespaces.SelectionBinding, actionservice.RouteTarget, actionservice.ExecInit) (actionservice.ExecTicketDTO, error) {
	return actionservice.ExecTicketDTO{}, nil
}

func (service *execTestService) AuthorizeUpgrade(_ context.Context, binding namespaces.SelectionBinding, sessionID string, offered []string) (actionservice.ExecGrant, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.authorized = true
	service.offered = append([]string(nil), offered...)
	if service.authorizeErr != nil {
		return actionservice.ExecGrant{}, service.authorizeErr
	}
	return actionservice.ExecGrant{SessionID: sessionID, Generation: binding.Generation}, nil
}

func (service *execTestService) Start(_ context.Context, grant actionservice.ExecGrant) (actionservice.ActiveExec, error) {
	return actionservice.ActiveExec{
		SessionID: grant.SessionID, Generation: grant.Generation, TTY: true, Stdin: true,
		Remote: service.remote, Terminal: service.terminal,
	}, nil
}

func (service *execTestService) Touch(string) error {
	service.mu.Lock()
	service.touches++
	service.mu.Unlock()
	return nil
}

func (service *execTestService) Cancel(string) error {
	service.mu.Lock()
	service.canceled = true
	service.mu.Unlock()
	return nil
}

func (service *execTestService) Abort(_ string, reason actionservice.ExecAbortReason) error {
	service.mu.Lock()
	service.abort = reason
	service.mu.Unlock()
	service.terminal <- actionservice.ExecTerminal{
		Type: actionservice.ExecTerminalError, Code: actionservice.CodeProtocolViolation,
		Message: "The exec protocol was violated.", CloseCode: 1008,
	}
	return nil
}

func (service *execTestService) ReleaseUpgrade(actionservice.ExecGrant) error {
	service.mu.Lock()
	service.released = true
	service.mu.Unlock()
	return nil
}

func (*execTestService) OnGeneration(string) {}
func (*execTestService) Shutdown()           {}

func TestExecStreamSelectsOnlyPublicProtocolAndBridgesFrames(t *testing.T) {
	service := newExecTestService()
	selection := execTestSelection{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen_1"}}
	handler := newExecStream(service, selection, "placeholder", execStreamOptions{
		heartbeatInterval: time.Hour,
		heartbeatTimeout:  time.Second,
		pingInterval:      time.Hour,
		pingTimeout:       time.Second,
		writeTimeout:      time.Second,
	})
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/exec/{sessionId}/stream", handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	handler.origin = server.URL

	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/exec/exec_test/stream"
	connection, response, err := websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{
		Subprotocols: []string{actionservice.ExecWebSocketProtocol, actionservice.ExecTicketPrefix + "ticket"},
		HTTPHeader:   http.Header{"Origin": []string{server.URL}},
	})
	if err != nil {
		if response != nil {
			t.Fatalf("dial status=%d err=%v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	if response.Header.Get("Sec-WebSocket-Extensions") != "" {
		t.Fatalf("compression negotiated: %q", response.Header.Get("Sec-WebSocket-Extensions"))
	}
	defer connection.CloseNow()
	if connection.Subprotocol() != actionservice.ExecWebSocketProtocol {
		t.Fatalf("selected protocol = %q", connection.Subprotocol())
	}
	readContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	messageType, payload, err := connection.Read(readContext)
	if err != nil || messageType != websocket.MessageText {
		t.Fatalf("ready type=%v payload=%q err=%v", messageType, payload, err)
	}
	var ready readyControl
	if json.Unmarshal(payload, &ready) != nil || ready.Type != "ready" || ready.SessionID != "exec_test" || ready.Generation != "gen_1" || !ready.TTY || !ready.Stdin {
		t.Fatalf("ready = %s", payload)
	}
	if err := connection.Write(readContext, websocket.MessageBinary, append([]byte{0x00}, []byte("whoami")...)); err != nil {
		t.Fatal(err)
	}
	if err := connection.Write(readContext, websocket.MessageText, []byte(`{"type":"resize","columns":120,"rows":40}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case size := <-service.remote.resize:
		if size != [2]uint16{120, 40} {
			t.Fatalf("resize = %#v", size)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resize was not bridged")
	}
	service.remote.stdin.mu.Lock()
	stdin := service.remote.stdin.buffer.String()
	service.remote.stdin.mu.Unlock()
	if stdin != "whoami" {
		t.Fatalf("stdin = %q", stdin)
	}
	code := 0
	service.terminal <- actionservice.ExecTerminal{Type: actionservice.ExecTerminalExit, ExitCode: &code, Reason: actionservice.ExecExitCompleted, CloseCode: 1000}
	messageType, payload, err = connection.Read(readContext)
	if err != nil || messageType != websocket.MessageText || !bytes.Contains(payload, []byte(`"type":"exit"`)) {
		t.Fatalf("exit type=%v payload=%q err=%v", messageType, payload, err)
	}
	service.mu.Lock()
	offered := append([]string(nil), service.offered...)
	service.mu.Unlock()
	if len(offered) != 2 || offered[0] != actionservice.ExecWebSocketProtocol || offered[1] != actionservice.ExecTicketPrefix+"ticket" {
		t.Fatalf("offered protocols = %#v", offered)
	}
}

func TestExecStreamInitiatesApplicationHeartbeatAndAcceptsExactEcho(t *testing.T) {
	service := newExecTestService()
	selection := execTestSelection{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen_heartbeat"}}
	handler := newExecStream(service, selection, "placeholder", execStreamOptions{
		heartbeatInterval: 20 * time.Millisecond,
		heartbeatTimeout:  500 * time.Millisecond,
		pingInterval:      time.Hour,
		pingTimeout:       time.Second,
		writeTimeout:      time.Second,
	})
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/exec/{sessionId}/stream", handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	handler.origin = server.URL
	connection, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/exec/exec_heartbeat/stream", &websocket.DialOptions{
		Subprotocols: []string{actionservice.ExecWebSocketProtocol, actionservice.ExecTicketPrefix + "ticket"},
		HTTPHeader:   http.Header{"Origin": []string{server.URL}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := connection.Read(ctx); err != nil {
		t.Fatal(err)
	}
	messageType, heartbeat, err := connection.Read(ctx)
	if err != nil || messageType != websocket.MessageText {
		t.Fatalf("heartbeat type=%v payload=%q err=%v", messageType, heartbeat, err)
	}
	var control heartbeatControl
	if json.Unmarshal(heartbeat, &control) != nil || control.Type != "heartbeat" || !validHeartbeatNonce(control.Nonce) {
		t.Fatalf("heartbeat = %s", heartbeat)
	}
	if err := connection.Write(ctx, websocket.MessageText, heartbeat); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	service.mu.Lock()
	abort := service.abort
	service.mu.Unlock()
	if abort != "" {
		t.Fatalf("exact heartbeat echo aborted session: %q", abort)
	}
	code := 0
	service.terminal <- actionservice.ExecTerminal{Type: actionservice.ExecTerminalExit, ExitCode: &code, Reason: actionservice.ExecExitCompleted, CloseCode: 1000}
	for {
		_, payload, readErr := connection.Read(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if bytes.Contains(payload, []byte(`"type":"exit"`)) {
			break
		}
	}
}

func TestExecControlDecoderAndOutboundQueueAreStrictlyBounded(t *testing.T) {
	valid := []byte(`{"type":"resize","columns":80,"rows":24}`)
	if _, err := decodeExecClientControl(valid); err != nil {
		t.Fatalf("valid resize: %v", err)
	}
	for _, payload := range [][]byte{
		[]byte(`{"type":"resize","columns":80,"rows":24,"command":"sh"}`),
		[]byte(`{"type":"heartbeat","nonce":"hb_a"} {}`),
		append([]byte(`{"type":"close","stream":"session"}`), 0xff),
		bytes.Repeat([]byte("x"), actionservice.MaximumExecControlMessageBytes+1),
	} {
		if _, err := decodeExecClientControl(payload); err == nil {
			t.Fatalf("accepted invalid control %q", payload)
		}
	}
	queue := newExecOutboundQueue()
	for index := 0; index < execOutboundMessages; index++ {
		if !queue.enqueue(execOutboundMessage{messageType: websocket.MessageBinary, payload: []byte{byte(index)}}) {
			t.Fatalf("queue rejected message %d before limit", index)
		}
	}
	if queue.enqueue(execOutboundMessage{messageType: websocket.MessageBinary, payload: []byte("overflow")}) {
		t.Fatal("queue accepted message 65")
	}
	byteLimited := newExecOutboundQueue()
	if byteLimited.enqueue(execOutboundMessage{messageType: websocket.MessageBinary, payload: bytes.Repeat([]byte("x"), execOutboundBytes+1)}) {
		t.Fatal("queue exceeded one MiB")
	}
}

func TestExecStreamRejectsBeforeUpgradeAndInvalidControlAborts(t *testing.T) {
	selection := execTestSelection{binding: namespaces.SelectionBinding{ClusterProfileID: 1, Context: "dev", Generation: "gen_1"}}
	rejected := newExecTestService()
	rejected.authorizeErr = &actionservice.Error{Code: actionservice.CodeForbidden, Message: "Kubernetes denied this operation.", HTTPStatus: http.StatusForbidden}
	handler := NewExecStream(rejected, selection, "http://127.0.0.1:8080")
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/exec/exec_test/stream", nil)
	request.SetPathValue("sessionId", "exec_test")
	request.Header.Set("Sec-WebSocket-Protocol", actionservice.ExecWebSocketProtocol+", "+actionservice.ExecTicketPrefix+"ticket")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"code":"FORBIDDEN"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	service := newExecTestService()
	stream := newExecStream(service, selection, "placeholder", execStreamOptions{
		heartbeatInterval: time.Hour, heartbeatTimeout: time.Second,
		pingInterval: time.Hour, pingTimeout: time.Second, writeTimeout: time.Second,
	})
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/exec/{sessionId}/stream", stream)
	server := httptest.NewServer(mux)
	defer server.Close()
	stream.origin = server.URL
	connection, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/exec/exec_bad/stream", &websocket.DialOptions{
		Subprotocols: []string{actionservice.ExecWebSocketProtocol, actionservice.ExecTicketPrefix + "ticket"},
		HTTPHeader:   http.Header{"Origin": []string{server.URL}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := connection.Read(ctx); err != nil {
		t.Fatal(err)
	}
	if err := connection.Write(ctx, websocket.MessageText, []byte(`{"type":"resize","columns":1,"rows":1,"command":"forbidden"}`)); err != nil {
		t.Fatal(err)
	}
	messageType, payload, err := connection.Read(ctx)
	if err != nil || messageType != websocket.MessageText || !bytes.Contains(payload, []byte(`"code":"PROTOCOL_VIOLATION"`)) {
		t.Fatalf("terminal type=%v payload=%q err=%v", messageType, payload, err)
	}
	service.mu.Lock()
	abort := service.abort
	service.mu.Unlock()
	if abort != actionservice.ExecAbortProtocolViolation {
		t.Fatalf("abort = %q", abort)
	}
}

var _ ExecBridgeService = (*execTestService)(nil)
