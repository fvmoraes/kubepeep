// Package clusterprofiles persists kubeconfig source references without ever
// persisting kubeconfig content, credentials, rest.Config values, or transient
// fingerprints.
package clusterprofiles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("cluster profile not found")

type Profile struct {
	ID        int64
	Name      string
	Context   *string
	IsDefault bool
	Paths     []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type FileDTO struct {
	Position    int    `json:"position"`
	DisplayPath string `json:"displayPath"`
}

type DTO struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Context         *string   `json:"context"`
	IsDefault       bool      `json:"isDefault"`
	KubeconfigFiles []FileDTO `json:"kubeconfigFiles"`
}

type Repository struct {
	db          *sql.DB
	now         func() time.Time
	reconcileMu sync.Mutex
}

// Service is the sanitized read surface consumed by HTTP handlers. Write and
// path-bearing operations remain available only through Repository to keep
// normalized machine paths away from JSON boundaries.
type Service struct {
	repository *Repository
	home       string
	active     ActiveProfileSource
}

// ActiveProfileSource exposes only the local profile identifier selected by
// the coordinator. It deliberately carries no kubeconfig path or credential.
type ActiveProfileSource interface {
	ActiveProfileID() int64
}

func NewService(repository *Repository, home string, active ...ActiveProfileSource) (*Service, error) {
	if repository == nil {
		return nil, errors.New("cluster profiles: repository is required")
	}
	var source ActiveProfileSource
	if len(active) > 0 {
		source = active[0]
	}
	return &Service{repository: repository, home: home, active: source}, nil
}

func (s *Service) List(ctx context.Context) ([]DTO, error) {
	return s.repository.DTOs(ctx, s.home)
}

func (s *Service) Active(ctx context.Context) (DTO, error) {
	var (
		profile Profile
		err     error
	)
	activeID := int64(0)
	if s.active != nil {
		activeID = s.active.ActiveProfileID()
	}
	if activeID > 0 {
		profile, err = s.repository.Get(ctx, activeID)
	} else {
		profile, err = s.repository.Default(ctx)
	}
	if err != nil {
		return DTO{}, err
	}
	return ToDTO(profile, s.home), nil
}

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("cluster profiles: database is required")
	}
	return &Repository{db: db, now: time.Now}, nil
}

// Reconcile reuses the profile whose ordered path set is exactly equal or
// creates one atomically. Paths are references only; callers must normalize
// and validate them before this boundary.
func (r *Repository) Reconcile(ctx context.Context, suggestedName string, paths []string, makeDefault bool) (Profile, bool, error) {
	if err := validatePaths(paths); err != nil {
		return Profile{}, false, err
	}
	// A kubePeep process is the sole writer (the runtime lock enforces that
	// boundary). Serialize the read-or-create sequence so concurrent startup
	// callers cannot create two profiles for one ordered source set.
	r.reconcileMu.Lock()
	defer r.reconcileMu.Unlock()
	profiles, err := r.List(ctx)
	if err != nil {
		return Profile{}, false, err
	}
	for _, profile := range profiles {
		if slices.Equal(profile.Paths, paths) {
			if makeDefault && !profile.IsDefault {
				profile, err = r.SetContext(ctx, profile.ID, profile.Context, true)
			}
			return profile, false, err
		}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, false, fmt.Errorf("cluster profiles: begin reconcile: %w", err)
	}
	defer tx.Rollback()

	name, err := availableName(ctx, tx, suggestedName)
	if err != nil {
		return Profile{}, false, err
	}
	now := r.now().UTC().UnixMilli()
	if makeDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE cluster_profiles SET is_default = 0, updated_at = ? WHERE is_default = 1`, now); err != nil {
			return Profile{}, false, fmt.Errorf("cluster profiles: clear default: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO cluster_profiles(name, context_name, is_default, created_at, updated_at)
		VALUES (?, NULL, ?, ?, ?)
	`, name, boolInt(makeDefault), now, now)
	if err != nil {
		return Profile{}, false, fmt.Errorf("cluster profiles: insert profile: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Profile{}, false, fmt.Errorf("cluster profiles: read inserted id: %w", err)
	}
	for position, path := range paths {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cluster_profile_kubeconfig_files(cluster_profile_id, position, path, created_at)
			VALUES (?, ?, ?, ?)
		`, id, position, path, now); err != nil {
			return Profile{}, false, fmt.Errorf("cluster profiles: insert path reference: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, false, fmt.Errorf("cluster profiles: commit reconcile: %w", err)
	}
	created, err := r.Get(ctx, id)
	return created, true, err
}

func (r *Repository) List(ctx context.Context) ([]Profile, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, context_name, is_default, created_at, updated_at
		FROM cluster_profiles
		ORDER BY is_default DESC, name ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("cluster profiles: list: %w", err)
	}
	defer rows.Close()

	profiles := make([]Profile, 0)
	for rows.Next() {
		profile, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profile.Paths, err = r.paths(ctx, profile.ID)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cluster profiles: iterate: %w", err)
	}
	return profiles, nil
}

func (r *Repository) Get(ctx context.Context, id int64) (Profile, error) {
	if id <= 0 {
		return Profile{}, ErrNotFound
	}
	profile, err := scanProfile(r.db.QueryRowContext(ctx, `
		SELECT id, name, context_name, is_default, created_at, updated_at
		FROM cluster_profiles WHERE id = ?
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	profile.Paths, err = r.paths(ctx, profile.ID)
	return profile, err
}

func (r *Repository) Default(ctx context.Context) (Profile, error) {
	profile, err := scanProfile(r.db.QueryRowContext(ctx, `
		SELECT id, name, context_name, is_default, created_at, updated_at
		FROM cluster_profiles WHERE is_default = 1
	`))
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	profile.Paths, err = r.paths(ctx, profile.ID)
	return profile, err
}

// SetContext updates the selected context and, optionally, the single default
// profile in one transaction. A nil context is used only during initial
// reconciliation; public selections always provide a non-empty value.
func (r *Repository) SetContext(ctx context.Context, id int64, contextName *string, makeDefault bool) (Profile, error) {
	if id <= 0 || (contextName != nil && strings.TrimSpace(*contextName) == "") {
		return Profile{}, ErrNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, fmt.Errorf("cluster profiles: begin selection: %w", err)
	}
	defer tx.Rollback()

	now := r.now().UTC().UnixMilli()
	if makeDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE cluster_profiles SET is_default = 0, updated_at = ? WHERE is_default = 1 AND id <> ?`, now, id); err != nil {
			return Profile{}, fmt.Errorf("cluster profiles: clear previous default: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE cluster_profiles
		SET context_name = ?, is_default = CASE WHEN ? = 1 THEN 1 ELSE is_default END, updated_at = ?
		WHERE id = ?
	`, contextName, boolInt(makeDefault), now, id)
	if err != nil {
		return Profile{}, fmt.Errorf("cluster profiles: update selection: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Profile{}, fmt.Errorf("cluster profiles: inspect update: %w", err)
	}
	if changed != 1 {
		return Profile{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, fmt.Errorf("cluster profiles: commit selection: %w", err)
	}
	return r.Get(ctx, id)
}

func (r *Repository) DTOs(ctx context.Context, home string) ([]DTO, error) {
	profiles, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]DTO, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, ToDTO(profile, home))
	}
	return result, nil
}

func ToDTO(profile Profile, home string) DTO {
	files := make([]FileDTO, 0, len(profile.Paths))
	for position, path := range profile.Paths {
		files = append(files, FileDTO{Position: position, DisplayPath: displayPath(path, home)})
	}
	return DTO{ID: profile.ID, Name: profile.Name, Context: profile.Context, IsDefault: profile.IsDefault, KubeconfigFiles: files}
}

type rowScanner interface {
	Scan(...any) error
}

func scanProfile(row rowScanner) (Profile, error) {
	var profile Profile
	var contextName sql.NullString
	var isDefault int
	var createdAt, updatedAt int64
	if err := row.Scan(&profile.ID, &profile.Name, &contextName, &isDefault, &createdAt, &updatedAt); err != nil {
		return Profile{}, err
	}
	if contextName.Valid {
		profile.Context = &contextName.String
	}
	profile.IsDefault = isDefault == 1
	profile.CreatedAt = time.UnixMilli(createdAt).UTC()
	profile.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return profile, nil
}

func (r *Repository) paths(ctx context.Context, profileID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT path FROM cluster_profile_kubeconfig_files
		WHERE cluster_profile_id = ? ORDER BY position ASC
	`, profileID)
	if err != nil {
		return nil, fmt.Errorf("cluster profiles: list path references: %w", err)
	}
	defer rows.Close()
	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("cluster profiles: scan path reference: %w", err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cluster profiles: iterate path references: %w", err)
	}
	return paths, nil
}

func availableName(ctx context.Context, tx *sql.Tx, suggested string) (string, error) {
	base := strings.TrimSpace(suggested)
	if base == "" {
		base = "Kubernetes"
	}
	if len(base) > 110 {
		base = base[:110]
	}
	for suffix := 1; suffix <= 9999; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s %d", base, suffix)
		}
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM cluster_profiles WHERE name = ?`, candidate).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("cluster profiles: find available name: %w", err)
		}
	}
	return "", errors.New("cluster profiles: no available profile name")
}

func validatePaths(paths []string) error {
	if len(paths) == 0 {
		return errors.New("cluster profiles: at least one kubeconfig path is required")
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
			return errors.New("cluster profiles: paths must be non-empty and absolute")
		}
		if _, duplicate := seen[path]; duplicate {
			return errors.New("cluster profiles: duplicate kubeconfig path")
		}
		seen[path] = struct{}{}
	}
	return nil
}

func displayPath(path, home string) string {
	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if home != "" && cleanPath == cleanHome {
		return "~"
	}
	if home != "" {
		if relative, err := filepath.Rel(cleanHome, cleanPath); err == nil && relative != "." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".." {
			return filepath.ToSlash(filepath.Join("~", relative))
		}
	}
	return filepath.Base(cleanPath)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
