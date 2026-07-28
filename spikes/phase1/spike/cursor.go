package spike

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrCursorInvalid = errors.New("cursor is invalid")
	ErrCursorExpired = errors.New("cursor has expired")
	ErrCursorQuery   = errors.New("cursor does not belong to this query")
	ErrCursorContext = errors.New("cursor does not belong to this context generation")
	ErrCursorState   = errors.New("cursor contains invalid continuation state")
)

type CursorPosition struct {
	Namespace string `json:"namespace"`
	APIGroup  string `json:"apiGroup"`
	Kind      string `json:"kind"`
	Continue  string `json:"continue"`
}

type Cursor struct {
	Version           int              `json:"version"`
	ContextGeneration string           `json:"contextGeneration"`
	QueryHash         string           `json:"queryHash"`
	Positions         []CursorPosition `json:"positions"`
	ExpiresAt         time.Time        `json:"expiresAt"`
}

type CursorCodec struct {
	key []byte
	now func() time.Time
}

func NewCursorCodec(key []byte, now func() time.Time) (*CursorCodec, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("cursor signing key must contain at least 32 bytes")
	}
	if now == nil {
		now = time.Now
	}
	return &CursorCodec{key: append([]byte(nil), key...), now: now}, nil
}

func (c *CursorCodec) Encode(cursor Cursor) (string, error) {
	normalized, err := normalizeCursor(cursor)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal cursor: %w", err)
	}
	signature := c.sign(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(signature), nil
}

func (c *CursorCodec) Decode(token, contextGeneration, queryHash string) (Cursor, error) {
	var cursor Cursor

	parts := splitToken(token)
	if len(parts) != 2 {
		return cursor, ErrCursorInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return cursor, ErrCursorInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, c.sign(payload)) {
		return cursor, ErrCursorInvalid
	}
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return cursor, ErrCursorInvalid
	}
	cursor, err = normalizeCursor(cursor)
	if err != nil {
		return cursor, ErrCursorInvalid
	}
	if !cursor.ExpiresAt.After(c.now()) {
		return cursor, ErrCursorExpired
	}
	if cursor.ContextGeneration != contextGeneration {
		return cursor, ErrCursorContext
	}
	if cursor.QueryHash != queryHash {
		return cursor, ErrCursorQuery
	}
	return cursor, nil
}

func normalizeCursor(cursor Cursor) (Cursor, error) {
	if cursor.Version == 0 {
		cursor.Version = 1
	}
	if cursor.Version != 1 ||
		strings.TrimSpace(cursor.ContextGeneration) == "" ||
		strings.TrimSpace(cursor.QueryHash) == "" ||
		cursor.ExpiresAt.IsZero() ||
		len(cursor.Positions) == 0 {
		return Cursor{}, ErrCursorState
	}

	cursor.Positions = append([]CursorPosition(nil), cursor.Positions...)
	sort.Slice(cursor.Positions, func(i, j int) bool {
		left := cursor.Positions[i]
		right := cursor.Positions[j]
		if left.Namespace != right.Namespace {
			return left.Namespace < right.Namespace
		}
		if left.APIGroup != right.APIGroup {
			return left.APIGroup < right.APIGroup
		}
		return left.Kind < right.Kind
	})

	var previous string
	for index, position := range cursor.Positions {
		if strings.TrimSpace(position.Namespace) == "" ||
			strings.TrimSpace(position.Kind) == "" ||
			strings.TrimSpace(position.Continue) == "" {
			return Cursor{}, ErrCursorState
		}
		key := position.Namespace + "\x00" + position.APIGroup + "\x00" + position.Kind
		if index > 0 && key == previous {
			return Cursor{}, ErrCursorState
		}
		previous = key
	}
	return cursor, nil
}

func (c *CursorCodec) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func splitToken(token string) []string {
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			return []string{token[:i], token[i+1:]}
		}
	}
	return nil
}
