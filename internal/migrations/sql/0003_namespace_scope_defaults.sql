ALTER TABLE namespace_scopes
ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1));

CREATE UNIQUE INDEX one_default_namespace_scope_per_context
    ON namespace_scopes(cluster_profile_id, context_name)
    WHERE is_default = 1;
