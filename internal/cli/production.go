package cli

import (
	"context"
	"errors"

	"github.com/fvmoraes/kubepeep/internal/application"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
)

func productionStart(ctx context.Context, options StartOptions) (localruntime.RunResult, error) {
	if options.Config.Version == 0 {
		return localruntime.RunResult{}, errors.New("startup: effective configuration is required")
	}
	factory := &productionFactory{options: options}
	return localruntime.RunForeground(ctx, localruntime.RunOptions{
		DataRoot:         options.Layout.Root,
		RuntimeDirectory: options.Layout.RuntimeDir,
		Port:             options.Port,
		ShutdownTimeout:  options.Config.Server.ShutdownTimeout.Duration,
		Factory:          factory,
		OnReady:          options.OnReady,
	})
}

type productionFactory struct {
	options StartOptions
}

func (factory *productionFactory) Build(ctx context.Context, dependencies localruntime.ServiceDependencies) (localruntime.Service, error) {
	if dependencies.DataRoot != factory.options.Layout.Root {
		return localruntime.Service{}, errors.New("startup: runtime data root does not match canonical layout")
	}
	options := factory.options
	platform, err := application.Compose(ctx, application.Options{
		Layout:        options.Layout,
		Config:        options.Config,
		Kubeconfig:    options.Kubeconfig,
		KubeconfigSet: options.KubeconfigSet,
		Context:       options.Context,
		ContextSet:    options.ContextSet,
		Namespace:     options.Namespace,
		NamespaceSet:  options.NamespaceSet,
		LogOutput:     options.LogOutput,
		Port:          dependencies.Port,
	})
	if err != nil {
		return localruntime.Service{}, err
	}
	return localruntime.Service{
		Handler:  platform.Handler,
		Cleanups: platform.Cleanups,
	}, nil
}
