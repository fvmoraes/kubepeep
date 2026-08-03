package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type testCursorState struct {
	ContinueByNamespace map[string]string `json:"continueByNamespace"`
	LastIdentity        string            `json:"lastIdentity"`
}

func TestCursorRoundTripIsCanonicalAndOpaque(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	codec := fixedCursorCodec(t, &now, 1)
	binding := testCursorBinding()
	first, err := codec.Encode(binding, testCursorState{
		ContinueByNamespace: map[string]string{"zeta": "next-z", "alpha": "next-a"},
		LastIdentity:        "pods/default/example",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := codec.Encode(binding, map[string]any{
		"lastIdentity":        "pods/default/example",
		"continueByNamespace": map[string]string{"alpha": "next-a", "zeta": "next-z"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equivalent cursor state did not produce canonical token bytes")
	}
	for _, prohibited := range []string{"next-a", "pods/default/example", "development"} {
		if strings.Contains(first, prohibited) {
			t.Fatalf("cursor exposed payload fragment %q", prohibited)
		}
	}
	var decoded testCursorState
	if err := codec.Decode(first, binding, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.LastIdentity != "pods/default/example" || decoded.ContinueByNamespace["alpha"] != "next-a" {
		t.Fatalf("decoded cursor state = %#v", decoded)
	}
}

func TestCursorExpiresWithoutSlidingTTL(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	codec := fixedCursorCodec(t, &now, 2)
	token, err := codec.Encode(testCursorBinding(), testCursorState{})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(CursorTTL - time.Millisecond)
	if err := codec.Decode(token, testCursorBinding(), &testCursorState{}); err != nil {
		t.Fatalf("cursor expired early: %v", err)
	}
	now = now.Add(time.Millisecond)
	assertCursorError(t, codec.Decode(token, testCursorBinding(), &testCursorState{}), http.StatusGone, CodeCursorExpired)
	// Repeated decoding cannot extend the expiration embedded in the token.
	now = now.Add(time.Minute)
	assertCursorError(t, codec.Decode(token, testCursorBinding(), &testCursorState{}), http.StatusGone, CodeCursorExpired)
}

func TestCursorRejectsTamperingAndPreviousProcess(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	codec := fixedCursorCodec(t, &now, 3)
	token, err := codec.Encode(testCursorBinding(), testCursorState{})
	if err != nil {
		t.Fatal(err)
	}
	tampered := token[:len(token)-1] + string(differentBase64Byte(token[len(token)-1]))
	assertCursorError(t, codec.Decode(tampered, testCursorBinding(), &testCursorState{}), http.StatusBadRequest, CodeCursorInvalid)

	restarted := fixedCursorCodec(t, &now, 4)
	assertCursorError(t, restarted.Decode(token, testCursorBinding(), &testCursorState{}), http.StatusBadRequest, CodeCursorInvalid)
	assertCursorError(t, codec.Decode(strings.Repeat("x", MaxCursorTokenBytes+1), testCursorBinding(), &testCursorState{}), http.StatusBadRequest, CodeCursorInvalid)
}

func TestCursorRejectsEveryBindingMismatch(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	codec := fixedCursorCodec(t, &now, 5)
	binding := testCursorBinding()
	token, err := codec.Encode(binding, testCursorState{})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]CursorBinding{
		"query":      {QueryHash: HashCursorQuery("limit=200"), Context: binding.Context, Scope: binding.Scope, Generation: binding.Generation},
		"context":    {QueryHash: binding.QueryHash, Context: "staging", Scope: binding.Scope, Generation: binding.Generation},
		"scope":      {QueryHash: binding.QueryHash, Context: binding.Context, Scope: "all", Generation: binding.Generation},
		"generation": {QueryHash: binding.QueryHash, Context: binding.Context, Scope: binding.Scope, Generation: "gen_next"},
	}
	for name, expected := range tests {
		t.Run(name, func(t *testing.T) {
			assertCursorError(t, codec.Decode(token, expected, &testCursorState{}), http.StatusBadRequest, CodeCursorMismatch)
		})
	}
}

func TestCursorStrictlyDecodesEnvelopeAndState(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	codec := fixedCursorCodec(t, &now, 6)
	binding := testCursorBinding()
	token, err := codec.Encode(binding, map[string]any{"lastIdentity": "one", "unexpected": true})
	if err != nil {
		t.Fatal(err)
	}
	assertCursorError(t, codec.Decode(token, binding, &testCursorState{}), http.StatusBadRequest, CodeCursorInvalid)

	payload := cursorPayload{
		Version: cursorVersion, ExpiresAt: now.Add(CursorTTL).UnixMilli(),
		QueryHash: binding.QueryHash, Context: binding.Context, Scope: binding.Scope,
		Generation: binding.Generation, State: json.RawMessage(`{}`),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw[:len(raw)-1], []byte(`,"unknown":true}`)...)
	unknownEnvelope := codec.signPayloadForTest(raw)
	assertCursorError(t, codec.Decode(unknownEnvelope, binding, &testCursorState{}), http.StatusBadRequest, CodeCursorInvalid)
}

func TestCursorEncodeRejectsInvalidBindingAndOversizedState(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	codec := fixedCursorCodec(t, &now, 7)
	invalid := testCursorBinding()
	invalid.QueryHash = "not-a-sha256"
	if _, err := codec.Encode(invalid, testCursorState{}); err == nil {
		t.Fatal("invalid binding was accepted")
	}
	if _, err := codec.Encode(testCursorBinding(), map[string]string{"state": strings.Repeat("x", MaxCursorTokenBytes)}); err == nil {
		t.Fatal("oversized cursor was accepted")
	}
}

func fixedCursorCodec(t *testing.T, now *time.Time, marker byte) *CursorCodec {
	t.Helper()
	secret := make([]byte, cursorSecretBytes)
	for index := range secret {
		secret[index] = marker
	}
	codec, err := newCursorCodec(secret, func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	return codec
}

func testCursorBinding() CursorBinding {
	return CursorBinding{
		QueryHash:  HashCursorQuery("kind=pods&limit=100&order=asc"),
		Context:    "development",
		Scope:      "scope_42",
		Generation: "gen_42",
	}
}

func assertCursorError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.Status != status || string(httpError.AppError.Code) != code {
		t.Fatalf("error = %#v, want status=%d code=%s", err, status, code)
	}
	if httpError.AppError.Message == "" || httpError.Details != nil {
		t.Fatalf("cursor error is not sanitized: %#v", httpError)
	}
}

func differentBase64Byte(value byte) byte {
	if value == 'A' {
		return 'B'
	}
	return 'A'
}

func (codec *CursorCodec) signPayloadForTest(payload []byte) string {
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	signed := cursorTokenPrefix + "." + payloadPart
	return signed + "." + base64.RawURLEncoding.EncodeToString(codec.sign(signed))
}
