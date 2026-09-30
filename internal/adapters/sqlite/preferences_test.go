package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/migrations"
	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

func TestExpandPreferencesMigrationPreservesLegacyRowsAndAcceptsCurrentSnapshot(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := sql.Open("sqlite", dataSourceName(path, false))
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(MaxOpenConnections)
	database.SetMaxIdleConns(MaxIdleConnections)
	store := &Store{db: database, path: path}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := database.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		t.Fatal(err)
	}
	set, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyMigrations(ctx, set[:1]); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO preferences(key, value_json, schema_version, updated_at) VALUES ('logs.wrap', 'true', 1, 1234)`); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyMigrations(ctx, set); err != nil {
		t.Fatal(err)
	}
	var preserved string
	if err := database.QueryRowContext(ctx, `SELECT value_json FROM preferences WHERE key = 'logs.wrap'`).Scan(&preserved); err != nil || preserved != "true" {
		t.Fatalf("legacy preference was not preserved: value=%q err=%v", preserved, err)
	}
	if err := NewPreferenceRepository(store).Replace(ctx, validPreferenceRecords()); err != nil {
		t.Fatalf("current preference snapshot rejected after migration: %v", err)
	}
}

func TestPreferenceRepositoryReplaceLoadAndAtomicRollback(t *testing.T) {
	store := openTestStore(t)
	repository := NewPreferenceRepository(store)
	repository.now = func() time.Time { return time.UnixMilli(1234) }
	records := validPreferenceRecords()
	if err := repository.Replace(context.Background(), records); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	loaded, err := repository.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(loaded, records) {
		t.Fatalf("loaded records mismatch\n got: %#v\nwant: %#v", loaded, records)
	}

	repository.now = func() time.Time { return time.UnixMilli(-1) }
	changed := append([]resources.PreferenceRecord(nil), records...)
	changed[0].ValueJSON = []byte(`"pt-BR"`)
	if err := repository.Replace(context.Background(), changed); err == nil {
		t.Fatal("Replace accepted a database constraint failure")
	}
	loaded, err = repository.Load(context.Background())
	if err != nil {
		t.Fatalf("Load after rollback: %v", err)
	}
	if !reflect.DeepEqual(loaded, records) {
		t.Fatalf("failed replacement was not atomic\n got: %#v\nwant: %#v", loaded, records)
	}
}

func TestPreferenceRepositoryRejectsPartialDuplicateAndUnknownSnapshots(t *testing.T) {
	repository := NewPreferenceRepository(openTestStore(t))
	records := validPreferenceRecords()
	for name, candidate := range map[string][]resources.PreferenceRecord{
		"partial":   records[:len(records)-1],
		"duplicate": append(append([]resources.PreferenceRecord(nil), records[:len(records)-1]...), records[0]),
		"unknown":   append(append([]resources.PreferenceRecord(nil), records[:len(records)-1]...), resources.PreferenceRecord{Key: "arbitrary", ValueJSON: []byte(`true`), SchemaVersion: 1}),
	} {
		t.Run(name, func(t *testing.T) {
			if err := repository.Replace(context.Background(), candidate); err == nil {
				t.Fatal("Replace accepted invalid snapshot")
			}
		})
	}
}

func validPreferenceRecords() []resources.PreferenceRecord {
	return []resources.PreferenceRecord{
		{Key: "columns.hidden", ValueJSON: []byte(`{}`), SchemaVersion: 1},
		{Key: "dashboard.hidden_sections", ValueJSON: []byte(`[]`), SchemaVersion: 1},
		{Key: "dashboard.log_scan_window", ValueJSON: []byte(`"15m"`), SchemaVersion: 1},
		{Key: "dashboard.section_order", ValueJSON: []byte(`["summary","problems","restarts","workloads","events","logScan","metrics"]`), SchemaVersion: 1},
		{Key: "favorites", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "filters.events", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "filters.logs", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "filters.pods", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "filters.workloads", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "logs.tail_lines", ValueJSON: []byte(`200`), SchemaVersion: 1},
		{Key: "logs.timestamps", ValueJSON: []byte(`true`), SchemaVersion: 1},
		{Key: "logs.wrap", ValueJSON: []byte(`false`), SchemaVersion: 1},
		{Key: "recent", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "shell.collapsed_groups", ValueJSON: []byte(`[]`), SchemaVersion: 1},
		{Key: "shell.sidebar_compact", ValueJSON: []byte(`false`), SchemaVersion: 1},
		{Key: "ui.language", ValueJSON: []byte(`"en"`), SchemaVersion: 1},
	}
}
