package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	"github.com/fvmoraes/kubepeep/internal/buildinfo"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
	"github.com/spf13/cobra"
)

const DoctorSchemaVersion = 1

type DoctorStatus string

const (
	DoctorPass DoctorStatus = "pass"
	DoctorWarn DoctorStatus = "warn"
	DoctorFail DoctorStatus = "fail"
	DoctorSkip DoctorStatus = "skip"
)

// DoctorCheck is the fixed five-field diagnostic contract.
type DoctorCheck struct {
	Group   string       `json:"group"`
	Name    string       `json:"name"`
	Status  DoctorStatus `json:"status"`
	Code    string       `json:"code"`
	Message string       `json:"message"`
}

// DoctorReport is emitted directly, without an HTTP envelope.
type DoctorReport struct {
	Schema  int           `json:"schema"`
	Overall DoctorStatus  `json:"overall"`
	Checks  []DoctorCheck `json:"checks"`
}

type DoctorChecker interface {
	Check(context.Context, userdirs.Layout) ([]DoctorCheck, error)
}

type DoctorCheckerFunc func(context.Context, userdirs.Layout) ([]DoctorCheck, error)

func (checker DoctorCheckerFunc) Check(ctx context.Context, layout userdirs.Layout) ([]DoctorCheck, error) {
	return checker(ctx, layout)
}

// CompositeDoctor lets config, SQLite and frontend slices append their checks
// without coupling the CLI to their implementations.
type CompositeDoctor []DoctorChecker

func (composite CompositeDoctor) Check(ctx context.Context, layout userdirs.Layout) ([]DoctorCheck, error) {
	var checks []DoctorCheck
	for _, checker := range composite {
		if checker == nil {
			continue
		}
		part, err := checker.Check(ctx, layout)
		if err != nil {
			return nil, err
		}
		checks = append(checks, part...)
	}
	return checks, nil
}

// LocalDoctor covers build, canonical paths, runtime state/lock and loopback
// bind. SQLite/frontend/config checkers can be composed by their owning slices.
type LocalDoctor struct{}

func (LocalDoctor) Check(ctx context.Context, layout userdirs.Layout) ([]DoctorCheck, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	checks := []DoctorCheck{
		{Group: "build", Name: "metadata", Status: DoctorPass, Code: "BUILD_METADATA_AVAILABLE", Message: fmt.Sprintf("kubePeep %s (%s, %s).", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate)},
		{Group: "build", Name: "platform", Status: DoctorPass, Code: "PLATFORM_SUPPORTED", Message: fmt.Sprintf("Go %s on %s/%s.", runtime.Version(), runtime.GOOS, runtime.GOARCH)},
	}
	if err := layout.EnsureDirectories(); err != nil {
		checks = append(checks, DoctorCheck{Group: "diretórios", Name: "canonical_paths", Status: DoctorFail, Code: "LOCAL_PATHS_UNAVAILABLE", Message: "Canonical local directories could not be prepared."})
	} else {
		checks = append(checks, DoctorCheck{Group: "diretórios", Name: "canonical_paths", Status: DoctorPass, Code: "LOCAL_PATHS_READY", Message: "Canonical local directories are private and available."})
	}

	state, stateErr := localruntime.LoadInstanceState(layout.RuntimeDir)
	switch {
	case stateErr == nil:
		checks = append(checks, DoctorCheck{Group: "processo", Name: "instance_state", Status: DoctorPass, Code: "INSTANCE_STATE_VALID", Message: fmt.Sprintf("Published runtime state is valid on port %d.", state.Port)})
	case errors.Is(stateErr, localruntime.ErrNotRunning):
		checks = append(checks, DoctorCheck{Group: "processo", Name: "instance_state", Status: DoctorSkip, Code: "INSTANCE_NOT_RUNNING", Message: "No published local instance is present."})
	default:
		checks = append(checks, DoctorCheck{Group: "processo", Name: "instance_state", Status: DoctorFail, Code: "INSTANCE_STATE_INVALID", Message: "Published runtime state is unsafe or invalid."})
	}

	lock, lockErr := localruntime.AcquireFileLock(layout.Lock)
	switch {
	case lockErr == nil:
		closeErr := lock.Close()
		if closeErr != nil {
			checks = append(checks, DoctorCheck{Group: "processo", Name: "instance_lock", Status: DoctorFail, Code: "LOCK_RELEASE_FAILED", Message: "The local instance lock could not be released safely."})
		} else {
			checks = append(checks, DoctorCheck{Group: "processo", Name: "instance_lock", Status: DoctorPass, Code: "LOCK_AVAILABLE", Message: "The local instance lock is available."})
		}
	case errors.Is(lockErr, localruntime.ErrLocked):
		checks = append(checks, DoctorCheck{Group: "processo", Name: "instance_lock", Status: DoctorPass, Code: "LOCK_HELD", Message: "The local instance lock is held by a process."})
	default:
		checks = append(checks, DoctorCheck{Group: "processo", Name: "instance_lock", Status: DoctorFail, Code: "LOCK_UNAVAILABLE", Message: "The local instance lock could not be inspected."})
	}

	listener, port, bindErr := localruntime.BindLoopback(nil)
	if bindErr != nil {
		checks = append(checks, DoctorCheck{Group: "segurança", Name: "loopback_bind", Status: DoctorFail, Code: "LOOPBACK_BIND_FAILED", Message: "No approved loopback port could be acquired."})
	} else {
		closeErr := listener.Close()
		if closeErr != nil {
			checks = append(checks, DoctorCheck{Group: "segurança", Name: "loopback_bind", Status: DoctorFail, Code: "LOOPBACK_CLOSE_FAILED", Message: "The loopback diagnostic listener could not be closed."})
		} else {
			checks = append(checks, DoctorCheck{Group: "segurança", Name: "loopback_bind", Status: DoctorPass, Code: "LOOPBACK_BIND_READY", Message: fmt.Sprintf("Loopback port %d can be acquired by real bind.", port)})
		}
	}

	checks = append(checks, observabilityChecks(ctx, layout, state, stateErr)...)
	return checks, nil
}

// observabilityChecks covers the observability group (F5-04): the local log
// sink must be writable and, when a local instance is running, the optional
// /metrics endpoint must behave per its opt-in contract.
func observabilityChecks(ctx context.Context, layout userdirs.Layout, state localruntime.InstanceStateV1, stateErr error) []DoctorCheck {
	checks := make([]DoctorCheck, 0, 2)
	sink, sinkErr := os.OpenFile(layout.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if sinkErr != nil {
		checks = append(checks, DoctorCheck{Group: "observabilidade", Name: "log_sink", Status: DoctorFail, Code: "LOG_SINK_UNAVAILABLE", Message: "The local JSONL log file could not be opened for append."})
	} else {
		_ = sink.Close()
		checks = append(checks, DoctorCheck{Group: "observabilidade", Name: "log_sink", Status: DoctorPass, Code: "LOG_SINK_READY", Message: "The local JSONL log file is writable and private."})
	}

	switch {
	case stateErr != nil:
		if errors.Is(stateErr, localruntime.ErrNotRunning) {
			checks = append(checks, DoctorCheck{Group: "observabilidade", Name: "metrics_endpoint", Status: DoctorSkip, Code: "METRICS_NOT_RUNNING", Message: "No local instance is running; /metrics was not probed."})
		} else {
			checks = append(checks, DoctorCheck{Group: "observabilidade", Name: "metrics_endpoint", Status: DoctorSkip, Code: "METRICS_INSTANCE_UNKNOWN", Message: "Runtime state is unavailable; /metrics was not probed."})
		}
	default:
		endpoint := fmt.Sprintf("http://127.0.0.1:%d/metrics", state.Port)
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if requestErr != nil {
			checks = append(checks, DoctorCheck{Group: "observabilidade", Name: "metrics_endpoint", Status: DoctorWarn, Code: "METRICS_PROBE_INVALID", Message: "The /metrics probe could not be constructed."})
			break
		}
		client := &http.Client{Timeout: 2 * time.Second}
		response, probeErr := client.Do(request)
		if probeErr != nil {
			checks = append(checks, DoctorCheck{Group: "observabilidade", Name: "metrics_endpoint", Status: DoctorWarn, Code: "METRICS_UNREACHABLE", Message: "A local instance is running but /metrics could not be reached."})
			break
		}
		defer response.Body.Close()
		switch response.StatusCode {
		case http.StatusOK:
			checks = append(checks, DoctorCheck{Group: "observabilidade", Name: "metrics_endpoint", Status: DoctorPass, Code: "METRICS_READY", Message: "The optional /metrics endpoint is enabled and responding on loopback."})
		case http.StatusNotFound:
			checks = append(checks, DoctorCheck{Group: "observabilidade", Name: "metrics_endpoint", Status: DoctorSkip, Code: "METRICS_DISABLED", Message: "The optional /metrics endpoint is disabled, as configured by default."})
		default:
			checks = append(checks, DoctorCheck{Group: "observabilidade", Name: "metrics_endpoint", Status: DoctorWarn, Code: "METRICS_UNEXPECTED", Message: fmt.Sprintf("The /metrics endpoint answered with HTTP %d.", response.StatusCode)})
		}
	}
	return checks
}

func newDoctorCommand(dependencies Dependencies) *cobra.Command {
	var outputJSON bool
	command := &cobra.Command{
		Use: "doctor", Short: "Run sanitized local diagnostics", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			layout, err := dependencies.ResolveLayout()
			if err != nil {
				return &ExitError{Code: ExitInternal, Err: errors.New("doctor could not resolve local paths")}
			}
			checks, err := dependencies.Doctor.Check(command.Context(), layout)
			if err != nil {
				return &ExitError{Code: ExitInternal, Err: errors.New("doctor could not construct the report")}
			}
			report, err := buildDoctorReport(checks)
			if err != nil {
				return &ExitError{Code: ExitInternal, Err: errors.New("doctor produced an invalid report")}
			}
			if outputJSON {
				data, err := json.Marshal(report)
				if err != nil {
					return &ExitError{Code: ExitInternal, Err: errors.New("doctor could not encode the report")}
				}
				if _, err := command.OutOrStdout().Write(data); err != nil {
					return &ExitError{Code: ExitInternal, Err: errors.New("doctor could not write the report")}
				}
			} else {
				for _, check := range report.Checks {
					_, _ = fmt.Fprintf(command.OutOrStdout(), "[%s] %s/%s %s %s\n", check.Status, check.Group, check.Name, check.Code, check.Message)
				}
			}
			switch report.Overall {
			case DoctorFail:
				return &ExitError{Code: ExitOperational, Silent: true}
			case DoctorWarn:
				return &ExitError{Code: ExitDegraded, Silent: true}
			default:
				return nil
			}
		},
	}
	command.Flags().BoolVar(&outputJSON, "json", false, "emit strict JSON")
	return command
}

func buildDoctorReport(checks []DoctorCheck) (DoctorReport, error) {
	groups := map[string]int{"build": 0, "diretórios": 1, "processo": 2, "configuração": 3, "SQLite": 4, "kubeconfig": 5, "cluster": 6, "segurança": 7, "observabilidade": 8}
	result := append([]DoctorCheck(nil), checks...)
	for index := range result {
		check := &result[index]
		if _, ok := groups[check.Group]; !ok || !asciiDiagnosticIdentifier(check.Name) || !asciiDiagnosticIdentifier(check.Code) {
			return DoctorReport{}, errors.New("invalid doctor check identity")
		}
		switch check.Status {
		case DoctorPass, DoctorWarn, DoctorFail, DoctorSkip:
		default:
			return DoctorReport{}, errors.New("invalid doctor check status")
		}
		check.Message = strings.Join(strings.Fields(check.Message), " ")
		if check.Message == "" {
			return DoctorReport{}, errors.New("empty doctor check message")
		}
		if len(check.Message) > 1024 {
			check.Message = check.Message[:1024]
		}
	}
	sort.SliceStable(result, func(left, right int) bool {
		leftGroup, rightGroup := groups[result[left].Group], groups[result[right].Group]
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		return result[left].Name < result[right].Name
	})
	overall := DoctorPass
	for _, check := range result {
		if check.Status == DoctorFail {
			overall = DoctorFail
			break
		}
		if check.Status == DoctorWarn {
			overall = DoctorWarn
		}
	}
	return DoctorReport{Schema: DoctorSchemaVersion, Overall: overall, Checks: result}, nil
}

func asciiDiagnosticIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}
