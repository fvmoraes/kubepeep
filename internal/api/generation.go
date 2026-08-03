package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
)

// GenerationSource supplies the non-empty process/selection generation bound
// to sessions and future mutations.
type GenerationSource interface {
	Current() string
}

// GenerationStore owns an opaque in-memory generation. Rotate is intended to
// be called only after a selection commit succeeds.
type GenerationStore struct {
	mu      sync.RWMutex
	current string
}

func NewGenerationStore() (*GenerationStore, error) {
	generation, err := newOpaqueValue("gen_", 24)
	if err != nil {
		return nil, err
	}
	return &GenerationStore{current: generation}, nil
}

func (s *GenerationStore) Current() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *GenerationStore) Rotate() (string, error) {
	next, err := newOpaqueValue("gen_", 24)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.current = next
	s.mu.Unlock()
	return next, nil
}

func newOpaqueValue(prefix string, size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("api: generate opaque value: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}
