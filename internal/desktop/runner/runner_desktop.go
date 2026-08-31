//go:build desktop

package runner

import (
	"context"

	"github.com/fvmoraes/kubepeep/internal/desktop"
	wailsglue "github.com/fvmoraes/kubepeep/internal/desktop/wails"
)

func init() {
	Enabled = true
	Run = func(ctx context.Context, options desktop.RunOptions) error {
		return wailsglue.Run(ctx, wailsglue.Params{
			Layout:        options.Layout,
			Config:        options.Config,
			Context:       options.Context,
			ContextSet:    options.ContextSet,
			Kubeconfig:    options.Kubeconfig,
			KubeconfigSet: options.KubeconfigSet,
			Namespace:     options.Namespace,
			NamespaceSet:  options.NamespaceSet,
			Port:          options.Port,
			LogOutput:     options.LogOutput,
			ExtraHosts:    desktop.DesktopExtraHosts(),
			ExtraOrigins:  desktop.DesktopExtraOrigins(),
		})
	}
}
