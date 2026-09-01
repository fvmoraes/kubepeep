// Package app composes Ginger components with the Kube Peep HTTP contract.
// The process lifecycle remains owned by internal/runtime; this package never
// calls ginger/app.Run or installs signal handlers.
package app

import (
	"fmt"
	"io/fs"
	"net/http"
	"time"

	gingerapp "github.com/fvmoraes/ginger/pkg/app"
	gingerconfig "github.com/fvmoraes/ginger/pkg/config"
	gingerlogger "github.com/fvmoraes/ginger/pkg/logger"
	gingermiddleware "github.com/fvmoraes/ginger/pkg/middleware"
	gingerrouter "github.com/fvmoraes/ginger/pkg/router"

	"github.com/fvmoraes/kubepeep/internal/api"
	"github.com/fvmoraes/kubepeep/internal/api/handlers"
	apiMiddleware "github.com/fvmoraes/kubepeep/internal/api/middlewares"
	"github.com/fvmoraes/kubepeep/internal/observability"
	actionservice "github.com/fvmoraes/kubepeep/internal/services/actions"
	"github.com/fvmoraes/kubepeep/internal/web"
)

type Options struct {
	Config       *gingerconfig.Config
	Port         int
	Build        api.BuildInfo
	Snapshots    api.SnapshotProvider
	Generation   api.GenerationSource
	Sessions     *api.SessionStore
	Profiles     handlers.ClusterProfileService
	Scopes       handlers.NamespaceScopeService
	Namespaces   handlers.NamespaceCatalog
	Permissions  handlers.PermissionMatrixService
	Selection    handlers.SelectionReader
	Contexts     handlers.ContextService
	Dashboard    handlers.DashboardService
	Resources    handlers.ResourceService
	Streams      handlers.ResourceStreamService
	Preferences  handlers.PreferenceService
	Actions      actionservice.ActionService
	PortForwards actionservice.PortForwardService
	Exec         handlers.ExecBridgeService
	Cursors      *api.CursorCodec
	SessionTTL   time.Duration
	Frontend     fs.FS
	Logger       *gingerlogger.Logger
	ExtraHosts   []string
	ExtraOrigins []string
	// Metrics optionally enables the local process metrics endpoint. A nil
	// registry keeps /metrics unregistered; this is the default.
	Metrics *observability.Registry
}

type Application struct {
	Ginger     *gingerapp.App
	Handler    http.Handler
	Generation api.GenerationSource
	Sessions   *api.SessionStore
	Origin     string
}

func New(options Options) (*Application, error) {
	if options.Config == nil {
		return nil, fmt.Errorf("app: Ginger config is required")
	}
	if options.Config.HTTP.Host != "127.0.0.1" {
		return nil, fmt.Errorf("app: HTTP host must be exactly 127.0.0.1")
	}
	if options.Port < 1024 || options.Port > 65535 {
		return nil, fmt.Errorf("app: published port is outside 1024-65535")
	}
	if options.Snapshots == nil {
		return nil, fmt.Errorf("app: snapshot provider is required")
	}
	if options.Logger == nil || options.Logger.Logger == nil {
		return nil, fmt.Errorf("app: sanitized logger is required")
	}
	frontendFS := options.Frontend
	if frontendFS == nil {
		var err error
		frontendFS, err = web.Embedded()
		if err != nil {
			return nil, fmt.Errorf("app: load embedded frontend: %w", err)
		}
	}

	generation := options.Generation
	if generation == nil {
		store, err := api.NewGenerationStore()
		if err != nil {
			return nil, err
		}
		generation = store
	}
	if generation.Current() == "" {
		return nil, fmt.Errorf("app: generation must not be empty")
	}
	sessions := options.Sessions
	if sessions == nil {
		var err error
		sessions, err = api.NewSessionStore(options.SessionTTL)
		if err != nil {
			return nil, err
		}
	}

	origin := fmt.Sprintf("http://127.0.0.1:%d", options.Port)
	host := fmt.Sprintf("127.0.0.1:%d", options.Port)
	security := apiMiddleware.SecurityConfig{
		Host:         host,
		Origin:       origin,
		Sessions:     sessions,
		Generation:   generation,
		ExtraHosts:   append([]string(nil), options.ExtraHosts...),
		ExtraOrigins: append([]string(nil), options.ExtraOrigins...),
	}

	gingerConfig := *options.Config
	gingerConfig.HTTP.Port = options.Port
	application := gingerapp.New(&gingerConfig)
	// app.New installs Ginger's standard middleware with its default logger.
	// Rebuild the still-empty router so every captured logger reference uses
	// the required Kube Peep JSONL handler before any route is registered.
	application.Logger = options.Logger
	application.Router = gingerrouter.New()
	application.Router.Use(
		gingermiddleware.Recover(options.Logger),
		gingermiddleware.RequestID(),
		gingermiddleware.Logger(options.Logger),
	)
	application.Router.HandleRaw("GET /health", application.Health)
	// The exact recovery and browser security middleware are appended before
	// any product route is registered. Ginger's built-ins remain reusable for
	// normal JSON routes, while raw routes use RawChain separately.
	application.Router.Use(
		apiMiddleware.Recovery(application.Logger),
		apiMiddleware.BrowserAPI(security),
	)
	handlers.Register(application.Router, handlers.Dependencies{
		Snapshots:    options.Snapshots,
		Sessions:     sessions,
		Generation:   generation,
		Profiles:     options.Profiles,
		Scopes:       options.Scopes,
		Namespaces:   options.Namespaces,
		Permissions:  options.Permissions,
		Selection:    options.Selection,
		Contexts:     options.Contexts,
		Dashboard:    options.Dashboard,
		Resources:    options.Resources,
		Preferences:  options.Preferences,
		Actions:      options.Actions,
		PortForwards: options.PortForwards,
		Exec:         options.Exec,
		Cursors:      options.Cursors,
		Origin:       origin,
		Port:         options.Port,
		Build:        options.Build,
		ExtraOrigins: options.ExtraOrigins,
	})
	if options.Streams != nil && options.Selection != nil {
		resourceStreams := handlers.NewResourceStreams(options.Streams, options.Selection, sessions, origin).WithExtraOrigins(options.ExtraOrigins)
		application.Router.HandleRaw(
			"GET /api/v1/pods/{namespace}/{name}/logs/stream",
			apiMiddleware.RawChain(application.Logger, security, 4*time.Hour, http.HandlerFunc(resourceStreams.LogFollow)),
		)
		application.Router.HandleRaw(
			"GET /api/v1/stream",
			apiMiddleware.RawChain(application.Logger, security, 0, http.HandlerFunc(resourceStreams.Resources)),
		)
	}
	if options.Exec != nil && options.Selection != nil {
		execStream := handlers.NewExecStream(options.Exec, options.Selection, origin).WithExtraOrigins(options.ExtraOrigins)
		application.Router.HandleRaw(
			"GET /api/v1/exec/{sessionId}/stream",
			apiMiddleware.RawChain(application.Logger, security, actionservice.DefaultExecDuration+time.Minute, execStream),
		)
	}

	apiFallback := gingermiddleware.Chain(
		apiMiddleware.RequestID(),
		apiMiddleware.Recovery(application.Logger),
		apiMiddleware.SameOriginRead(security),
	)(handlers.NewAPIFallback())
	application.Router.HandleRaw("/api/v1", apiFallback)
	application.Router.HandleRaw("/api/v1/", apiFallback)

	frontend, err := web.New(frontendFS, options.Port)
	if err != nil {
		return nil, err
	}
	application.Router.HandleRaw("/", gingermiddleware.Chain(
		apiMiddleware.RequestID(),
		apiMiddleware.Recovery(application.Logger),
		apiMiddleware.SameOriginRead(security),
	)(frontend))

	// This outer mux deliberately shadows Ginger's built-in /health. Other
	// requests continue through Ginger's router and middleware composition.
	health := handlers.NewHealth(options.Snapshots)
	mux := http.NewServeMux()
	mux.Handle("GET /health", health)
	if options.Metrics != nil {
		registry := options.Metrics
		mux.Handle("GET /metrics", http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			response.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			_, _ = response.Write([]byte(registry.Render()))
		}))
	}
	mux.Handle("/", application.Router)

	outerMiddlewares := []gingermiddleware.Func{
		apiMiddleware.RequestID(),
		apiMiddleware.Recovery(application.Logger),
	}
	if options.Metrics != nil {
		outerMiddlewares = append(outerMiddlewares, observability.RequestsMiddleware(options.Metrics))
	}
	outerMiddlewares = append(outerMiddlewares, apiMiddleware.Host(host, options.ExtraHosts...))

	handler := gingermiddleware.Chain(outerMiddlewares...)(mux)

	return &Application{
		Ginger:     application,
		Handler:    handler,
		Generation: generation,
		Sessions:   sessions,
		Origin:     origin,
	}, nil
}
