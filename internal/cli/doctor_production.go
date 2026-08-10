package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/fvmoraes/kubepeep/internal/adapters/kubernetes"
	"github.com/fvmoraes/kubepeep/internal/adapters/sqlite"
	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	productconfig "github.com/fvmoraes/kubepeep/internal/config"
	"github.com/fvmoraes/kubepeep/internal/securefs"
	"github.com/fvmoraes/kubepeep/internal/services/authorization"
	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
	webassets "github.com/fvmoraes/kubepeep/internal/web"
)

// ProductionDoctor composes local and read-only Kubernetes diagnostics.
type ProductionDoctor struct{}

func (ProductionDoctor) Check(ctx context.Context, layout userdirs.Layout) ([]DoctorCheck, error) {
	checks, err := (LocalDoctor{}).Check(ctx, layout)
	if err != nil {
		return nil, err
	}
	checks = append(checks, checkConfiguration(layout), checkSQLite(ctx, layout), checkFrontend(), checkPermissions(layout))
	checks = append(checks, checkKubernetes(ctx, layout)...)
	return checks, nil
}

func checkKubernetes(ctx context.Context, layout userdirs.Layout) []DoctorCheck {
	skipped := func(code, message string) []DoctorCheck {
		return []DoctorCheck{
			{Group: "kubeconfig", Name: "source", Status: DoctorSkip, Code: code, Message: message},
			{Group: "kubeconfig", Name: "context", Status: DoctorSkip, Code: "CONTEXT_NOT_SELECTED", Message: "No Kubernetes context is available for inspection."},
			{Group: "kubeconfig", Name: "exec_plugin", Status: DoctorSkip, Code: "EXEC_PLUGIN_NOT_CHECKED", Message: "No selected authentication plugin is available for inspection."},
			{Group: "cluster", Name: "connectivity", Status: DoctorSkip, Code: "CLUSTER_NOT_CHECKED", Message: "Cluster connectivity was not checked without a selected context."},
			{Group: "cluster", Name: "basic_capability", Status: DoctorSkip, Code: "CAPABILITY_NOT_CHECKED", Message: "Kubernetes capabilities were not checked without connectivity."},
		}
	}

	store, err := sqlite.Open(ctx, layout.Database)
	if err != nil {
		return skipped("KUBECONFIG_NOT_CHECKED", "Kubeconfig was not checked because local profile storage is unavailable.")
	}
	defer store.Close()
	repository, err := clusterprofiles.NewRepository(store.SQLDB())
	if err != nil {
		return skipped("KUBECONFIG_NOT_CHECKED", "Kubeconfig was not checked because local profile storage is unavailable.")
	}
	var persisted *kubernetes.ProfileReference
	if profile, defaultErr := repository.Default(ctx); defaultErr == nil {
		contextName := ""
		if profile.Context != nil {
			contextName = *profile.Context
		}
		persisted = &kubernetes.ProfileReference{Paths: append([]string(nil), profile.Paths...), Context: contextName}
	} else if !errors.Is(defaultErr, clusterprofiles.ErrNotFound) {
		return skipped("KUBECONFIG_NOT_CHECKED", "Kubeconfig was not checked because local profile storage is unavailable.")
	}

	loader := kubernetes.NewLoader(kubernetes.LoaderOptions{})
	resolution, err := loader.Resolve(ctx, kubernetes.ResolveRequest{Persisted: persisted, FirstReconcile: true})
	if err != nil {
		code, _, _ := kubernetes.ErrorDetails(err)
		status := DoctorWarn
		if code == kubernetes.CodeKubeconfigNotFound {
			status = DoctorSkip
		}
		checks := skipped(string(code), "No usable Kubernetes configuration is currently available.")
		checks[0].Status = status
		return checks
	}
	checks := []DoctorCheck{{Group: "kubeconfig", Name: "source", Status: DoctorPass, Code: "KUBECONFIG_VALID", Message: "The selected kubeconfig source can be parsed safely."}}
	if _, selected := resolution.SelectedContext(); !selected {
		return append(checks, skipped("KUBECONFIG_VALID", "The kubeconfig source is valid.")[1:]...)
	}
	checks = append(checks, DoctorCheck{Group: "kubeconfig", Name: "context", Status: DoctorPass, Code: "CONTEXT_VALID", Message: "The selected Kubernetes context exists and is locally valid."})
	plugin := resolution.ExecPlugin()
	switch {
	case !plugin.Configured:
		checks = append(checks, DoctorCheck{Group: "kubeconfig", Name: "exec_plugin", Status: DoctorSkip, Code: "EXEC_PLUGIN_NOT_CONFIGURED", Message: "The selected context does not use an exec authentication plugin."})
	case !plugin.CommandAvailable:
		checks = append(checks, DoctorCheck{Group: "kubeconfig", Name: "exec_plugin", Status: DoctorWarn, Code: "EXEC_PLUGIN_UNAVAILABLE", Message: "The selected authentication plugin executable is unavailable."})
		return append(checks,
			DoctorCheck{Group: "cluster", Name: "connectivity", Status: DoctorSkip, Code: "CLUSTER_NOT_CHECKED", Message: "Cluster connectivity was not attempted with an unavailable authentication plugin."},
			DoctorCheck{Group: "cluster", Name: "basic_capability", Status: DoctorSkip, Code: "CAPABILITY_NOT_CHECKED", Message: "Kubernetes capabilities were not checked without connectivity."},
		)
	case !plugin.NonInteractive || !plugin.VersionCompatible:
		checks = append(checks, DoctorCheck{Group: "kubeconfig", Name: "exec_plugin", Status: DoctorWarn, Code: "EXEC_PLUGIN_INCOMPATIBLE", Message: "The selected authentication plugin is incompatible with non-interactive diagnostics."})
		return append(checks,
			DoctorCheck{Group: "cluster", Name: "connectivity", Status: DoctorSkip, Code: "CLUSTER_NOT_CHECKED", Message: "Cluster connectivity was not attempted with an incompatible authentication plugin."},
			DoctorCheck{Group: "cluster", Name: "basic_capability", Status: DoctorSkip, Code: "CAPABILITY_NOT_CHECKED", Message: "Kubernetes capabilities were not checked without connectivity."},
		)
	default:
		checks = append(checks, DoctorCheck{Group: "kubeconfig", Name: "exec_plugin", Status: DoctorPass, Code: "EXEC_PLUGIN_READY", Message: "The selected authentication plugin is available and non-interactive."})
	}

	factory, err := kubernetes.NewClientFactory(kubernetes.FactoryOptions{UnaryTimeout: 5 * time.Second})
	if err != nil {
		return append(checks,
			DoctorCheck{Group: "cluster", Name: "connectivity", Status: DoctorWarn, Code: "KUBERNETES_CLIENT_UNAVAILABLE", Message: "The Kubernetes client could not be constructed safely."},
			DoctorCheck{Group: "cluster", Name: "basic_capability", Status: DoctorSkip, Code: "CAPABILITY_NOT_CHECKED", Message: "Kubernetes capabilities were not checked without connectivity."},
		)
	}
	clients, err := factory.Build(ctx, resolution)
	if err != nil {
		code, _, _ := kubernetes.ErrorDetails(err)
		return append(checks,
			DoctorCheck{Group: "cluster", Name: "connectivity", Status: DoctorWarn, Code: string(code), Message: "The Kubernetes client could not be activated safely."},
			DoctorCheck{Group: "cluster", Name: "basic_capability", Status: DoctorSkip, Code: "CAPABILITY_NOT_CHECKED", Message: "Kubernetes capabilities were not checked without connectivity."},
		)
	}
	defer clients.CloseIdleConnections()
	checkContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	connectivity := kubernetes.CheckConnectivity(checkContext, clients)
	cancel()
	if connectivity.Status != kubernetes.ConnectivityHealthy {
		return append(checks,
			DoctorCheck{Group: "cluster", Name: "connectivity", Status: DoctorWarn, Code: string(connectivity.Code), Message: "The Kubernetes cluster is currently unavailable or could not be authenticated."},
			DoctorCheck{Group: "cluster", Name: "basic_capability", Status: DoctorSkip, Code: "CAPABILITY_NOT_CHECKED", Message: "Kubernetes capabilities were not checked without connectivity."},
		)
	}
	checks = append(checks, DoctorCheck{Group: "cluster", Name: "connectivity", Status: DoctorPass, Code: "CLUSTER_REACHABLE", Message: "The Kubernetes API version endpoint is reachable."})
	reviewer, err := authorization.NewKubernetesReviewer(clients.UnaryKubernetes().AuthorizationV1())
	if err != nil {
		return append(checks, DoctorCheck{Group: "cluster", Name: "basic_capability", Status: DoctorSkip, Code: "CAPABILITY_NOT_CHECKED", Message: "The basic Kubernetes capability could not be inspected."})
	}
	reviewContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, reviewErr := reviewer.ReviewAccess(reviewContext, authorization.Key{Generation: "doctor", Resource: "namespaces", Verb: "list"})
	cancel()
	switch {
	case reviewErr != nil || !result.Complete:
		checks = append(checks, DoctorCheck{Group: "cluster", Name: "basic_capability", Status: DoctorSkip, Code: "CAPABILITY_UNKNOWN", Message: "The basic Kubernetes capability could not be verified."})
	case result.Allowed:
		checks = append(checks, DoctorCheck{Group: "cluster", Name: "basic_capability", Status: DoctorPass, Code: "NAMESPACES_LIST_ALLOWED", Message: "The selected identity may list Kubernetes namespaces."})
	default:
		checks = append(checks, DoctorCheck{Group: "cluster", Name: "basic_capability", Status: DoctorWarn, Code: "NAMESPACES_LIST_DENIED", Message: "The selected identity cannot list Kubernetes namespaces; manual scopes remain available."})
	}
	return checks
}

func checkConfiguration(layout userdirs.Layout) DoctorCheck {
	configuration, err := productconfig.Load(layout.Config)
	if err != nil {
		return DoctorCheck{Group: "configuração", Name: "strict_config", Status: DoctorFail, Code: "CONFIG_INVALID", Message: "The local configuration is invalid or cannot be read safely."}
	}
	if configuration.Observability.OTel.Enabled {
		return DoctorCheck{Group: "configuração", Name: "strict_config", Status: DoctorWarn, Code: "OTEL_OPT_IN_DEFERRED", Message: "The configuration is valid; the optional telemetry exporter is not active in this build."}
	}
	return DoctorCheck{Group: "configuração", Name: "strict_config", Status: DoctorPass, Code: "CONFIG_VALID", Message: "The local configuration is strict, valid, and telemetry is disabled."}
}

func checkSQLite(ctx context.Context, layout userdirs.Layout) DoctorCheck {
	store, err := sqlite.Open(ctx, layout.Database)
	if err != nil {
		return DoctorCheck{Group: "SQLite", Name: "integrity", Status: DoctorFail, Code: "SQLITE_UNAVAILABLE", Message: "SQLite could not be opened or migrated safely."}
	}
	healthy := true
	var result string
	if err := store.SQLDB().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil || result != "ok" {
		healthy = false
	}
	if healthy {
		rows, err := store.SQLDB().QueryContext(ctx, "PRAGMA foreign_key_check")
		if err != nil {
			healthy = false
		} else {
			healthy = !rows.Next()
			if rows.Err() != nil {
				healthy = false
			}
			_ = rows.Close()
		}
	}
	if healthy {
		transaction, err := store.SQLDB().BeginTx(ctx, nil)
		if err != nil {
			healthy = false
		} else {
			if _, err := transaction.ExecContext(ctx, "CREATE TABLE __kubepeep_doctor_probe (id INTEGER NOT NULL)"); err != nil {
				healthy = false
			}
			if healthy {
				if _, err := transaction.ExecContext(ctx, "INSERT INTO __kubepeep_doctor_probe(id) VALUES (1)"); err != nil {
					healthy = false
				}
			}
			if err := transaction.Rollback(); err != nil {
				healthy = false
			}
		}
		var leftovers int
		if err := store.SQLDB().QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name = '__kubepeep_doctor_probe'").Scan(&leftovers); err != nil || leftovers != 0 {
			healthy = false
		}
	}
	if err := store.Close(); err != nil {
		healthy = false
	}
	if !healthy {
		return DoctorCheck{Group: "SQLite", Name: "integrity", Status: DoctorFail, Code: "SQLITE_INTEGRITY_FAILED", Message: "SQLite integrity, foreign keys, or reversible writes failed."}
	}
	return DoctorCheck{Group: "SQLite", Name: "integrity", Status: DoctorPass, Code: "SQLITE_READY", Message: "SQLite migrations, integrity, foreign keys, and reversible writes passed."}
}

func checkFrontend() DoctorCheck {
	frontend, err := webassets.Embedded()
	if err == nil {
		var info fs.FileInfo
		info, err = fs.Stat(frontend, "index.html")
		if err == nil && (info.IsDir() || info.Size() == 0) {
			err = errors.New("embedded index is empty")
		}
	}
	if err != nil {
		return DoctorCheck{Group: "build", Name: "embedded_frontend", Status: DoctorFail, Code: "FRONTEND_INVALID", Message: "The embedded frontend is missing or invalid."}
	}
	return DoctorCheck{Group: "build", Name: "embedded_frontend", Status: DoctorPass, Code: "FRONTEND_EMBEDDED", Message: "The embedded frontend index is present and readable."}
}

func checkPermissions(layout userdirs.Layout) DoctorCheck {
	for _, path := range []string{layout.Root, layout.LogsDir, layout.RuntimeDir, layout.Cache} {
		err := securefs.ValidatePrivateDirectory(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return DoctorCheck{Group: "segurança", Name: "local_permissions", Status: DoctorFail, Code: "LOCAL_PERMISSIONS_INVALID", Message: "A local path is not private or has an unsafe type."}
		}
	}
	for _, path := range []string{layout.Config, layout.Database, layout.Log, layout.Lock, layout.Instance} {
		err := securefs.ValidatePrivateRegular(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return DoctorCheck{Group: "segurança", Name: "local_permissions", Status: DoctorFail, Code: "LOCAL_PERMISSIONS_INVALID", Message: "A local path is not private or has an unsafe type."}
		}
	}
	return DoctorCheck{Group: "segurança", Name: "local_permissions", Status: DoctorPass, Code: "LOCAL_PERMISSIONS_PRIVATE", Message: "Existing local paths have private types and permissions."}
}
