package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
)

type fakeBrowser struct {
	urls []string
}

func (browser *fakeBrowser) Open(_ context.Context, rawURL string) error {
	browser.urls = append(browser.urls, rawURL)
	return nil
}

func cliLayout(t *testing.T) userdirs.Layout {
	t.Helper()
	layout, err := userdirs.ForRoot(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func cliIdentity(t *testing.T) localruntime.ControlIdentityDTO {
	t.Helper()
	state, err := localruntime.NewInstanceState(os.Getpid(), localruntime.DefaultFirstPort, "test-fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	return state.Identity()
}

type observedStart struct {
	Context       string
	ContextSet    bool
	Kubeconfig    string
	KubeconfigSet bool
	Namespace     string
	NamespaceSet  bool
	NoBrowser     bool
	Port          int
	PortSet       bool
}

func observe(options StartOptions) observedStart {
	result := observedStart{
		Context: options.Context, ContextSet: options.ContextSet,
		Kubeconfig: options.Kubeconfig, KubeconfigSet: options.KubeconfigSet,
		Namespace: options.Namespace, NamespaceSet: options.NamespaceSet,
		NoBrowser: options.NoBrowser,
	}
	if options.Port != nil {
		result.Port, result.PortSet = *options.Port, true
	}
	return result
}

func TestRootAndStartUseTheSameContract(t *testing.T) {
	identity := cliIdentity(t)
	arguments := []string{"--context", "dev", "--kubeconfig", "/tmp/config", "--namespace", "payments", "--no-browser", "--port", "43210"}
	var rootObserved, startObserved observedStart
	for index, commandArguments := range [][]string{arguments, append([]string{"start"}, arguments...)} {
		layout := cliLayout(t)
		dependencies := Dependencies{
			ResolveLayout: func() (userdirs.Layout, error) { return layout, nil },
			Browser:       &fakeBrowser{},
			Stdout:        &bytes.Buffer{},
			Stderr:        &bytes.Buffer{},
			Start: func(_ context.Context, options StartOptions) (localruntime.RunResult, error) {
				if options.OnReady != nil {
					options.OnReady(identity)
				}
				if index == 0 {
					rootObserved = observe(options)
				} else {
					startObserved = observe(options)
				}
				return localruntime.RunResult{Identity: identity, Existing: true}, nil
			},
		}
		if code := ExecuteContext(context.Background(), commandArguments, dependencies); code != ExitSuccess {
			t.Fatalf("args %v exit code = %d", commandArguments, code)
		}
	}
	if !reflect.DeepEqual(rootObserved, startObserved) {
		t.Fatalf("root options = %#v, start options = %#v", rootObserved, startObserved)
	}
}

func TestNoBrowserSuppressesLauncher(t *testing.T) {
	layout := cliLayout(t)
	browser := &fakeBrowser{}
	identity := cliIdentity(t)
	code := ExecuteContext(context.Background(), []string{"--no-browser"}, Dependencies{
		ResolveLayout: func() (userdirs.Layout, error) { return layout, nil },
		Browser:       browser,
		Stdout:        &bytes.Buffer{},
		Stderr:        &bytes.Buffer{},
		Start: func(_ context.Context, options StartOptions) (localruntime.RunResult, error) {
			options.OnReady(identity)
			return localruntime.RunResult{Identity: identity, Existing: true}, nil
		},
	})
	if code != ExitSuccess || len(browser.urls) != 0 {
		t.Fatalf("exit=%d browser URLs=%v", code, browser.urls)
	}
}

func TestExplicitInvalidPortIsInvocationError(t *testing.T) {
	called := false
	stderr := &bytes.Buffer{}
	code := ExecuteContext(context.Background(), []string{"--port", "1023"}, Dependencies{
		Start: func(context.Context, StartOptions) (localruntime.RunResult, error) {
			called = true
			return localruntime.RunResult{}, nil
		},
		Stdout: &bytes.Buffer{}, Stderr: stderr,
	})
	if code != ExitInvalid || called {
		t.Fatalf("exit=%d called=%v stderr=%q", code, called, stderr.String())
	}
}

func TestDefaultCompositionIsAvailable(t *testing.T) {
	layout := cliLayout(t)
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code := ExecuteContext(ctx, nil, Dependencies{
		ResolveLayout: func() (userdirs.Layout, error) { return layout, nil },
		Stdout:        &bytes.Buffer{}, Stderr: stderr,
	})
	if code != ExitSuccess || bytes.Contains(stderr.Bytes(), []byte("service factory is not configured")) {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
}

func TestStatusAndStopWithoutStateHaveDefinedExitCodes(t *testing.T) {
	for _, test := range []struct {
		command string
		code    int
	}{
		{command: "status", code: ExitDegraded},
		{command: "stop", code: ExitSuccess},
	} {
		layout := cliLayout(t)
		stdout := &bytes.Buffer{}
		code := ExecuteContext(context.Background(), []string{test.command}, Dependencies{
			ResolveLayout: func() (userdirs.Layout, error) { return layout, nil },
			Stdout:        stdout, Stderr: &bytes.Buffer{},
		})
		if code != test.code || stdout.String() != "not running\n" {
			t.Fatalf("%s exit=%d output=%q", test.command, code, stdout.String())
		}
	}
}

func TestStartCleanupFailureUsesOperationalExitCode(t *testing.T) {
	layout := cliLayout(t)
	stderr := &bytes.Buffer{}
	cleanupFailure := errors.New("synthetic cleanup failure")
	code := ExecuteContext(context.Background(), []string{"start", "--no-browser"}, Dependencies{
		ResolveLayout: func() (userdirs.Layout, error) { return layout, nil },
		Start: func(context.Context, StartOptions) (localruntime.RunResult, error) {
			return localruntime.RunResult{Started: true}, cleanupFailure
		},
		Stdout: &bytes.Buffer{},
		Stderr: stderr,
	})
	if code != ExitOperational || !bytes.Contains(stderr.Bytes(), []byte("synthetic cleanup failure")) {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
}

func TestVersionPrintsAllBuildFields(t *testing.T) {
	stdout := &bytes.Buffer{}
	if code := ExecuteContext(context.Background(), []string{"version"}, Dependencies{Stdout: stdout, Stderr: &bytes.Buffer{}}); code != ExitSuccess {
		t.Fatalf("exit = %d", code)
	}
	for _, field := range []string{"version=", "commit=", "build_date="} {
		if !bytes.Contains(stdout.Bytes(), []byte(field)) {
			t.Fatalf("version output %q lacks %q", stdout.String(), field)
		}
	}
}

func TestDoctorJSONIsOneStrictObject(t *testing.T) {
	layout := cliLayout(t)
	stdout := &bytes.Buffer{}
	checker := DoctorCheckerFunc(func(context.Context, userdirs.Layout) ([]DoctorCheck, error) {
		return []DoctorCheck{{Group: "build", Name: "metadata", Status: DoctorPass, Code: "OK", Message: "Ready."}}, nil
	})
	code := ExecuteContext(context.Background(), []string{"doctor", "--json"}, Dependencies{
		ResolveLayout: func() (userdirs.Layout, error) { return layout, nil },
		Doctor:        checker, Stdout: stdout, Stderr: &bytes.Buffer{},
	})
	if code != ExitSuccess {
		t.Fatalf("exit = %d", code)
	}
	if bytes.HasSuffix(stdout.Bytes(), []byte("\n")) {
		t.Fatalf("doctor JSON has trailing newline: %q", stdout.String())
	}
	var report DoctorReport
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != DoctorSchemaVersion || report.Overall != DoctorPass || len(report.Checks) != 1 {
		t.Fatalf("report = %#v", report)
	}
}
