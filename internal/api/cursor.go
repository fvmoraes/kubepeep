package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	CursorTTL           = 5 * time.Minute
	MaxCursorTokenBytes = 16 << 10
	cursorTokenPrefix   = "kpc1"
	cursorVersion       = 1
	cursorSecretBytes   = 32
)

// CursorBinding identifies the immutable request context to which a cursor is
// bound. Context and Scope may be empty when the corresponding route has no
// selected value, but they are always authenticated as part of the token.
type CursorBinding struct {
	QueryHash  string
	Context    string
	Scope      string
	Generation string
}

// CursorCodec signs opaque, process-local cursors. Its secret is deliberately
// ephemeral and cannot be serialized or supplied through configuration.
type CursorCodec struct {
	secret [cursorSecretBytes]byte
	now    func() time.Time
}

type cursorPayload struct {
	Version    int             `json:"version"`
	ExpiresAt  int64           `json:"expiresAt"`
	QueryHash  string          `json:"queryHash"`
	Context    string          `json:"context"`
	Scope      string          `json:"scope"`
	Generation string          `json:"generation"`
	State      json.RawMessage `json:"state"`
}

// NewCursorCodec creates one codec for the lifetime of the current process.
// A restart therefore invalidates every cursor from the previous instance.
func NewCursorCodec() (*CursorCodec, error) {
	var secret [cursorSecretBytes]byte
	if _, err := io.ReadFull(rand.Reader, secret[:]); err != nil {
		return nil, fmt.Errorf("api: generate cursor secret: %w", err)
	}
	return newCursorCodec(secret[:], time.Now)
}

func newCursorCodec(secret []byte, now func() time.Time) (*CursorCodec, error) {
	if len(secret) != cursorSecretBytes {
		return nil, errors.New("api: cursor secret must contain 32 bytes")
	}
	if now == nil {
		return nil, errors.New("api: cursor clock is required")
	}
	codec := &CursorCodec{now: now}
	copy(codec.secret[:], secret)
	return codec, nil
}

// HashCursorQuery hashes a route-owned canonical query. The raw query never
// enters the cursor, logs, or public errors.
func HashCursorQuery(canonicalQuery string) string {
	digest := sha256.Sum256([]byte(canonicalQuery))
	return hex.EncodeToString(digest[:])
}

// Encode creates a cursor with a fresh, non-sliding five-minute lifetime.
// State is normalized before signing so equivalent maps produce the same
// canonical payload regardless of insertion order.
func (codec *CursorCodec) Encode(binding CursorBinding, state any) (string, error) {
	if err := validateCursorBinding(binding); err != nil {
		return "", err
	}
	canonicalState, err := canonicalCursorJSON(state)
	if err != nil {
		return "", fmt.Errorf("api: encode cursor state: %w", err)
	}
	payload := cursorPayload{
		Version:    cursorVersion,
		ExpiresAt:  codec.now().UTC().Add(CursorTTL).UnixMilli(),
		QueryHash:  binding.QueryHash,
		Context:    binding.Context,
		Scope:      binding.Scope,
		Generation: binding.Generation,
		State:      canonicalState,
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("api: encode cursor payload: %w", err)
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(encodedPayload)
	signed := cursorTokenPrefix + "." + payloadPart
	signature := codec.sign(signed)
	token := signed + "." + base64.RawURLEncoding.EncodeToString(signature)
	if len(token) > MaxCursorTokenBytes {
		return "", errors.New("api: cursor exceeds 16 KiB")
	}
	return token, nil
}

// Decode authenticates a cursor, enforces its original binding and strictly
// decodes its route-owned state. Token details and causes never reach clients.
func (codec *CursorCodec) Decode(token string, expected CursorBinding, destination any) error {
	if codec == nil || codec.now == nil || destination == nil || validateCursorBinding(expected) != nil {
		return invalidCursor(nil)
	}
	if token == "" || len(token) > MaxCursorTokenBytes {
		return invalidCursor(nil)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != cursorTokenPrefix {
		return invalidCursor(nil)
	}
	payloadBytes, err := decodeCanonicalBase64(parts[1])
	if err != nil {
		return invalidCursor(err)
	}
	signature, err := decodeCanonicalBase64(parts[2])
	if err != nil || len(signature) != sha256.Size {
		return invalidCursor(err)
	}
	signed := parts[0] + "." + parts[1]
	if !hmac.Equal(signature, codec.sign(signed)) {
		return invalidCursor(nil)
	}

	var payload cursorPayload
	if err := decodeCursorJSON(payloadBytes, &payload); err != nil {
		return invalidCursor(err)
	}
	if payload.Version != cursorVersion || payload.ExpiresAt <= 0 ||
		validateCursorBinding(CursorBinding{
			QueryHash: payload.QueryHash, Context: payload.Context,
			Scope: payload.Scope, Generation: payload.Generation,
		}) != nil || len(payload.State) == 0 {
		return invalidCursor(nil)
	}
	expiresAt := time.UnixMilli(payload.ExpiresAt)
	if !codec.now().Before(expiresAt) {
		return NewHTTPError(http.StatusGone, CodeCursorExpired, "The cursor has expired.", nil, nil)
	}
	actual := CursorBinding{
		QueryHash: payload.QueryHash, Context: payload.Context,
		Scope: payload.Scope, Generation: payload.Generation,
	}
	if actual != expected {
		return NewHTTPError(http.StatusBadRequest, CodeCursorMismatch, "The cursor does not match the current query or selection.", nil, nil)
	}
	if err := decodeCursorJSON(payload.State, destination); err != nil {
		return invalidCursor(err)
	}
	return nil
}

func (codec *CursorCodec) sign(value string) []byte {
	mac := hmac.New(sha256.New, codec.secret[:])
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func validateCursorBinding(binding CursorBinding) error {
	if len(binding.QueryHash) != sha256.Size*2 {
		return errors.New("api: cursor query hash is invalid")
	}
	digest, err := hex.DecodeString(binding.QueryHash)
	if err != nil || hex.EncodeToString(digest) != binding.QueryHash {
		return errors.New("api: cursor query hash is not canonical")
	}
	if len(binding.Context) > 1024 || len(binding.Scope) > 1024 ||
		binding.Generation == "" || len(binding.Generation) > 256 {
		return errors.New("api: cursor binding is invalid")
	}
	return nil
}

func canonicalCursorJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("cursor state contains trailing JSON")
	}
	return json.Marshal(normalized)
}

func decodeCursorJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("trailing JSON")
	}
	return nil
}

func decodeCanonicalBase64(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return nil, err
	}
	if base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, errors.New("non-canonical base64url")
	}
	return decoded, nil
}

func invalidCursor(cause error) error {
	return NewHTTPError(http.StatusBadRequest, CodeCursorInvalid, "The cursor is invalid.", nil, cause)
}
