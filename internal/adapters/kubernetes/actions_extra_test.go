package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/actions"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/httpstream"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type fakeRequestURL string

func (url fakeRequestURL) String() string { return string(url) }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("synthetic read failure") }

type fakeSPDYStream struct {
	reader     io.Reader
	writer     io.Writer
	headers    http.Header
	mu         sync.Mutex
	closeCount int
	resetCount int
}

func (stream *fakeSPDYStream) Read(buffer []byte) (int, error) {
	return stream.reader.Read(buffer)
}

func (stream *fakeSPDYStream) Write(buffer []byte) (int, error) {
	return stream.writer.Write(buffer)
}

func (stream *fakeSPDYStream) Close() error {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	stream.closeCount++
	return nil
}

func (stream *fakeSPDYStream) Reset() error {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	stream.resetCount++
	return nil
}

func (stream *fakeSPDYStream) Headers() http.Header { return stream.headers }
func (stream *fakeSPDYStream) Identifier() uint32   { return 1 }

func (stream *fakeSPDYStream) closeCountValue() int {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.closeCount
}

func (stream *fakeSPDYStream) resetCountValue() int {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.resetCount
}

// selfClosingStream returns a stream whose reads end immediately, mimicking an
// upstream that closed its direction of the multiplexed channel.
func selfClosingStream() *fakeSPDYStream {
	return &fakeSPDYStream{reader: strings.NewReader(""), writer: io.Discard, headers: http.Header{}}
}

func streamStub() func(http.Header) (httpstream.Stream, error) {
	return func(http.Header) (httpstream.Stream, error) { return selfClosingStream(), nil }
}

// pipeStream returns a fake stream backed by one end of an in-memory duplex
// pipe; the test drives the peer end.
func pipeStream() (*fakeSPDYStream, net.Conn) {
	adapterSide, peerSide := net.Pipe()
	return &fakeSPDYStream{reader: adapterSide, writer: adapterSide, headers: http.Header{}}, peerSide
}

type fakeSPDYConnection struct {
	mu         sync.Mutex
	factories  map[string]func(http.Header) (httpstream.Stream, error)
	created    []string
	closeCount int
	closed     chan bool
	closeOnce  sync.Once
}

func newFakeSPDYConnection() *fakeSPDYConnection {
	return &fakeSPDYConnection{
		factories: make(map[string]func(http.Header) (httpstream.Stream, error)),
		closed:    make(chan bool),
	}
}

func (connection *fakeSPDYConnection) CreateStream(headers http.Header) (httpstream.Stream, error) {
	connection.mu.Lock()
	streamType := headers.Get(corev1.StreamType)
	connection.created = append(connection.created, streamType)
	factory := connection.factories[streamType]
	// Libera o mutex antes de invocar a factory: factories de teste podem
	// bloquear (ex.: cancelamento) e não podem travar Close/closeCountValue.
	connection.mu.Unlock()
	if factory == nil {
		return nil, errors.New("synthetic stream refused")
	}
	return factory(headers)
}

func (connection *fakeSPDYConnection) Close() error {
	connection.mu.Lock()
	connection.closeCount++
	connection.mu.Unlock()
	connection.closeOnce.Do(func() { close(connection.closed) })
	return nil
}

func (connection *fakeSPDYConnection) CloseChan() <-chan bool { return connection.closed }

func (connection *fakeSPDYConnection) SetIdleTimeout(time.Duration) {}

func (connection *fakeSPDYConnection) RemoveStreams(...httpstream.Stream) {}

func (connection *fakeSPDYConnection) createdStreamTypes() []string {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return append([]string(nil), connection.created...)
}

func (connection *fakeSPDYConnection) closeCountValue() int {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.closeCount
}

func TestCreateExecStreamsBuildsRequestedStreams(t *testing.T) {
	tests := []struct {
		name     string
		command  actions.ExecCommand
		expected []string
	}{
		{name: "plain", command: actions.ExecCommand{Command: []string{"sh"}}, expected: []string{corev1.StreamTypeError, corev1.StreamTypeStdout, corev1.StreamTypeStderr}},
		{name: "stdin", command: actions.ExecCommand{Command: []string{"sh"}, Stdin: true}, expected: []string{corev1.StreamTypeError, corev1.StreamTypeStdin, corev1.StreamTypeStdout, corev1.StreamTypeStderr}},
		{name: "tty", command: actions.ExecCommand{Command: []string{"sh"}, TTY: true}, expected: []string{corev1.StreamTypeError, corev1.StreamTypeStdout, corev1.StreamTypeResize}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connection := newFakeSPDYConnection()
			for _, streamType := range []string{corev1.StreamTypeError, corev1.StreamTypeStdin, corev1.StreamTypeStdout, corev1.StreamTypeStderr, corev1.StreamTypeResize} {
				connection.factories[streamType] = streamStub()
			}
			streams, err := createExecStreams(context.Background(), connection, test.command)
			if err != nil {
				t.Fatal(err)
			}
			if streams.error == nil || streams.stdout == nil {
				t.Fatalf("mandatory streams missing: %+v", streams)
			}
			created := connection.createdStreamTypes()
			if len(created) != len(test.expected) {
				t.Fatalf("created streams = %v, want %v", created, test.expected)
			}
			for position, streamType := range test.expected {
				if created[position] != streamType {
					t.Fatalf("created streams = %v, want %v", created, test.expected)
				}
			}
			if test.command.Stdin && streams.stdin == nil {
				t.Fatal("stdin stream missing")
			}
			if test.command.TTY && streams.resize == nil {
				t.Fatal("resize stream missing")
			}
			if !test.command.TTY && streams.stderr == nil {
				t.Fatal("stderr stream missing")
			}
		})
	}
}

func TestCreateExecStreamsRejectsProtocolAndCancellation(t *testing.T) {
	errorless := newFakeSPDYConnection()
	errorless.factories[corev1.StreamTypeError] = func(http.Header) (httpstream.Stream, error) { return nil, nil }
	if _, err := createExecStreams(context.Background(), errorless, actions.ExecCommand{}); !errors.Is(err, errExecProtocol) {
		t.Fatalf("missing error stream err = %v", err)
	}

	stdoutFailure := errors.New("stdout refused")
	failing := newFakeSPDYConnection()
	failing.factories[corev1.StreamTypeError] = streamStub()
	failing.factories[corev1.StreamTypeStdout] = func(http.Header) (httpstream.Stream, error) { return nil, stdoutFailure }
	if _, err := createExecStreams(context.Background(), failing, actions.ExecCommand{}); !errors.Is(err, stdoutFailure) {
		t.Fatalf("stdout failure err = %v", err)
	}

	stdinFailure := errors.New("stdin refused")
	failingStdin := newFakeSPDYConnection()
	failingStdin.factories[corev1.StreamTypeError] = streamStub()
	failingStdin.factories[corev1.StreamTypeStdin] = func(http.Header) (httpstream.Stream, error) { return nil, stdinFailure }
	if _, err := createExecStreams(context.Background(), failingStdin, actions.ExecCommand{Stdin: true}); !errors.Is(err, stdinFailure) {
		t.Fatalf("stdin failure err = %v", err)
	}

	stderrFailure := errors.New("stderr refused")
	failingStderr := newFakeSPDYConnection()
	failingStderr.factories[corev1.StreamTypeError] = streamStub()
	failingStderr.factories[corev1.StreamTypeStdout] = streamStub()
	failingStderr.factories[corev1.StreamTypeStderr] = func(http.Header) (httpstream.Stream, error) { return nil, stderrFailure }
	if _, err := createExecStreams(context.Background(), failingStderr, actions.ExecCommand{}); !errors.Is(err, stderrFailure) {
		t.Fatalf("stderr failure err = %v", err)
	}

	resizeFailure := errors.New("resize refused")
	failingResize := newFakeSPDYConnection()
	failingResize.factories[corev1.StreamTypeError] = streamStub()
	failingResize.factories[corev1.StreamTypeStdout] = streamStub()
	failingResize.factories[corev1.StreamTypeResize] = func(http.Header) (httpstream.Stream, error) { return nil, resizeFailure }
	if _, err := createExecStreams(context.Background(), failingResize, actions.ExecCommand{TTY: true}); !errors.Is(err, resizeFailure) {
		t.Fatalf("resize failure err = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	blocked := make(chan struct{})
	blocking := newFakeSPDYConnection()
	blocking.factories[corev1.StreamTypeError] = func(http.Header) (httpstream.Stream, error) {
		<-blocked
		return nil, nil
	}
	t.Cleanup(func() { close(blocked) })
	_, err := createExecStreams(canceled, blocking, actions.ExecCommand{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled setup err = %v", err)
	}
	if blocking.closeCountValue() != 1 {
		t.Fatalf("canceled setup connection close count = %d", blocking.closeCountValue())
	}
}

func TestClientGoRemoteExecStreamsStatusAndResizes(t *testing.T) {
	errorStream, errorPeer := pipeStream()
	stdoutStream, stdoutPeer := pipeStream()
	stderrStream, stderrPeer := pipeStream()
	stdinStream, stdinPeer := pipeStream()
	resizeStream, resizePeer := pipeStream()
	connection := newFakeSPDYConnection()

	lifetime, cancelLifetime := context.WithCancel(context.Background())
	defer cancelLifetime()
	remote := newClientGoRemoteExec(lifetime, connection, execStreams{
		error: errorStream, stdin: stdinStream, stdout: stdoutStream, stderr: stderrStream, resize: resizeStream,
	})

	stdoutComplete := make(chan struct{})
	go func() {
		defer close(stdoutComplete)
		payload, err := io.ReadAll(remote.Stdout())
		if err != nil {
			t.Errorf("stdout read: %v", err)
			return
		}
		if string(payload) != "stdout-payload" {
			t.Errorf("stdout payload = %q", payload)
		}
	}()
	if _, err := stdoutPeer.Write([]byte("stdout-payload")); err != nil {
		t.Fatal(err)
	}
	if _, err := errorPeer.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Success"}`)); err != nil {
		t.Fatal(err)
	}
	if err := errorPeer.Close(); err != nil {
		t.Fatal(err)
	}
	resizeComplete := make(chan error, 1)
	go func() {
		resizeComplete <- remote.Resize(lifetime, 80, 24)
	}()
	resizeBuffer := make([]byte, 32)
	resizeRead, err := resizePeer.Read(resizeBuffer)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-resizeComplete:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("resize did not complete")
	}
	var size struct {
		Width  uint16
		Height uint16
	}
	if err := json.Unmarshal(resizeBuffer[:resizeRead], &size); err != nil {
		t.Fatal(err)
	}
	if size.Width != 80 || size.Height != 24 {
		t.Fatalf("resize frame = %+v", size)
	}

	stdinDrained := make(chan struct{})
	go func() {
		defer close(stdinDrained)
		_, _ = io.Copy(io.Discard, stdinPeer)
	}()
	if _, err := remote.Stdin().Write([]byte("stdin-payload")); err != nil {
		t.Fatal(err)
	}
	stderrComplete := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(remote.Stderr())
		stderrComplete <- err
	}()

	if err := stdoutPeer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stdoutComplete:
	case <-time.After(time.Second):
		t.Fatal("stdout reader did not finish")
	}
	if err := stderrPeer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-stderrComplete:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stderr reader did not finish")
	}
	if err := stdinPeer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stdinDrained:
	case <-time.After(time.Second):
		t.Fatal("stdin drain did not finish")
	}

	result := remote.Wait()
	if result.Err != nil || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("wait result = %+v", result)
	}

	if err := remote.Close(); err != nil {
		t.Fatal(err)
	}
	if connection.closeCountValue() != 1 {
		t.Fatalf("connection close count = %d", connection.closeCountValue())
	}
	if err := remote.Close(); err != nil {
		t.Fatal(err)
	}
	if connection.closeCountValue() != 1 {
		t.Fatal("close is not idempotent")
	}
	_ = resizePeer.Close()
	if err := remote.Resize(lifetime, 80, 24); !errors.Is(err, context.Canceled) {
		t.Fatalf("resize after close err = %v", err)
	}
}

func TestClientGoRemoteExecResizeGuards(t *testing.T) {
	lifetime, cancelLifetime := context.WithCancel(context.Background())
	defer cancelLifetime()
	connection := newFakeSPDYConnection()
	resizeStream, resizePeer := pipeStream()
	t.Cleanup(func() { _ = resizePeer.Close() })
	remote := newClientGoRemoteExec(lifetime, connection, execStreams{error: selfClosingStream(), resize: resizeStream})
	if err := remote.Resize(lifetime, 0, 24); !errors.Is(err, errExecProtocol) {
		t.Fatalf("zero columns err = %v", err)
	}
	if err := remote.Resize(lifetime, 80, 0); !errors.Is(err, errExecProtocol) {
		t.Fatalf("zero rows err = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := remote.Resize(canceled, 80, 24); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled resize err = %v", err)
	}
	remote.Close()
	if err := remote.Resize(lifetime, 80, 24); !errors.Is(err, context.Canceled) {
		t.Fatalf("closed remote resize err = %v", err)
	}
	(*clientGoRemoteExec)(nil).Close()
	if result := (*clientGoRemoteExec)(nil).Wait(); result.Err != errActionsClientUnavailable {
		t.Fatalf("nil remote wait = %+v", result)
	}
}

func TestClientGoRemoteExecReportsIdleStderrAndDisabledStdin(t *testing.T) {
	stdoutStream, stdoutPeer := pipeStream()
	errorStream, errorPeer := pipeStream()
	remote := newClientGoRemoteExec(context.Background(), newFakeSPDYConnection(), execStreams{
		error:  errorStream,
		stdout: stdoutStream,
	})
	if err := remote.Stdin().Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Stdin().Write([]byte("x")); err == nil || err.Error() != "stdin is disabled" {
		t.Fatalf("disabled stdin write err = %v", err)
	}
	stdoutComplete := make(chan struct{})
	go func() {
		defer close(stdoutComplete)
		_, _ = io.ReadAll(remote.Stdout())
	}()
	if _, err := stdoutPeer.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if _, err := errorPeer.Write([]byte(`{"status":"Failure","reason":"NonZeroExitCode","details":{"causes":[{"reason":"ExitCode","message":"23"}]}}`)); err != nil {
		t.Fatal(err)
	}
	_ = errorPeer.Close()
	_ = stdoutPeer.Close()
	select {
	case <-stdoutComplete:
	case <-time.After(time.Second):
		t.Fatal("stdout reader did not finish")
	}
	result := remote.Wait()
	if result.Err != nil || result.ExitCode == nil || *result.ExitCode != 23 {
		t.Fatalf("exit code result = %+v", result)
	}
	remote.Close()
}

func TestClientGoRemoteExecCancelOverridesPendingStreams(t *testing.T) {
	stdoutStream, stdoutPeer := pipeStream()
	stderrStream, stderrPeer := pipeStream()
	errorStream, errorPeer := pipeStream()
	lifetime, cancelLifetime := context.WithCancel(context.Background())
	remote := newClientGoRemoteExec(lifetime, newFakeSPDYConnection(), execStreams{
		error:  errorStream,
		stdout: stdoutStream,
		stderr: stderrStream,
	})
	_ = errorPeer.Close()
	stdoutComplete := make(chan struct{})
	go func() {
		defer close(stdoutComplete)
		_, _ = io.ReadAll(remote.Stdout())
	}()
	_ = stdoutPeer.Close()
	select {
	case <-stdoutComplete:
	case <-time.After(time.Second):
		t.Fatal("stdout reader did not finish")
	}
	time.Sleep(50 * time.Millisecond)
	cancelLifetime()
	result := remote.Wait()
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("canceled wait result = %+v", result)
	}
	_ = stderrPeer.Close()
}

func TestTrackedReaderAndDisabledStdinGuards(t *testing.T) {
	if _, err := (*trackedReader)(nil).Read(make([]byte, 4)); !errors.Is(err, io.EOF) {
		t.Fatalf("nil tracked reader err = %v", err)
	}
	if _, err := newTrackedReader(nil).Read(make([]byte, 4)); !errors.Is(err, io.EOF) {
		t.Fatalf("nil stream err = %v", err)
	}
	if err := (disabledStdin{}).Close(); err != nil {
		t.Fatalf("disabled stdin close = %v", err)
	}
}

func TestReadExecResultCoversProtocolFailures(t *testing.T) {
	oversize := strings.Repeat("a", maximumUpstreamStatusBytes+1)
	tests := []struct {
		name    string
		payload io.Reader
		want    error
	}{
		{name: "read failure", payload: errReader{}, want: errors.New("synthetic read failure")},
		{name: "empty payload", payload: strings.NewReader(""), want: errExecProtocol},
		{name: "invalid json", payload: strings.NewReader("{not-json"), want: errExecProtocol},
		{name: "oversized payload", payload: strings.NewReader(oversize), want: errExecProtocol},
		{name: "unparsable exit code", payload: strings.NewReader(`{"status":"Failure","reason":"NonZeroExitCode","details":{"causes":[{"reason":"ExitCode","message":"not-a-number"}]}}`), want: errExecProtocol},
		{name: "missing exit code cause", payload: strings.NewReader(`{"status":"Failure","reason":"NonZeroExitCode","details":{"causes":[{"reason":"other","message":"1"}]}}`), want: errExecProtocol},
		{name: "unknown status", payload: strings.NewReader(`{"status":"surprise"}`), want: errExecProtocol},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := readExecResult(test.payload)
			if result.Err == nil || result.Err.Error() != test.want.Error() {
				t.Fatalf("err = %v, want %v", result.Err, test.want)
			}
		})
	}

	forbidden := readExecResult(strings.NewReader(`{"status":"Failure","reason":"Forbidden","code":403,"message":"upstream detail"}`))
	if !apierrors.IsForbidden(forbidden.Err) {
		t.Fatalf("forbidden result = %+v", forbidden)
	}
}

func TestReadBoundedStatusLimitsPayloads(t *testing.T) {
	if _, err := readBoundedStatus(strings.NewReader("")); !errors.Is(err, errExecProtocol) {
		t.Fatalf("empty payload err = %v", err)
	}
	payload, err := readBoundedStatus(strings.NewReader(strings.Repeat("a", maximumUpstreamStatusBytes)))
	if err != nil || len(payload) != maximumUpstreamStatusBytes {
		t.Fatalf("boundary payload len=%d err=%v", len(payload), err)
	}
	if _, err := readBoundedStatus(strings.NewReader(strings.Repeat("a", maximumUpstreamStatusBytes+1))); !errors.Is(err, errExecProtocol) {
		t.Fatalf("oversized payload err = %v", err)
	}
	if _, err := readBoundedStatus(errReader{}); err == nil {
		t.Fatal("read failure not surfaced")
	}
}

func TestDecodePortForwardErrorClassifiesStatuses(t *testing.T) {
	tests := []struct {
		name    string
		payload io.Reader
		want    error
	}{
		{name: "empty", payload: strings.NewReader(""), want: nil},
		{name: "success status", payload: strings.NewReader(`{"status":"Success"}`), want: errPortForwardConnection},
		{name: "pod gone", payload: strings.NewReader(`{"status":"Failure","reason":"NotFound","code":404}`), want: actions.ErrPortForwardPodGone},
		{name: "other failure", payload: strings.NewReader(`{"status":"Failure","reason":"Forbidden","code":403,"message":"upstream detail"}`), want: errPortForwardConnection},
		{name: "invalid json", payload: strings.NewReader("{"), want: errPortForwardConnection},
		{name: "oversized", payload: strings.NewReader(strings.Repeat("a", maximumUpstreamStatusBytes+1)), want: errPortForwardConnection},
		{name: "read failure", payload: errReader{}, want: errPortForwardConnection},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := decodePortForwardError(test.payload); got != test.want {
				t.Fatalf("err = %v, want %v", got, test.want)
			}
		})
	}
}

func TestClientGoPortForwardEchoesDataThroughStreams(t *testing.T) {
	dataStream, dataPeer := pipeStream()
	connection := newFakeSPDYConnection()
	connection.factories[corev1.StreamTypeError] = streamStub()
	connection.factories[corev1.StreamTypeData] = func(http.Header) (httpstream.Stream, error) { return dataStream, nil }
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lifetime, cancelLifetime := context.WithCancel(context.Background())
	defer cancelLifetime()
	handle := newClientGoPortForward(lifetime, connection, listener, 8080)

	local, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	if _, err := local.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	received := make([]byte, 4)
	if _, err := io.ReadFull(dataPeer, received); err != nil {
		t.Fatal(err)
	}
	if string(received) != "ping" {
		t.Fatalf("forwarded payload = %q", received)
	}
	if _, err := dataPeer.Write([]byte("pong")); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)
	local.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.ReadFull(local, response); err != nil {
		t.Fatal(err)
	}
	if string(response) != "pong" {
		t.Fatalf("returned payload = %q", response)
	}
	_ = local.Close()
	_ = dataPeer.Close()
	handle.Close()
	if err := handle.Wait(); err != nil {
		t.Fatalf("wait after close = %v", err)
	}
	if connection.closeCountValue() != 1 {
		t.Fatal("connection was not closed exactly once")
	}
	if dataStream.resetCountValue() != 1 {
		t.Fatal("data stream was not reset")
	}
	handle.Close()
}

func TestClientGoPortForwardFailsWhenStreamsAreRefused(t *testing.T) {
	connection := newFakeSPDYConnection()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lifetime, cancelLifetime := context.WithCancel(context.Background())
	defer cancelLifetime()
	handle := newClientGoPortForward(lifetime, connection, listener, 80)
	local, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	if err := handle.Wait(); !errors.Is(err, errPortForwardConnection) {
		t.Fatalf("wait = %v", err)
	}
	if connection.closeCountValue() != 1 {
		t.Fatal("failed handle did not close its connection")
	}
}

func TestClientGoPortForwardReportsListenerFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handle := newClientGoPortForward(context.Background(), newFakeSPDYConnection(), listener, 80)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Wait(); !errors.Is(err, errPortForwardConnection) {
		t.Fatalf("wait = %v", err)
	}
	handle.Close()
}

func TestClientGoPortForwardReportsClosedUpstreamConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	connection := newFakeSPDYConnection()
	handle := newClientGoPortForward(context.Background(), connection, listener, 80)
	_ = connection.Close()
	if err := handle.Wait(); !errors.Is(err, errPortForwardConnection) {
		t.Fatalf("wait = %v", err)
	}
	handle.Close()
}

func TestClientGoPortForwardNilGuards(t *testing.T) {
	if err := (*clientGoPortForward)(nil).Close(); err != nil {
		t.Fatalf("nil close = %v", err)
	}
	if err := (*clientGoPortForward)(nil).Wait(); err != errActionsClientUnavailable {
		t.Fatalf("nil wait = %v", err)
	}
}

func TestNegotiateSPDYRejectsInvalidTransportsAndRequests(t *testing.T) {
	badTLS := &rest.Config{Host: "https://127.0.0.1:1", TLSClientConfig: rest.TLSClientConfig{CAData: []byte("not-a-pem-certificate")}}
	if _, _, err := negotiateSPDY(context.Background(), badTLS, fakeRequestURL("https://127.0.0.1:1/exec"), "unused"); err == nil {
		t.Fatal("invalid TLS configuration was accepted")
	}
	_, _, err := negotiateSPDY(context.Background(), &rest.Config{Host: "https://127.0.0.1:1"}, fakeRequestURL("https://127.0.0.1:1/\x7f"), "unused")
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("invalid request URL err = %v", err)
	}
	setup, cancelSetup := context.WithCancel(context.Background())
	cancelSetup()
	if _, _, err := negotiateSPDY(setup, &rest.Config{Host: "https://127.0.0.1:1"}, fakeRequestURL("https://127.0.0.1:1/exec"), "unused"); err == nil {
		t.Fatal("canceled negotiation was accepted")
	}
}

func TestActionClientStreamingStartsFailOnUnreachableCluster(t *testing.T) {
	config := &rest.Config{Host: "https://127.0.0.1:1"}
	clientset, err := kubeclient.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	client := &ActionClient{unary: clientset, streaming: clientset, config: config}
	setup, cancelSetup := context.WithCancel(context.Background())
	defer cancelSetup()
	if _, err := client.Start(setup, setup, actions.ExecCommand{Target: actions.MutationTarget{Namespace: "ns", Name: "pod"}, Command: []string{"sh"}}); err == nil {
		t.Fatal("exec start accepted an unreachable cluster")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if _, err := client.StartPortForward(setup, setup, actions.PortForwardCommand{Target: actions.MutationTarget{Namespace: "ns", Name: "pod"}, RemotePort: 80}, listener); err == nil {
		t.Fatal("port-forward start accepted an unreachable cluster")
	}
	if _, err := client.StartPortForward(setup, setup, actions.PortForwardCommand{}, nil); !errors.Is(err, errActionsClientUnavailable) {
		t.Fatalf("nil listener err = %v", err)
	}
}

func TestActionClientStreamingStartsGuardIncompleteClients(t *testing.T) {
	setup := context.Background()
	tests := []struct {
		name   string
		client *ActionClient
	}{
		{name: "nil", client: nil},
		{name: "no streaming", client: &ActionClient{}},
		{name: "no config", client: &ActionClient{streaming: fake.NewSimpleClientset()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.client.Start(setup, setup, actions.ExecCommand{}); !errors.Is(err, errActionsClientUnavailable) {
				t.Fatalf("exec start err = %v", err)
			}
			if _, err := test.client.StartPortForward(setup, setup, actions.PortForwardCommand{}, nil); !errors.Is(err, errActionsClientUnavailable) {
				t.Fatalf("port-forward start err = %v", err)
			}
		})
	}
}

func TestClientsActionClientRequiresCompleteGroups(t *testing.T) {
	config := &rest.Config{Host: "https://127.0.0.1:1"}
	unary := &clientGroup{kubernetes: fake.NewSimpleClientset(), config: config}
	streaming := &clientGroup{kubernetes: fake.NewSimpleClientset(), config: config}
	tests := map[string]*Clients{
		"nil":                      nil,
		"empty":                    {},
		"missing streaming group":  {unary: unary},
		"missing unary client":     {unary: &clientGroup{}, streaming: streaming},
		"missing streaming client": {unary: unary, streaming: &clientGroup{}},
	}
	for name, clients := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := clients.ActionClient(); !errors.Is(err, errActionsClientUnavailable) {
				t.Fatalf("action client err = %v", err)
			}
		})
	}
	client, err := (&Clients{unary: unary, streaming: streaming}).ActionClient()
	if err != nil || client == nil {
		t.Fatalf("action client = %#v err = %v", client, err)
	}
	if client.unary == nil || client.streaming == nil || client.config == nil {
		t.Fatalf("action client fields = %#v", client)
	}
}

func TestActionClientUnavailableForUnknownKindsAndNilReceivers(t *testing.T) {
	ctx := context.Background()
	client := &ActionClient{unary: fake.NewSimpleClientset()}
	if _, err := client.UpdateScale(ctx, actions.ScaleCommand{Target: actions.MutationTarget{Kind: "cronjobs"}}); !errors.Is(err, errActionsClientUnavailable) {
		t.Fatalf("unknown kind err = %v", err)
	}
	if _, err := (*ActionClient)(nil).RestartDeployment(ctx, actions.RestartDeploymentCommand{}); !errors.Is(err, errActionsClientUnavailable) {
		t.Fatalf("nil restart err = %v", err)
	}
	if _, err := (*ActionClient)(nil).UpdateScale(ctx, actions.ScaleCommand{}); !errors.Is(err, errActionsClientUnavailable) {
		t.Fatalf("nil scale err = %v", err)
	}
	if _, err := (*ActionClient)(nil).DeletePod(ctx, actions.DeletePodCommand{}); !errors.Is(err, errActionsClientUnavailable) {
		t.Fatalf("nil delete err = %v", err)
	}
	if _, err := (*ActionClient)(nil).InspectExecTarget(ctx, actions.MutationTarget{}, ""); !errors.Is(err, errActionsClientUnavailable) {
		t.Fatalf("nil inspect err = %v", err)
	}
	if _, err := client.InspectExecTarget(ctx, actions.MutationTarget{Namespace: "ns", Name: "missing"}, "app"); err == nil {
		t.Fatal("missing pod inspect accepted")
	}
}
