package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
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

func TestPreferenceServicePersistsContextColorsColumnOrderAndCustomViewsAfterReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "preferences.db")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &resources.PreferenceService{Repository: NewPreferenceRepository(store), Detector: resources.DefaultSensitiveDetector{}}
	value := resources.DefaultPreferences()
	value.UI.Language = "pt-BR"
	value.UI.ContextColors = []resources.ContextColor{
		{ClusterProfileID: 7, Context: "production", Color: "#f87171"},
		{ClusterProfileID: 8, Context: "production", Color: "#38bdf8"},
	}
	value.Columns.Hidden = map[string][]string{"pods": {"age"}}
	value.Columns.Order = map[string][]string{"pods": {"name", "containers", "namespace", "status"}}
	value.CustomViews = []resources.CustomViewContext{
		{ClusterProfileID: 7, Context: "production", Cluster: "cluster-a", Items: []resources.DynamicResource{
			{Group: "apps.example.org", Version: "v1", Resource: "widgets", Kind: "Widget", Namespaced: true, ShortNames: []string{"wdgt"}},
			{Group: "infra.example.org", Version: "v1beta1", Resource: "machines", Kind: "Machine", Namespaced: false},
		}},
		{ClusterProfileID: 7, Context: "production", Cluster: "cluster-b", Items: []resources.DynamicResource{
			{Group: "apps.example.org", Version: "v2", Resource: "widgets", Kind: "Widget", Namespaced: true},
		}},
		{ClusterProfileID: 8, Context: "production", Cluster: "cluster-a", Items: []resources.DynamicResource{
			{Group: "apps.example.org", Version: "v1", Resource: "gadgets", Kind: "Gadget", Namespaced: false},
		}},
	}
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatalf("service Put through SQLite: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	service = &resources.PreferenceService{Repository: NewPreferenceRepository(reopened), Detector: resources.DefaultSensitiveDetector{}}
	loaded, err := service.Get(t.Context())
	if err != nil {
		t.Fatalf("service Get after reopening SQLite: %v", err)
	}
	if !reflect.DeepEqual(loaded, value) {
		t.Fatalf("preferences lost during save/reopen\n got: %#v\nwant: %#v", loaded, value)
	}
	var storedSections int
	if err := reopened.SQLDB().QueryRowContext(t.Context(), `SELECT count(*) FROM preferences WHERE key IN ('ui.context_colors', 'columns.order', 'custom_views')`).Scan(&storedSections); err != nil || storedSections != 3 {
		t.Fatalf("new sections were not stored: count=%d error=%v", storedSections, err)
	}

	// Removing the new section is a complete snapshot replacement, so an old
	// view cannot reappear on the next launch and other settings remain intact.
	value.CustomViews = nil
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	loaded, err = service.Get(t.Context())
	if err != nil || !reflect.DeepEqual(loaded, value) {
		t.Fatalf("clearing custom views changed other preferences: %#v / %v", loaded, err)
	}
}

func TestContextPreferenceMigrationPreservesCompleteLegacySnapshot(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "legacy-v3.db")
	database, err := sql.Open("sqlite", dataSourceName(path, false))
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(MaxOpenConnections)
	database.SetMaxIdleConns(MaxIdleConnections)
	store := &Store{db: database, path: path}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := database.ExecContext(t.Context(), "PRAGMA journal_mode = WAL"); err != nil {
		t.Fatal(err)
	}
	set, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyMigrations(t.Context(), set[:3]); err != nil {
		t.Fatal(err)
	}
	legacy := []resources.PreferenceRecord{}
	for _, record := range validPreferenceRecords() {
		if record.Key == "ui.context_colors" || record.Key == "columns.order" || record.Key == "custom_views" || record.Key == "port_forwards" {
			continue
		}
		if record.Key == "ui.language" {
			record.ValueJSON = []byte(`"pt-BR"`)
		}
		if record.Key == "logs.wrap" {
			record.ValueJSON = []byte(`true`)
		}
		legacy = append(legacy, record)
		if _, err := database.ExecContext(t.Context(), `INSERT INTO preferences(key, value_json, schema_version, updated_at) VALUES (?, ?, ?, 1234)`, record.Key, string(record.ValueJSON), record.SchemaVersion); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ApplyMigrations(t.Context(), set); err != nil {
		t.Fatal(err)
	}
	loadedRecords, err := NewPreferenceRepository(store).Load(t.Context())
	if err != nil || !reflect.DeepEqual(loadedRecords, legacy) {
		t.Fatalf("migration changed legacy records: %#v / %v", loadedRecords, err)
	}
	var changedTimestamps int
	if err := database.QueryRowContext(t.Context(), `SELECT count(*) FROM preferences WHERE updated_at <> 1234`).Scan(&changedTimestamps); err != nil || changedTimestamps != 0 {
		t.Fatalf("migration changed legacy timestamps: %d / %v", changedTimestamps, err)
	}
	service := &resources.PreferenceService{Repository: NewPreferenceRepository(store), Detector: resources.DefaultSensitiveDetector{}}
	value, err := service.Get(t.Context())
	if err != nil || value.UI.Language != "pt-BR" || !value.Logs.Wrap || len(value.UI.ContextColors) != 0 || len(value.Columns.Order) != 0 || len(value.CustomViews) != 0 {
		t.Fatalf("legacy service defaults: %#v / %v", value, err)
	}
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatalf("legacy snapshot could not be saved with current schema: %v", err)
	}
}

func TestSavedPortForwardPreferencesSurviveReopenUpdateAndRemoval(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "port-forwards.db")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &resources.PreferenceService{Repository: NewPreferenceRepository(store), Detector: resources.DefaultSensitiveDetector{}}
	value := resources.DefaultPreferences()
	value.Logs.Wrap = true
	value.UI.ContextColors = []resources.ContextColor{{ClusterProfileID: 7, Context: "production", Color: "#f87171"}}
	value.PortForwards = []resources.SavedPortForward{
		{ClusterProfileID: 7, Context: "production", Namespace: "payments", Kind: "Pod", Name: "api", RemotePort: 8080, LocalPort: 18080},
		{ClusterProfileID: 7, Context: "production", Namespace: "payments", Kind: "Service", Name: "api", RemotePort: 80, LocalPort: 18081},
		{ClusterProfileID: 8, Context: "production", Namespace: "payments", Kind: "Pod", Name: "api", RemotePort: 9090, LocalPort: 19090},
		{ClusterProfileID: 7, Context: "staging", Namespace: "payments", Kind: "Pod", Name: "api", RemotePort: 9091, LocalPort: 19091},
	}
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatalf("save recipes: %v", err)
	}
	// The recipe is replaced when the user changes the selected ports;
	// previous ports must not return on the next application launch.
	value.PortForwards[0].LocalPort = 28080
	value.PortForwards[0].RemotePort = 8081
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatalf("update remembered ports: %v", err)
	}
	reopen := func() resources.PreferencesDTO {
		t.Helper()
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = Open(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		service = &resources.PreferenceService{Repository: NewPreferenceRepository(store), Detector: resources.DefaultSensitiveDetector{}}
		loaded, err := service.Get(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return loaded
	}
	if loaded := reopen(); !reflect.DeepEqual(loaded, value) {
		t.Fatalf("recipes or unrelated preferences changed after reopen: %#v", loaded)
	}
	var encoded []byte
	if err := store.SQLDB().QueryRowContext(t.Context(), `SELECT value_json FROM preferences WHERE key = 'port_forwards'`).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var stored []map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	for _, recipe := range stored {
		if len(recipe) != 7 {
			t.Fatalf("persisted recipe contains non-metadata fields: %s", encoded)
		}
		for _, key := range []string{"clusterProfileId", "context", "namespace", "kind", "name", "remotePort", "localPort"} {
			if _, ok := recipe[key]; !ok {
				t.Fatalf("persisted recipe is missing %s", key)
			}
		}
	}
	value.PortForwards = value.PortForwards[1:]
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatalf("delete one saved recipe: %v", err)
	}
	if loaded := reopen(); !reflect.DeepEqual(loaded, value) {
		t.Fatalf("deleted recipe reappeared or another target changed: %#v", loaded)
	}
	value.PortForwards = nil
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	if loaded := reopen(); !reflect.DeepEqual(loaded, value) {
		t.Fatalf("cleared recipes reappeared or unrelated preferences changed: %#v", loaded)
	}
}

func TestSavedPortForwardMigrationPreservesExistingPreferences(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "legacy-v4.db")
	database, err := sql.Open("sqlite", dataSourceName(path, false))
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(MaxOpenConnections)
	database.SetMaxIdleConns(MaxIdleConnections)
	store := &Store{db: database, path: path}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := database.ExecContext(t.Context(), "PRAGMA journal_mode = WAL"); err != nil {
		t.Fatal(err)
	}
	set, err := migrations.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyMigrations(t.Context(), set[:4]); err != nil {
		t.Fatal(err)
	}
	var legacy []resources.PreferenceRecord
	for _, record := range validPreferenceRecords() {
		if record.Key == "port_forwards" {
			continue
		}
		if record.Key == "ui.context_colors" {
			record.ValueJSON = []byte(`[{"clusterProfileId":7,"context":"production","color":"#f87171"}]`)
		}
		if record.Key == "logs.wrap" {
			record.ValueJSON = []byte(`true`)
		}
		legacy = append(legacy, record)
		if _, err := database.ExecContext(t.Context(), `INSERT INTO preferences(key, value_json, schema_version, updated_at) VALUES (?, ?, ?, 1234)`, record.Key, string(record.ValueJSON), record.SchemaVersion); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Normal startup applies the new migration automatically to an existing database.
	store, err = Open(t.Context(), path)
	if err != nil {
		t.Fatalf("automatic schema upgrade: %v", err)
	}
	loadedRecords, err := NewPreferenceRepository(store).Load(t.Context())
	if err != nil || !reflect.DeepEqual(loadedRecords, legacy) {
		t.Fatalf("migration changed existing preference rows: %#v / %v", loadedRecords, err)
	}
	var changedTimestamps int
	if err := store.SQLDB().QueryRowContext(t.Context(), `SELECT count(*) FROM preferences WHERE updated_at <> 1234`).Scan(&changedTimestamps); err != nil || changedTimestamps != 0 {
		t.Fatalf("migration changed existing timestamps: %d / %v", changedTimestamps, err)
	}
	service := &resources.PreferenceService{Repository: NewPreferenceRepository(store), Detector: resources.DefaultSensitiveDetector{}}
	value, err := service.Get(t.Context())
	if err != nil || len(value.PortForwards) != 0 || !value.Logs.Wrap || len(value.UI.ContextColors) != 1 {
		t.Fatalf("legacy preferences were not preserved with empty recipes: %#v / %v", value, err)
	}
	value.PortForwards = []resources.SavedPortForward{{ClusterProfileID: 7, Context: "production", Namespace: "payments", Kind: "Service", Name: "api", RemotePort: 80, LocalPort: 18080}}
	if _, err := service.Put(t.Context(), value); err != nil {
		t.Fatalf("new recipe rejected after migration: %v", err)
	}
	loaded, err := service.Get(t.Context())
	if err != nil || !reflect.DeepEqual(loaded, value) {
		t.Fatalf("upgraded preferences could not roundtrip: %#v / %v", loaded, err)
	}
}

func validPreferenceRecords() []resources.PreferenceRecord {
	return []resources.PreferenceRecord{
		{Key: "columns.hidden", ValueJSON: []byte(`{}`), SchemaVersion: 1},
		{Key: "columns.order", ValueJSON: []byte(`{}`), SchemaVersion: 1},
		{Key: "custom_views", ValueJSON: []byte(`[]`), SchemaVersion: 1},
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
		{Key: "port_forwards", ValueJSON: []byte(`[]`), SchemaVersion: 1},
		{Key: "recent", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "shell.collapsed_groups", ValueJSON: []byte(`[]`), SchemaVersion: 1},
		{Key: "shell.sidebar_compact", ValueJSON: []byte(`false`), SchemaVersion: 1},
		{Key: "ui.context_colors", ValueJSON: []byte(`[]`), SchemaVersion: 1},
		{Key: "ui.language", ValueJSON: []byte(`"en"`), SchemaVersion: 1},
	}
}
