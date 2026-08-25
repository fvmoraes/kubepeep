package handlers

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	actionservice "github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

func TestExecStreamAcceptsFragmentedMaskedControlMessage(t *testing.T) {
	service := newExecTestService()
	server, websocketURL := newExecWireServer(t, service, "exec_fragmented", execStreamOptions{
		heartbeatInterval: time.Hour,
		heartbeatTimeout:  time.Second,
		pingInterval:      time.Hour,
		pingTimeout:       time.Second,
		writeTimeout:      time.Second,
	})
	connection := dialExecWire(t, server.URL, websocketURL, []string{
		actionservice.ExecWebSocketProtocol,
		actionservice.ExecTicketPrefix + "ticket",
	})
	defer connection.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	readExecReady(t, ctx, connection)

	writer, err := connection.Writer(ctx, websocket.MessageText)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`{"type":"res`, `ize","columns":137`, `,"rows":43}`} {
		if _, err := writer.Write([]byte(fragment)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case size := <-service.remote.resize:
		if size != [2]uint16{137, 43} {
			t.Fatalf("fragmented resize = %#v", size)
		}
	case <-ctx.Done():
		t.Fatal("fragmented control message was not bridged")
	}

	finishExecWire(t, ctx, connection, service, actionservice.ExecTerminal{
		Type: actionservice.ExecTerminalExit, Reason: actionservice.ExecExitCompleted, CloseCode: 1000,
	})
}

func TestExecStreamRejectsUnmaskedClientFrame(t *testing.T) {
	base := newExecTestService()
	service := &disconnectAwareExecService{execTestService: base}
	server, websocketURL := newExecWireServer(t, service, "exec_unmasked", execStreamOptions{
		heartbeatInterval: time.Hour,
		heartbeatTimeout:  time.Second,
		pingInterval:      time.Hour,
		pingTimeout:       time.Second,
		writeTimeout:      time.Second,
	})
	connection, reader := rawExecUpgrade(t, server.URL, websocketURL)
	defer connection.Close()
	if opcode, _, err := readRawWebSocketFrame(reader); err != nil || opcode != 0x1 {
		t.Fatalf("ready opcode=%d err=%v", opcode, err)
	}

	payload := []byte(`{"type":"resize","columns":80,"rows":24}`)
	frame := append([]byte{0x81, byte(len(payload))}, payload...)
	if _, err := connection.Write(frame); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 2*time.Second, func() bool {
		base.mu.Lock()
		defer base.mu.Unlock()
		return base.canceled
	})
	if opcode, _, err := readRawWebSocketFrame(reader); err == nil && opcode != 0x8 {
		t.Fatalf("unmasked client frame yielded opcode %d instead of closing", opcode)
	}
	select {
	case size := <-base.remote.resize:
		t.Fatalf("unmasked resize reached the remote stream: %#v", size)
	default:
	}
}

func TestExecStreamRejectsAdversarialSubprotocolOffersBeforeUpgrade(t *testing.T) {
	valid := []string{actionservice.ExecWebSocketProtocol, actionservice.ExecTicketPrefix + "ticket"}
	for _, test := range []struct {
		name      string
		protocols []string
	}{
		{name: "missing ticket", protocols: []string{actionservice.ExecWebSocketProtocol}},
		{name: "duplicate ticket", protocols: []string{actionservice.ExecWebSocketProtocol, valid[1], valid[1]}},
		{name: "extra protocol", protocols: []string{actionservice.ExecWebSocketProtocol, valid[1], "unexpected.v1"}},
		{name: "duplicate public protocol", protocols: []string{actionservice.ExecWebSocketProtocol, actionservice.ExecWebSocketProtocol, valid[1]}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &exactOfferExecService{execTestService: newExecTestService(), expected: valid}
			server, websocketURL := newExecWireServer(t, service, "exec_protocol", execStreamOptions{
				heartbeatInterval: time.Hour,
				heartbeatTimeout:  time.Second,
				pingInterval:      time.Hour,
				pingTimeout:       time.Second,
				writeTimeout:      time.Second,
			})
			connection, response, err := websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{
				Subprotocols: test.protocols,
				HTTPHeader:   http.Header{"Origin": []string{server.URL}},
			})
			if connection != nil {
				connection.CloseNow()
				t.Fatal("malformed subprotocol offer upgraded")
			}
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			if err == nil || response == nil || response.StatusCode != http.StatusBadRequest {
				t.Fatalf("dial response=%v err=%v", response, err)
			}
		})
	}
}

func TestExecStreamRejectsHeartbeatMismatchAndTimeout(t *testing.T) {
	for _, test := range []struct {
		name    string
		respond func(context.Context, *websocket.Conn, []byte) error
		timeout time.Duration
	}{
		{
			name: "mismatched nonce",
			respond: func(ctx context.Context, connection *websocket.Conn, _ []byte) error {
				return connection.Write(ctx, websocket.MessageText, []byte(`{"type":"heartbeat","nonce":"aA"}`))
			},
			timeout: 500 * time.Millisecond,
		},
		{name: "missing echo", timeout: 20 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newExecTestService()
			server, websocketURL := newExecWireServer(t, service, "exec_heartbeat_bad", execStreamOptions{
				heartbeatInterval: 5 * time.Millisecond,
				heartbeatTimeout:  test.timeout,
				pingInterval:      time.Hour,
				pingTimeout:       time.Second,
				writeTimeout:      time.Second,
			})
			connection := dialExecWire(t, server.URL, websocketURL, []string{
				actionservice.ExecWebSocketProtocol,
				actionservice.ExecTicketPrefix + "ticket",
			})
			defer connection.CloseNow()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			readExecReady(t, ctx, connection)
			messageType, heartbeat, err := connection.Read(ctx)
			if err != nil || messageType != websocket.MessageText {
				t.Fatalf("heartbeat type=%v payload=%q err=%v", messageType, heartbeat, err)
			}
			var control heartbeatControl
			if err := json.Unmarshal(heartbeat, &control); err != nil || control.Type != "heartbeat" || !validHeartbeatNonce(control.Nonce) {
				t.Fatalf("heartbeat = %s err=%v", heartbeat, err)
			}
			if test.respond != nil {
				if err := test.respond(ctx, connection, heartbeat); err != nil {
					t.Fatal(err)
				}
			}
			readExecTerminalAndClose(t, ctx, connection, "error", websocket.StatusPolicyViolation)
			service.mu.Lock()
			abort := service.abort
			service.mu.Unlock()
			if abort != actionservice.ExecAbortProtocolViolation {
				t.Fatalf("heartbeat abort = %q", abort)
			}
		})
	}
}

func TestExecStreamSeparatesOutputChannelsAndSuppressesStderrForTTY(t *testing.T) {
	for _, tty := range []bool{true, false} {
		name := "non-tty"
		if tty {
			name = "tty"
		}
		t.Run(name, func(t *testing.T) {
			base := newExecTestService()
			stdoutReader := newOutputExecReader("stdout-wire-evidence")
			stderrReader := newOutputExecReader("stderr-wire-evidence")
			service := &outputExecService{
				execTestService: base,
				tty:             tty,
				remote: &outputExecRemote{
					stdin:  &execTestWriteCloser{},
					stdout: stdoutReader,
					stderr: stderrReader,
				},
			}
			server, websocketURL := newExecWireServer(t, service, "exec_output", execStreamOptions{
				heartbeatInterval: time.Hour,
				heartbeatTimeout:  time.Second,
				pingInterval:      time.Hour,
				pingTimeout:       time.Second,
				writeTimeout:      time.Second,
			})
			connection := dialExecWire(t, server.URL, websocketURL, []string{
				actionservice.ExecWebSocketProtocol,
				actionservice.ExecTicketPrefix + "ticket",
			})
			defer connection.CloseNow()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			messageType, payload, err := connection.Read(ctx)
			if err != nil || messageType != websocket.MessageText {
				t.Fatalf("ready type=%v payload=%q err=%v", messageType, payload, err)
			}
			var ready readyControl
			if err := json.Unmarshal(payload, &ready); err != nil || ready.Type != "ready" || ready.TTY != tty {
				t.Fatalf("ready=%s err=%v", payload, err)
			}

			expected := map[byte]string{0x01: "stdout-wire-evidence"}
			if !tty {
				expected[0x02] = "stderr-wire-evidence"
			}
			for len(expected) > 0 {
				messageType, payload, err = connection.Read(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if messageType != websocket.MessageBinary || len(payload) < 1 {
					t.Fatalf("output type=%v payload=%q", messageType, payload)
				}
				want, ok := expected[payload[0]]
				if !ok {
					t.Fatalf("unexpected output channel %#x for tty=%v", payload[0], tty)
				}
				if string(payload[1:]) != want {
					t.Fatalf("channel %#x payload=%q", payload[0], payload[1:])
				}
				delete(expected, payload[0])
			}
			zero := 0
			base.terminal <- actionservice.ExecTerminal{
				Type: actionservice.ExecTerminalExit, ExitCode: &zero, Reason: actionservice.ExecExitCompleted, CloseCode: 1000,
			}
			readExecTerminalAndClose(t, ctx, connection, "exit", websocket.StatusNormalClosure)
			if stdoutReader.ReadCount() == 0 {
				t.Fatal("stdout reader was not consumed")
			}
			if tty && stderrReader.ReadCount() != 0 {
				t.Fatal("TTY session consumed the separate stderr stream")
			}
			if !tty && stderrReader.ReadCount() == 0 {
				t.Fatal("non-TTY session did not consume stderr")
			}
		})
	}
}

func TestExecStreamEmitsTerminalBeforeEveryCloseMapping(t *testing.T) {
	zero := 0
	for _, test := range []struct {
		name      string
		terminal  actionservice.ExecTerminal
		control   string
		closeCode websocket.StatusCode
	}{
		{
			name: "normal exit", control: "exit", closeCode: websocket.StatusNormalClosure,
			terminal: actionservice.ExecTerminal{Type: actionservice.ExecTerminalExit, ExitCode: &zero, Reason: actionservice.ExecExitCompleted, CloseCode: 1000},
		},
		{
			name: "lifecycle invalidation", control: "error", closeCode: websocket.StatusGoingAway,
			terminal: actionservice.ExecTerminal{Type: actionservice.ExecTerminalError, Code: actionservice.CodeGenerationChanged, Message: "The active generation changed.", CloseCode: 1001},
		},
		{
			name: "policy violation", control: "error", closeCode: websocket.StatusPolicyViolation,
			terminal: actionservice.ExecTerminal{Type: actionservice.ExecTerminalError, Code: actionservice.CodeForbidden, Message: "Kubernetes denied this operation.", CloseCode: 1008},
		},
		{
			name: "message too large", control: "error", closeCode: websocket.StatusMessageTooBig,
			terminal: actionservice.ExecTerminal{Type: actionservice.ExecTerminalError, Code: actionservice.CodeProtocolViolation, Message: "The exec message was too large.", CloseCode: 1009},
		},
		{
			name: "upstream failure", control: "error", closeCode: websocket.StatusInternalError,
			terminal: actionservice.ExecTerminal{Type: actionservice.ExecTerminalError, Code: actionservice.CodeExecUpstreamError, Message: "The exec upstream failed.", CloseCode: 1011},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newExecTestService()
			server, websocketURL := newExecWireServer(t, service, "exec_close", execStreamOptions{
				heartbeatInterval: time.Hour,
				heartbeatTimeout:  time.Second,
				pingInterval:      time.Hour,
				pingTimeout:       time.Second,
				writeTimeout:      time.Second,
			})
			connection := dialExecWire(t, server.URL, websocketURL, []string{
				actionservice.ExecWebSocketProtocol,
				actionservice.ExecTicketPrefix + "ticket",
			})
			defer connection.CloseNow()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			readExecReady(t, ctx, connection)
			service.terminal <- test.terminal
			readExecTerminalAndClose(t, ctx, connection, test.control, test.closeCode)
		})
	}
}

type exactOfferExecService struct {
	*execTestService
	expected []string
}

func (service *exactOfferExecService) AuthorizeUpgrade(ctx context.Context, binding namespaces.SelectionBinding, sessionID string, offered []string) (actionservice.ExecGrant, error) {
	if !slices.Equal(offered, service.expected) {
		return actionservice.ExecGrant{}, &actionservice.Error{
			Code: actionservice.CodeValidationFailed, Message: "The action request is invalid.", HTTPStatus: http.StatusBadRequest,
		}
	}
	return service.execTestService.AuthorizeUpgrade(ctx, binding, sessionID, offered)
}

type disconnectAwareExecService struct{ *execTestService }

func (service *disconnectAwareExecService) Cancel(sessionID string) error {
	if err := service.execTestService.Cancel(sessionID); err != nil {
		return err
	}
	select {
	case service.terminal <- actionservice.ExecTerminal{Type: actionservice.ExecTerminalExit, Reason: actionservice.ExecExitCanceled, CloseCode: 1000}:
	default:
	}
	return nil
}

type outputExecService struct {
	*execTestService
	tty    bool
	remote *outputExecRemote
}

func (service *outputExecService) Start(_ context.Context, grant actionservice.ExecGrant) (actionservice.ActiveExec, error) {
	return actionservice.ActiveExec{
		SessionID: grant.SessionID, Generation: grant.Generation, TTY: service.tty, Stdin: false,
		Remote: service.remote, Terminal: service.terminal,
	}, nil
}

type outputExecRemote struct {
	stdin  *execTestWriteCloser
	stdout io.Reader
	stderr io.Reader
}

type outputExecReader struct {
	mu     sync.Mutex
	reader *strings.Reader
	reads  int
}

func newOutputExecReader(value string) *outputExecReader {
	return &outputExecReader{reader: strings.NewReader(value)}
}

func (reader *outputExecReader) Read(payload []byte) (int, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.reads++
	return reader.reader.Read(payload)
}

func (reader *outputExecReader) ReadCount() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.reads
}

func (remote *outputExecRemote) Stdin() io.WriteCloser { return remote.stdin }
func (remote *outputExecRemote) Stdout() io.Reader     { return remote.stdout }
func (remote *outputExecRemote) Stderr() io.Reader     { return remote.stderr }
func (*outputExecRemote) Resize(context.Context, uint16, uint16) error {
	return nil
}

func newExecWireServer(t *testing.T, service ExecBridgeService, sessionID string, options execStreamOptions) (*httptest.Server, string) {
	t.Helper()
	selection := execTestSelection{binding: namespaces.SelectionBinding{
		ClusterProfileID: 1, Context: "dev", Generation: "gen_wire",
	}}
	handler := newExecStream(service, selection, "placeholder", options)
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/exec/{sessionId}/stream", handler)
	server := httptest.NewServer(mux)
	handler.origin = server.URL
	t.Cleanup(server.Close)
	return server, "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/exec/" + sessionID + "/stream"
}

func dialExecWire(t *testing.T, origin, websocketURL string, protocols []string) *websocket.Conn {
	t.Helper()
	connection, response, err := websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{
		Subprotocols: protocols,
		HTTPHeader:   http.Header{"Origin": []string{origin}},
	})
	if err != nil {
		if response != nil {
			t.Fatalf("dial status=%d err=%v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	return connection
}

func readExecReady(t *testing.T, ctx context.Context, connection *websocket.Conn) {
	t.Helper()
	messageType, payload, err := connection.Read(ctx)
	if err != nil || messageType != websocket.MessageText {
		t.Fatalf("ready type=%v payload=%q err=%v", messageType, payload, err)
	}
	var ready readyControl
	if err := json.Unmarshal(payload, &ready); err != nil || ready.Type != "ready" {
		t.Fatalf("ready payload=%s err=%v", payload, err)
	}
}

func finishExecWire(t *testing.T, ctx context.Context, connection *websocket.Conn, service *execTestService, terminal actionservice.ExecTerminal) {
	t.Helper()
	zero := 0
	if terminal.Type == actionservice.ExecTerminalExit && terminal.ExitCode == nil {
		terminal.ExitCode = &zero
	}
	service.terminal <- terminal
	readExecTerminalAndClose(t, ctx, connection, string(terminal.Type), websocket.StatusCode(terminal.CloseCode))
}

func readExecTerminalAndClose(t *testing.T, ctx context.Context, connection *websocket.Conn, controlType string, closeCode websocket.StatusCode) {
	t.Helper()
	for {
		messageType, payload, err := connection.Read(ctx)
		if err != nil {
			t.Fatalf("terminal type=%v payload=%q err=%v", messageType, payload, err)
		}
		if messageType != websocket.MessageText {
			continue
		}
		var discriminator struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(payload, &discriminator) == nil && discriminator.Type == controlType {
			break
		}
	}
	_, _, err := connection.Read(ctx)
	if status := websocket.CloseStatus(err); status != closeCode {
		t.Fatalf("close status=%d want=%d err=%v", status, closeCode, err)
	}
}

func rawExecUpgrade(t *testing.T, origin, websocketURL string) (net.Conn, *bufio.Reader) {
	t.Helper()
	parsed, err := url.Parse(websocketURL)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", parsed.Host, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		connection.Close()
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(connection,
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nOrigin: %s\r\nSec-WebSocket-Protocol: %s, %sticket\r\n\r\n",
		parsed.RequestURI(), parsed.Host, origin, actionservice.ExecWebSocketProtocol, actionservice.ExecTicketPrefix,
	)
	if err != nil {
		connection.Close()
		t.Fatal(err)
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		connection.Close()
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols || response.Header.Get("Sec-WebSocket-Protocol") != actionservice.ExecWebSocketProtocol {
		connection.Close()
		t.Fatalf("upgrade status=%d protocol=%q", response.StatusCode, response.Header.Get("Sec-WebSocket-Protocol"))
	}
	return connection, reader
}

func readRawWebSocketFrame(reader *bufio.Reader) (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, nil, err
	}
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var encoded [2]byte
		if _, err := io.ReadFull(reader, encoded[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(encoded[:]))
	case 127:
		var encoded [8]byte
		if _, err := io.ReadFull(reader, encoded[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(encoded[:])
	}
	if length > 1<<20 {
		return 0, nil, fmt.Errorf("unexpected raw frame length %d", length)
	}
	var mask [4]byte
	if header[1]&0x80 != 0 {
		if _, err := io.ReadFull(reader, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, err
	}
	if header[1]&0x80 != 0 {
		for index := range payload {
			payload[index] ^= mask[index%len(mask)]
		}
	}
	return header[0] & 0x0f, payload, nil
}

func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not reached before timeout")
}

var _ ExecBridgeService = (*exactOfferExecService)(nil)
var _ ExecBridgeService = (*disconnectAwareExecService)(nil)
var _ ExecBridgeService = (*outputExecService)(nil)
var _ actionservice.ExecStreams = (*outputExecRemote)(nil)
