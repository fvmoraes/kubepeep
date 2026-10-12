-- kubepeep:destructive
-- Persist inactive port-forward recipes while preserving prior preferences.
-- The migration runner verifies a backup before replacing the CHECK constraint.
ALTER TABLE preferences RENAME TO preferences_legacy;

CREATE TABLE preferences (
    key TEXT NOT NULL PRIMARY KEY CHECK (key IN (
        'ui.language',
        'ui.context_colors',
        'logs.wrap',
        'logs.timestamps',
        'logs.tail_lines',
        'dashboard.log_scan_window',
        'dashboard.section_order',
        'dashboard.hidden_sections',
        'filters.workloads',
        'filters.pods',
        'filters.events',
        'filters.logs',
        'favorites',
        'shell.sidebar_compact',
        'shell.collapsed_groups',
        'columns.hidden',
        'columns.order',
        'recent',
        'custom_views',
        'port_forwards'
    )),
    value_json TEXT NOT NULL
        CHECK (json_valid(value_json) AND length(CAST(value_json AS BLOB)) <= 65536),
    schema_version INTEGER NOT NULL CHECK (schema_version >= 1),
    updated_at INTEGER NOT NULL CHECK (updated_at >= 0)
);

INSERT INTO preferences(key, value_json, schema_version, updated_at)
SELECT key, value_json, schema_version, updated_at FROM preferences_legacy;

DROP TABLE preferences_legacy;
