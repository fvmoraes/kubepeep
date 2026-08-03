package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"
)

const MaxSessionTTL = 8 * time.Hour

type SessionStoreOptions struct {
	TTL    time.Duration
	Now    func() time.Time
	Random io.Reader
}

// SessionStore keeps the CSRF nonce exclusively in memory. A generation
// change, expiration or process restart produces a fresh nonce.
type SessionStore struct {
	mu         sync.Mutex
	ttl        time.Duration
	now        func() time.Time
	random     io.Reader
	token      string
	generation string
	expiresAt  time.Time
}

func NewSessionStore(ttl time.Duration) (*SessionStore, error) {
	return NewSessionStoreWithOptions(SessionStoreOptions{TTL: ttl})
}

func NewSessionStoreWithOptions(options SessionStoreOptions) (*SessionStore, error) {
	if options.TTL == 0 {
		options.TTL = MaxSessionTTL
	}
	if options.TTL < 0 || options.TTL > MaxSessionTTL {
		return nil, fmt.Errorf("api: session TTL must be between zero and %s", MaxSessionTTL)
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	return &SessionStore{ttl: options.TTL, now: options.Now, random: options.Random}, nil
}

func (s *SessionStore) Current(origin, generation string) (SessionData, error) {
	if generation == "" {
		return SessionData{}, fmt.Errorf("api: generation must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	if s.token == "" || s.generation != generation || !now.Before(s.expiresAt) {
		if err := s.rotateLocked(generation, now); err != nil {
			return SessionData{}, err
		}
	}
	return SessionData{
		CSRFToken:  s.token,
		Origin:     origin,
		Generation: s.generation,
		ExpiresAt:  s.expiresAt,
	}, nil
}

func (s *SessionStore) Rotate(generation string) error {
	if generation == "" {
		return fmt.Errorf("api: generation must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rotateLocked(generation, s.now().UTC())
}

func (s *SessionStore) Validate(token, generation string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token == "" || generation == "" || s.token == "" || s.generation != generation || !s.now().UTC().Before(s.expiresAt) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func (s *SessionStore) rotateLocked(generation string, now time.Time) error {
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(s.random, bytes); err != nil {
		return fmt.Errorf("api: generate session nonce: %w", err)
	}
	s.token = base64.RawURLEncoding.EncodeToString(bytes)
	s.generation = generation
	s.expiresAt = now.Add(s.ttl)
	return nil
}
