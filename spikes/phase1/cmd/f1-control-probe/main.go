package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/fvmoraes/kubepeep/spikes/phase1/control"
	"github.com/spf13/cobra"
)

type output struct {
	Running  bool                    `json:"running"`
	Instance *control.PublicInstance `json:"instance,omitempty"`
}

func main() {
	ctx, stopSignals := signal.NotifyContext(context.Background(), terminationSignals()...)
	defer stopSignals()

	command := newRootCommand(os.Stdout)
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand(stdout *os.File) *cobra.Command {
	var runtimeDir string

	root := &cobra.Command{
		Use:           "f1-control-probe",
		Short:         "Isolated F1-44 native process-control proof",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVar(
		&runtimeDir,
		"runtime-dir",
		"",
		"isolated runtime directory (required)",
	)
	_ = root.MarkPersistentFlagRequired("runtime-dir")

	var port int
	var attempts int
	var shutdownTimeout time.Duration
	start := &cobra.Command{
		Use:   "start",
		Short: "Run the probe in the foreground",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := normalizedRuntimeDir(runtimeDir)
			if err != nil {
				return err
			}
			return control.RunForeground(cmd.Context(), control.RunOptions{
				RuntimeDir:      dir,
				FirstPort:       port,
				PortAttempts:    attempts,
				ShutdownTimeout: shutdownTimeout,
				OnReady: func(instance control.PublicInstance) {
					_ = json.NewEncoder(stdout).Encode(output{
						Running:  true,
						Instance: &instance,
					})
				},
			})
		},
	}
	start.Flags().IntVar(&port, "port", 2748, "first loopback port; 0 selects an ephemeral port")
	start.Flags().IntVar(&attempts, "port-attempts", 50, "number of consecutive bind attempts")
	start.Flags().DurationVar(&shutdownTimeout, "shutdown-timeout", 2*time.Second, "HTTP shutdown timeout")

	status := &cobra.Command{
		Use:   "status",
		Short: "Authenticate and prove the running instance identity",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := normalizedRuntimeDir(runtimeDir)
			if err != nil {
				return err
			}
			requestContext, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			defer cancel()
			instance, err := control.Status(requestContext, dir)
			if err != nil {
				return err
			}
			return json.NewEncoder(stdout).Encode(output{
				Running:  true,
				Instance: &instance,
			})
		},
	}

	stop := &cobra.Command{
		Use:   "stop",
		Short: "Authenticate and request graceful foreground cancellation",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := normalizedRuntimeDir(runtimeDir)
			if err != nil {
				return err
			}
			requestContext, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			defer cancel()
			instance, running, err := control.Stop(requestContext, dir)
			if err != nil {
				return err
			}
			result := output{Running: running}
			if running {
				result.Instance = &instance
			}
			return json.NewEncoder(stdout).Encode(result)
		},
	}

	root.AddCommand(start, status, stop)
	return root
}

func normalizedRuntimeDir(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("runtime directory is required")
	}
	dir, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve runtime directory: %w", err)
	}
	return dir, nil
}
