package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/fvmoraes/kubepeep/internal/adapters/sqlite"
	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	productconfig "github.com/fvmoraes/kubepeep/internal/config"
	"github.com/fvmoraes/kubepeep/internal/securefs"
	webassets "github.com/fvmoraes/kubepeep/internal/web"
)

// ProductionDoctor composes all Phase 3 local diagnostics. Kubernetes checks
// are deliberately deferred until the client integration exists in Phase 4.
type ProductionDoctor struct{}

func (ProductionDoctor) Check(ctx context.Context, layout userdirs.Layout) ([]DoctorCheck, error) {
	checks, err := (LocalDoctor{}).Check(ctx, layout)
	if err != nil {
		return nil, err
	}
	checks = append(checks, checkConfiguration(layout), checkSQLite(ctx, layout), checkFrontend(), checkPermissions(layout))
	return checks, nil
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
