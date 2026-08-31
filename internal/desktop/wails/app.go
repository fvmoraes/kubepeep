//go:build desktop

// Package wails hosts the desktop-only glue between the Kube Peep core and
// Wails v2. It is compiled exclusively under the desktop build tag, keeping
// the Wails dependency out of the web binary and out of the business rules.
// This package deliberately imports no internal/desktop code.
package wails

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	"github.com/fvmoraes/kubepeep/internal/application"
	productconfig "github.com/fvmoraes/kubepeep/internal/config"
	"github.com/fvmoraes/kubepeep/internal/desktop"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
	"github.com/fvmoraes/kubepeep/internal/web"
)

//go:embed appicon.png
var iconBytes []byte

// Params carries the plain inputs the desktop glue needs. It is defined here,
// without importing internal/desktop, so the desktop runner can map its own
// options into it and avoid an import cycle.
type Params struct {
	Layout        userdirs.Layout
	Config        productconfig.Config
	Context       string
	ContextSet    bool
	Kubeconfig    string
	KubeconfigSet bool
	Namespace     string
	NamespaceSet  bool
	Port          *int
	LogOutput     io.Writer
	ExtraHosts    []string
	ExtraOrigins  []string
}

// Run composes the application core, starts the internal loopback listener
// reserved for streaming transports, and opens the native Wails window. The
// SPA assets are served from the embedded filesystem; no external browser is
// ever opened.
func Run(ctx context.Context, params Params) error {
	listener, port, err := localruntime.BindLoopback(params.Port)
	if err != nil {
		return fmt.Errorf("desktop: bind loopback: %w", err)
	}
	cleanups := localruntime.CleanupRegistry{}
	if err := cleanups.Add("loopback listener", func(context.Context) error { return listener.Close() }); err != nil {
		_ = listener.Close()
		return err
	}

	platform, err := application.Compose(ctx, application.Options{
		Layout:        params.Layout,
		Config:        params.Config,
		Kubeconfig:    params.Kubeconfig,
		KubeconfigSet: params.KubeconfigSet,
		Context:       params.Context,
		ContextSet:    params.ContextSet,
		Namespace:     params.Namespace,
		NamespaceSet:  params.NamespaceSet,
		LogOutput:     params.LogOutput,
		Port:          port,
		ExtraHosts:    params.ExtraHosts,
		ExtraOrigins:  params.ExtraOrigins,
	})
	if err != nil {
		_ = listener.Close()
		return err
	}
	for _, cleanup := range platform.Cleanups {
		cleanup := cleanup
		if err := cleanups.Add(cleanup.Name, cleanup.Func); err != nil {
			_ = listener.Close()
			return err
		}
	}
	loopback, err := desktop.NewLoopbackFromListener(listener, port, platform.Handler, params.ExtraOrigins)
	if err != nil {
		_ = listener.Close()
		return err
	}
	if err := cleanups.Add("internal loopback server", func(shutdown context.Context) error {
		closeContext, cancel := context.WithTimeout(shutdown, 5*time.Second)
		defer cancel()
		return loopback.Close(closeContext)
	}); err != nil {
		_ = listener.Close()
		return err
	}

	frontendFS, err := web.Embedded()
	if err != nil {
		return err
	}
	bridge := desktop.NewBridge(platform.Handler, platform.Origin, loopback.Base())

	applicationOptions := &options.App{
		Title:            "Kube Peep",
		Width:            1360,
		Height:           860,
		MinWidth:         1024,
		MinHeight:        640,
		BackgroundColour: &options.RGBA{R: 22, G: 18, B: 31, A: 255},
		AssetServer: &assetserver.Options{
			Assets:  frontendFS,
			Handler: platform.Handler,
		},
		Bind: []interface{}{bridge},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "kubepeep-desktop",
		},
		Windows: &windows.Options{
			Theme:        windows.SystemDefault,
			BackdropType: windows.Auto,
		},
		Linux: &linux.Options{
			Icon:             iconBytes,
			ProgramName:      "kubePeep",
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
		},
		Mac: &mac.Options{
			Appearance: mac.DefaultAppearance,
		},
		OnShutdown: func(context.Context) {
			shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = cleanups.Run(shutdownContext)
		},
	}
	if err := wails.Run(applicationOptions); err != nil {
		return fmt.Errorf("desktop: wails runtime: %w", err)
	}
	return nil
}
