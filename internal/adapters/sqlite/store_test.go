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
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/migrations"
	"github.com/fvmoraes/kubepeep/internal/securefs"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "kubePeep.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return store
}

func TestOpenCreatesCompleteSchemaAndReopensIdempotently(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "kubePeep.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	wantTables := []string{
		"cluster_profile_kubeconfig_files",
		"cluster_profiles",
		"namespace_scope_items",
		"namespace_scopes",
		"preferences",
		"schema_migrations",
	}
	if got := schemaTables(t, store); strings.Join(got, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("tables = %v, want %v", got, wantTables)
	}
	var migrationCount int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration count = %d, want 1", migrationCount)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("database permissions = %o, want 600", info.Mode().Perm())
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration count after reopen = %d, want 1", migrationCount)
	}
}

func TestEveryPooledConnectionHasRequiredPragmas(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	if store.db.Stats().MaxOpenConnections != MaxOpenConnections {
		t.Fatalf("MaxOpenConnections = %d", store.db.Stats().MaxOpenConnections)
	}
	connections := make([]*sql.Conn, 0, MaxOpenConnections)
	for index := 0; index < MaxOpenConnections; index++ {
		connection, err := store.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
	}
	defer func() {
		for _, connection := range connections {
			connection.Close()
		}
	}()
	for index, connection := range connections {
		var foreignKeys, busyTimeout, synchronous int
		var journalMode string
		if err := connection.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
			t.Fatal(err)
		}
		if err := connection.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
			t.Fatal(err)
		}
		if err := connection.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
			t.Fatal(err)
		}
		if err := connection.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
			t.Fatal(err)
		}
		if foreignKeys != 1 || busyTimeout != BusyTimeoutMillis || synchronous != 1 || journalMode != "wal" {
			t.Fatalf("connection %d pragmas: foreign_keys=%d busy_timeout=%d synchronous=%d journal=%q", index, foreignKeys, busyTimeout, synchronous, journalMode)
		}
	}
}

func TestSchemaConstraintsTriggersAndCascade(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	now := int64(1)
	result, err := store.db.ExecContext(ctx, `INSERT INTO cluster_profiles(name, context_name, is_default, created_at, updated_at) VALUES ('Default', 'local', 1, ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	profileID, _ := result.LastInsertId()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO cluster_profile_kubeconfig_files(cluster_profile_id, position, path, created_at) VALUES (?, 0, '/synthetic/config', ?)`, profileID, now); err != nil {
		t.Fatal(err)
	}
	allResult, err := store.db.ExecContext(ctx, `INSERT INTO namespace_scopes(cluster_profile_id, context_name, name, mode, version, created_at, updated_at) VALUES (?, 'local', 'Everything', 'all', 1, ?, ?)`, profileID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	allID, _ := allResult.LastInsertId()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO namespace_scope_items(namespace_scope_id, namespace, position, created_at) VALUES (?, 'default', 0, ?)`, allID, now); err == nil {
		t.Fatal("all scope accepted an item")
	}
	singleResult, err := store.db.ExecContext(ctx, `INSERT INTO namespace_scopes(cluster_profile_id, context_name, name, mode, version, created_at, updated_at) VALUES (?, 'local', 'One', 'single', 1, ?, ?)`, profileID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	singleID, _ := singleResult.LastInsertId()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO namespace_scope_items(namespace_scope_id, namespace, position, created_at) VALUES (?, 'default', 0, ?)`, singleID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO namespace_scope_items(namespace_scope_id, namespace, position, created_at) VALUES (?, 'other', 1, ?)`, singleID, now); err == nil {
		t.Fatal("single scope accepted a second item")
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO preferences(key, value_json, schema_version, updated_at) VALUES ('arbitrary.key', 'true', 1, ?)`, now); err == nil {
		t.Fatal("preferences accepted a non-allowlisted key")
	}
	if _, err := store.db.ExecContext(ctx, "DELETE FROM cluster_profiles WHERE id = ?", profileID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"cluster_profile_kubeconfig_files", "namespace_scopes", "namespace_scope_items"} {
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s cascade count = %d", table, count)
		}
	}
}

func TestChangedAppliedChecksumStopsStartup(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "kubePeep.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE schema_migrations SET checksum = ? WHERE version = 1", strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(ctx, path)
	if !errors.Is(err, ErrMigrationChecksum) {
		t.Fatalf("error = %v, want ErrMigrationChecksum", err)
	}
}

func TestMigrationSetMustAlreadyBeStrictlyOrdered(t *testing.T) {
	store := openTestStore(t)
	first := testMigration(3, "third", "CREATE TABLE third_probe (id INTEGER);", false)
	second := testMigration(2, "second", "CREATE TABLE second_probe (id INTEGER);", false)
	if err := store.ApplyMigrations(context.Background(), []migrations.Migration{first, second}); !errors.Is(err, ErrMigrationHistory) {
		t.Fatalf("error = %v, want ErrMigrationHistory", err)
	}
}

func TestAppliedMigrationHistoryMustBeAnExactPrefix(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	embedded, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	second := testMigration(2, "second", "CREATE TABLE second_probe (id INTEGER);", false)
	set := append(append([]migrations.Migration(nil), embedded...), second)
	if err := store.ApplyMigrations(ctx, set); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = 1"); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyMigrations(ctx, set); !errors.Is(err, ErrMigrationHistory) {
		t.Fatalf("error = %v, want ErrMigrationHistory for non-prefix history", err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version = 1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("runner mutated a non-prefix history before rejecting it")
	}
}

func TestBackupIsVerifiedAndRestoreRecoversSnapshot(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	insertProfile(t, store, "Before backup")
	backupPath := filepath.Join(filepath.Dir(store.path), "snapshot.db")
	if err := store.Backup(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := Verify(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE cluster_profiles SET name = 'After backup', updated_at = updated_at + 1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Restore(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := store.db.QueryRowContext(ctx, "SELECT name FROM cluster_profiles").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Before backup" {
		t.Fatalf("restored name = %q", name)
	}
	if err := store.Backup(ctx, backupPath); err == nil {
		t.Fatal("backup overwrote an existing destination")
	}
}

func TestFailedDestructiveMigrationLeavesOriginalDatabaseIntact(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO preferences(key, value_json, schema_version, updated_at) VALUES ('logs.wrap', 'true', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	embedded, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	failing := testMigration(2, "destructive_failure", "DROP TABLE preferences;\nTHIS IS NOT VALID SQL;", true)
	err = store.ApplyMigrations(ctx, append(embedded, failing))
	if err == nil {
		t.Fatal("expected migration failure")
	}
	var value string
	if err := store.db.QueryRowContext(ctx, `SELECT value_json FROM preferences WHERE key = 'logs.wrap'`).Scan(&value); err != nil {
		t.Fatalf("original preferences were not preserved: %v", err)
	}
	if value != "true" {
		t.Fatalf("preference = %q", value)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version = 2").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration was recorded")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(store.path), ".kubePeep.db.migration-*.backup"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary migration backups remain: %v", matches)
	}
}

func TestFailedRestoreRetainsTheVerifiedMigrationBackup(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	backup, err := store.temporaryBackupLocked(ctx, 99)
	if err != nil {
		t.Fatal(err)
	}
	backupPath := backup.Path()
	restoreFailure := errors.New("synthetic restore failure")
	applyFailure := errors.New("synthetic migration failure")
	err = recoverFailedMigration(applyFailure, backup, func() error { return restoreFailure })
	if !errors.Is(err, applyFailure) || !errors.Is(err, restoreFailure) {
		t.Fatalf("combined recovery error = %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("verified backup was not retained: %v", err)
	}
	if err := Verify(ctx, backupPath); err != nil {
		t.Fatalf("retained backup is not verifiable: %v", err)
	}
	if err := os.Remove(backupPath); err != nil {
		t.Fatal(err)
	}

	recovered, err := store.temporaryBackupLocked(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	recoveredPath := recovered.Path()
	err = recoverFailedMigration(applyFailure, recovered, func() error { return nil })
	if !errors.Is(err, applyFailure) {
		t.Fatalf("recovered migration error = %v", err)
	}
	if _, err := os.Stat(recoveredPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup after proven recovery still exists: %v", err)
	}
}

func TestStorageContainsNoProhibitedSchemaOrSyntheticMarkers(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	const allowedMarker = "ALLOWED_LOCAL_PROFILE_MARKER_a63"
	now := int64(1)
	profile, err := store.db.ExecContext(ctx,
		`INSERT INTO cluster_profiles(name, context_name, is_default, created_at, updated_at) VALUES (?, 'local', 1, ?, ?)`,
		allowedMarker, now, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	profileID, err := profile.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO cluster_profile_kubeconfig_files(cluster_profile_id, position, path, created_at) VALUES (?, 0, ?, ?)`,
		profileID, "/synthetic/"+allowedMarker, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO preferences(key, value_json, schema_version, updated_at) VALUES ('logs.wrap', 'true', 1, ?)`, now,
	); err != nil {
		t.Fatal(err)
	}

	markers := []string{
		"TOKEN_SHOULD_NOT_PERSIST_7f3",
		"PRIVATE_KEY_SHOULD_NOT_PERSIST_",
		"LOG_LINE_SHOULD_NOT_PERSIST_",
	}
	for index, marker := range markers {
		key := fmt.Sprintf("prohibited.synthetic.%d", index)
		if _, err := store.db.ExecContext(ctx,
			`INSERT INTO preferences(key, value_json, schema_version, updated_at) VALUES (?, ?, 1, ?)`,
			key, strconv.Quote(marker), now,
		); err == nil {
			t.Fatalf("preference allowlist accepted prohibited corpus key %q", key)
		}
	}
	var preferenceCount int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM preferences").Scan(&preferenceCount); err != nil {
		t.Fatal(err)
	}
	if preferenceCount != 1 {
		t.Fatalf("rejected corpus changed preferences: count=%d", preferenceCount)
	}

	prohibitedSchema := []string{"secret", "cluster_object", "resource_yaml", "event_cache", "exec_output", "log_line"}
	rows, err := store.db.QueryContext(ctx, `SELECT type, name, coalesce(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var objectType, name, statement string
		if err := rows.Scan(&objectType, &name, &statement); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		definition := strings.ToLower(objectType + " " + name + " " + statement)
		for _, prohibited := range prohibitedSchema {
			if strings.Contains(definition, prohibited) {
				rows.Close()
				t.Fatalf("prohibited schema marker %q in %s %q", prohibited, objectType, name)
			}
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(filepath.Dir(store.path), "inspection.db")
	if err := store.Backup(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	temporaryBackup, err := store.temporaryBackupLocked(ctx, 9999)
	if err != nil {
		t.Fatal(err)
	}
	temporaryBackupPath := temporaryBackup.Path()
	defer func() {
		_ = temporaryBackup.Close()
		_ = os.Remove(temporaryBackupPath)
	}()

	paths := map[string]struct{}{
		store.path:                       {},
		store.path + "-wal":              {},
		store.path + "-shm":              {},
		store.path + "-journal":          {},
		backupPath:                       {},
		temporaryBackupPath:              {},
		backupPath + "-journal":          {},
		backupPath + "-wal":              {},
		backupPath + "-shm":              {},
		temporaryBackupPath + "-journal": {},
	}
	entries, err := os.ReadDir(filepath.Dir(store.path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "kubePeep.db") || strings.HasPrefix(name, ".kubePeep.db") || strings.HasPrefix(name, "inspection.db") {
			paths[filepath.Join(filepath.Dir(store.path), name)] = struct{}{}
		}
	}
	foundAllowed := false
	inspected := 0
	for path := range paths {
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			t.Fatalf("SQLite artifact is not a regular file: %s", filepath.Base(path))
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("SQLite artifact %s permissions = %o, want 600", filepath.Base(path), info.Mode().Perm())
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		inspected++
		if strings.Contains(string(contents), allowedMarker) {
			foundAllowed = true
		}
		for _, marker := range markers {
			if strings.Contains(string(contents), marker) {
				t.Fatalf("synthetic prohibited marker persisted in %s", filepath.Base(path))
			}
		}
	}
	if inspected < 3 {
		t.Fatalf("inspected only %d SQLite artifacts, want main DB and both backups", inspected)
	}
	if !foundAllowed {
		t.Fatal("allowed corpus was not found in any inspected artifact; inspection is not proving real persisted bytes")
	}
}

func TestSymlinksAreRejectedAndPublicationNeverReplaces(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "source.db")
	store, err := Open(ctx, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "database-link.db")
	if err := os.Symlink(databasePath, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
	} else {
		if _, err := Open(ctx, link); err == nil {
			t.Fatal("Open followed a database symlink")
		}
		if err := Verify(ctx, link); err == nil {
			t.Fatal("Verify followed a database symlink")
		}
	}

	temporary, err := securefs.CreateTemp(directory, ".publish-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	temporaryPath := temporary.Path()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if _, err := temporary.File().WriteString("new-content"); err != nil {
		t.Fatal(err)
	}
	if err := temporary.File().Sync(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "published.db")
	if err := os.WriteFile(destination, []byte("existing-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := temporary.PublishNoReplace(destination); err == nil {
		t.Fatal("no-replace publication replaced an existing destination")
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "existing-content" {
		t.Fatalf("destination was changed to %q", contents)
	}
}

func schemaTables(t *testing.T, store *Store) []string {
	t.Helper()
	rows, err := store.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(tables)
	return tables
}

func insertProfile(t *testing.T, store *Store, name string) {
	t.Helper()
	if _, err := store.db.Exec(`INSERT INTO cluster_profiles(name, context_name, is_default, created_at, updated_at) VALUES (?, 'local', 1, 1, 1)`, name); err != nil {
		t.Fatal(err)
	}
}

func testMigration(version int, name, statement string, destructive bool) migrations.Migration {
	digest := sha256.Sum256([]byte(statement))
	return migrations.Migration{
		Version:     version,
		Name:        name,
		SQL:         statement,
		Checksum:    hex.EncodeToString(digest[:]),
		Destructive: destructive,
	}
}
