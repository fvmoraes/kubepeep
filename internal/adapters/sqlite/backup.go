package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fvmoraes/kubepeep/internal/securefs"
	modernsqlite "modernc.org/sqlite"
)

var ErrIntegrityCheck = errors.New("sqlite: integrity check failed")

type backupConnection interface {
	NewBackup(string) (*modernsqlite.Backup, error)
}

type restoreConnection interface {
	NewRestore(string) (*modernsqlite.Backup, error)
}

// Backup checkpoints WAL, creates a consistent online backup with SQLite's
// Backup API, verifies it, and publishes it atomically without replacing an
// existing same-directory destination.
func (s *Store) Backup(ctx context.Context, destination string) error {
	s.maintenance.Lock()
	defer s.maintenance.Unlock()
	if destination == "" {
		return fmt.Errorf("sqlite: backup destination is required")
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("sqlite: resolve backup destination: %w", err)
	}
	if absolute == s.path {
		return fmt.Errorf("sqlite: backup destination must differ from database")
	}
	if err := securefs.EnsurePrivateDirectory(filepath.Dir(absolute)); err != nil {
		return fmt.Errorf("sqlite: unsafe backup directory: %w", err)
	}
	if _, err := os.Lstat(absolute); err == nil {
		return fmt.Errorf("sqlite: backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("sqlite: inspect backup destination: %w", err)
	}
	temporary, err := securefs.CreateTemp(filepath.Dir(absolute), ".kubePeep.db.backup-*.tmp")
	if err != nil {
		return fmt.Errorf("sqlite: create backup temporary: %w", err)
	}
	temporaryPath := temporary.Path()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := s.backupIntoLocked(ctx, temporary); err != nil {
		return err
	}
	if err := temporary.PublishNoReplace(absolute); err != nil {
		return fmt.Errorf("sqlite: publish backup without replacement: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("sqlite: close published backup: %w", err)
	}
	return nil
}

func (s *Store) backupIntoLocked(ctx context.Context, destination *securefs.Guard) error {
	if err := destination.Validate(); err != nil {
		return fmt.Errorf("sqlite: reject changed backup temporary: %w", err)
	}
	if err := checkpoint(ctx, s.db); err != nil {
		return err
	}
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("sqlite: acquire backup connection: %w", err)
	}
	defer connection.Close()
	if err := connection.Raw(func(raw any) error {
		backuper, ok := raw.(backupConnection)
		if !ok {
			return fmt.Errorf("sqlite: driver does not expose Backup API")
		}
		operation, err := backuper.NewBackup(destination.Path())
		if err != nil {
			return err
		}
		return completeBackup(ctx, operation)
	}); err != nil {
		return fmt.Errorf("sqlite: create backup: %w", err)
	}
	if err := destination.Validate(); err != nil {
		return fmt.Errorf("sqlite: backup temporary identity changed: %w", err)
	}
	if err := destination.Protect(0o600); err != nil {
		return fmt.Errorf("sqlite: protect backup: %w", err)
	}
	if err := destination.File().Sync(); err != nil {
		return fmt.Errorf("sqlite: sync backup: %w", err)
	}
	if err := verifyGuarded(ctx, destination); err != nil {
		return fmt.Errorf("sqlite: verify backup: %w", err)
	}
	return nil
}

// Restore verifies the source first and restores it into the open database via
// SQLite's Restore API. It is serialized against other maintenance operations.
func (s *Store) Restore(ctx context.Context, source string) error {
	s.maintenance.Lock()
	defer s.maintenance.Unlock()
	return s.restoreLocked(ctx, source)
}

func (s *Store) restoreLocked(ctx context.Context, source string) error {
	sourceFile, err := securefs.OpenRegular(source, os.O_RDONLY)
	if err != nil {
		return fmt.Errorf("sqlite: reject unsafe restore source: %w", err)
	}
	defer sourceFile.Close()
	return s.restoreGuardedLocked(ctx, sourceFile)
}

func (s *Store) restoreGuardedLocked(ctx context.Context, source *securefs.Guard) error {
	if err := verifyGuarded(ctx, source); err != nil {
		return fmt.Errorf("sqlite: reject invalid restore source: %w", err)
	}
	if err := checkpoint(ctx, s.db); err != nil {
		return err
	}
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("sqlite: acquire restore connection: %w", err)
	}
	defer connection.Close()
	if err := connection.Raw(func(raw any) error {
		restorer, ok := raw.(restoreConnection)
		if !ok {
			return fmt.Errorf("sqlite: driver does not expose Restore API")
		}
		if err := source.Validate(); err != nil {
			return err
		}
		operation, err := restorer.NewRestore(source.Path())
		if err != nil {
			return err
		}
		return completeBackup(ctx, operation)
	}); err != nil {
		return fmt.Errorf("sqlite: restore backup: %w", err)
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("sqlite: restore source identity changed: %w", err)
	}
	if err := IntegrityCheck(ctx, s.db); err != nil {
		return fmt.Errorf("sqlite: verify restored database: %w", err)
	}
	return nil
}

func completeBackup(ctx context.Context, operation *modernsqlite.Backup) (returnErr error) {
	finished := false
	defer func() {
		if !finished {
			returnErr = errors.Join(returnErr, operation.Finish())
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		more, err := operation.Step(128)
		if err != nil {
			return err
		}
		if !more {
			break
		}
	}
	finished = true
	return operation.Finish()
}

// Verify opens a database read-only and runs both SQLite integrity and foreign
// key checks.
func Verify(ctx context.Context, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("sqlite: resolve verification path: %w", err)
	}
	guard, err := securefs.OpenRegular(absolute, os.O_RDONLY)
	if err != nil {
		return fmt.Errorf("sqlite: reject unsafe verification path: %w", err)
	}
	defer guard.Close()
	return verifyGuarded(ctx, guard)
}

func verifyGuarded(ctx context.Context, guard *securefs.Guard) error {
	if err := guard.Validate(); err != nil {
		return err
	}
	database, err := sql.Open("sqlite", dataSourceName(guard.Path(), true))
	if err != nil {
		return fmt.Errorf("sqlite: open verification database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	defer database.Close()
	if err := IntegrityCheck(ctx, database); err != nil {
		return err
	}
	return guard.Validate()
}

// IntegrityCheck validates SQLite pages and declared foreign keys.
func IntegrityCheck(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrIntegrityCheck, err)
	}
	defer rows.Close()
	resultCount := 0
	for rows.Next() {
		resultCount++
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("%w: decode result", ErrIntegrityCheck)
		}
		if result != "ok" {
			return fmt.Errorf("%w: database reported a problem", ErrIntegrityCheck)
		}
	}
	if err := rows.Err(); err != nil || resultCount != 1 {
		return fmt.Errorf("%w: incomplete result", ErrIntegrityCheck)
	}
	foreignKeys, err := database.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("%w: foreign key check", ErrIntegrityCheck)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		return fmt.Errorf("%w: foreign key violation", ErrIntegrityCheck)
	}
	if err := foreignKeys.Err(); err != nil {
		return fmt.Errorf("%w: foreign key result", ErrIntegrityCheck)
	}
	return nil
}
