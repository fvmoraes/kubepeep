package namespaces

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// ScopeWriteRequest is shared by create, update, and validate adapters. The
// Presence fields distinguish an omitted collection from an explicit empty
// collection without changing the documented JSON representation.
type ScopeWriteRequest struct {
	ClusterProfileID   int64
	Name               string
	Context            string
	Mode               ScopeMode
	Namespaces         []string
	NamespacesPresent  bool
	RawInput           string
	RawInputPresent    bool
	DefaultNamespace   *string
	Version            int64
	ExpectedGeneration string
}

type scopeWriteWire struct {
	ClusterProfileID   int64           `json:"clusterProfileId"`
	Name               string          `json:"name"`
	Context            string          `json:"context"`
	Mode               ScopeMode       `json:"mode"`
	Namespaces         json.RawMessage `json:"namespaces"`
	RawInput           json.RawMessage `json:"rawInput"`
	DefaultNamespace   json.RawMessage `json:"defaultNamespace"`
	Version            int64           `json:"version"`
	ExpectedGeneration string          `json:"expectedGeneration"`
}

func (request *ScopeWriteRequest) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire scopeWriteWire
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}

	*request = ScopeWriteRequest{
		ClusterProfileID:   wire.ClusterProfileID,
		Name:               wire.Name,
		Context:            wire.Context,
		Mode:               wire.Mode,
		Version:            wire.Version,
		ExpectedGeneration: wire.ExpectedGeneration,
	}
	if wire.Namespaces != nil {
		request.NamespacesPresent = true
		if bytes.Equal(bytes.TrimSpace(wire.Namespaces), []byte("null")) || json.Unmarshal(wire.Namespaces, &request.Namespaces) != nil {
			return fmt.Errorf("namespaces must be an array of strings")
		}
		if request.Namespaces == nil {
			return fmt.Errorf("namespaces must be an array of strings")
		}
	}
	if wire.RawInput != nil {
		request.RawInputPresent = true
		if bytes.Equal(bytes.TrimSpace(wire.RawInput), []byte("null")) || json.Unmarshal(wire.RawInput, &request.RawInput) != nil {
			return fmt.Errorf("rawInput must be a string")
		}
	}
	if wire.DefaultNamespace != nil && !bytes.Equal(bytes.TrimSpace(wire.DefaultNamespace), []byte("null")) {
		var value string
		if err := json.Unmarshal(wire.DefaultNamespace, &value); err != nil {
			return fmt.Errorf("defaultNamespace must be a string or null")
		}
		request.DefaultNamespace = &value
	}
	return nil
}

type ScopeDeleteRequest struct {
	Confirmed          bool   `json:"confirmed"`
	Version            int64  `json:"version"`
	ReplacementScopeID int64  `json:"replacementScopeId"`
	ExpectedGeneration string `json:"expectedGeneration"`
}

type ScopeSelectRequest struct {
	ExpectedGeneration string `json:"expectedGeneration"`
}

type ScopeDTO struct {
	ID               int64     `json:"id"`
	ClusterProfileID int64     `json:"clusterProfileId"`
	Name             string    `json:"name"`
	Context          string    `json:"context"`
	Mode             ScopeMode `json:"mode"`
	Namespaces       []string  `json:"namespaces"`
	DefaultNamespace *string   `json:"defaultNamespace"`
	Version          int64     `json:"version"`
	CreatedAt        string    `json:"createdAt"`
	UpdatedAt        string    `json:"updatedAt"`
}

func NewScopeDTO(scope Scope) ScopeDTO {
	items := make([]string, len(scope.Namespaces))
	copy(items, scope.Namespaces)
	return ScopeDTO{
		ID:               scope.ID,
		ClusterProfileID: scope.ClusterProfileID,
		Name:             scope.Name,
		Context:          scope.Context,
		Mode:             scope.Mode,
		Namespaces:       items,
		DefaultNamespace: copyStringPointer(scope.DefaultNamespace),
		Version:          scope.Version,
		CreatedAt:        scope.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:        scope.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func DecodeScopeWriteRequest(reader io.Reader) (ScopeWriteRequest, error) {
	return decodeStrictJSON[ScopeWriteRequest](reader)
}

func DecodeScopeDeleteRequest(reader io.Reader) (ScopeDeleteRequest, error) {
	return decodeStrictJSON[ScopeDeleteRequest](reader)
}

func DecodeScopeSelectRequest(reader io.Reader) (ScopeSelectRequest, error) {
	return decodeStrictJSON[ScopeSelectRequest](reader)
}

func decodeStrictJSON[T any](reader io.Reader) (T, error) {
	var zero T
	limited := io.LimitReader(reader, MaxScopeBodyBytes+1)
	contents, err := io.ReadAll(limited)
	if err != nil {
		return zero, fmt.Errorf("%w", ErrInvalidJSON)
	}
	if len(contents) > MaxScopeBodyBytes {
		return zero, ErrBodyTooLarge
	}
	if len(bytes.TrimSpace(contents)) == 0 {
		return zero, ErrInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var decoded T
	if err := decoder.Decode(&decoded); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return zero, ErrUnknownField
		}
		return zero, ErrInvalidJSON
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return zero, ErrInvalidJSON
	}
	return decoded, nil
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
