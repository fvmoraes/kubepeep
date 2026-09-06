package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// CursorStoreTTL mirrors CursorTTL: a stored state never outlives the
	// signed token that references it. Both expire together, so a live token
	// always finds its state and a dead state never serves a forged token.
	CursorStoreTTL = CursorTTL
	// CursorStoreMaxEntries bounds how many distinct pagination windows can be
	// parked in memory at once.
	CursorStoreMaxEntries = 1024
	// CursorStoreMaxBytes is the aggregate memory budget for parked cursor
	// state. Eviction is least-recently-used.
	CursorStoreMaxBytes = 32 << 20
	// CursorStoreMaxEntryBytes rejects a single oversized state instead of
	// letting one query evict the whole store.
	CursorStoreMaxEntryBytes = 1 << 20
	// cursorStoreIDBytes supplies 128 bits of entropy per reference, making
	// references impractical to guess.
	cursorStoreIDBytes = 16
)

// cursorStoreEntry holds the canonical serialization of one composite cursor
// state. Only the payload bytes are stored, so the memory accounting is exact.
type cursorStoreEntry struct {
	payload   []byte
	expiresAt time.Time
	usedAt    uint64
}

// CursorStore keeps composite cursor state server-side so DTO buffers and
// per-origin continuations never travel through the HTTP token. The store is
// process-local, never persisted, and every entry expires with the signed
// token that references it.
type CursorStore struct {
	mu         sync.Mutex
	entries    map[string]*cursorStoreEntry
	now        func() time.Time
	ttl        time.Duration
	maxEntries int
	maxBytes   int64
	entryBytes int64
	bytes      int64
	clock      uint64
}

// NewCursorStore creates one store for the lifetime of the current process.
// A restart therefore invalidates every reference from the previous instance,
// matching the cursor codec's ephemeral secret.
func NewCursorStore(now func() time.Time) *CursorStore {
	if now == nil {
		now = time.Now
	}
	return &CursorStore{
		entries:    make(map[string]*cursorStoreEntry),
		now:        now,
		ttl:        CursorStoreTTL,
		maxEntries: CursorStoreMaxEntries,
		maxBytes:   CursorStoreMaxBytes,
		entryBytes: CursorStoreMaxEntryBytes,
	}
}

// Put serializes one cursor state and returns its random reference. The state
// is normalized before storage so equivalent cursors occupy the same bytes
// regardless of insertion order.
func (store *CursorStore) Put(state any) (string, error) {
	payload, err := canonicalCursorJSON(state)
	if err != nil {
		return "", fmt.Errorf("api: encode cursor store state: %w", err)
	}
	if int64(len(payload)) > store.entryBytes {
		return "", fmt.Errorf("api: cursor state exceeds %d bytes", store.entryBytes)
	}
	reference, err := newCursorStoreReference()
	if err != nil {
		return "", err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeLocked()
	store.evictLocked(int64(len(payload)))
	store.entries[reference] = &cursorStoreEntry{
		payload:   payload,
		expiresAt: store.now().UTC().Add(store.ttl),
		usedAt:    store.touchLocked(),
	}
	store.bytes += int64(len(payload))
	return reference, nil
}

// Get returns the state parked under reference. A missing or expired entry is
// reported as CURSOR_EXPIRED (HTTP 410): the client restarts the list, which
// is the documented recovery for exhausted pagination state.
func (store *CursorStore) Get(reference string, destination any) error {
	if reference == "" || destination == nil {
		return cursorStoreExpired()
	}
	store.mu.Lock()
	entry, ok := store.entries[reference]
	if ok && !store.now().Before(entry.expiresAt) {
		store.bytes -= int64(len(entry.payload))
		delete(store.entries, reference)
		ok = false
	}
	if !ok {
		store.mu.Unlock()
		return cursorStoreExpired()
	}
	payload := append([]byte(nil), entry.payload...)
	entry.usedAt = store.touchLocked()
	store.mu.Unlock()
	if err := decodeCursorJSON(payload, destination); err != nil {
		return invalidCursor(err)
	}
	return nil
}

// Delete drops one reference explicitly; used when a window is abandoned.
func (store *CursorStore) Delete(reference string) {
	if reference == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if entry, ok := store.entries[reference]; ok {
		store.bytes -= int64(len(entry.payload))
		delete(store.entries, reference)
	}
}

// Bytes reports the aggregate payload memory currently parked in the store.
func (store *CursorStore) Bytes() int64 {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.bytes
}

// Len reports how many live references the store currently holds.
func (store *CursorStore) Len() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.entries)
}

func (store *CursorStore) touchLocked() uint64 {
	store.clock++
	return store.clock
}

func (store *CursorStore) purgeLocked() {
	now := store.now()
	for reference, entry := range store.entries {
		if !now.Before(entry.expiresAt) {
			store.bytes -= int64(len(entry.payload))
			delete(store.entries, reference)
		}
	}
}

func (store *CursorStore) evictLocked(incoming int64) {
	for len(store.entries) > 0 && (int64(len(store.entries))+1 > int64(store.maxEntries) || store.bytes+incoming > store.maxBytes) {
		victim := ""
		var oldest uint64
		for reference, entry := range store.entries {
			if victim == "" || entry.usedAt < oldest {
				victim = reference
				oldest = entry.usedAt
			}
		}
		if victim == "" {
			return
		}
		store.bytes -= int64(len(store.entries[victim].payload))
		delete(store.entries, victim)
	}
}

func newCursorStoreReference() (string, error) {
	var id [cursorStoreIDBytes]byte
	if _, err := io.ReadFull(rand.Reader, id[:]); err != nil {
		return "", fmt.Errorf("api: generate cursor reference: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(id[:]), nil
}

func cursorStoreExpired() error {
	return NewHTTPError(http.StatusGone, CodeCursorExpired, "The cursor has expired.", nil, nil)
}
