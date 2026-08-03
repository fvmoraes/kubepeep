package api

import (
	"bytes"
	"testing"
	"time"
)

func TestSessionStoreExpiresAndRotatesWithGeneration(t *testing.T) {
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	randomBytes := append(bytes.Repeat([]byte{0x42}, 32), bytes.Repeat([]byte{0x43}, 32)...)
	randomBytes = append(randomBytes, bytes.Repeat([]byte{0x44}, 32)...)
	random := bytes.NewReader(randomBytes)
	store, err := NewSessionStoreWithOptions(SessionStoreOptions{
		TTL:    time.Hour,
		Now:    func() time.Time { return now },
		Random: random,
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Current("http://127.0.0.1:2748", "gen_a")
	if err != nil {
		t.Fatal(err)
	}
	if first.CSRFToken == "" || !store.Validate(first.CSRFToken, "gen_a") {
		t.Fatal("fresh nonce was not accepted")
	}

	second, err := store.Current("http://127.0.0.1:2748", "gen_b")
	if err != nil {
		t.Fatal(err)
	}
	if second.CSRFToken == first.CSRFToken || store.Validate(first.CSRFToken, "gen_b") {
		t.Fatal("generation change did not invalidate and rotate the nonce")
	}

	now = now.Add(time.Hour)
	if store.Validate(second.CSRFToken, "gen_b") {
		t.Fatal("nonce remained valid at its exact expiry")
	}
	third, err := store.Current("http://127.0.0.1:2748", "gen_b")
	if err != nil {
		t.Fatal(err)
	}
	if third.CSRFToken == second.CSRFToken {
		t.Fatal("expired nonce was not rotated")
	}
}

func TestSessionStoreRejectsTTLAboveContractMaximum(t *testing.T) {
	if _, err := NewSessionStore(MaxSessionTTL + time.Nanosecond); err == nil {
		t.Fatal("expected TTL validation error")
	}
}
