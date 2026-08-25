// Package cli defines the Cobra command surface and maps runtime outcomes to
// stable process exit codes.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fvmoraes/kubepeep/internal/adapters/browser"
	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	"github.com/fvmoraes/kubepeep/internal/buildinfo"
	productconfig "github.com/fvmoraes/kubepeep/internal/config"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
	"github.com/fvmoraes/kubepeep/internal/updater"
)

const (
	ExitSuccess     = 0
	ExitInvalid     = 1
	ExitOperational = 2
	ExitDegraded    = 3
	ExitInternal    = 4
)

// StartOptions preserves which CLI values were explicit so composition can
// apply flag > config > default precedence without guessing from zero values.
type StartOptions struct {
	Layout        userdirs.Layout
	Context       string
	ContextSet    bool
	Kubeconfig    string
	KubeconfigSet bool
	Namespace     string
	NamespaceSet  bool
	NoBrowser     bool
	Port          *int
	Config        productconfig.Config
	LogOutput     io.Writer
	OnReady       func(localruntime.ControlIdentityDTO)
}

type StartFunc func(context.Context, StartOptions) (localruntime.RunResult, error)

type Browser interface {
	Open(context.Context, string) error
}

type UpdateService interface {
	Update(context.Context, updater.Request) (updater.Result, error)
}

// Dependencies makes process creation, filesystem resolution and service
// composition replaceable in tests.
type Dependencies struct {
	Start         StartFunc
	ResolveLayout func() (userdirs.Layout, error)
	Controller    localruntime.Controller
	Browser       Browser
	Doctor        DoctorChecker
	Updater       UpdateService
	Stdout        io.Writer
	Stderr        io.Writer
}

// Execute is the production entrypoint used by cmd/kubePeep.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ExecuteContext(ctx, os.Args[1:], Dependencies{})
}

// ExecuteContext runs the command with explicit arguments and dependencies.
func ExecuteContext(ctx context.Context, args []string, dependencies Dependencies) int {
	dependencies = normalizeDependencies(dependencies)
	command := NewRootCommand(dependencies)
	command.SetArgs(args)
	command.SetOut(dependencies.Stdout)
	command.SetErr(dependencies.Stderr)
	err := command.ExecuteContext(ctx)
	if err == nil {
		return ExitSuccess
	}
	var exitError *ExitError
	if errors.As(err, &exitError) {
		if !exitError.Silent && exitError.Err != nil {
			_, _ = fmt.Fprintln(dependencies.Stderr, sanitizeCLIError(exitError.Err))
		}
		return exitError.Code
	}
	_, _ = fmt.Fprintln(dependencies.Stderr, sanitizeCLIError(err))
	return ExitInvalid
}

// ExitError carries a stable process exit code without forcing os.Exit inside
// handlers or the runtime coordinator.
type ExitError struct {
	Code   int
	Err    error
	Silent bool
}

func (err *ExitError) Error() string {
	if err.Err == nil {
		return fmt.Sprintf("command exited with code %d", err.Code)
	}
	return err.Err.Error()
}
func (err *ExitError) Unwrap() error { return err.Err }

func normalizeDependencies(dependencies Dependencies) Dependencies {
	if dependencies.Start == nil {
		dependencies.Start = productionStart
	}
	if dependencies.ResolveLayout == nil {
		dependencies.ResolveLayout = userdirs.Resolve
	}
	if dependencies.Browser == nil {
		dependencies.Browser = browser.Launcher{}
	}
	if dependencies.Doctor == nil {
		dependencies.Doctor = ProductionDoctor{}
	}
	if dependencies.Updater == nil {
		dependencies.Updater = updater.Default()
	}
	if dependencies.Stdout == nil {
		dependencies.Stdout = os.Stdout
	}
	if dependencies.Stderr == nil {
		dependencies.Stderr = os.Stderr
	}
	return dependencies
}

func sanitizeCLIError(err error) string {
	if err == nil {
		return "operation failed"
	}
	message := err.Error()
	if len(message) > 1024 {
		message = message[:1024]
	}
	for _, separator := range []string{"\r", "\n", "\t"} {
		message = strings.ReplaceAll(message, separator, " ")
	}
	return message
}

func versionLine() string {
	return fmt.Sprintf("version=%s commit=%s build_date=%s", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate)
}
