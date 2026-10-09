package clusterprofiles

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrImportInvalid       = errors.New("kubeconfig import is invalid")
	ErrImportConflict      = errors.New("kubeconfig import conflicts with existing entries")
	ErrImportRelativePaths = errors.New("kubeconfig import contains relative credential paths")
	ErrImportUnavailable   = errors.New("kubeconfig import is unavailable")
)

type ImportRequest struct {
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

// ConfigImporter validates without executing authentication plugins. Content
// is stored only in the OS kubeconfig location, never in the preferences DB.
type ConfigImporter interface {
	Import(context.Context, ImportRequest) (path string, err error)
}

func (s *Service) Import(ctx context.Context, request ImportRequest) (DTO, error) {
	if s.Importer == nil {
		return DTO{}, ErrImportUnavailable
	}
	path, err := s.Importer.Import(ctx, request)
	if err != nil {
		return DTO{}, fmt.Errorf("import kubeconfig: %w", err)
	}
	profiles, err := s.repository.List(ctx)
	if err != nil {
		return DTO{}, fmt.Errorf("list imported profiles: %w", err)
	}
	profile, _, err := s.repository.Reconcile(ctx, "Kubernetes", []string{path}, len(profiles) == 0)
	if err != nil {
		return DTO{}, fmt.Errorf("register imported profile: %w", err)
	}
	return ToDTO(profile, s.home), nil
}
