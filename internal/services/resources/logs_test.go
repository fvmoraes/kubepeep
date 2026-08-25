package resources

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/lifecycle"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

type fakeLogPort struct {
	mu         sync.Mutex
	content    string
	options    []LogSourceOptions
	targets    []LogTarget
	opens      int
	err        error
	terminated bool
}

func (fake *fakeLogPort) ContainerTerminated(context.Context, LogTarget) bool {
	return fake.terminated
}

type contextLogReader struct{ ctx context.Context }

func (reader *contextLogReader) Read([]byte) (int, error) {
	<-reader.ctx.Done()
	return 0, context.Cause(reader.ctx)
}
func (*contextLogReader) Close() error { return nil }

type blockingLogPort struct{ opened chan struct{} }

func (port *blockingLogPort) Open(ctx context.Context, _ LogTarget, _ LogSourceOptions) (io.ReadCloser, error) {
	close(port.opened)
	return &contextLogReader{ctx: ctx}, nil
}

func (fake *fakeLogPort) Open(_ context.Context, target LogTarget, options LogSourceOptions) (io.ReadCloser, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.opens++
	fake.options = append(fake.options, options)
	fake.targets = append(fake.targets, target)
	if fake.err != nil {
		return nil, fake.err
	}
	return io.NopCloser(strings.NewReader(fake.content)), nil
}
func boolPointer(value bool) *bool { return &value }
func logSelection() Selection {
	return Selection{Generation: "gen", Context: "ctx", Scope: "scope", Namespaces: []string{"payments"}}
}
func allowedLogService(port LogPort) *LogService {
	return &LogService{Port: port, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{}}, Redactor: TextRedactorFunc(func(value string) string { return strings.ReplaceAll(value, "secret", "[REDACTED]") })}
}

func TestNormalizeLogQueryAppliesDefaultsAndClosedGrammar(t *testing.T) {
	query, err := NormalizeLogQuery(LogQuery{Container: "api"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if query.TailLines != 200 || query.Timestamps == nil || !*query.Timestamps {
		t.Fatalf("defaults = %#v", query)
	}
	tests := []LogQuery{{Container: ""}, {Container: "INVALID_NAME"}, {Container: "api", TailLines: 2001}, {Container: "api", Since: "0m"}, {Container: "api", Since: "4h30m"}, {Container: "api", Since: "5h"}}
	for _, test := range tests {
		if _, err = NormalizeLogQuery(test, false); ErrorCodeOf(err) != CodeValidationFailed {
			t.Fatalf("query %#v: %v", test, err)
		}
	}
	if _, err = NormalizeLogQuery(LogQuery{Container: "api", Previous: true}, true); ErrorCodeOf(err) != CodeValidationFailed {
		t.Fatalf("previous follow: %v", err)
	}
}

func TestLogReadAuthorizesBeforeOpeningAndReturnsBoundedRedactedLines(t *testing.T) {
	port := &fakeLogPort{content: "2026-08-17T12:00:00Z token=secret\n" + strings.Repeat("x", MaximumLogLineBytes+100) + "\n"}
	service := allowedLogService(port)
	result, err := service.Read(context.Background(), logSelection(), LogTarget{Namespace: "payments", Pod: "api-1"}, LogQuery{Container: "api", Previous: true, Timestamps: boolPointer(true), TailLines: 20, Since: "15m"})
	if err != nil {
		t.Fatal(err)
	}
	if port.opens != 1 || port.targets[0].Container != "api" || !port.options[0].Previous || port.options[0].SinceSeconds == nil || *port.options[0].SinceSeconds != 900 {
		t.Fatalf("port call = %#v %#v", port.targets, port.options)
	}
	if len(result.Lines) != 2 || result.Lines[0].Timestamp == nil || strings.Contains(result.Lines[0].Text, "secret") || !result.Lines[1].Truncated || !result.Truncated {
		t.Fatalf("result = %#v", result)
	}
}

func TestLogReadNeverOpensPortWhenDenied(t *testing.T) {
	port := &fakeLogPort{content: "secret"}
	service := &LogService{Port: port, Authorizer: &fakeAuthorization{decisions: map[string]authorization.Decision{"payments": authorization.DecisionDenied}}}
	_, err := service.Read(context.Background(), logSelection(), LogTarget{Namespace: "payments", Pod: "api"}, LogQuery{Container: "api"})
	if ErrorCodeOf(err) != CodeForbidden || port.opens != 0 {
		t.Fatalf("err=%v opens=%d", err, port.opens)
	}
}

func TestLogFollowBoundsGiantUnterminatedLineAndSerializedEvent(t *testing.T) {
	port := &fakeLogPort{content: strings.Repeat("\"\\", MaximumLogLineBytes)}
	service := allowedLogService(port)
	events := []LogLineDTO{}
	terminal, err := service.Follow(context.Background(), logSelection(), LogTarget{Namespace: "payments", Pod: "api"}, LogQuery{Container: "api", Timestamps: boolPointer(false)}, func(line LogLineDTO) error { events = append(events, line); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Reason != "upstream_eof" || len(events) != 1 || !events[0].Truncated {
		t.Fatalf("terminal=%#v events=%#v", terminal, events)
	}
	encoded, _ := json.Marshal(events[0])
	if len(encoded) > MaximumLogEventBytes {
		t.Fatalf("event bytes = %d", len(encoded))
	}
}

func TestLogFollowSurfacesSlowConsumerWithoutDroppingSilently(t *testing.T) {
	service := allowedLogService(&fakeLogPort{content: "one\n"})
	_, err := service.Follow(context.Background(), logSelection(), LogTarget{Namespace: "payments", Pod: "api"}, LogQuery{Container: "api"}, func(LogLineDTO) error { return ErrSlowConsumer })
	if ErrorCodeOf(err) != CodeLimitExceeded {
		t.Fatalf("error = %v", err)
	}
}

func TestLogFollowEmitsEveryDocumentedTerminalReasonFromEvidence(t *testing.T) {
	target := LogTarget{Namespace: "payments", Pod: "api", Container: "api"}
	query := LogQuery{Container: "api", Timestamps: boolPointer(false)}
	emit := func(LogLineDTO) error { return nil }

	containerService := allowedLogService(&fakeLogPort{terminated: true})
	terminal, err := containerService.Follow(context.Background(), logSelection(), target, query, emit)
	if err != nil || terminal.Reason != "container_terminated" {
		t.Fatalf("container terminal=%#v err=%v", terminal, err)
	}

	limitService := allowedLogService(&fakeLogPort{content: strings.Repeat("x", MaximumLogFollowBytes+1)})
	terminal, err = limitService.Follow(context.Background(), logSelection(), target, query, emit)
	if err != nil || terminal.Reason != "limit_reached" || !terminal.Truncated {
		t.Fatalf("limit terminal=%#v err=%v", terminal, err)
	}

	durationPort := &blockingLogPort{opened: make(chan struct{})}
	durationService := allowedLogService(durationPort)
	durationContext, cancelDuration := context.WithTimeout(context.Background(), 10*time.Millisecond)
	terminal, err = durationService.Follow(durationContext, logSelection(), target, query, emit)
	cancelDuration()
	if err != nil || terminal.Reason != "duration_reached" {
		t.Fatalf("duration terminal=%#v err=%v", terminal, err)
	}

	for _, test := range []struct {
		name   string
		cause  error
		reason string
	}{
		{name: "generation", cause: context.Canceled, reason: "generation_changed"},
		{name: "shutdown", cause: lifecycle.ErrServerShutdown, reason: "server_shutdown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			port := &blockingLogPort{opened: make(chan struct{})}
			service := allowedLogService(port)
			followContext, cancelFollow := context.WithCancelCause(context.Background())
			type outcome struct {
				terminal FollowTerminal
				err      error
			}
			result := make(chan outcome, 1)
			go func() {
				value, followErr := service.Follow(followContext, logSelection(), target, query, emit)
				result <- outcome{terminal: value, err: followErr}
			}()
			<-port.opened
			cancelFollow(test.cause)
			select {
			case value := <-result:
				if value.err != nil || value.terminal.Reason != test.reason {
					t.Fatalf("terminal=%#v err=%v", value.terminal, value.err)
				}
			case <-time.After(time.Second):
				t.Fatal("follow did not terminate after cancellation")
			}
		})
	}

	upstreamService := allowedLogService(&fakeLogPort{})
	terminal, err = upstreamService.Follow(context.Background(), logSelection(), target, query, emit)
	if err != nil || terminal.Reason != "upstream_eof" {
		t.Fatalf("upstream terminal=%#v err=%v", terminal, err)
	}
}
