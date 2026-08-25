package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/actions"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/httpstream"
	remotecommandconsts "k8s.io/apimachinery/pkg/util/remotecommand"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clientspdy "k8s.io/client-go/transport/spdy"
)

const (
	maximumUpstreamStatusBytes = 64 << 10
	portForwardProtocolV1Name  = "portforward.k8s.io"
)

var (
	errActionsClientUnavailable = errors.New("kubernetes actions client is unavailable")
	errExecProtocol             = errors.New("kubernetes exec protocol failed")
	errPortForwardConnection    = errors.New("kubernetes port-forward connection failed")
)

// ActionClient keeps rest.Config inside the Kubernetes adapter boundary. In
// particular, handlers and service DTOs never receive credentials or a
// transport from the active kubeconfig.
type ActionClient struct {
	unary     kubeclient.Interface
	streaming kubeclient.Interface
	config    *rest.Config
}

func (clients *Clients) ActionClient() (*ActionClient, error) {
	if clients == nil || clients.unary == nil || clients.streaming == nil || clients.unary.kubernetes == nil || clients.streaming.kubernetes == nil {
		return nil, errActionsClientUnavailable
	}
	config := clients.streamingConfigCopy()
	if config == nil {
		return nil, errActionsClientUnavailable
	}
	return &ActionClient{
		unary:     clients.unary.kubernetes,
		streaming: clients.streaming.kubernetes,
		config:    config,
	}, nil
}

func (client *ActionClient) RestartDeployment(ctx context.Context, command actions.RestartDeploymentCommand) (actions.MutationResult, error) {
	if client == nil || client.unary == nil {
		return actions.MutationResult{}, errActionsClientUnavailable
	}
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]string{"resourceVersion": command.ExpectedResourceVersion},
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]string{
						"kubectl.kubernetes.io/restartedAt": command.RestartedAt.UTC().Format(time.RFC3339),
					},
				},
			},
		},
	})
	if err != nil {
		return actions.MutationResult{}, errActionsClientUnavailable
	}
	deployment, err := client.unary.AppsV1().Deployments(command.Target.Namespace).Patch(
		ctx,
		command.Target.Name,
		types.StrategicMergePatchType,
		patch,
		metav1.PatchOptions{},
	)
	if err != nil {
		return actions.MutationResult{}, err
	}
	return actions.MutationResult{ResourceVersion: deployment.ResourceVersion}, nil
}

func (client *ActionClient) UpdateScale(ctx context.Context, command actions.ScaleCommand) (actions.MutationResult, error) {
	if client == nil || client.unary == nil {
		return actions.MutationResult{}, errActionsClientUnavailable
	}
	scale := &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{
			Name:            command.Target.Name,
			Namespace:       command.Target.Namespace,
			ResourceVersion: command.ExpectedResourceVersion,
		},
		Spec: autoscalingv1.ScaleSpec{Replicas: command.Replicas},
	}
	var (
		updated *autoscalingv1.Scale
		err     error
	)
	switch command.Target.Kind {
	case "deployments":
		updated, err = client.unary.AppsV1().Deployments(command.Target.Namespace).UpdateScale(ctx, command.Target.Name, scale, metav1.UpdateOptions{})
	case "statefulsets":
		updated, err = client.unary.AppsV1().StatefulSets(command.Target.Namespace).UpdateScale(ctx, command.Target.Name, scale, metav1.UpdateOptions{})
	default:
		return actions.MutationResult{}, errActionsClientUnavailable
	}
	if err != nil {
		return actions.MutationResult{}, err
	}
	return actions.MutationResult{ResourceVersion: updated.ResourceVersion}, nil
}

func (client *ActionClient) DeletePod(ctx context.Context, command actions.DeletePodCommand) (actions.MutationResult, error) {
	if client == nil || client.unary == nil {
		return actions.MutationResult{}, errActionsClientUnavailable
	}
	uid := types.UID(command.ExpectedUID)
	resourceVersion := command.ExpectedResourceVersion
	err := client.unary.CoreV1().Pods(command.Target.Namespace).Delete(ctx, command.Target.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &resourceVersion},
	})
	return actions.MutationResult{}, err
}

func (client *ActionClient) InspectExecTarget(ctx context.Context, target actions.MutationTarget, container string) (actions.ExecTargetState, error) {
	if client == nil || client.unary == nil {
		return actions.ExecTargetState{}, errActionsClientUnavailable
	}
	pod, err := client.unary.CoreV1().Pods(target.Namespace).Get(ctx, target.Name, metav1.GetOptions{})
	if err != nil {
		return actions.ExecTargetState{}, err
	}
	state := actions.ExecTargetState{PodExists: true}
	for _, candidate := range pod.Spec.Containers {
		if candidate.Name == container {
			state.ContainerExists = true
		}
	}
	for _, candidate := range pod.Spec.InitContainers {
		if candidate.Name == container {
			state.ContainerExists = true
		}
	}
	for _, candidate := range pod.Spec.EphemeralContainers {
		if candidate.Name == container {
			state.ContainerExists = true
		}
	}
	for _, candidate := range appendContainerStatuses(pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses, pod.Status.EphemeralContainerStatuses) {
		if candidate.Name == container && candidate.State.Running != nil {
			state.ContainerRunning = true
			break
		}
	}
	return state, nil
}

func appendContainerStatuses(groups ...[]corev1.ContainerStatus) []corev1.ContainerStatus {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	result := make([]corev1.ContainerStatus, 0, total)
	for _, group := range groups {
		result = append(result, group...)
	}
	return result
}

func (client *ActionClient) Start(setup context.Context, lifetime context.Context, command actions.ExecCommand) (actions.RemoteExec, error) {
	if client == nil || client.streaming == nil || client.config == nil {
		return nil, errActionsClientUnavailable
	}
	requestURL := client.streaming.CoreV1().RESTClient().Post().
		Namespace(command.Target.Namespace).
		Resource("pods").
		Name(command.Target.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: command.Container,
			Command:   append([]string(nil), command.Command...),
			Stdin:     command.Stdin,
			Stdout:    true,
			Stderr:    !command.TTY,
			TTY:       command.TTY,
		}, scheme.ParameterCodec).
		URL()
	connection, protocol, err := negotiateSPDY(setup, client.config, requestURL, remotecommandconsts.StreamProtocolV5Name, remotecommandconsts.StreamProtocolV4Name)
	if err != nil {
		return nil, err
	}
	if protocol != remotecommandconsts.StreamProtocolV5Name && protocol != remotecommandconsts.StreamProtocolV4Name {
		_ = connection.Close()
		return nil, errExecProtocol
	}
	streams, err := createExecStreams(setup, connection, command)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	remote := newClientGoRemoteExec(lifetime, connection, streams)
	return remote, nil
}

func negotiateSPDY(ctx context.Context, config *rest.Config, requestURL interface{ String() string }, protocols ...string) (httpstream.Connection, string, error) {
	transport, upgrader, err := clientspdy.RoundTripperFor(config)
	if err != nil {
		return nil, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), nil)
	if err != nil {
		return nil, "", err
	}
	httpClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("redirect rejected")
		},
	}
	return clientspdy.Negotiate(upgrader, httpClient, request, protocols...)
}

type execStreams struct {
	error  httpstream.Stream
	stdin  httpstream.Stream
	stdout httpstream.Stream
	stderr httpstream.Stream
	resize httpstream.Stream
}

func createExecStreams(ctx context.Context, connection httpstream.Connection, command actions.ExecCommand) (execStreams, error) {
	type result struct {
		streams execStreams
		err     error
	}
	created := make(chan result, 1)
	go func() {
		var streams execStreams
		streams.error, _ = createSPDYStream(connection, corev1.StreamTypeError)
		if streams.error == nil {
			created <- result{err: errExecProtocol}
			return
		}
		var err error
		if command.Stdin {
			streams.stdin, err = createSPDYStream(connection, corev1.StreamTypeStdin)
			if err != nil {
				created <- result{err: err}
				return
			}
		}
		streams.stdout, err = createSPDYStream(connection, corev1.StreamTypeStdout)
		if err != nil {
			created <- result{err: err}
			return
		}
		if !command.TTY {
			streams.stderr, err = createSPDYStream(connection, corev1.StreamTypeStderr)
			if err != nil {
				created <- result{err: err}
				return
			}
		}
		if command.TTY {
			streams.resize, err = createSPDYStream(connection, corev1.StreamTypeResize)
			if err != nil {
				created <- result{err: err}
				return
			}
		}
		created <- result{streams: streams}
	}()
	select {
	case outcome := <-created:
		return outcome.streams, outcome.err
	case <-ctx.Done():
		_ = connection.Close()
		return execStreams{}, ctx.Err()
	}
}

func createSPDYStream(connection httpstream.Connection, streamType string) (httpstream.Stream, error) {
	headers := http.Header{}
	headers.Set(corev1.StreamType, streamType)
	return connection.CreateStream(headers)
}

type disabledStdin struct{}

func (disabledStdin) Write([]byte) (int, error) { return 0, errors.New("stdin is disabled") }
func (disabledStdin) Close() error              { return nil }

type trackedReader struct {
	stream httpstream.Stream
	done   chan struct{}
	once   sync.Once
}

func newTrackedReader(stream httpstream.Stream) *trackedReader {
	return &trackedReader{stream: stream, done: make(chan struct{})}
}

func (reader *trackedReader) Read(payload []byte) (int, error) {
	if reader == nil || reader.stream == nil {
		return 0, io.EOF
	}
	n, err := reader.stream.Read(payload)
	if err != nil {
		reader.once.Do(func() { close(reader.done) })
	}
	return n, err
}

type clientGoRemoteExec struct {
	ctx        context.Context
	cancel     context.CancelFunc
	connection httpstream.Connection
	stdin      io.WriteCloser
	stdout     *trackedReader
	stderr     *trackedReader
	resize     httpstream.Stream
	resizeMu   sync.Mutex
	done       chan struct{}
	closeOnce  sync.Once
	resultMu   sync.Mutex
	result     actions.RemoteExecExit
}

func newClientGoRemoteExec(parent context.Context, connection httpstream.Connection, streams execStreams) *clientGoRemoteExec {
	ctx, cancel := context.WithCancel(parent)
	remote := &clientGoRemoteExec{
		ctx:        ctx,
		cancel:     cancel,
		connection: connection,
		stdout:     newTrackedReader(streams.stdout),
		resize:     streams.resize,
		done:       make(chan struct{}),
	}
	if streams.stdin != nil {
		remote.stdin = streams.stdin
		go func() { _, _ = io.Copy(io.Discard, streams.stdin) }()
	} else {
		remote.stdin = disabledStdin{}
	}
	if streams.stderr != nil {
		remote.stderr = newTrackedReader(streams.stderr)
	} else {
		remote.stderr = newTrackedReader(nil)
		remote.stderr.once.Do(func() { close(remote.stderr.done) })
	}
	go remote.observe(streams.error)
	go func() {
		<-ctx.Done()
		_ = remote.Close()
	}()
	return remote
}

func (remote *clientGoRemoteExec) Stdin() io.WriteCloser { return remote.stdin }
func (remote *clientGoRemoteExec) Stdout() io.Reader     { return remote.stdout }
func (remote *clientGoRemoteExec) Stderr() io.Reader     { return remote.stderr }

func (remote *clientGoRemoteExec) Resize(ctx context.Context, columns, rows uint16) error {
	if remote == nil || remote.resize == nil || columns == 0 || rows == 0 {
		return errExecProtocol
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	remote.resizeMu.Lock()
	defer remote.resizeMu.Unlock()
	if err := remote.ctx.Err(); err != nil {
		return err
	}
	return json.NewEncoder(remote.resize).Encode(struct {
		Width  uint16
		Height uint16
	}{Width: columns, Height: rows})
}

func (remote *clientGoRemoteExec) observe(errorStream httpstream.Stream) {
	result := readExecResult(errorStream)
	select {
	case <-remote.stdout.done:
	case <-remote.ctx.Done():
		result = actions.RemoteExecExit{Err: remote.ctx.Err()}
	}
	select {
	case <-remote.stderr.done:
	case <-remote.ctx.Done():
		result = actions.RemoteExecExit{Err: remote.ctx.Err()}
	}
	remote.resultMu.Lock()
	remote.result = result
	remote.resultMu.Unlock()
	close(remote.done)
}

func readExecResult(stream io.Reader) actions.RemoteExecExit {
	payload, err := readBoundedStatus(stream)
	if err != nil {
		return actions.RemoteExecExit{Err: err}
	}
	var status metav1.Status
	if err := json.Unmarshal(payload, &status); err != nil {
		return actions.RemoteExecExit{Err: errExecProtocol}
	}
	if status.Status == metav1.StatusSuccess {
		code := 0
		return actions.RemoteExecExit{ExitCode: &code}
	}
	if status.Status == metav1.StatusFailure && status.Reason == remotecommandconsts.NonZeroExitCodeReason && status.Details != nil {
		for _, cause := range status.Details.Causes {
			if cause.Type != remotecommandconsts.ExitCodeCauseType {
				continue
			}
			code, parseErr := strconv.ParseUint(cause.Message, 10, 8)
			if parseErr != nil {
				return actions.RemoteExecExit{Err: errExecProtocol}
			}
			value := int(code)
			return actions.RemoteExecExit{ExitCode: &value}
		}
		return actions.RemoteExecExit{Err: errExecProtocol}
	}
	if status.Status == metav1.StatusFailure {
		statusErr := apierrors.FromObject(&status)
		if apierrors.IsNotFound(statusErr) {
			return actions.RemoteExecExit{Err: actions.ErrExecTargetGone}
		}
		return actions.RemoteExecExit{Err: statusErr}
	}
	return actions.RemoteExecExit{Err: errExecProtocol}
}

func readBoundedStatus(reader io.Reader) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, maximumUpstreamStatusBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || len(payload) > maximumUpstreamStatusBytes {
		return nil, errExecProtocol
	}
	return payload, nil
}

func (remote *clientGoRemoteExec) Wait() actions.RemoteExecExit {
	if remote == nil {
		return actions.RemoteExecExit{Err: errActionsClientUnavailable}
	}
	<-remote.done
	remote.resultMu.Lock()
	defer remote.resultMu.Unlock()
	return remote.result
}

func (remote *clientGoRemoteExec) Close() error {
	if remote == nil {
		return nil
	}
	var closeErr error
	remote.closeOnce.Do(func() {
		remote.cancel()
		if remote.stdin != nil {
			_ = remote.stdin.Close()
		}
		if remote.resize != nil {
			_ = remote.resize.Close()
		}
		closeErr = remote.connection.Close()
	})
	return closeErr
}

func (client *ActionClient) StartPortForward(setup context.Context, lifetime context.Context, command actions.PortForwardCommand, listener net.Listener) (actions.PortForwardHandle, error) {
	if client == nil || client.streaming == nil || client.config == nil || listener == nil {
		return nil, errActionsClientUnavailable
	}
	requestURL := client.streaming.CoreV1().RESTClient().Post().
		Namespace(command.Target.Namespace).
		Resource("pods").
		Name(command.Target.Name).
		SubResource("portforward").
		URL()
	connection, protocol, err := negotiateSPDY(setup, client.config, requestURL, portForwardProtocolV1Name)
	if err != nil {
		return nil, err
	}
	if protocol != portForwardProtocolV1Name {
		_ = connection.Close()
		return nil, errPortForwardConnection
	}
	handle := newClientGoPortForward(lifetime, connection, listener, command.RemotePort)
	return handle, nil
}

type clientGoPortForward struct {
	ctx        context.Context
	cancel     context.CancelFunc
	connection httpstream.Connection
	listener   net.Listener
	remotePort int
	done       chan struct{}
	resultMu   sync.Mutex
	result     error
	closeOnce  sync.Once
	mu         sync.Mutex
	locals     map[net.Conn]struct{}
	nextID     uint64
	fatal      chan error
}

func newClientGoPortForward(parent context.Context, connection httpstream.Connection, listener net.Listener, remotePort int) *clientGoPortForward {
	ctx, cancel := context.WithCancel(parent)
	handle := &clientGoPortForward{
		ctx:        ctx,
		cancel:     cancel,
		connection: connection,
		listener:   listener,
		remotePort: remotePort,
		done:       make(chan struct{}),
		locals:     make(map[net.Conn]struct{}),
		fatal:      make(chan error, 1),
	}
	go handle.run()
	go func() {
		<-ctx.Done()
		_ = handle.Close()
	}()
	return handle
}

func (handle *clientGoPortForward) run() {
	var result error
	defer func() {
		handle.resultMu.Lock()
		handle.result = result
		handle.resultMu.Unlock()
		close(handle.done)
	}()
	accepted := make(chan struct {
		connection net.Conn
		err        error
	}, 1)
	for {
		go func() {
			connection, err := handle.listener.Accept()
			accepted <- struct {
				connection net.Conn
				err        error
			}{connection: connection, err: err}
		}()
		select {
		case outcome := <-accepted:
			if outcome.err != nil {
				if handle.ctx.Err() != nil {
					return
				}
				result = errPortForwardConnection
				return
			}
			handle.track(outcome.connection, true)
			go handle.forward(outcome.connection)
		case result = <-handle.fatal:
			_ = handle.Close()
			return
		case <-handle.connection.CloseChan():
			if handle.ctx.Err() == nil {
				result = errPortForwardConnection
			}
			return
		case <-handle.ctx.Done():
			return
		}
	}
}

func (handle *clientGoPortForward) forward(local net.Conn) {
	defer func() {
		handle.track(local, false)
		_ = local.Close()
	}()
	handle.mu.Lock()
	requestID := handle.nextID
	handle.nextID++
	handle.mu.Unlock()
	headers := http.Header{}
	headers.Set(corev1.StreamType, corev1.StreamTypeError)
	headers.Set(corev1.PortHeader, strconv.Itoa(handle.remotePort))
	headers.Set(corev1.PortForwardRequestIDHeader, strconv.FormatUint(requestID, 10))
	errorStream, err := handle.connection.CreateStream(headers)
	if err != nil {
		handle.fail(errPortForwardConnection)
		return
	}
	_ = errorStream.Close()
	defer handle.connection.RemoveStreams(errorStream)
	errorResult := make(chan error, 1)
	go func() { errorResult <- decodePortForwardError(errorStream) }()

	headers.Set(corev1.StreamType, corev1.StreamTypeData)
	dataStream, err := handle.connection.CreateStream(headers)
	if err != nil {
		handle.fail(errPortForwardConnection)
		return
	}
	defer handle.connection.RemoveStreams(dataStream)
	remoteDone := make(chan struct{})
	localDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(local, dataStream)
		close(remoteDone)
	}()
	go func() {
		_, _ = io.Copy(dataStream, local)
		_ = dataStream.Close()
		close(localDone)
	}()
	select {
	case <-remoteDone:
	case <-localDone:
	case <-handle.ctx.Done():
	}
	_ = dataStream.Reset()
	select {
	case err := <-errorResult:
		if err != nil {
			handle.fail(err)
		}
	case <-handle.ctx.Done():
	}
}

func decodePortForwardError(reader io.Reader) error {
	payload, err := io.ReadAll(io.LimitReader(reader, maximumUpstreamStatusBytes+1))
	if err != nil || len(payload) > maximumUpstreamStatusBytes {
		return errPortForwardConnection
	}
	if len(payload) == 0 {
		return nil
	}
	var status metav1.Status
	if json.Unmarshal(payload, &status) == nil && status.Status == metav1.StatusFailure {
		if apierrors.IsNotFound(apierrors.FromObject(&status)) {
			return actions.ErrPortForwardPodGone
		}
	}
	return errPortForwardConnection
}

func (handle *clientGoPortForward) track(connection net.Conn, add bool) {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if add {
		handle.locals[connection] = struct{}{}
	} else {
		delete(handle.locals, connection)
	}
}

func (handle *clientGoPortForward) fail(err error) {
	select {
	case handle.fatal <- err:
	default:
	}
}

func (handle *clientGoPortForward) Wait() error {
	if handle == nil {
		return errActionsClientUnavailable
	}
	<-handle.done
	handle.resultMu.Lock()
	defer handle.resultMu.Unlock()
	return handle.result
}

func (handle *clientGoPortForward) Close() error {
	if handle == nil {
		return nil
	}
	var closeErr error
	handle.closeOnce.Do(func() {
		handle.cancel()
		_ = handle.listener.Close()
		handle.mu.Lock()
		for connection := range handle.locals {
			_ = connection.Close()
		}
		handle.locals = make(map[net.Conn]struct{})
		handle.mu.Unlock()
		closeErr = handle.connection.Close()
	})
	return closeErr
}
