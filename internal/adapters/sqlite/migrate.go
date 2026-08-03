package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/fvmoraes/kubepeep/internal/migrations"
	"github.com/fvmoraes/kubepeep/internal/securefs"
)

var (
	ErrMigrationChecksum = errors.New("sqlite: migration checksum mismatch")
	ErrMigrationHistory  = errors.New("sqlite: invalid migration history")
	migrationNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)
)

const migrationTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(name) > 0),
    checksum TEXT NOT NULL CHECK (length(checksum) = 64),
    applied_at INTEGER NOT NULL CHECK (applied_at >= 0)
)`

type appliedMigration struct {
	Name     string
	Checksum string
}

// ApplyMigrations applies an ordered migration set. It is exported so update
// code can apply future embedded sets through the same checksum and backup
// guarantees; normal startup passes migrations.Embedded().
func (s *Store) ApplyMigrations(ctx context.Context, set []migrations.Migration) error {
	s.maintenance.Lock()
	defer s.maintenance.Unlock()
	return s.applyMigrationsLocked(ctx, set)
}

func (s *Store) applyMigrationsLocked(ctx context.Context, set []migrations.Migration) error {
	ordered, err := validateMigrationSet(set)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, migrationTableSQL); err != nil {
		return fmt.Errorf("sqlite: initialize migration history: %w", err)
	}
	applied, err := loadAppliedMigrations(ctx, s.db)
	if err != nil {
		return err
	}
	known := make(map[int]migrations.Migration, len(ordered))
	for _, migration := range ordered {
		known[migration.Version] = migration
	}
	for version, record := range applied {
		migration, ok := known[version]
		if !ok {
			return fmt.Errorf("%w: database contains unknown version", ErrMigrationHistory)
		}
		if record.Name != migration.Name {
			return fmt.Errorf("%w: migration name changed", ErrMigrationHistory)
		}
		if record.Checksum != migration.Checksum {
			return fmt.Errorf("%w: version %d", ErrMigrationChecksum, version)
		}
	}
	if len(applied) > len(ordered) {
		return fmt.Errorf("%w: applied history is longer than embedded history", ErrMigrationHistory)
	}
	for index := 0; index < len(applied); index++ {
		if _, exists := applied[ordered[index].Version]; !exists {
			return fmt.Errorf("%w: applied migrations are not an exact prefix", ErrMigrationHistory)
		}
	}

	for _, migration := range ordered {
		if _, exists := applied[migration.Version]; exists {
			continue
		}
		var backup *securefs.Guard
		if migration.Destructive {
			backup, err = s.temporaryBackupLocked(ctx, migration.Version)
			if err != nil {
				return fmt.Errorf("sqlite: backup before migration: %w", err)
			}
		}
		applyErr := s.applyOne(ctx, migration)
		if applyErr == nil {
			if backup != nil {
				if err := removeVerifiedBackup(backup); err != nil {
					return fmt.Errorf("sqlite: remove successful migration backup: %w", err)
				}
			}
			continue
		}
		if backup == nil {
			return applyErr
		}
		return recoverFailedMigration(applyErr, backup, func() error {
			return s.restoreGuardedLocked(ctx, backup)
		})
	}
	return nil
}

func recoverFailedMigration(applyErr error, backup *securefs.Guard, restore func() error) error {
	if restoreErr := restore(); restoreErr != nil {
		closeErr := backup.Close()
		return errors.Join(
			applyErr,
			fmt.Errorf("sqlite: restore failed migration; verified backup retained: %w", restoreErr),
			closeErr,
		)
	}
	return errors.Join(applyErr, removeVerifiedBackup(backup))
}

func removeVerifiedBackup(backup *securefs.Guard) error {
	path := backup.Path()
	if err := backup.Close(); err != nil {
		return fmt.Errorf("close verified backup before removal: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func validateMigrationSet(set []migrations.Migration) ([]migrations.Migration, error) {
	ordered := append([]migrations.Migration(nil), set...)
	for index, migration := range ordered {
		if migration.Version <= 0 || !migrationNamePattern.MatchString(migration.Name) || migration.SQL == "" {
			return nil, fmt.Errorf("%w: invalid migration metadata", ErrMigrationHistory)
		}
		digest := sha256.Sum256([]byte(migration.SQL))
		if migration.Checksum != hex.EncodeToString(digest[:]) {
			return nil, fmt.Errorf("%w: supplied migration checksum", ErrMigrationChecksum)
		}
		if index > 0 && migration.Version <= ordered[index-1].Version {
			return nil, fmt.Errorf("%w: migration versions are not strictly increasing", ErrMigrationHistory)
		}
	}
	return ordered, nil
}

func loadAppliedMigrations(ctx context.Context, database queryer) (map[int]appliedMigration, error) {
	rows, err := database.QueryContext(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read migration history: %w", err)
	}
	defer rows.Close()
	applied := make(map[int]appliedMigration)
	for rows.Next() {
		var version int
		var record appliedMigration
		if err := rows.Scan(&version, &record.Name, &record.Checksum); err != nil {
			return nil, fmt.Errorf("sqlite: decode migration history: %w", err)
		}
		applied[version] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate migration history: %w", err)
	}
	return applied, nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *Store) applyOne(ctx context.Context, migration migrations.Migration) error {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin migration: %w", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("sqlite: apply migration %d: %w", migration.Version, err)
	}
	appliedAt := time.Now().UTC().UnixMilli()
	if _, err := transaction.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		migration.Version, migration.Name, migration.Checksum, appliedAt,
	); err != nil {
		return fmt.Errorf("sqlite: record migration %d: %w", migration.Version, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit migration %d: %w", migration.Version, err)
	}
	return nil
}

func (s *Store) temporaryBackupLocked(ctx context.Context, version int) (*securefs.Guard, error) {
	temporary, err := securefs.CreateTemp(filepath.Dir(s.path), fmt.Sprintf(".kubePeep.db.migration-%04d-*.backup", version))
	if err != nil {
		return nil, err
	}
	path := temporary.Path()
	if err := s.backupIntoLocked(ctx, temporary); err != nil {
		_ = temporary.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return temporary, nil
}
