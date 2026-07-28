package spike

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/ginger/pkg/ws"
)

func TestGingerWebSocketIgnoresOpcodeFINAndResizeSemantics(t *testing.T) {
	testCases := []struct {
		name    string
		fin     bool
		opcode  byte
		payload map[string]any
	}{
		{
			name:   "binary opcode accepted as JSON",
			fin:    true,
			opcode: 0x2,
			payload: map[string]any{
				"type": "binary",
			},
		},
		{
			name:   "non-final text delivered before continuation",
			fin:    false,
			opcode: 0x1,
			payload: map[string]any{
				"type": "fragment-without-final",
			},
		},
		{
			name:   "resize is only untyped application JSON",
			fin:    true,
			opcode: 0x1,
			payload: map[string]any{
				"type": "resize",
				"cols": float64(120),
				"rows": float64(40),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			received := make(chan map[string]any, 1)
			connection, stop := startRawGingerWebSocket(t, func(server *ws.Conn) {
				var message map[string]any
				if err := server.Recv(&message); err == nil {
					received <- message
				}
			})
			defer stop()
			defer connection.Close()

			payload, err := json.Marshal(testCase.payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			if err := writeClientFrame(connection, testCase.fin, testCase.opcode, payload); err != nil {
				t.Fatalf("write frame: %v", err)
			}

			select {
			case message := <-received:
				if message["type"] != testCase.payload["type"] {
					t.Fatalf("received unexpected message: %#v", message)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Ginger did not deliver the protocol-invalid frame")
			}
		})
	}
}

func TestGingerWebSocketTreatsPingAsJSONAndDoesNotPong(t *testing.T) {
	received := make(chan map[string]any, 1)
	release := make(chan struct{})
	connection, stop := startRawGingerWebSocket(t, func(server *ws.Conn) {
		var message map[string]any
		if err := server.Recv(&message); err == nil {
			received <- message
		}
		<-release
	})
	defer stop()
	defer connection.Close()

	if err := writeClientFrame(
		connection,
		true,
		0x9,
		[]byte(`{"type":"ping-as-json"}`),
	); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	select {
	case message := <-received:
		if message["type"] != "ping-as-json" {
			t.Fatalf("ping became unexpected message: %#v", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ginger did not expose ping payload to JSON handler")
	}

	if err := connection.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var oneByte [1]byte
	_, err := connection.Read(oneByte[:])
	var networkError net.Error
	if !errors.As(err, &networkError) || !networkError.Timeout() {
		t.Fatalf("expected no automatic pong before deadline, got %v", err)
	}
	close(release)
}

func TestGingerWebSocketHasNoHeartbeatAndReportsDisconnect(t *testing.T) {
	recvResult := make(chan error, 1)
	connection, stop := startRawGingerWebSocket(t, func(server *ws.Conn) {
		var message map[string]any
		recvResult <- server.Recv(&message)
	})
	defer stop()

	if err := connection.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var oneByte [1]byte
	_, err := connection.Read(oneByte[:])
	var networkError net.Error
	if !errors.As(err, &networkError) || !networkError.Timeout() {
		t.Fatalf("expected idle connection without heartbeat, got %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	select {
	case err := <-recvResult:
		if err == nil {
			t.Fatal("disconnect returned nil from Recv")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server Recv did not observe client disconnect")
	}
}

func TestGingerWebSocketLargeFrameHasNoExplicitBudgetAndIsTruncated(t *testing.T) {
	recvResult := make(chan error, 1)
	connection, stop := startRawGingerWebSocket(t, func(server *ws.Conn) {
		var message map[string]any
		recvResult <- server.Recv(&message)
	})
	defer stop()

	payload, err := json.Marshal(map[string]string{
		"value": strings.Repeat("x", 128<<10),
	})
	if err != nil {
		t.Fatalf("marshal large payload: %v", err)
	}
	if err := writeClientFrame(connection, true, 0x1, payload); err != nil {
		t.Fatalf("write large frame: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}

	select {
	case err := <-recvResult:
		if err == nil {
			t.Fatal("large frame unexpectedly decoded without an explicit budget")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("large frame left Recv blocked after disconnect")
	}
}

func TestGingerWebSocketSurvivesLongerThanServerWriteTimeout(t *testing.T) {
	if os.Getenv("KUBEPEEP_LONG_TEST") != "1" {
		t.Skip("set KUBEPEEP_LONG_TEST=1 to execute the 16-second WebSocket proof")
	}

	received := make(chan map[string]any, 1)
	connection, stop := startRawGingerWebSocket(t, func(server *ws.Conn) {
		var message map[string]any
		if err := server.Recv(&message); err == nil {
			received <- message
		}
	})
	defer stop()
	defer connection.Close()

	time.Sleep(16 * time.Second)
	if err := writeClientFrame(
		connection,
		true,
		0x1,
		[]byte(`{"type":"after-16-seconds"}`),
	); err != nil {
		t.Fatalf("write delayed frame: %v", err)
	}
	select {
	case message := <-received:
		if message["type"] != "after-16-seconds" {
			t.Fatalf("received unexpected delayed message: %#v", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WebSocket did not survive for 16 seconds")
	}
}

func startRawGingerWebSocket(t *testing.T, handler func(*ws.Conn)) (net.Conn, func()) {
	t.Helper()
	application := newGingerApp(t)
	application.Router.HandleRaw("GET /ws-matrix", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws.Handle(w, r, handler)
	}))
	localRuntime, err := NewRuntime(application, 2748, 50)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	runContext, cancelRun := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- localRuntime.Run(runContext)
	}()

	readyContext, cancelReady := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelReady()
	if err := localRuntime.WaitReady(readyContext); err != nil {
		cancelRun()
		t.Fatalf("wait ready: %v", err)
	}

	connection, err := net.Dial("tcp4", localRuntime.Listener.Addr().String())
	if err != nil {
		cancelRun()
		t.Fatalf("dial websocket: %v", err)
	}
	_, _ = io.WriteString(
		connection,
		"GET /ws-matrix HTTP/1.1\r\n"+
			"Host: "+localRuntime.Listener.Addr().String()+"\r\n"+
			"Connection: Upgrade\r\n"+
			"Upgrade: websocket\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"+
			"Origin: http://"+localRuntime.Listener.Addr().String()+"\r\n\r\n",
	)
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = connection.Close()
		cancelRun()
		t.Fatalf("read handshake: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		_ = connection.Close()
		cancelRun()
		t.Fatalf("handshake returned %d", response.StatusCode)
	}

	stop := func() {
		_ = connection.Close()
		cancelRun()
		select {
		case err := <-result:
			if err != nil {
				t.Errorf("stop runtime: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("runtime did not stop")
		}
	}
	return connection, stop
}

func writeClientFrame(connection net.Conn, fin bool, opcode byte, payload []byte) error {
	first := opcode & 0x0f
	if fin {
		first |= 0x80
	}
	header := []byte{first}
	switch {
	case len(payload) < 126:
		header = append(header, 0x80|byte(len(payload)))
	case len(payload) <= 0xffff:
		header = append(header, 0x80|126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(len(payload)))
	default:
		header = append(header, 0x80|127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(len(payload)))
	}
	mask := [4]byte{0x11, 0x22, 0x33, 0x44}
	header = append(header, mask[:]...)
	maskedPayload := append([]byte(nil), payload...)
	for index := range maskedPayload {
		maskedPayload[index] ^= mask[index%len(mask)]
	}
	if _, err := connection.Write(header); err != nil {
		return err
	}
	_, err := connection.Write(maskedPayload)
	return err
}
