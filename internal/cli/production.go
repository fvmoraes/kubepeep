package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	gingerhealth "github.com/fvmoraes/ginger/pkg/health"

	"github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/adapters/sqlite"
	"github.com/fvmoraes/kubepeep/internal/api"
	httpapp "github.com/fvmoraes/kubepeep/internal/app"
	"github.com/fvmoraes/kubepeep/internal/buildinfo"
	kuberuntime "github.com/fvmoraes/kubepeep/internal/integration/kubernetesruntime"
	"github.com/fvmoraes/kubepeep/internal/logging"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
	contextservice "github.com/fvmoraes/kubepeep/internal/services/contexts"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	"github.com/fvmoraes/kubepeep/internal/services/selection"
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
	profileRepository, err := clusterprofiles.NewRepository(store.SQLDB())
	if err != nil {
		return localruntime.Service{}, err
	}
	home, _ := os.UserHomeDir()
	generation, err := api.NewGenerationStore()
	if err != nil {
		return localruntime.Service{}, err
	}
	cursors, err := api.NewCursorCodec()
	if err != nil {
		return localruntime.Service{}, err
	}
	sessions, err := api.NewSessionStore(0)
	if err != nil {
		return localruntime.Service{}, err
	}
	clientFactory, err := kubernetes.NewClientFactory(kubernetes.FactoryOptions{})
	if err != nil {
		return localruntime.Service{}, err
	}
	kubernetesRuntime, err := kuberuntime.New(ctx, kubernetes.NewLoader(kubernetes.LoaderOptions{}), clientFactory)
	if err != nil {
		return localruntime.Service{}, err
	}
	closeRuntimeOnError := true
	defer func() {
		if closeRuntimeOnError {
			_ = kubernetesRuntime.Close()
		}
	}()
	authorizationService, err := authorization.New(kubernetesRuntime, authorization.Options{})
	if err != nil {
		return localruntime.Service{}, err
	}
	dashboardBackend, err := kuberuntime.NewDashboardBackend(kubernetesRuntime, authorizationService)
	if err != nil {
		return localruntime.Service{}, err
	}
	coordinator, err := selection.NewCoordinator(generation, sessions, func(next string) {
		kubernetesRuntime.OnGeneration(next)
		authorizationService.InvalidateAll()
		dashboardBackend.OnGeneration(next)
	})
	if err != nil {
		return localruntime.Service{}, err
	}
	closeCoordinatorOnError := true
	defer func() {
		if closeCoordinatorOnError {
			coordinator.Close()
		}
	}()
	selectionState, err := selection.NewState(coordinator, namespaces.SelectionBinding{Generation: generation.Current()}, namespaces.ScopeResolution{})
	if err != nil {
		return localruntime.Service{}, err
	}
	profileService, err := clusterprofiles.NewService(profileRepository, home, selectionState)
	if err != nil {
		return localruntime.Service{}, err
	}

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
	contexts, err := contextservice.NewService(profileRepository, kubernetesRuntime, selectionState, snapshots)
	if err != nil {
		return localruntime.Service{}, err
	}
	var explicitPath, explicitContext *string
	if factory.options.KubeconfigSet {
		explicitPath = &factory.options.Kubeconfig
	}
	if factory.options.ContextSet {
		explicitContext = &factory.options.Context
	}
	ephemeralNamespace := ""
	if factory.options.NamespaceSet {
		ephemeralNamespace = factory.options.Namespace
	}
	if err := contexts.Bootstrap(ctx, contextservice.BootstrapRequest{
		ExplicitPath: explicitPath, ExplicitContext: explicitContext, EphemeralNS: ephemeralNamespace,
	}); err != nil {
		return localruntime.Service{}, fmt.Errorf("startup: bootstrap Kubernetes selection: %w", err)
	}
	namespaceRepository := sqlite.NewNamespaceScopeRepository(store)
	namespaceService := namespaces.NewService(namespaceRepository, selectionState, kubernetesRuntime)

	gingerConfig := factory.options.Config.ToGinger(factory.options.Layout.Database)
	application, err := httpapp.New(httpapp.Options{
		Config: &gingerConfig,
		Port:   dependencies.Port,
		Build: api.BuildInfo{
			Version:   buildinfo.Version,
			Commit:    buildinfo.Commit,
			BuildDate: buildinfo.BuildDate,
		},
		Snapshots:   snapshots,
		Profiles:    profileService,
		Contexts:    contexts,
		Scopes:      namespaceService,
		Namespaces:  kubernetesRuntime,
		Permissions: authorizationService,
		Selection:   selectionState,
		Dashboard:   dashboardBackend,
		Cursors:     cursors,
		Generation:  generation,
		Sessions:    sessions,
		Logger:      logger,
	})
	if err != nil {
		return localruntime.Service{}, fmt.Errorf("startup: compose HTTP application: %w", err)
	}

	closeStoreOnError = false
	closeLogOnError = false
	closeRuntimeOnError = false
	closeCoordinatorOnError = false
	return localruntime.Service{
		Handler: application.Handler,
		Cleanups: []localruntime.NamedCleanup{
			{Name: "selection coordinator", Func: func(context.Context) error { coordinator.Close(); return nil }},
			{Name: "Kubernetes clients", Func: func(context.Context) error { return kubernetesRuntime.Close() }},
			{Name: "local log", Func: func(context.Context) error { return logSink.Close() }},
			{Name: "SQLite", Func: func(context.Context) error { return store.Close() }},
		},
	}, nil
}
