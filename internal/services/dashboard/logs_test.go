package dashboard

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolveLogScanRequestDefaultsAndClosedValidation(t *testing.T) {
	t.Parallel()
	resolved, err := ResolveLogScanRequest(LogScanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Window != 15*time.Minute || resolved.TailLines != 200 || resolved.MaxPods != 20 || resolved.MaxConcurrentContainers != 4 {
		t.Fatalf("unexpected defaults: %+v", resolved)
	}
	validWindow := "4h"
	tail, pods, concurrency := 2_000, 50, 8
	resolved, err = ResolveLogScanRequest(LogScanRequest{Window: &validWindow, TailLines: &tail, MaxPods: &pods, MaxConcurrentContainers: &concurrency})
	if err != nil || resolved.Window != 4*time.Hour {
		t.Fatalf("valid maxima rejected: %+v/%v", resolved, err)
	}
	for _, request := range []LogScanRequest{
		{Window: stringTestPointer("5m")},
		{TailLines: intTestPointer(0)}, {TailLines: intTestPointer(2_001)},
		{MaxPods: intTestPointer(0)}, {MaxPods: intTestPointer(51)},
		{MaxConcurrentContainers: intTestPointer(0)}, {MaxConcurrentContainers: intTestPointer(9)},
	} {
		if _, err := ResolveLogScanRequest(request); err == nil {
			t.Fatalf("invalid request accepted: %+v", request)
		}
	}
}

func TestSelectLogTargetsPrioritiesBoundariesAndLimits(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	pods := []corev1.Pod{
		logPod("ns", "problem", "api", now, nil),
		logPod("ns", "restart", "worker", now, &corev1.ContainerStateTerminated{FinishedAt: metav1.NewTime(now.Add(-time.Hour))}),
		logPod("ns", "recent", "job", now, &corev1.ContainerStateTerminated{FinishedAt: metav1.NewTime(now.Add(-15 * time.Minute))}),
		logPod("ns", "old", "old", now, &corev1.ContainerStateTerminated{FinishedAt: metav1.NewTime(now.Add(-15*time.Minute - time.Nanosecond))}),
	}
	problems := []ProblemPodDTO{{Namespace: "ns", Pod: "problem", Container: stringPointer("api"), ContainerType: containerTypePointer(ContainerRegular), Severity: ProblemCritical}}
	restarts := []RestartDTO{{Namespace: "ns", Pod: "restart", Container: "worker", ContainerType: ContainerRegular, Restarts: 4}}
	request := ResolvedLogScanRequest{Window: 15 * time.Minute, MaxPods: 20}
	selection := SelectLogTargets(pods, problems, restarts, now, request, LogBudget{MaxContainers: 100})
	if selection.Truncated || len(selection.Targets) != 3 {
		t.Fatalf("unexpected selection: %+v", selection)
	}
	if selection.Targets[0].Pod != "problem" || selection.Targets[1].Pod != "restart" || selection.Targets[2].Pod != "recent" {
		t.Fatalf("priority order mismatch: %+v", selection.Targets)
	}
	request.MaxPods = 2
	selection = SelectLogTargets(pods, problems, restarts, now, request, LogBudget{MaxContainers: 100})
	if !selection.Truncated || len(selection.Targets) != 2 {
		t.Fatalf("pod limit not visible: %+v", selection)
	}
}

func TestLogScanRedactsAndReturnsDeterministicMatches(t *testing.T) {
	t.Parallel()
	reader := &mapLogReader{values: map[string]string{
		"ns/pod/b": "normal\nerror password=super-secret\n",
		"ns/pod/a": `{"level":"error","message":"request failed"}` + "\n",
	}}
	service := NewLogService(reader, allowLogs{}, fixedClock{time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}, LogBudget{})
	targets := []LogTarget{
		{Namespace: "ns", Pod: "pod", Container: "b", ContainerType: ContainerRegular, priority: 1},
		{Namespace: "ns", Pod: "pod", Container: "a", ContainerType: ContainerRegular, priority: 0},
	}
	block := service.Scan(context.Background(), LogScanRequest{}, targets)
	if !block.Complete || block.Truncated || len(block.Value) != 2 || block.Coverage.CompletedNamespaces != 1 {
		t.Fatalf("unexpected scan: %+v", block)
	}
	if block.Value[0].Container != "a" || block.Value[0].ReasonCode != LogJSONErrorLevel {
		t.Fatalf("concurrency changed deterministic target order: %+v", block.Value)
	}
	if block.Value[1].Container != "b" || !block.Value[1].Redacted || strings.Contains(block.Value[1].Excerpt, "super-secret") {
		t.Fatalf("secret was returned: %+v", block.Value[1])
	}
	if len(reader.requests) != 2 || !reader.requests[0].Timestamps || reader.requests[0].TailLines != 200 {
		t.Fatalf("reader did not receive bounded request: %+v", reader.requests)
	}
}

func TestLogScanLineContainerTailAndResponseBudgetsAreVisible(t *testing.T) {
	t.Parallel()
	content := "error " + strings.Repeat("x", 100) + "\nerror second\nerror third\n"
	reader := &mapLogReader{values: map[string]string{"ns/pod/app": content}}
	service := NewLogService(reader, allowLogs{}, fixedClock{time.Now()}, LogBudget{MaxLineBytes: 16, MaxContainerBytes: 64, MaxScanBytes: 100, MaxContainers: 1})
	tail := 2
	block := service.Scan(context.Background(), LogScanRequest{TailLines: &tail}, []LogTarget{{Namespace: "ns", Pod: "pod", Container: "app", ContainerType: ContainerRegular}})
	if block.Complete || !block.Truncated {
		t.Fatalf("limits were not visible: %+v", block)
	}
	for _, match := range block.Value {
		if len(match.Excerpt) > 16 {
			t.Fatalf("overlong line escaped limit: %+v", match)
		}
	}
}

func TestLogScanMaxScanBytesBoundsAggregateReadsAcrossConcurrentContainers(t *testing.T) {
	reader := &aggregateCountingLogReader{content: strings.Repeat("error aggregate-sensitive-value\n", 64)}
	service := NewLogService(reader, allowLogs{}, fixedClock{time.Now()}, LogBudget{
		MaxLineBytes:      128,
		MaxContainerBytes: 4 << 10,
		MaxScanBytes:      257,
		MaxContainers:     8,
	})
	concurrency := 8
	targets := make([]LogTarget, 8)
	for index := range targets {
		targets[index] = LogTarget{Namespace: "ns", Pod: "pod", Container: string(rune('a' + index)), ContainerType: ContainerRegular}
	}
	block := service.Scan(context.Background(), LogScanRequest{MaxConcurrentContainers: &concurrency}, targets)
	if got := reader.total.Load(); got != 257 {
		t.Fatalf("aggregate source bytes = %d, want exact scan ceiling 257", got)
	}
	if block.Complete || !block.Truncated {
		t.Fatalf("aggregate input limit was not visible: %+v", block)
	}
}

func TestLogScanAggregateExhaustionCancelsPendingTargets(t *testing.T) {
	reader := &mapLogReader{values: map[string]string{
		"ns/pod/a": strings.Repeat("error first\n", 32),
		"ns/pod/b": "error second\n",
		"ns/pod/c": "error third\n",
	}}
	service := NewLogService(reader, allowLogs{}, fixedClock{time.Now()}, LogBudget{
		MaxLineBytes:      64,
		MaxContainerBytes: 1 << 10,
		MaxScanBytes:      32,
		MaxContainers:     3,
	})
	concurrency := 1
	block := service.Scan(context.Background(), LogScanRequest{MaxConcurrentContainers: &concurrency}, []LogTarget{
		{Namespace: "ns", Pod: "pod", Container: "c", ContainerType: ContainerRegular},
		{Namespace: "ns", Pod: "pod", Container: "b", ContainerType: ContainerRegular},
		{Namespace: "ns", Pod: "pod", Container: "a", ContainerType: ContainerRegular},
	})
	if block.Complete || !block.Truncated {
		t.Fatalf("aggregate exhaustion was not visible: %+v", block)
	}
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if len(reader.requests) != 1 || reader.requests[0].Container != "a" {
		t.Fatalf("pending readers were opened after cancellation: %+v", reader.requests)
	}
}

func TestAggregateAndContainerBudgetsCountEachSourceByteOnce(t *testing.T) {
	t.Parallel()
	content := "prefix error happened\n"
	var total atomic.Int64
	aggregate := newLogScanByteBudget(int64(len(content)))
	source := &logScanBudgetReader{
		ctx: context.Background(),
		source: &byteCountingReader{
			source: strings.NewReader(content),
			total:  &total,
		},
		budget: aggregate,
	}
	lines, truncated, err := readBoundedLogLines(context.Background(), source, LogBudget{
		MaxLineBytes:      64,
		MaxContainerBytes: 64,
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total.Load() != int64(len(content)) || len(lines) != 1 || string(lines[0].value) != strings.TrimSuffix(content, "\n") {
		t.Fatalf("bytes were skipped or counted twice: total=%d lines=%+v", total.Load(), lines)
	}
	if !truncated || !aggregate.Exhausted() {
		t.Fatalf("exact aggregate ceiling must be conservatively truncated: truncated=%v exhausted=%v", truncated, aggregate.Exhausted())
	}
	if _, ok := DetectLogLine(lines[0].value, lines[0].truncated); !ok {
		t.Fatal("content at the aggregate boundary was not analyzed")
	}
}

func TestLogScanChecksAuthorizationBeforeReading(t *testing.T) {
	t.Parallel()
	reader := &mapLogReader{values: map[string]string{"denied/pod/app": "error secret"}}
	service := NewLogService(reader, denyLogs{}, fixedClock{time.Now()}, LogBudget{})
	block := service.Scan(context.Background(), LogScanRequest{}, []LogTarget{{Namespace: "denied", Pod: "pod", Container: "app"}})
	if block.Complete || len(block.Coverage.DeniedNamespaces) != 1 || len(reader.requests) != 0 {
		t.Fatalf("denied logs were read or not reported: %+v requests=%+v", block, reader.requests)
	}
}

func TestLogScanHonorsConcurrencyLimit(t *testing.T) {
	ready := make(chan struct{}, 8)
	release := make(chan struct{})
	reader := &gatedLogReader{ready: ready, release: release}
	service := NewLogService(reader, allowLogs{}, fixedClock{time.Now()}, LogBudget{Timeout: 8 * time.Second, MaxContainers: 8})
	concurrency := 4
	targets := make([]LogTarget, 8)
	for index := range targets {
		targets[index] = LogTarget{Namespace: "ns", Pod: "pod", Container: string(rune('a' + index)), ContainerType: ContainerRegular}
	}
	done := make(chan DashboardBlockDTO[[]LogMatchDTO], 1)
	go func() {
		done <- service.Scan(context.Background(), LogScanRequest{MaxConcurrentContainers: &concurrency}, targets)
	}()
	for index := 0; index < concurrency; index++ {
		select {
		case <-ready:
		case <-time.After(time.Second):
			t.Fatal("workers did not reach concurrency gate")
		}
	}
	if got := reader.maximum.Load(); got != int32(concurrency) {
		t.Fatalf("maximum concurrency = %d, want %d", got, concurrency)
	}
	close(release)
	select {
	case block := <-done:
		if !block.Complete || len(block.Value) != 8 {
			t.Fatalf("unexpected concurrent scan: %+v", block)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scan did not finish")
	}
}

func TestLogScanCancellationClosesActiveReader(t *testing.T) {
	started := make(chan struct{})
	reader := &blockingLogReader{started: started}
	service := NewLogService(reader, allowLogs{}, fixedClock{time.Now()}, LogBudget{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan DashboardBlockDTO[[]LogMatchDTO], 1)
	go func() {
		done <- service.Scan(ctx, LogScanRequest{}, []LogTarget{{Namespace: "ns", Pod: "pod", Container: "app"}})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("reader did not start")
	}
	cancel()
	select {
	case block := <-done:
		if block.Complete || len(block.Errors) == 0 {
			t.Fatalf("cancellation not visible: %+v", block)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not close the active reader")
	}
}

func stringTestPointer(value string) *string { return &value }
func intTestPointer(value int) *int          { return &value }

func logPod(namespace, name, container string, now time.Time, last *corev1.ContainerStateTerminated) corev1.Pod {
	status := corev1.ContainerStatus{Name: container, Ready: true}
	if last != nil {
		status.LastTerminationState.Terminated = last
	}
	return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, CreationTimestamp: metav1.NewTime(now.Add(-time.Hour))}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{status}}}
}

type allowLogs struct{}

func (allowLogs) CanReadPodLogs(context.Context, string, string) (PermissionDecision, error) {
	return PermissionAllowed, nil
}

type denyLogs struct{}

func (denyLogs) CanReadPodLogs(context.Context, string, string) (PermissionDecision, error) {
	return PermissionDenied, nil
}

type mapLogReader struct {
	mu       sync.Mutex
	values   map[string]string
	requests []LogReadRequest
}

func (reader *mapLogReader) ReadLogs(_ context.Context, request LogReadRequest) (io.ReadCloser, error) {
	reader.mu.Lock()
	reader.requests = append(reader.requests, request)
	reader.mu.Unlock()
	key := request.Namespace + "/" + request.Pod + "/" + request.Container
	return io.NopCloser(strings.NewReader(reader.values[key])), nil
}

type gatedLogReader struct {
	ready   chan<- struct{}
	release <-chan struct{}
	current atomic.Int32
	maximum atomic.Int32
}

func (reader *gatedLogReader) ReadLogs(context.Context, LogReadRequest) (io.ReadCloser, error) {
	return &gatedReadCloser{parent: reader}, nil
}

type gatedReadCloser struct {
	parent *gatedLogReader
	done   bool
}

func (reader *gatedReadCloser) Read(buffer []byte) (int, error) {
	if reader.done {
		return 0, io.EOF
	}
	current := reader.parent.current.Add(1)
	for {
		maximum := reader.parent.maximum.Load()
		if current <= maximum || reader.parent.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	reader.parent.ready <- struct{}{}
	<-reader.parent.release
	reader.parent.current.Add(-1)
	reader.done = true
	return copy(buffer, "error\n"), nil
}

func (reader *gatedReadCloser) Close() error { return nil }

type blockingLogReader struct{ started chan<- struct{} }

func (reader *blockingLogReader) ReadLogs(context.Context, LogReadRequest) (io.ReadCloser, error) {
	return &blockingReadCloser{started: reader.started, closed: make(chan struct{})}, nil
}

type blockingReadCloser struct {
	started   chan<- struct{}
	closed    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func (reader *blockingReadCloser) Read([]byte) (int, error) {
	reader.startOnce.Do(func() { reader.started <- struct{}{} })
	<-reader.closed
	return 0, io.ErrClosedPipe
}

func (reader *blockingReadCloser) Close() error {
	reader.closeOnce.Do(func() { close(reader.closed) })
	return nil
}

type aggregateCountingLogReader struct {
	content string
	total   atomic.Int64
}

func (reader *aggregateCountingLogReader) ReadLogs(context.Context, LogReadRequest) (io.ReadCloser, error) {
	return io.NopCloser(&byteCountingReader{source: strings.NewReader(reader.content), total: &reader.total}), nil
}

type byteCountingReader struct {
	source io.Reader
	total  *atomic.Int64
}

func (reader *byteCountingReader) Read(destination []byte) (int, error) {
	read, err := reader.source.Read(destination)
	reader.total.Add(int64(read))
	return read, err
}
