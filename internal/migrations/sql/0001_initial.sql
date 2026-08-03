CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(name) > 0),
    checksum TEXT NOT NULL CHECK (length(checksum) = 64),
    applied_at INTEGER NOT NULL CHECK (applied_at >= 0)
);

CREATE TABLE cluster_profiles (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
        CHECK (name = trim(name) AND length(name) BETWEEN 1 AND 120),
    context_name TEXT
        CHECK (context_name IS NULL OR length(CAST(context_name AS BLOB)) BETWEEN 1 AND 1024),
    is_default INTEGER NOT NULL CHECK (is_default IN (0, 1)),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    UNIQUE (name)
);

CREATE UNIQUE INDEX one_default_cluster_profile
    ON cluster_profiles(is_default)
    WHERE is_default = 1;

CREATE TABLE cluster_profile_kubeconfig_files (
    cluster_profile_id INTEGER NOT NULL
        REFERENCES cluster_profiles(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    path TEXT NOT NULL CHECK (length(path) > 0),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    PRIMARY KEY (cluster_profile_id, position),
    UNIQUE (cluster_profile_id, path)
);

CREATE TABLE namespace_scopes (
    id INTEGER PRIMARY KEY,
    cluster_profile_id INTEGER NOT NULL
        REFERENCES cluster_profiles(id) ON DELETE CASCADE,
    context_name TEXT NOT NULL
        CHECK (length(CAST(context_name AS BLOB)) BETWEEN 1 AND 1024),
    name TEXT NOT NULL
        CHECK (name = trim(name) AND length(name) BETWEEN 1 AND 120),
    mode TEXT NOT NULL CHECK (mode IN ('single', 'list', 'all')),
    default_namespace TEXT,
    version INTEGER NOT NULL CHECK (version >= 1),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    CHECK (mode <> 'all' OR default_namespace IS NULL),
    UNIQUE (cluster_profile_id, context_name, name)
);

CREATE INDEX namespace_scopes_by_profile_context
    ON namespace_scopes(cluster_profile_id, context_name, id);

CREATE TABLE namespace_scope_items (
    id INTEGER PRIMARY KEY,
    namespace_scope_id INTEGER NOT NULL
        REFERENCES namespace_scopes(id) ON DELETE CASCADE,
    namespace TEXT NOT NULL CHECK (length(namespace) > 0 AND namespace <> '*'),
    position INTEGER NOT NULL CHECK (position >= 0),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    UNIQUE (namespace_scope_id, namespace),
    UNIQUE (namespace_scope_id, position)
);

CREATE INDEX namespace_scope_items_by_scope
    ON namespace_scope_items(namespace_scope_id, position);

CREATE TRIGGER namespace_scope_items_reject_all
BEFORE INSERT ON namespace_scope_items
WHEN (SELECT mode FROM namespace_scopes WHERE id = NEW.namespace_scope_id) = 'all'
BEGIN
    SELECT RAISE(ABORT, 'all scope cannot contain namespace items');
END;

CREATE TRIGGER namespace_scope_items_limit_single
BEFORE INSERT ON namespace_scope_items
WHEN (SELECT mode FROM namespace_scopes WHERE id = NEW.namespace_scope_id) = 'single'
 AND EXISTS (
     SELECT 1 FROM namespace_scope_items
     WHERE namespace_scope_id = NEW.namespace_scope_id
 )
BEGIN
    SELECT RAISE(ABORT, 'single scope cannot contain multiple namespace items');
END;

CREATE TRIGGER namespace_scopes_guard_mode_update
BEFORE UPDATE OF mode ON namespace_scopes
WHEN (NEW.mode = 'all' AND EXISTS (
        SELECT 1 FROM namespace_scope_items WHERE namespace_scope_id = NEW.id
    ))
 OR (NEW.mode = 'single' AND (
        SELECT count(*) FROM namespace_scope_items WHERE namespace_scope_id = NEW.id
    ) > 1)
BEGIN
    SELECT RAISE(ABORT, 'scope mode conflicts with namespace items');
END;

CREATE TABLE preferences (
    key TEXT NOT NULL PRIMARY KEY CHECK (key IN (
        'ui.language',
        'logs.wrap',
        'logs.timestamps',
        'logs.tail_lines',
        'dashboard.log_scan_window',
        'dashboard.section_order',
        'dashboard.hidden_sections',
        'filters.workloads',
        'filters.pods',
        'filters.events',
        'filters.logs'
    )),
    value_json TEXT NOT NULL
        CHECK (json_valid(value_json) AND length(CAST(value_json AS BLOB)) <= 65536),
    schema_version INTEGER NOT NULL CHECK (schema_version >= 1),
    updated_at INTEGER NOT NULL CHECK (updated_at >= 0)
);
