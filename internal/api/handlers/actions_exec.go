package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/fvmoraes/kubepeep/internal/api"
	actionservice "github.com/fvmoraes/kubepeep/internal/services/actions"
)

const (
	execHeartbeatInterval = 15 * time.Second
	execHeartbeatTimeout  = 10 * time.Second
	execPingInterval      = 15 * time.Second
	execPingTimeout       = 10 * time.Second
	execWriteTimeout      = 10 * time.Second
	execTerminalTimeout   = 2 * time.Second
	execOutboundMessages  = 64
	execOutboundBytes     = 1 << 20
)

type ExecBridgeService interface {
	actionservice.ExecService
	Abort(string, actionservice.ExecAbortReason) error
	ReleaseUpgrade(actionservice.ExecGrant) error
}

type execStreamOptions struct {
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
	pingInterval      time.Duration
	pingTimeout       time.Duration
	writeTimeout      time.Duration
}

type ExecStream struct {
	service   ExecBridgeService
	selection SelectionReader
	origin    string
	options   execStreamOptions
}

func NewExecStream(service ExecBridgeService, selection SelectionReader, origin string) *ExecStream {
	return newExecStream(service, selection, origin, execStreamOptions{
		heartbeatInterval: execHeartbeatInterval,
		heartbeatTimeout:  execHeartbeatTimeout,
		pingInterval:      execPingInterval,
		pingTimeout:       execPingTimeout,
		writeTimeout:      execWriteTimeout,
	})
}

func newExecStream(service ExecBridgeService, selection SelectionReader, origin string, options execStreamOptions) *ExecStream {
	return &ExecStream{service: service, selection: selection, origin: origin, options: options}
}

func (handler *ExecStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handler == nil || handler.service == nil || handler.selection == nil || handler.origin == "" {
		api.WriteError(w, r, api.NewHTTPError(http.StatusServiceUnavailable, api.CodeInternal, "The exec transport is unavailable.", nil, nil))
		return
	}
	sessionID := r.PathValue("sessionId")
	protocols, err := offeredWebSocketProtocols(r)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	binding, _ := handler.selection.Snapshot()
	grant, err := handler.service.AuthorizeUpgrade(r.Context(), binding, sessionID, protocols)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:    []string{actionservice.ExecWebSocketProtocol},
		OriginPatterns:  []string{handler.origin},
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		_ = handler.service.ReleaseUpgrade(grant)
		return
	}
	defer connection.CloseNow()
	if connection.Subprotocol() != actionservice.ExecWebSocketProtocol {
		_ = handler.service.ReleaseUpgrade(grant)
		_ = connection.Close(websocket.StatusPolicyViolation, string(actionservice.CodeProtocolViolation))
		return
	}
	active, err := handler.service.Start(r.Context(), grant)
	if err != nil {
		handler.writeStartFailure(connection, err)
		return
	}
	handler.serveActive(r.Context(), connection, active)
}

func offeredWebSocketProtocols(r *http.Request) ([]string, error) {
	values := r.Header.Values("Sec-WebSocket-Protocol")
	protocols := make([]string, 0, 2)
	for _, value := range values {
		for _, protocol := range strings.Split(value, ",") {
			protocol = strings.TrimSpace(protocol)
			if protocol == "" {
				return nil, &actionservice.Error{Code: actionservice.CodeValidationFailed, Message: "The action request is invalid.", HTTPStatus: http.StatusBadRequest}
			}
			protocols = append(protocols, protocol)
		}
	}
	return protocols, nil
}

type readyControl struct {
	Type       string `json:"type"`
	SessionID  string `json:"sessionId"`
	Generation string `json:"generation"`
	TTY        bool   `json:"tty"`
	Stdin      bool   `json:"stdin"`
}

type heartbeatControl struct {
	Type  string `json:"type"`
	Nonce string `json:"nonce"`
}

type exitControl struct {
	Type     string                       `json:"type"`
	ExitCode *int                         `json:"exitCode"`
	Reason   actionservice.ExecExitReason `json:"reason"`
}

type errorControl struct {
	Type      string                  `json:"type"`
	Code      actionservice.ErrorCode `json:"code"`
	Message   string                  `json:"message"`
	Retryable bool                    `json:"retryable"`
}

func (handler *ExecStream) serveActive(parent context.Context, connection *websocket.Conn, active actionservice.ActiveExec) {
	connection.SetReadLimit(actionservice.MaximumExecDataMessageBytes)
	ready, _ := json.Marshal(readyControl{
		Type:       "ready",
		SessionID:  active.SessionID,
		Generation: active.Generation,
		TTY:        active.TTY,
		Stdin:      active.Stdin,
	})
	if err := writeWebSocket(parent, connection, websocket.MessageText, ready, handler.options.writeTimeout); err != nil {
		_ = handler.service.Cancel(active.SessionID)
		return
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	queue := newExecOutboundQueue()
	writerFailure := make(chan error, 1)
	go handler.writeLoop(ctx, connection, queue, writerFailure)

	heartbeats := &execHeartbeatState{}
	readResult := make(chan execBridgeFailure, 1)
	go func() { readResult <- handler.readLoop(ctx, connection, active, heartbeats) }()
	go handler.outputLoop(ctx, active, active.Remote.Stdout(), 0x01, queue, readResult)
	if !active.TTY {
		go handler.outputLoop(ctx, active, active.Remote.Stderr(), 0x02, queue, readResult)
	}
	go handler.heartbeatLoop(ctx, active.SessionID, heartbeats, queue, readResult)
	go handler.pingLoop(ctx, connection, readResult)

	var terminal actionservice.ExecTerminal
	writable := true
	select {
	case terminal = <-active.Terminal:
	case failure := <-readResult:
		writable = !failure.disconnected
		if failure.requestedClose {
			_ = handler.service.Cancel(active.SessionID)
		} else if failure.disconnected {
			_ = handler.service.Cancel(active.SessionID)
		} else {
			_ = handler.service.Abort(active.SessionID, failure.reason)
		}
		terminal = waitExecTerminal(active.Terminal)
	case <-writerFailure:
		writable = false
		_ = handler.service.Cancel(active.SessionID)
		terminal = waitExecTerminal(active.Terminal)
	case <-parent.Done():
		writable = false
		_ = handler.service.Cancel(active.SessionID)
		terminal = waitExecTerminal(active.Terminal)
	}
	if terminal.Type == "" {
		terminal = actionservice.ExecTerminal{Type: actionservice.ExecTerminalError, Code: actionservice.CodeInternal, Message: "The action failed internally.", CloseCode: 1011}
	}
	if !writable {
		return
	}
	payload, messageType := encodeExecTerminal(terminal)
	done := make(chan error, 1)
	if !queue.terminal(execOutboundMessage{messageType: messageType, payload: payload, done: done}) {
		return
	}
	select {
	case <-done:
	case <-time.After(execTerminalTimeout):
		return
	}
	_ = connection.Close(websocket.StatusCode(terminal.CloseCode), closeReason(terminal))
}

type execBridgeFailure struct {
	reason         actionservice.ExecAbortReason
	disconnected   bool
	requestedClose bool
}

func (handler *ExecStream) readLoop(ctx context.Context, connection *websocket.Conn, active actionservice.ActiveExec, heartbeats *execHeartbeatState) execBridgeFailure {
	stdinClosed := !active.Stdin
	for {
		messageType, payload, err := connection.Read(ctx)
		if err != nil {
			if errors.Is(err, websocket.ErrMessageTooBig) {
				return execBridgeFailure{reason: actionservice.ExecAbortMessageTooLarge}
			}
			return execBridgeFailure{disconnected: true}
		}
		switch messageType {
		case websocket.MessageBinary:
			if !active.Stdin || stdinClosed || len(payload) < 1 || len(payload) > actionservice.MaximumExecDataMessageBytes || payload[0] != 0x00 {
				return execBridgeFailure{reason: actionservice.ExecAbortProtocolViolation}
			}
			if len(payload) > 1 {
				n, writeErr := active.Remote.Stdin().Write(payload[1:])
				if writeErr != nil || n != len(payload)-1 {
					return execBridgeFailure{reason: actionservice.ExecAbortInternal}
				}
				_ = handler.service.Touch(active.SessionID)
			}
		case websocket.MessageText:
			control, decodeErr := decodeExecClientControl(payload)
			if decodeErr != nil {
				return execBridgeFailure{reason: actionservice.ExecAbortProtocolViolation}
			}
			switch value := control.(type) {
			case resizeClientControl:
				if !active.TTY || value.Columns < 1 || value.Columns > 4096 || value.Rows < 1 || value.Rows > 4096 {
					return execBridgeFailure{reason: actionservice.ExecAbortProtocolViolation}
				}
				if err := active.Remote.Resize(ctx, uint16(value.Columns), uint16(value.Rows)); err != nil {
					return execBridgeFailure{reason: actionservice.ExecAbortInternal}
				}
				_ = handler.service.Touch(active.SessionID)
			case heartbeatClientControl:
				if !validHeartbeatNonce(value.Nonce) || !heartbeats.ack(value.Nonce) {
					return execBridgeFailure{reason: actionservice.ExecAbortProtocolViolation}
				}
			case closeClientControl:
				switch value.Stream {
				case "stdin":
					if !active.Stdin || stdinClosed {
						return execBridgeFailure{reason: actionservice.ExecAbortProtocolViolation}
					}
					stdinClosed = true
					if err := active.Remote.Stdin().Close(); err != nil {
						return execBridgeFailure{reason: actionservice.ExecAbortInternal}
					}
					_ = handler.service.Touch(active.SessionID)
				case "session":
					return execBridgeFailure{requestedClose: true}
				default:
					return execBridgeFailure{reason: actionservice.ExecAbortProtocolViolation}
				}
			default:
				return execBridgeFailure{reason: actionservice.ExecAbortProtocolViolation}
			}
		default:
			return execBridgeFailure{reason: actionservice.ExecAbortProtocolViolation}
		}
	}
}

func (handler *ExecStream) outputLoop(ctx context.Context, active actionservice.ActiveExec, reader io.Reader, stream byte, queue *execOutboundQueue, failures chan<- execBridgeFailure) {
	buffer := make([]byte, actionservice.MaximumExecDataMessageBytes-1)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			payload := make([]byte, n+1)
			payload[0] = stream
			copy(payload[1:], buffer[:n])
			if !queue.enqueue(execOutboundMessage{messageType: websocket.MessageBinary, payload: payload}) {
				publishExecFailure(failures, execBridgeFailure{reason: actionservice.ExecAbortBackpressure})
				return
			}
			_ = handler.service.Touch(active.SessionID)
		}
		if err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (handler *ExecStream) heartbeatLoop(ctx context.Context, sessionID string, state *execHeartbeatState, queue *execOutboundQueue, failures chan<- execBridgeFailure) {
	ticker := time.NewTicker(handler.options.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			nonce, err := newHeartbeatNonce()
			if err != nil {
				publishExecFailure(failures, execBridgeFailure{reason: actionservice.ExecAbortInternal})
				return
			}
			ack := state.arm(nonce)
			payload, _ := json.Marshal(heartbeatControl{Type: "heartbeat", Nonce: nonce})
			if !queue.enqueue(execOutboundMessage{messageType: websocket.MessageText, payload: payload}) {
				state.clear(nonce)
				publishExecFailure(failures, execBridgeFailure{reason: actionservice.ExecAbortBackpressure})
				return
			}
			timer := time.NewTimer(handler.options.heartbeatTimeout)
			select {
			case <-ack:
				if !timer.Stop() {
					<-timer.C
				}
			case <-timer.C:
				state.clear(nonce)
				publishExecFailure(failures, execBridgeFailure{reason: actionservice.ExecAbortProtocolViolation})
				return
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (handler *ExecStream) pingLoop(ctx context.Context, connection *websocket.Conn, failures chan<- execBridgeFailure) {
	ticker := time.NewTicker(handler.options.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			pingContext, cancel := context.WithTimeout(ctx, handler.options.pingTimeout)
			err := connection.Ping(pingContext)
			cancel()
			if err != nil {
				publishExecFailure(failures, execBridgeFailure{disconnected: true})
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (handler *ExecStream) writeLoop(ctx context.Context, connection *websocket.Conn, queue *execOutboundQueue, failed chan<- error) {
	for {
		select {
		case message := <-queue.messages:
			err := writeWebSocket(ctx, connection, message.messageType, message.payload, handler.options.writeTimeout)
			queue.release(len(message.payload))
			if message.done != nil {
				message.done <- err
				close(message.done)
			}
			if err != nil {
				select {
				case failed <- err:
				default:
				}
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

type execOutboundMessage struct {
	messageType websocket.MessageType
	payload     []byte
	done        chan error
}

type execOutboundQueue struct {
	messages    chan execOutboundMessage
	mu          sync.Mutex
	bytes       int
	terminalSet bool
}

func newExecOutboundQueue() *execOutboundQueue {
	return &execOutboundQueue{messages: make(chan execOutboundMessage, execOutboundMessages)}
}

func (queue *execOutboundQueue) enqueue(message execOutboundMessage) bool {
	message.payload = append([]byte(nil), message.payload...)
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.terminalSet || queue.bytes+len(message.payload) > execOutboundBytes {
		return false
	}
	select {
	case queue.messages <- message:
		queue.bytes += len(message.payload)
		return true
	default:
		return false
	}
}

func (queue *execOutboundQueue) terminal(message execOutboundMessage) bool {
	message.payload = append([]byte(nil), message.payload...)
	queue.mu.Lock()
	if queue.terminalSet {
		queue.mu.Unlock()
		return false
	}
	queue.terminalSet = true
	queue.mu.Unlock()
	timer := time.NewTimer(execTerminalTimeout)
	defer timer.Stop()
	select {
	case queue.messages <- message:
		queue.mu.Lock()
		queue.bytes += len(message.payload)
		queue.mu.Unlock()
		return true
	case <-timer.C:
		return false
	}
}

func (queue *execOutboundQueue) release(size int) {
	queue.mu.Lock()
	queue.bytes -= size
	if queue.bytes < 0 {
		queue.bytes = 0
	}
	queue.mu.Unlock()
}

type execHeartbeatState struct {
	mu           sync.Mutex
	expected     string
	acknowledged chan struct{}
}

func (state *execHeartbeatState) arm(nonce string) <-chan struct{} {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.expected = nonce
	state.acknowledged = make(chan struct{})
	return state.acknowledged
}

func (state *execHeartbeatState) ack(nonce string) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if nonce == "" || state.expected != nonce || state.acknowledged == nil {
		return false
	}
	close(state.acknowledged)
	state.expected = ""
	state.acknowledged = nil
	return true
}

func (state *execHeartbeatState) clear(nonce string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.expected == nonce {
		state.expected = ""
		state.acknowledged = nil
	}
}

type resizeClientControl struct {
	Type    string `json:"type"`
	Columns int    `json:"columns"`
	Rows    int    `json:"rows"`
}

type heartbeatClientControl struct {
	Type  string `json:"type"`
	Nonce string `json:"nonce"`
}

type closeClientControl struct {
	Type   string `json:"type"`
	Stream string `json:"stream"`
}

func decodeExecClientControl(payload []byte) (any, error) {
	if len(payload) == 0 || len(payload) > actionservice.MaximumExecControlMessageBytes || !utf8.Valid(payload) {
		return nil, errors.New("invalid exec control")
	}
	var discriminator struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &discriminator); err != nil {
		return nil, errors.New("invalid exec control")
	}
	var destination any
	switch discriminator.Type {
	case "resize":
		destination = &resizeClientControl{}
	case "heartbeat":
		destination = &heartbeatClientControl{}
	case "close":
		destination = &closeClientControl{}
	default:
		return nil, errors.New("invalid exec control")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return nil, errors.New("invalid exec control")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid exec control")
	}
	switch value := destination.(type) {
	case *resizeClientControl:
		return *value, nil
	case *heartbeatClientControl:
		return *value, nil
	case *closeClientControl:
		return *value, nil
	default:
		return nil, errors.New("invalid exec control")
	}
}

func validHeartbeatNonce(nonce string) bool {
	if nonce == "" || len(nonce) > 64 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(nonce)
	return err == nil
}

func newHeartbeatNonce() (string, error) {
	payload := make([]byte, 18)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	return "hb_" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func writeWebSocket(parent context.Context, connection *websocket.Conn, messageType websocket.MessageType, payload []byte, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return connection.Write(ctx, messageType, payload)
}

func publishExecFailure(channel chan<- execBridgeFailure, failure execBridgeFailure) {
	select {
	case channel <- failure:
	default:
	}
}

func waitExecTerminal(channel <-chan actionservice.ExecTerminal) actionservice.ExecTerminal {
	timer := time.NewTimer(execTerminalTimeout)
	defer timer.Stop()
	select {
	case terminal, ok := <-channel:
		if ok {
			return terminal
		}
	case <-timer.C:
	}
	return actionservice.ExecTerminal{}
}

func encodeExecTerminal(terminal actionservice.ExecTerminal) ([]byte, websocket.MessageType) {
	if terminal.Type == actionservice.ExecTerminalExit {
		payload, _ := json.Marshal(exitControl{Type: "exit", ExitCode: terminal.ExitCode, Reason: terminal.Reason})
		return payload, websocket.MessageText
	}
	payload, _ := json.Marshal(errorControl{Type: "error", Code: terminal.Code, Message: terminal.Message, Retryable: terminal.Retryable})
	return payload, websocket.MessageText
}

func closeReason(terminal actionservice.ExecTerminal) string {
	if terminal.Type == actionservice.ExecTerminalError {
		return string(terminal.Code)
	}
	return ""
}

func (handler *ExecStream) writeStartFailure(connection *websocket.Conn, err error) {
	terminal := terminalForActionError(err)
	payload, messageType := encodeExecTerminal(terminal)
	_ = writeWebSocket(context.Background(), connection, messageType, payload, handler.options.writeTimeout)
	_ = connection.Close(websocket.StatusCode(terminal.CloseCode), closeReason(terminal))
}

func terminalForActionError(err error) actionservice.ExecTerminal {
	var actionError *actionservice.Error
	if !errors.As(err, &actionError) || actionError == nil {
		return actionservice.ExecTerminal{Type: actionservice.ExecTerminalError, Code: actionservice.CodeInternal, Message: "The action failed internally.", CloseCode: 1011}
	}
	closeCode := 1011
	switch actionError.Code {
	case actionservice.CodeGenerationChanged, actionservice.CodeExecIdleTimeout, actionservice.CodeExecDurationLimit, actionservice.CodeServerShutdown:
		closeCode = 1001
	case actionservice.CodeForbidden, actionservice.CodeAuthorizationUnavailable, actionservice.CodeExecTargetGone, actionservice.CodeProtocolViolation:
		closeCode = 1008
	case actionservice.CodeLimitExceeded:
		closeCode = 1008
	}
	return actionservice.ExecTerminal{
		Type:      actionservice.ExecTerminalError,
		Code:      actionError.Code,
		Message:   actionError.Message,
		Retryable: actionError.Retryable,
		CloseCode: closeCode,
	}
}

var _ http.Handler = (*ExecStream)(nil)
