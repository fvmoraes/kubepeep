package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

const maximumStoredPreferenceBytes = 64 << 10

var preferenceKeys = map[string]struct{}{
	"ui.language": {}, "logs.wrap": {}, "logs.timestamps": {}, "logs.tail_lines": {},
	"dashboard.log_scan_window": {}, "dashboard.section_order": {}, "dashboard.hidden_sections": {},
	"filters.workloads": {}, "filters.pods": {}, "filters.events": {}, "filters.logs": {},
}

type PreferenceRepository struct {
	store *Store
	now   func() time.Time
}

func NewPreferenceRepository(store *Store) *PreferenceRepository {
	return &PreferenceRepository{store: store, now: time.Now}
}

func (repository *PreferenceRepository) Load(ctx context.Context) ([]resources.PreferenceRecord, error) {
	if repository == nil || repository.store == nil || repository.store.SQLDB() == nil {
		return nil, errors.New("sqlite preferences: store is unavailable")
	}
	rows, err := repository.store.SQLDB().QueryContext(ctx, `SELECT key, value_json, schema_version FROM preferences ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("sqlite preferences: load: %w", err)
	}
	defer rows.Close()
	records := []resources.PreferenceRecord{}
	for rows.Next() {
		var record resources.PreferenceRecord
		if err := rows.Scan(&record.Key, &record.ValueJSON, &record.SchemaVersion); err != nil {
			return nil, fmt.Errorf("sqlite preferences: scan: %w", err)
		}
		if err := validatePreferenceRecord(record); err != nil {
			return nil, err
		}
		record.ValueJSON = append([]byte(nil), record.ValueJSON...)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite preferences: rows: %w", err)
	}
	return records, nil
}

func (repository *PreferenceRepository) Replace(ctx context.Context, records []resources.PreferenceRecord) error {
	if repository == nil || repository.store == nil || repository.store.SQLDB() == nil {
		return errors.New("sqlite preferences: store is unavailable")
	}
	if len(records) != len(preferenceKeys) {
		return errors.New("sqlite preferences: replacement must contain the complete allowlist")
	}
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if err := validatePreferenceRecord(record); err != nil {
			return err
		}
		if _, duplicate := seen[record.Key]; duplicate {
			return errors.New("sqlite preferences: replacement contains a duplicate key")
		}
		seen[record.Key] = struct{}{}
	}
	for key := range preferenceKeys {
		if _, ok := seen[key]; !ok {
			return errors.New("sqlite preferences: replacement is incomplete")
		}
	}
	updatedAt := repository.now().UTC().UnixMilli()
	return withImmediate(ctx, repository.store.SQLDB(), func(connection *sql.Conn) error {
		if _, err := connection.ExecContext(ctx, `DELETE FROM preferences`); err != nil {
			return fmt.Errorf("sqlite preferences: clear snapshot: %w", err)
		}
		for _, record := range records {
			if _, err := connection.ExecContext(ctx, `INSERT INTO preferences(key, value_json, schema_version, updated_at) VALUES (?, ?, ?, ?)`, record.Key, string(record.ValueJSON), record.SchemaVersion, updatedAt); err != nil {
				return fmt.Errorf("sqlite preferences: insert snapshot: %w", err)
			}
		}
		return nil
	})
}

func validatePreferenceRecord(record resources.PreferenceRecord) error {
	if _, ok := preferenceKeys[record.Key]; !ok {
		return errors.New("sqlite preferences: key is not allowlisted")
	}
	if record.SchemaVersion < 1 {
		return errors.New("sqlite preferences: schema version is invalid")
	}
	if len(record.ValueJSON) == 0 || len(record.ValueJSON) > maximumStoredPreferenceBytes || !json.Valid(record.ValueJSON) {
		return errors.New("sqlite preferences: value is invalid")
	}
	return nil
}

var _ resources.PreferenceRepository = (*PreferenceRepository)(nil)
