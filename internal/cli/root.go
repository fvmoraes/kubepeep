package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"

	productconfig "github.com/fvmoraes/kubepeep/internal/config"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
	"github.com/spf13/cobra"
)

type startFlags struct {
	context    string
	kubeconfig string
	namespace  string
	noBrowser  bool
	port       int
}

// NewRootCommand creates kubePeep's complete Phase 3 command surface.
func NewRootCommand(dependencies Dependencies) *cobra.Command {
	dependencies = normalizeDependencies(dependencies)
	rootFlags := &startFlags{}
	root := &cobra.Command{
		Use:           "kubePeep",
		Short:         "Local Kubernetes observability dashboard",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	bindStartFlags(root, rootFlags)
	root.RunE = runStart(dependencies, rootFlags)

	startFlags := &startFlags{}
	start := &cobra.Command{Use: "start", Short: "Start kubePeep in the foreground", Args: cobra.NoArgs}
	bindStartFlags(start, startFlags)
	start.RunE = runStart(dependencies, startFlags)

	status := &cobra.Command{
		Use: "status", Short: "Show local process status", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			layout, err := dependencies.ResolveLayout()
			if err != nil {
				return &ExitError{Code: ExitOperational, Err: err}
			}
			identity, active, err := dependencies.Controller.Status(command.Context(), layout.RuntimeDir)
			if err != nil {
				return &ExitError{Code: ExitOperational, Err: err}
			}
			if !active {
				_, _ = fmt.Fprintln(command.OutOrStdout(), "not running")
				return &ExitError{Code: ExitDegraded, Silent: true}
			}
			printRunning(command, identity)
			return nil
		},
	}

	stop := &cobra.Command{
		Use: "stop", Short: "Stop the authenticated local instance", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			layout, err := dependencies.ResolveLayout()
			if err != nil {
				return &ExitError{Code: ExitOperational, Err: err}
			}
			identity, active, err := dependencies.Controller.Stop(command.Context(), layout.RuntimeDir)
			if err != nil {
				return &ExitError{Code: ExitOperational, Err: err}
			}
			if !active {
				_, _ = fmt.Fprintln(command.OutOrStdout(), "not running")
				return nil
			}
			_, _ = fmt.Fprintf(command.OutOrStdout(), "stop requested pid=%d port=%d protocol=%s\n", identity.PID, identity.Port, identity.Protocol)
			return nil
		},
	}

	version := &cobra.Command{
		Use: "version", Short: "Print build metadata", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(command.OutOrStdout(), versionLine())
			return err
		},
	}

	doctor := newDoctorCommand(dependencies)
	update := newUpdateCommand(dependencies)
	root.AddCommand(start, stop, status, version, doctor, update)
	return root
}

func bindStartFlags(command *cobra.Command, flags *startFlags) {
	command.Flags().StringVar(&flags.context, "context", "", "Kubernetes context")
	command.Flags().StringVar(&flags.kubeconfig, "kubeconfig", "", "ordered kubeconfig path list")
	command.Flags().StringVar(&flags.namespace, "namespace", "", "one-time initial namespace")
	command.Flags().BoolVar(&flags.noBrowser, "no-browser", false, "do not open the browser")
	command.Flags().IntVar(&flags.port, "port", 0, "local port (1024-65535)")
}

func runStart(dependencies Dependencies, flags *startFlags) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, _ []string) error {
		if command.Flags().Changed("context") && strings.TrimSpace(flags.context) == "" {
			return &ExitError{Code: ExitInvalid, Err: errors.New("context must not be empty")}
		}
		if command.Flags().Changed("kubeconfig") && strings.TrimSpace(flags.kubeconfig) == "" {
			return &ExitError{Code: ExitInvalid, Err: errors.New("kubeconfig must not be empty")}
		}
		if command.Flags().Changed("namespace") {
			if flags.namespace == "*" {
				return &ExitError{Code: ExitInvalid, Err: errors.New("namespace must be one explicit Kubernetes namespace, not *")}
			}
			if problems := validation.IsDNS1123Label(flags.namespace); len(problems) != 0 {
				return &ExitError{Code: ExitInvalid, Err: errors.New("namespace is not a valid Kubernetes namespace name")}
			}
		}
		var explicitPort *int
		if command.Flags().Changed("port") {
			if flags.port < localruntime.MinimumPort || flags.port > localruntime.MaximumPort {
				return &ExitError{Code: ExitInvalid, Err: fmt.Errorf("port must be between %d and %d", localruntime.MinimumPort, localruntime.MaximumPort)}
			}
			port := flags.port
			explicitPort = &port
		}
		layout, err := dependencies.ResolveLayout()
		if err != nil {
			return &ExitError{Code: ExitOperational, Err: err}
		}
		if err := layout.EnsureDirectories(); err != nil {
			return &ExitError{Code: ExitOperational, Err: err}
		}
		fileConfig, err := productconfig.Load(layout.Config)
		if err != nil {
			return &ExitError{Code: ExitInvalid, Err: err}
		}
		effectiveConfig, err := productconfig.ApplyFlags(fileConfig, productconfig.FlagOverrides{
			Port:      explicitPort,
			NoBrowser: flags.noBrowser,
		})
		if err != nil {
			return &ExitError{Code: ExitInvalid, Err: err}
		}
		ready := func(identity localruntime.ControlIdentityDTO) {
			printRunning(command, identity)
			if !effectiveConfig.Server.OpenBrowser {
				return
			}
			if err := dependencies.Browser.Open(command.Context(), identity.URL()); err != nil {
				_, _ = fmt.Fprintln(command.ErrOrStderr(), "warning:", sanitizeCLIError(err))
			}
		}
		_, err = dependencies.Start(command.Context(), StartOptions{
			Layout:        layout,
			Context:       flags.context,
			ContextSet:    command.Flags().Changed("context"),
			Kubeconfig:    flags.kubeconfig,
			KubeconfigSet: command.Flags().Changed("kubeconfig"),
			Namespace:     flags.namespace,
			NamespaceSet:  command.Flags().Changed("namespace"),
			NoBrowser:     flags.noBrowser,
			Port:          effectiveConfig.Server.Port,
			Config:        effectiveConfig,
			LogOutput:     dependencies.Stdout,
			OnReady:       ready,
		})
		if err != nil {
			code := ExitOperational
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return &ExitError{Code: code, Err: err}
		}
		return nil
	}
}

func printRunning(command *cobra.Command, identity localruntime.ControlIdentityDTO) {
	_, _ = fmt.Fprintf(command.OutOrStdout(), "running pid=%d port=%d protocol=%s\n", identity.PID, identity.Port, identity.Protocol)
}
