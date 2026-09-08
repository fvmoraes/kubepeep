package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/observability"
)

type storedCursorStateFixture struct {
	Version int                   `json:"version"`
	Origins []storedFixtureOrigin `json:"origins"`
}

type storedFixtureOrigin struct {
	Namespace string   `json:"namespace"`
	Buffered  []string `json:"buffered"`
}

func newTestCursorStore(now func() time.Time, ttl time.Duration, maxEntries int, maxBytes, entryBytes int64) *CursorStore {
	store := NewCursorStore(now)
	store.ttl = ttl
	store.maxEntries = maxEntries
	store.maxBytes = maxBytes
	store.entryBytes = entryBytes
	return store
}

func TestCursorStoreRoundTripPreservesState(t *testing.T) {
	store := NewCursorStore(nil)
	state := storedCursorStateFixture{Version: 1, Origins: []storedFixtureOrigin{{Namespace: "a", Buffered: []string{"a1", "a2"}}}}
	reference, err := store.Put(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(reference) < 22 {
		t.Fatalf("reference is too short to carry 128 bits: %q", reference)
	}
	var decoded storedCursorStateFixture
	if err := store.Get(reference, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Version != 1 || len(decoded.Origins) != 1 || len(decoded.Origins[0].Buffered) != 2 {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestCursorStoreGetRejectsUnknownAndEmptyReference(t *testing.T) {
	store := NewCursorStore(nil)
	var decoded storedCursorStateFixture
	err := store.Get("missing-reference", &decoded)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusGone || string(httpErr.AppError.Code) != CodeCursorExpired {
		t.Fatalf("missing reference error = %v", err)
	}
	if err := store.Get("", &decoded); !errors.As(err, &httpErr) || httpErr.Status != http.StatusGone {
		t.Fatalf("empty reference error = %v", err)
	}
}

func TestCursorStoreExpiresWithTTL(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	store := newTestCursorStore(func() time.Time { return now }, time.Minute, 8, 1<<20, 1<<20)
	reference, err := store.Put(storedCursorStateFixture{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	var decoded storedCursorStateFixture
	if err := store.Get(reference, &decoded); err == nil {
		t.Fatal("expired reference was served")
	}
	if store.Len() != 0 || store.Bytes() != 0 {
		t.Fatalf("expired entry was not reclaimed: len=%d bytes=%d", store.Len(), store.Bytes())
	}
}

func TestCursorStoreFixedTTLAndPurgeMetrics(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	metrics := observability.NewRegistry()
	store := NewCursorStoreWithMetrics(func() time.Time { return now }, metrics)
	store.ttl = time.Minute
	first, err := store.Put("first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put("second")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(59 * time.Second)
	var value string
	if err := store.Get(first, &value); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	third, err := store.Put("third") // Purge both expired entries, including the recently read one.
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{first, second} {
		var httpErr *HTTPError
		if err := store.Get(reference, &value); !errors.As(err, &httpErr) || httpErr.Status != http.StatusGone || string(httpErr.AppError.Code) != CodeCursorExpired {
			t.Fatalf("want recoverable 410, got %v", err)
		}
	}
	if store.Len() != 1 || store.Bytes() != int64(len(`"third"`)) {
		t.Fatalf("purge left stale occupancy: %d/%d", store.Len(), store.Bytes())
	}
	store.Delete(third)
	for _, want := range []string{"kubepeep_cursor_expired_total 2", "kubepeep_cursor_hits_total 1", "kubepeep_cursor_misses_total 2", "kubepeep_cursor_entries 0", "kubepeep_cursor_bytes 0"} {
		if !strings.Contains(metrics.Render(), want) {
			t.Fatalf("missing %q: %s", want, metrics.Render())
		}
	}
}

func TestCursorStoreDeleteRemovesEntry(t *testing.T) {
	store := NewCursorStore(nil)
	reference, err := store.Put(storedCursorStateFixture{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	store.Delete(reference)
	var decoded storedCursorStateFixture
	if err := store.Get(reference, &decoded); err == nil {
		t.Fatal("deleted reference was served")
	}
	if store.Len() != 0 || store.Bytes() != 0 {
		t.Fatalf("delete did not reclaim memory: len=%d bytes=%d", store.Len(), store.Bytes())
	}
	store.Delete("") // must be a no-op
}

func TestCursorStoreRejectsOversizedEntry(t *testing.T) {
	store := newTestCursorStore(nil, time.Minute, 8, 1<<20, 1<<10)
	_, err := store.Put(storedCursorStateFixture{Version: 1, Origins: []storedFixtureOrigin{{Namespace: strings.Repeat("a", 4<<10)}}})
	if err == nil {
		t.Fatal("oversized state was accepted")
	}
	if store.Len() != 0 {
		t.Fatalf("rejected entry was stored: %d", store.Len())
	}
}

func TestCursorStoreEvictsLeastRecentlyUsedEntries(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	store := newTestCursorStore(func() time.Time { return now }, time.Hour, 2, 1<<20, 1<<20)
	first, err := store.Put(storedCursorStateFixture{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put(storedCursorStateFixture{Version: 2})
	if err != nil {
		t.Fatal(err)
	}
	var touched storedCursorStateFixture
	if err := store.Get(first, &touched); err != nil {
		t.Fatal(err)
	}
	third, err := store.Put(storedCursorStateFixture{Version: 3})
	if err != nil {
		t.Fatal(err)
	}
	var decoded storedCursorStateFixture
	// `first` was touched after `second`, so `second` is the least recently
	// used entry and must be the eviction victim.
	if err := store.Get(second, &decoded); err == nil {
		t.Fatal("least recently used entry survived eviction")
	}
	if err := store.Get(first, &decoded); err != nil || decoded.Version != 1 {
		t.Fatalf("recently used entry evicted: %v", err)
	}
	if err := store.Get(third, &decoded); err != nil || decoded.Version != 3 {
		t.Fatalf("new entry evicted: %v", err)
	}
	if store.Len() != 2 {
		t.Fatalf("len = %d, want 2", store.Len())
	}
}

func TestCursorStoreByteBudgetEviction(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	// Three ~62-byte entries fit inside the 200-byte budget; the fourth must
	// evict the oldest reference instead of growing unbounded.
	store := newTestCursorStore(func() time.Time { return now }, time.Hour, 16, 200, 1<<20)
	references := make([]string, 0, 4)
	for index := 0; index < 4; index++ {
		reference, err := store.Put(storedCursorStateFixture{Version: index, Origins: []storedFixtureOrigin{{Namespace: "aaaa"}}})
		if err != nil {
			t.Fatal(err)
		}
		references = append(references, reference)
	}
	if store.Bytes() > 200 {
		t.Fatalf("byte budget exceeded: %d", store.Bytes())
	}
	var decoded storedCursorStateFixture
	if err := store.Get(references[0], &decoded); err == nil {
		t.Fatal("oldest entry survived budget eviction")
	}
	for index := 1; index < len(references); index++ {
		if err := store.Get(references[index], &decoded); err != nil || decoded.Version != index {
			t.Fatalf("entry %d was evicted: %v", index, err)
		}
	}
}

func TestCursorStoreConcurrentAccess(t *testing.T) {
	store := NewCursorStore(nil)
	var group sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for index := 0; index < 50; index++ {
				reference, err := store.Put(storedCursorStateFixture{Version: index, Origins: []storedFixtureOrigin{{Namespace: "ns", Buffered: []string{"item"}}}})
				if err != nil {
					t.Errorf("put: %v", err)
					return
				}
				var decoded storedCursorStateFixture
				if err := store.Get(reference, &decoded); err != nil {
					t.Errorf("get: %v", err)
					return
				}
				store.Delete(reference)
			}
		}(worker)
	}
	group.Wait()
	if store.Len() != 0 || store.Bytes() != 0 {
		t.Fatalf("store leaked entries: len=%d bytes=%d", store.Len(), store.Bytes())
	}
}

func TestCursorStoreNormalizesEquivalentState(t *testing.T) {
	store := NewCursorStore(nil)
	reference, err := store.Put(map[string]any{"b": 1, "a": "x"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := store.Get(reference, &decoded); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"a":"x","b":1}` {
		t.Fatalf("state was not canonically normalized: %s", encoded)
	}
}

// BenchmarkCursorStorePutGet measures the server-side cursor parking cost that
// replaced serializing buffered DTOs into the signed token.
func BenchmarkCursorStorePutGet(b *testing.B) {
	store := NewCursorStore(nil)
	state := storedCursorStateFixture{Version: 1, Origins: make([]storedFixtureOrigin, 100)}
	for index := range state.Origins {
		state.Origins[index] = storedFixtureOrigin{Namespace: fmt.Sprintf("ns-%04d", index), Buffered: []string{"buffered-item"}}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		reference, err := store.Put(state)
		if err != nil {
			b.Fatal(err)
		}
		var decoded storedCursorStateFixture
		if err := store.Get(reference, &decoded); err != nil {
			b.Fatal(err)
		}
		store.Delete(reference)
	}
}
