package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fvmoraes/kubepeep/internal/services/namespaces"
)

// NamespaceScopeRepository persists complete scope aggregates. Every write is
// serialized with BEGIN IMMEDIATE so profile/context and version checks happen
// before any concurrent writer can change their preconditions.
type NamespaceScopeRepository struct {
	database *sql.DB
	now      func() time.Time
}

func NewNamespaceScopeRepository(store *Store) *NamespaceScopeRepository {
	return &NamespaceScopeRepository{database: store.SQLDB(), now: time.Now}
}

func (repository *NamespaceScopeRepository) List(ctx context.Context, profileID int64, contextName string) ([]namespaces.Scope, error) {
	transaction, err := repository.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("sqlite namespace scopes: begin read: %w", err)
	}
	defer transaction.Rollback()

	rows, err := transaction.QueryContext(ctx, `
		SELECT id, cluster_profile_id, context_name, name, mode,
		       default_namespace, version, created_at, updated_at
		FROM namespace_scopes
		WHERE cluster_profile_id = ? AND context_name = ?
		ORDER BY name ASC, id ASC`, profileID, contextName)
	if err != nil {
		return nil, fmt.Errorf("sqlite namespace scopes: list: %w", err)
	}
	var scopes []namespaces.Scope
	for rows.Next() {
		scope, err := scanScope(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("sqlite namespace scopes: close list: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite namespace scopes: iterate list: %w", err)
	}
	for index := range scopes {
		items, err := loadScopeItems(ctx, transaction, scopes[index].ID)
		if err != nil {
			return nil, err
		}
		scopes[index].Namespaces = items
	}
	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite namespace scopes: finish read: %w", err)
	}
	if scopes == nil {
		scopes = make([]namespaces.Scope, 0)
	}
	return scopes, nil
}

func (repository *NamespaceScopeRepository) Get(ctx context.Context, id int64) (namespaces.Scope, error) {
	transaction, err := repository.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return namespaces.Scope{}, fmt.Errorf("sqlite namespace scopes: begin read: %w", err)
	}
	defer transaction.Rollback()
	scope, err := loadScope(ctx, transaction, id)
	if err != nil {
		return namespaces.Scope{}, err
	}
	if err := transaction.Commit(); err != nil {
		return namespaces.Scope{}, fmt.Errorf("sqlite namespace scopes: finish read: %w", err)
	}
	return scope, nil
}

func (repository *NamespaceScopeRepository) Create(ctx context.Context, draft namespaces.ScopeDraft) (namespaces.Scope, error) {
	if err := namespaces.ValidateDraft(draft); err != nil {
		return namespaces.Scope{}, err
	}
	var created namespaces.Scope
	err := withImmediate(ctx, repository.database, func(connection *sql.Conn) error {
		if err := validateProfileContext(ctx, connection, draft.ClusterProfileID, draft.Context); err != nil {
			return err
		}
		if conflict, err := scopeNameExists(ctx, connection, draft.ClusterProfileID, draft.Context, draft.Name, 0); err != nil {
			return err
		} else if conflict {
			return namespaces.ErrConflict
		}

		now := repository.now().UTC().UnixMilli()
		result, err := connection.ExecContext(ctx, `
			INSERT INTO namespace_scopes (
				cluster_profile_id, context_name, name, mode, default_namespace,
				version, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
			draft.ClusterProfileID, draft.Context, draft.Name, string(draft.Mode), nullableString(draft.DefaultNamespace), now, now)
		if err != nil {
			return mapScopeWriteError(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("sqlite namespace scopes: identify created scope: %w", err)
		}
		if err := insertScopeItems(ctx, connection, id, draft.Namespaces, now); err != nil {
			return err
		}
		if err := verifyStoredScope(ctx, connection, id); err != nil {
			return err
		}
		created = namespaces.Scope{
			ID: id, ClusterProfileID: draft.ClusterProfileID, Context: draft.Context,
			Name: draft.Name, Mode: draft.Mode, Namespaces: append([]string(nil), draft.Namespaces...),
			DefaultNamespace: copyNamespacePointer(draft.DefaultNamespace), Version: 1,
			CreatedAt: time.UnixMilli(now).UTC(), UpdatedAt: time.UnixMilli(now).UTC(),
		}
		return nil
	})
	if err != nil {
		return namespaces.Scope{}, err
	}
	return created, nil
}

func (repository *NamespaceScopeRepository) Update(ctx context.Context, id, expectedVersion int64, draft namespaces.ScopeDraft) (namespaces.Scope, error) {
	if err := namespaces.ValidateDraft(draft); err != nil {
		return namespaces.Scope{}, err
	}
	if id <= 0 || expectedVersion <= 0 {
		return namespaces.Scope{}, namespaces.ErrConflict
	}
	var updated namespaces.Scope
	err := withImmediate(ctx, repository.database, func(connection *sql.Conn) error {
		var (
			storedProfileID int64
			storedContext   string
			storedVersion   int64
			createdAt       int64
		)
		err := connection.QueryRowContext(ctx, `
			SELECT cluster_profile_id, context_name, version, created_at
			FROM namespace_scopes WHERE id = ?`, id).
			Scan(&storedProfileID, &storedContext, &storedVersion, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			return namespaces.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("sqlite namespace scopes: read update precondition: %w", err)
		}
		if storedVersion != expectedVersion {
			return namespaces.ErrConflict
		}
		if storedProfileID != draft.ClusterProfileID || storedContext != draft.Context {
			return namespaces.ErrSelectionMismatch
		}
		if err := validateProfileContext(ctx, connection, draft.ClusterProfileID, draft.Context); err != nil {
			return err
		}
		if conflict, err := scopeNameExists(ctx, connection, draft.ClusterProfileID, draft.Context, draft.Name, id); err != nil {
			return err
		} else if conflict {
			return namespaces.ErrConflict
		}

		// Delete children first so the defensive mode-update trigger observes a
		// valid transition. A later failure rolls this deletion back.
		if _, err := connection.ExecContext(ctx, `DELETE FROM namespace_scope_items WHERE namespace_scope_id = ?`, id); err != nil {
			return fmt.Errorf("sqlite namespace scopes: replace items: %w", err)
		}
		now := repository.now().UTC().UnixMilli()
		result, err := connection.ExecContext(ctx, `
			UPDATE namespace_scopes
			SET name = ?, mode = ?, default_namespace = ?, version = version + 1, updated_at = ?
			WHERE id = ? AND version = ? AND cluster_profile_id = ? AND context_name = ?`,
			draft.Name, string(draft.Mode), nullableString(draft.DefaultNamespace), now,
			id, expectedVersion, draft.ClusterProfileID, draft.Context)
		if err != nil {
			return mapScopeWriteError(err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite namespace scopes: inspect update: %w", err)
		}
		if rows != 1 {
			return namespaces.ErrConflict
		}
		if err := insertScopeItems(ctx, connection, id, draft.Namespaces, now); err != nil {
			return err
		}
		if err := verifyStoredScope(ctx, connection, id); err != nil {
			return err
		}
		updated = namespaces.Scope{
			ID: id, ClusterProfileID: draft.ClusterProfileID, Context: draft.Context,
			Name: draft.Name, Mode: draft.Mode, Namespaces: append([]string(nil), draft.Namespaces...),
			DefaultNamespace: copyNamespacePointer(draft.DefaultNamespace), Version: expectedVersion + 1,
			CreatedAt: time.UnixMilli(createdAt).UTC(), UpdatedAt: time.UnixMilli(now).UTC(),
		}
		return nil
	})
	if err != nil {
		return namespaces.Scope{}, err
	}
	return updated, nil
}

func (repository *NamespaceScopeRepository) Delete(ctx context.Context, id, expectedVersion int64) error {
	if id <= 0 || expectedVersion <= 0 {
		return namespaces.ErrConflict
	}
	return withImmediate(ctx, repository.database, func(connection *sql.Conn) error {
		result, err := connection.ExecContext(ctx, `DELETE FROM namespace_scopes WHERE id = ? AND version = ?`, id, expectedVersion)
		if err != nil {
			return fmt.Errorf("sqlite namespace scopes: delete: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite namespace scopes: inspect delete: %w", err)
		}
		if rows == 1 {
			return nil
		}
		var exists int
		if err := connection.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM namespace_scopes WHERE id = ?)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("sqlite namespace scopes: distinguish delete conflict: %w", err)
		}
		if exists == 0 {
			return namespaces.ErrNotFound
		}
		return namespaces.ErrConflict
	})
}

type scopeScanner interface {
	Scan(...any) error
}

type scopeQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func scanScope(scanner scopeScanner) (namespaces.Scope, error) {
	var (
		scope     namespaces.Scope
		mode      string
		defaultNS sql.NullString
		createdAt int64
		updatedAt int64
	)
	if err := scanner.Scan(
		&scope.ID, &scope.ClusterProfileID, &scope.Context, &scope.Name, &mode,
		&defaultNS, &scope.Version, &createdAt, &updatedAt,
	); err != nil {
		return namespaces.Scope{}, fmt.Errorf("sqlite namespace scopes: decode scope: %w", err)
	}
	scope.Mode = namespaces.ScopeMode(mode)
	if defaultNS.Valid {
		scope.DefaultNamespace = &defaultNS.String
	}
	scope.CreatedAt = time.UnixMilli(createdAt).UTC()
	scope.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return scope, nil
}

func loadScope(ctx context.Context, queryer scopeQueryer, id int64) (namespaces.Scope, error) {
	scope, err := scanScope(queryer.QueryRowContext(ctx, `
		SELECT id, cluster_profile_id, context_name, name, mode,
		       default_namespace, version, created_at, updated_at
		FROM namespace_scopes WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return namespaces.Scope{}, namespaces.ErrNotFound
	}
	if err != nil {
		return namespaces.Scope{}, err
	}
	items, err := loadScopeItems(ctx, queryer, id)
	if err != nil {
		return namespaces.Scope{}, err
	}
	scope.Namespaces = items
	return scope, nil
}

func loadScopeItems(ctx context.Context, queryer scopeQueryer, id int64) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT namespace FROM namespace_scope_items
		WHERE namespace_scope_id = ? ORDER BY position ASC`, id)
	if err != nil {
		return nil, fmt.Errorf("sqlite namespace scopes: read items: %w", err)
	}
	defer rows.Close()
	items := make([]string, 0)
	for rows.Next() {
		var namespace string
		if err := rows.Scan(&namespace); err != nil {
			return nil, fmt.Errorf("sqlite namespace scopes: decode item: %w", err)
		}
		items = append(items, namespace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite namespace scopes: iterate items: %w", err)
	}
	return items, nil
}

func withImmediate(ctx context.Context, database *sql.DB, operation func(*sql.Conn) error) error {
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("sqlite namespace scopes: reserve connection: %w", err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("sqlite namespace scopes: begin immediate: %w", err)
	}
	if err := operation(connection); err != nil {
		rollbackImmediate(connection)
		return err
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		rollbackImmediate(connection)
		return fmt.Errorf("sqlite namespace scopes: commit: %w", err)
	}
	return nil
}

func rollbackImmediate(connection *sql.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = connection.ExecContext(ctx, "ROLLBACK")
}

func validateProfileContext(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, profileID int64, contextName string) error {
	var stored sql.NullString
	err := queryer.QueryRowContext(ctx, `SELECT context_name FROM cluster_profiles WHERE id = ?`, profileID).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return namespaces.ErrSelectionMismatch
	}
	if err != nil {
		return fmt.Errorf("sqlite namespace scopes: validate profile context: %w", err)
	}
	if !stored.Valid || stored.String != contextName {
		return namespaces.ErrSelectionMismatch
	}
	return nil
}

func scopeNameExists(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, profileID int64, contextName, name string, exceptID int64) (bool, error) {
	var exists int
	err := queryer.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM namespace_scopes
			WHERE cluster_profile_id = ? AND context_name = ? AND name = ? AND id <> ?
		)`, profileID, contextName, name, exceptID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sqlite namespace scopes: check name conflict: %w", err)
	}
	return exists != 0, nil
}

func insertScopeItems(ctx context.Context, connection *sql.Conn, scopeID int64, items []string, createdAt int64) error {
	for position, namespace := range items {
		if _, err := connection.ExecContext(ctx, `
			INSERT INTO namespace_scope_items (namespace_scope_id, namespace, position, created_at)
			VALUES (?, ?, ?, ?)`, scopeID, namespace, position, createdAt); err != nil {
			return mapScopeWriteError(err)
		}
	}
	return nil
}

func verifyStoredScope(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) error {
	var (
		mode         string
		defaultNS    sql.NullString
		itemCount    int
		defaultCount int
	)
	err := queryer.QueryRowContext(ctx, `
		SELECT s.mode, s.default_namespace,
		       (SELECT count(*) FROM namespace_scope_items i WHERE i.namespace_scope_id = s.id),
		       (SELECT count(*) FROM namespace_scope_items i WHERE i.namespace_scope_id = s.id AND i.namespace = s.default_namespace)
		FROM namespace_scopes s WHERE s.id = ?`, id).Scan(&mode, &defaultNS, &itemCount, &defaultCount)
	if err != nil {
		return fmt.Errorf("sqlite namespace scopes: verify aggregate: %w", err)
	}
	valid := false
	switch namespaces.ScopeMode(mode) {
	case namespaces.ScopeModeSingle:
		valid = itemCount == 1 && (!defaultNS.Valid || defaultCount == 1)
	case namespaces.ScopeModeList:
		valid = itemCount >= 1 && (!defaultNS.Valid || defaultCount == 1)
	case namespaces.ScopeModeAll:
		valid = itemCount == 0 && !defaultNS.Valid
	}
	if !valid {
		return namespaces.ErrInvariant
	}
	return nil
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func copyNamespacePointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func mapScopeWriteError(err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
		return namespaces.ErrConflict
	}
	return fmt.Errorf("sqlite namespace scopes: write aggregate: %w", err)
}

var _ namespaces.Repository = (*NamespaceScopeRepository)(nil)
