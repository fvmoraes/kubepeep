package resources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/authorization"
)

func TestLogDownloadReadsAllRetainedLinesWithRedaction(t *testing.T) {
	for _, previous := range []bool{false, true} {
		name := "current"
		if previous {
			name = "previous"
		}
		t.Run(name, func(t *testing.T) {
			port := &fakeLogPort{content: strings.Repeat("2026-10-06T12:00:00Z secret\n", 3000)}
			count := 0
			terminal, err := allowedLogService(port).Download(t.Context(), logSelection(), LogTarget{Namespace: "payments", Pod: "api"}, LogQuery{Container: "api", Previous: previous}, func(line LogLineDTO) error {
				count++
				if line.Text != "[REDACTED]" || line.Timestamp == nil {
					t.Fatalf("unsafe line: %#v", line)
				}
				return nil
			})
			if err != nil || count != 3000 || terminal.Reason != "completed" || terminal.Truncated {
				t.Fatalf("count=%d terminal=%+v err=%v", count, terminal, err)
			}
			options := port.options[0]
			if options.TailLines != -1 || options.SinceSeconds != nil || options.Follow || options.Previous != previous {
				t.Fatalf("options=%+v", options)
			}
		})
	}
}

func TestLogDownloadRejectsFiltersAndDeniedTargets(t *testing.T) {
	for _, test := range []struct {
		name   string
		query  LogQuery
		denied bool
		code   ErrorCode
	}{
		{"tail", LogQuery{Container: "api", TailLines: 1}, false, CodeValidationFailed},
		{"since", LogQuery{Container: "api", Since: "1m"}, false, CodeValidationFailed},
		{"denied", LogQuery{Container: "api"}, true, CodeForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			port := &fakeLogPort{}
			service := allowedLogService(port)
			if test.denied {
				service.Authorizer = &fakeAuthorization{decisions: map[string]authorization.Decision{"payments": authorization.DecisionDenied}}
			}
			_, err := service.Download(t.Context(), logSelection(), LogTarget{Namespace: "payments", Pod: "api"}, test.query, func(LogLineDTO) error { return nil })
			if ErrorCodeOf(err) != test.code || port.opens != 0 {
				t.Fatalf("err=%v opens=%d", err, port.opens)
			}
		})
	}
}

func TestLogDownloadReportsTruncationAndCancellation(t *testing.T) {
	port := &fakeLogPort{content: strings.Repeat("x", MaximumLogLineBytes+1)}
	terminal, err := allowedLogService(port).Download(t.Context(), logSelection(), LogTarget{Namespace: "payments", Pod: "api"}, LogQuery{Container: "api"}, func(line LogLineDTO) error {
		if !line.Truncated || len(line.Text) > MaximumLogLineBytes {
			t.Fatalf("line was not bounded")
		}
		return nil
	})
	if err != nil || !terminal.Truncated {
		t.Fatalf("terminal=%+v err=%v", terminal, err)
	}
	port.content = "one\ntwo\n"
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	count := 0
	_, err = allowedLogService(port).Download(ctx, logSelection(), LogTarget{Namespace: "payments", Pod: "api"}, LogQuery{Container: "api"}, func(LogLineDTO) error { count++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
