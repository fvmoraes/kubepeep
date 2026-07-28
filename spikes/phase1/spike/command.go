package spike

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

type StartOptions struct {
	Context    string
	Kubeconfig string
	Namespace  string
	NoBrowser  bool
	Port       int
}

type StartFunc func(context.Context, StartOptions) error

func NewRootCommand(start StartFunc) *cobra.Command {
	var options StartOptions

	runStart := func(cmd *cobra.Command, _ []string) error {
		if options.Port < 1 || options.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
		return start(cmd.Context(), options)
	}

	root := &cobra.Command{
		Use:           "kubePeep",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          runStart,
	}
	root.PersistentFlags().StringVar(&options.Context, "context", "", "Kubernetes context")
	root.PersistentFlags().StringVar(&options.Kubeconfig, "kubeconfig", "", "kubeconfig path")
	root.PersistentFlags().StringVar(&options.Namespace, "namespace", "", "initial namespace")
	root.PersistentFlags().BoolVar(&options.NoBrowser, "no-browser", false, "do not open a browser")
	root.PersistentFlags().IntVar(&options.Port, "port", 2748, "first local port to bind")

	root.AddCommand(&cobra.Command{
		Use:  "start",
		RunE: runStart,
	})

	return root
}
