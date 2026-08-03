package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	gingerhealth "github.com/fvmoraes/ginger/pkg/health"

	"github.com/fvmoraes/kubepeep/internal/adapters/sqlite"
	"github.com/fvmoraes/kubepeep/internal/api"
	httpapp "github.com/fvmoraes/kubepeep/internal/app"
	"github.com/fvmoraes/kubepeep/internal/buildinfo"
	"github.com/fvmoraes/kubepeep/internal/logging"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
)

type namedChecker struct {
	name  string
	check func(context.Context) error
}

func (checker namedChecker) Name() string { return checker.name }

func (checker namedChecker) Check(ctx context.Context) error {
	return checker.check(ctx)
}

var _ gingerhealth.Checker = namedChecker{}

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
	logOutput := factory.options.LogOutput
	if logOutput == nil {
		logOutput = os.Stdout
	}
	logger, logSink, err := logging.New(factory.options.Layout.Log, logOutput, logging.Options{})
	if err != nil {
		return localruntime.Service{}, err
	}
	closeLogOnError := true
	defer func() {
		if closeLogOnError {
			_ = logSink.Close()
		}
	}()

	store, err := sqlite.Open(ctx, factory.options.Layout.Database)
	if err != nil {
		return localruntime.Service{}, err
	}
	closeStoreOnError := true
	defer func() {
		if closeStoreOnError {
			_ = store.Close()
		}
	}()

	applicationChecker := namedChecker{name: "application", check: func(context.Context) error {
		if !logSink.Healthy() {
			return errors.New("local logging is unavailable")
		}
		return nil
	}}
	sqliteChecker := namedChecker{name: "sqlite", check: store.SQLDB().PingContext}
	snapshots, err := api.NewCheckerSnapshotProvider(
		api.InitialSnapshot(),
		0,
		api.HealthyCheck(api.ComponentApplication, applicationChecker, "Application is ready.", "APPLICATION_UNAVAILABLE", "The local application is unavailable."),
		api.HealthyCheck(api.ComponentSQLite, sqliteChecker, "SQLite is available.", "SQLITE_UNAVAILABLE", "SQLite is unavailable."),
	)
	if err != nil {
		return localruntime.Service{}, fmt.Errorf("startup: create health provider: %w", err)
	}

	gingerConfig := factory.options.Config.ToGinger(factory.options.Layout.Database)
	application, err := httpapp.New(httpapp.Options{
		Config: &gingerConfig,
		Port:   dependencies.Port,
		Build: api.BuildInfo{
			Version:   buildinfo.Version,
			Commit:    buildinfo.Commit,
			BuildDate: buildinfo.BuildDate,
		},
		Snapshots: snapshots,
		Logger:    logger,
	})
	if err != nil {
		return localruntime.Service{}, fmt.Errorf("startup: compose HTTP application: %w", err)
	}

	closeStoreOnError = false
	closeLogOnError = false
	return localruntime.Service{
		Handler: application.Handler,
		Cleanups: []localruntime.NamedCleanup{
			{Name: "local log", Func: func(context.Context) error { return logSink.Close() }},
			{Name: "SQLite", Func: func(context.Context) error { return store.Close() }},
		},
	}, nil
}
