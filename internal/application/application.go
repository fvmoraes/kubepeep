// Package application composes the complete Kube Peep core - logging, SQLite,
// Kubernetes runtime, services, selection, health and the HTTP application -
// independently of any transport. CLI (web), desktop (Wails) and tests consume
// the same composition, so business rules and security policies stay in one
// place.
package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	gingerhealth "github.com/fvmoraes/ginger/pkg/health"

	"github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/adapters/sqlite"
	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	"github.com/fvmoraes/kubepeep/internal/api"
	httpapp "github.com/fvmoraes/kubepeep/internal/app"
	"github.com/fvmoraes/kubepeep/internal/buildinfo"
	productconfig "github.com/fvmoraes/kubepeep/internal/config"
	kuberuntime "github.com/fvmoraes/kubepeep/internal/integration/kubernetesruntime"
	"github.com/fvmoraes/kubepeep/internal/logging"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
	actionservice "github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
	contextservice "github.com/fvmoraes/kubepeep/internal/services/contexts"
	"github.com/fvmoraes/kubepeep/internal/services/dashboard"
	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
	resourcecore "github.com/fvmoraes/kubepeep/internal/services/resources"
	"github.com/fvmoraes/kubepeep/internal/services/selection"
)

type Options struct {
	Layout        userdirs.Layout
	Config        productconfig.Config
	Kubeconfig    string
	KubeconfigSet bool
	Context       string
	ContextSet    bool
	Namespace     string
	NamespaceSet  bool
	LogOutput     io.Writer
	Port          int
	ExtraHosts    []string
	ExtraOrigins  []string
}

// Platform is the composed, transport-independent core returned by Compose.
type Platform struct {
	Handler    http.Handler
	Cleanups   []localruntime.NamedCleanup
	Port       int
	Origin     string
	Sessions   *api.SessionStore
	Generation api.GenerationSource
}

type namedChecker struct {
	name  string
	check func(context.Context) error
}

func (checker namedChecker) Name() string { return checker.name }

func (checker namedChecker) Check(ctx context.Context) error {
	return checker.check(ctx)
}

var _ gingerhealth.Checker = namedChecker{}

// Compose builds every backend component and the HTTP application. It is the
// single source of truth for wiring; both the web runtime and the desktop
// shell call it.
func Compose(ctx context.Context, options Options) (*Platform, error) {
	if options.Layout.Root == "" {
		return nil, errors.New("startup: canonical layout is required")
	}
	if options.Port < 1 || options.Port > 65535 {
		return nil, errors.New("startup: port must be between 1 and 65535")
	}
	logOutput := options.LogOutput
	if logOutput == nil {
		logOutput = os.Stdout
	}
	logger, logSink, err := logging.New(options.Layout.Log, logOutput, logging.Options{})
	if err != nil {
		return nil, err
	}
	closeLogOnError := true
	defer func() {
		if closeLogOnError {
			_ = logSink.Close()
		}
	}()

	store, err := sqlite.Open(ctx, options.Layout.Database)
	if err != nil {
		return nil, err
	}
	closeStoreOnError := true
	defer func() {
		if closeStoreOnError {
			_ = store.Close()
		}
	}()
	profileRepository, err := clusterprofiles.NewRepository(store.SQLDB())
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	generation, err := api.NewGenerationStore()
	if err != nil {
		return nil, err
	}
	cursors, err := api.NewCursorCodec()
	if err != nil {
		return nil, err
	}
	sessions, err := api.NewSessionStore(0)
	if err != nil {
		return nil, err
	}
	clientFactory, err := kubernetes.NewClientFactory(kubernetes.FactoryOptions{})
	if err != nil {
		return nil, err
	}
	kubernetesRuntime, err := kuberuntime.New(ctx, kubernetes.NewLoader(kubernetes.LoaderOptions{}), clientFactory)
	if err != nil {
		return nil, err
	}
	closeRuntimeOnError := true
	defer func() {
		if closeRuntimeOnError {
			_ = kubernetesRuntime.Close()
		}
	}()
	authorizationService, err := authorization.New(kubernetesRuntime, authorization.Options{})
	if err != nil {
		return nil, err
	}
	dashboardBackend, err := kuberuntime.NewDashboardBackend(kubernetesRuntime, authorizationService, dashboard.NewQueryBudget(options.Config.Dashboard.BlockTimeout.Duration))
	if err != nil {
		return nil, err
	}
	resourceBackend, err := kuberuntime.NewResourceBackend(kubernetesRuntime, authorizationService, resourcecore.TextRedactorFunc(func(value string) string {
		redacted, _ := dashboard.Redact(value)
		return redacted
	}))
	if err != nil {
		return nil, err
	}
	preferenceService := &resourcecore.PreferenceService{
		Repository: sqlite.NewPreferenceRepository(store),
		Detector:   resourcecore.DefaultSensitiveDetector{},
	}
	actionBackends, err := kuberuntime.NewActionBackends(kubernetesRuntime)
	if err != nil {
		return nil, err
	}
	actionAudit := kuberuntime.NewActionAuditSink(logger)
	actions, err := actionservice.NewActionService(authorizationService, kubernetesRuntime, actionBackends.Mutations, actionAudit)
	if err != nil {
		return nil, err
	}
	portForwards, err := actionservice.NewPortForwardService(authorizationService, kubernetesRuntime, actionBackends.PortForward, actionAudit)
	if err != nil {
		actions.Shutdown()
		return nil, err
	}
	execSessions, err := actionservice.NewExecService(authorizationService, kubernetesRuntime, actionBackends.Mutations, actionBackends.Exec, actionAudit)
	if err != nil {
		portForwards.Shutdown()
		actions.Shutdown()
		return nil, err
	}
	closeActionsOnError := true
	defer func() {
		if closeActionsOnError {
			resourceBackend.Close()
			execSessions.Shutdown()
			portForwards.Shutdown()
			actions.Shutdown()
		}
	}()
	coordinator, err := selection.NewCoordinator(generation, sessions, func(next string) {
		kubernetesRuntime.OnGeneration(next)
		authorizationService.InvalidateAll()
		dashboardBackend.OnGeneration(next)
		resourceBackend.OnGeneration(next)
		actions.OnGeneration(next)
		portForwards.OnGeneration(next)
		execSessions.OnGeneration(next)
	})
	if err != nil {
		return nil, err
	}
	closeCoordinatorOnError := true
	defer func() {
		if closeCoordinatorOnError {
			coordinator.Close()
		}
	}()
	selectionState, err := selection.NewState(coordinator, namespaces.SelectionBinding{Generation: generation.Current()}, namespaces.ScopeResolution{})
	if err != nil {
		return nil, err
	}
	profileService, err := clusterprofiles.NewService(profileRepository, home, selectionState)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("startup: create health provider: %w", err)
	}
	contexts, err := contextservice.NewService(profileRepository, kubernetesRuntime, selectionState, snapshots)
	if err != nil {
		return nil, err
	}
	var explicitPath, explicitContext *string
	if options.KubeconfigSet {
		explicitPath = &options.Kubeconfig
	}
	if options.ContextSet {
		explicitContext = &options.Context
	}
	ephemeralNamespace := ""
	if options.NamespaceSet {
		ephemeralNamespace = options.Namespace
	}
	if err := contexts.Bootstrap(ctx, contextservice.BootstrapRequest{
		ExplicitPath: explicitPath, ExplicitContext: explicitContext, EphemeralNS: ephemeralNamespace,
	}); err != nil {
		return nil, fmt.Errorf("startup: bootstrap Kubernetes selection: %w", err)
	}
	namespaceRepository := sqlite.NewNamespaceScopeRepository(store)
	namespaceService := namespaces.NewService(namespaceRepository, selectionState, kubernetesRuntime)

	gingerConfig := options.Config.ToGinger(options.Layout.Database)
	application, err := httpapp.New(httpapp.Options{
		Config: &gingerConfig,
		Port:   options.Port,
		Build: api.BuildInfo{
			Version:   buildinfo.Version,
			Commit:    buildinfo.Commit,
			BuildDate: buildinfo.BuildDate,
		},
		Snapshots:    snapshots,
		Profiles:     profileService,
		Contexts:     contexts,
		Scopes:       namespaceService,
		Namespaces:   kubernetesRuntime,
		Permissions:  authorizationService,
		Selection:    selectionState,
		Dashboard:    dashboardBackend,
		Resources:    resourceBackend,
		Streams:      resourceBackend,
		Preferences:  preferenceService,
		Actions:      actions,
		PortForwards: portForwards,
		Exec:         execSessions,
		Cursors:      cursors,
		Generation:   generation,
		Sessions:     sessions,
		Logger:       logger,
		ExtraHosts:   options.ExtraHosts,
		ExtraOrigins: options.ExtraOrigins,
	})
	if err != nil {
		return nil, fmt.Errorf("startup: compose HTTP application: %w", err)
	}

	closeStoreOnError = false
	closeLogOnError = false
	closeRuntimeOnError = false
	closeCoordinatorOnError = false
	closeActionsOnError = false
	return &Platform{
		Handler:    application.Handler,
		Port:       options.Port,
		Origin:     application.Origin,
		Sessions:   sessions,
		Generation: generation,
		Cleanups: []localruntime.NamedCleanup{
			{Name: "Kubernetes resource watches", Func: func(context.Context) error { resourceBackend.Close(); return nil }},
			{Name: "Kubernetes actions", Func: func(context.Context) error {
				execSessions.Shutdown()
				portForwards.Shutdown()
				actions.Shutdown()
				return nil
			}},
			{Name: "selection coordinator", Func: func(context.Context) error { coordinator.Close(); return nil }},
			{Name: "Kubernetes clients", Func: func(context.Context) error { return kubernetesRuntime.Close() }},
			{Name: "local log", Func: func(context.Context) error { return logSink.Close() }},
			{Name: "SQLite", Func: func(context.Context) error { return store.Close() }},
		},
	}, nil
}
