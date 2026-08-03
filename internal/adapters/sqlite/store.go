// Package sqlite provides kubePeep's local, CGO-free SQLite adapter.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fvmoraes/kubepeep/internal/migrations"
	"github.com/fvmoraes/kubepeep/internal/securefs"
	_ "modernc.org/sqlite"
)

const (
	MaxOpenConnections = 4
	MaxIdleConnections = 4
	BusyTimeoutMillis  = 5000
)

// Store owns the SQL pool and serializes maintenance operations such as
// migrations, backups, and restores.
type Store struct {
	db          *sql.DB
	path        string
	maintenance sync.Mutex
	closeOnce   sync.Once
	closeErr    error
}

// Open creates or opens a private database, applies the per-connection
// PRAGMAs, enables WAL, and applies the embedded migrations before returning.
func Open(ctx context.Context, path string) (*Store, error) {
	absPath, existed, err := validateDatabasePath(path)
	if err != nil {
		return nil, err
	}
	var databaseFile *securefs.Guard
	if existed {
		databaseFile, err = securefs.OpenRegular(absPath, os.O_RDWR)
	} else {
		databaseFile, err = securefs.CreateExclusive(absPath)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: secure database file: %w", err)
	}
	defer databaseFile.Close()
	database, err := sql.Open("sqlite", dataSourceName(absPath, false))
	if err != nil {
		return nil, fmt.Errorf("sqlite: open pool: %w", err)
	}
	database.SetMaxOpenConns(MaxOpenConnections)
	database.SetMaxIdleConns(MaxIdleConnections)
	store := &Store{db: database, path: absPath}
	closeOnError := func(cause error) (*Store, error) {
		_ = database.Close()
		return nil, cause
	}
	if err := database.PingContext(ctx); err != nil {
		return closeOnError(fmt.Errorf("sqlite: connect: %w", err))
	}
	if err := databaseFile.Validate(); err != nil {
		return closeOnError(fmt.Errorf("sqlite: database identity changed while opening: %w", err))
	}
	if err := databaseFile.Protect(0o600); err != nil {
		return closeOnError(fmt.Errorf("sqlite: protect database: %w", err))
	}
	if !existed {
		if err := syncDirectory(filepath.Dir(absPath)); err != nil {
			return closeOnError(fmt.Errorf("sqlite: sync database directory: %w", err))
		}
	}
	var journalMode string
	if err := database.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		return closeOnError(fmt.Errorf("sqlite: enable WAL: %w", err))
	}
	if !strings.EqualFold(journalMode, "wal") {
		return closeOnError(fmt.Errorf("sqlite: WAL mode was not enabled"))
	}
	loaded, err := migrations.Embedded()
	if err != nil {
		return closeOnError(err)
	}
	if err := store.ApplyMigrations(ctx, loaded); err != nil {
		return closeOnError(err)
	}
	return store, nil
}

// SQLDB exposes the standard pool to repository adapters. Maintenance callers
// must use Store methods so migrations and backup/restore remain serialized.
func (s *Store) SQLDB() *sql.DB { return s.db }

func (s *Store) Path() string { return s.path }

// Close checkpoints WAL before closing the pool. Cleanup continues even when
// the checkpoint reports an error.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.maintenance.Lock()
		defer s.maintenance.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		checkpointErr := checkpoint(ctx, s.db)
		closeErr := s.db.Close()
		s.closeErr = errors.Join(checkpointErr, closeErr)
	})
	return s.closeErr
}

func validateDatabasePath(path string) (absolute string, existed bool, err error) {
	if strings.TrimSpace(path) == "" || path == ":memory:" {
		return "", false, fmt.Errorf("sqlite: database path is required")
	}
	absolute, err = filepath.Abs(path)
	if err != nil {
		return "", false, fmt.Errorf("sqlite: resolve database path: %w", err)
	}
	if err := securefs.EnsurePrivateDirectory(filepath.Dir(absolute)); err != nil {
		return "", false, fmt.Errorf("sqlite: inspect database directory: %w", err)
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return absolute, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("sqlite: inspect database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("sqlite: database must be a regular file")
	}
	return absolute, true, nil
}

func dataSourceName(path string, readOnly bool) string {
	normalized := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && len(normalized) >= 2 && normalized[1] == ':' {
		normalized = "/" + normalized
	}
	values := make(url.Values)
	if readOnly {
		values.Set("mode", "ro")
	} else {
		values.Set("mode", "rwc")
		values.Add("_pragma", "foreign_keys(1)")
		values.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", BusyTimeoutMillis))
		values.Add("_pragma", "synchronous(NORMAL)")
	}
	return (&url.URL{Scheme: "file", Path: normalized, RawQuery: values.Encode()}).String()
}

func checkpoint(ctx context.Context, database *sql.DB) error {
	var busy, logFrames, checkpointed int
	if err := database.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil {
		return fmt.Errorf("sqlite: checkpoint WAL: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("sqlite: checkpoint WAL remained busy")
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		if runtime.GOOS == "windows" || errors.Is(err, os.ErrInvalid) {
			return nil
		}
		return err
	}
	return nil
}
