package sqlite

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

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
		{Key: "dashboard.hidden_sections", ValueJSON: []byte(`[]`), SchemaVersion: 1},
		{Key: "dashboard.log_scan_window", ValueJSON: []byte(`"15m"`), SchemaVersion: 1},
		{Key: "dashboard.section_order", ValueJSON: []byte(`["summary","problems","restarts","workloads","events","logScan","metrics"]`), SchemaVersion: 1},
		{Key: "filters.events", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "filters.logs", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "filters.pods", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "filters.workloads", ValueJSON: []byte(`{"version":1,"items":[]}`), SchemaVersion: 1},
		{Key: "logs.tail_lines", ValueJSON: []byte(`200`), SchemaVersion: 1},
		{Key: "logs.timestamps", ValueJSON: []byte(`true`), SchemaVersion: 1},
		{Key: "logs.wrap", ValueJSON: []byte(`false`), SchemaVersion: 1},
		{Key: "ui.language", ValueJSON: []byte(`"en"`), SchemaVersion: 1},
	}
}
