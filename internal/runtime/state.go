// Package runtime owns kubePeep's local process lifecycle and private control
// channel. PID values are descriptive and are never used as termination authority.
package runtime

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	StateSchemaVersion = 1
	ControlProtocol    = "kubepeep-control/v1"
	InstanceFileName   = "instance.json"
	LockFileName       = "kubePeep.lock"
	MaxStateBytes      = 64 << 10
)

var (
	ErrNotRunning   = errors.New("runtime: instance is not running")
	ErrUnsafeState  = errors.New("runtime: instance state is unsafe")
	ErrInvalidState = errors.New("runtime: instance state is invalid")
)

// InstanceStateV1 is the complete private runtime state. ControlToken must
// never be logged, printed or returned by the HTTP control channel.
type InstanceStateV1 struct {
	Schema       int    `json:"schema"`
	InstanceID   string `json:"instance_id"`
	PID          int    `json:"pid"`
	Fingerprint  string `json:"fingerprint"`
	Port         int    `json:"port"`
	Protocol     string `json:"protocol"`
	ControlToken string `json:"control_token"`
}

// String and LogValue prevent accidental disclosure when state is passed to
// fmt or structured logging. JSON persistence still uses the explicit fields.
func (state InstanceStateV1) String() string {
	return fmt.Sprintf("InstanceStateV1{schema=%d instance_id=%s pid=%d port=%d protocol=%s control_token=<redacted>}", state.Schema, state.InstanceID, state.PID, state.Port, state.Protocol)
}

func (state InstanceStateV1) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("schema", state.Schema),
		slog.String("instance_id", state.InstanceID),
		slog.Int("pid", state.PID),
		slog.Int("port", state.Port),
		slog.String("protocol", state.Protocol),
	)
}

// ControlIdentityDTO is the six-field proof returned by the internal channel.
type ControlIdentityDTO struct {
	Schema      int    `json:"schema"`
	InstanceID  string `json:"instance_id"`
	PID         int    `json:"pid"`
	Fingerprint string `json:"fingerprint"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
}

// Identity returns the public proof and deliberately omits the control token.
func (state InstanceStateV1) Identity() ControlIdentityDTO {
	return ControlIdentityDTO{
		Schema: state.Schema, InstanceID: state.InstanceID, PID: state.PID,
		Fingerprint: state.Fingerprint, Port: state.Port, Protocol: state.Protocol,
	}
}

// URL returns the published local browser URL.
func (identity ControlIdentityDTO) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/", identity.Port)
}

// NewInstanceState creates a validated state using 128 bits for the instance
// ID and 256 bits for the private control token.
func NewInstanceState(pid, port int, fingerprint string) (InstanceStateV1, error) {
	instanceEntropy, err := randomBytes(16)
	if err != nil {
		return InstanceStateV1{}, fmt.Errorf("runtime: generate instance ID: %w", err)
	}
	tokenEntropy, err := randomBytes(32)
	if err != nil {
		return InstanceStateV1{}, fmt.Errorf("runtime: generate control token: %w", err)
	}
	state := InstanceStateV1{
		Schema:       StateSchemaVersion,
		InstanceID:   "inst_" + base64.RawURLEncoding.EncodeToString(instanceEntropy),
		PID:          pid,
		Fingerprint:  fingerprint,
		Port:         port,
		Protocol:     ControlProtocol,
		ControlToken: base64.RawURLEncoding.EncodeToString(tokenEntropy),
	}
	if err := ValidateInstanceState(state); err != nil {
		return InstanceStateV1{}, err
	}
	return state, nil
}

func randomBytes(size int) ([]byte, error) {
	buffer := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return nil, err
	}
	return buffer, nil
}

// ValidateInstanceState enforces the exact version-one state contract.
func ValidateInstanceState(state InstanceStateV1) error {
	if state.Schema != StateSchemaVersion || state.Protocol != ControlProtocol {
		return ErrInvalidState
	}
	if state.PID <= 0 || state.Port < MinimumPort || state.Port > MaximumPort {
		return ErrInvalidState
	}
	if state.Fingerprint == "" || len(state.Fingerprint) > 256 {
		return ErrInvalidState
	}
	if !strings.HasPrefix(state.InstanceID, "inst_") || !validRawURLSecret(strings.TrimPrefix(state.InstanceID, "inst_"), 16) {
		return ErrInvalidState
	}
	if !validRawURLSecret(state.ControlToken, 32) {
		return ErrInvalidState
	}
	return nil
}

func validRawURLSecret(value string, expectedBytes int) bool {
	if value == "" || strings.Contains(value, "=") {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == expectedBytes && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func InstancePath(runtimeDirectory string) string {
	return filepath.Join(runtimeDirectory, InstanceFileName)
}

func LockPath(runtimeDirectory string) string {
	return filepath.Join(runtimeDirectory, LockFileName)
}

// EnsureRuntimeDirectory creates and validates the private runtime directory.
func EnsureRuntimeDirectory(runtimeDirectory string) error {
	if runtimeDirectory == "" {
		return errors.New("runtime: runtime directory is empty")
	}
	if err := os.MkdirAll(runtimeDirectory, 0o700); err != nil {
		return fmt.Errorf("runtime: create runtime directory: %w", err)
	}
	info, err := os.Lstat(runtimeDirectory)
	if err != nil {
		return fmt.Errorf("runtime: inspect runtime directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: runtime path is not a directory", ErrUnsafeState)
	}
	if err := protectRuntimeDirectory(runtimeDirectory); err != nil {
		return fmt.Errorf("runtime: protect runtime directory: %w", err)
	}
	return nil
}

func inspectRuntimeDirectory(runtimeDirectory string) error {
	info, err := os.Lstat(runtimeDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotRunning
		}
		return fmt.Errorf("runtime: inspect runtime directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: runtime path is not a trusted directory", ErrUnsafeState)
	}
	if err := validateRuntimeDirectory(runtimeDirectory, info); err != nil {
		return fmt.Errorf("%w: runtime directory is not private", ErrUnsafeState)
	}
	return nil
}

// WriteInstanceStateAtomic privately and durably replaces instance.json.
func WriteInstanceStateAtomic(runtimeDirectory string, state InstanceStateV1) error {
	if err := ValidateInstanceState(state); err != nil {
		return err
	}
	if err := EnsureRuntimeDirectory(runtimeDirectory); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("runtime: encode instance state: %w", err)
	}
	data = append(data, '\n')
	if len(data) > MaxStateBytes {
		return ErrInvalidState
	}

	temporary, err := os.CreateTemp(runtimeDirectory, ".instance-*.tmp")
	if err != nil {
		return fmt.Errorf("runtime: create temporary state: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("runtime: protect temporary state: %w", err)
	}
	if err := protectStateFile(temporaryPath, temporary); err != nil {
		return fmt.Errorf("runtime: protect temporary state: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("runtime: write temporary state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("runtime: sync temporary state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("runtime: close temporary state: %w", err)
	}
	target := InstancePath(runtimeDirectory)
	if err := validateReplacementTarget(target); err != nil {
		return err
	}
	if err := atomicReplace(temporaryPath, target); err != nil {
		return fmt.Errorf("runtime: publish instance state: %w", err)
	}
	removeTemporary = false
	if err := syncRuntimeDirectory(runtimeDirectory); err != nil {
		return fmt.Errorf("runtime: sync runtime directory: %w", err)
	}
	return nil
}

// LoadInstanceState reads and strictly validates private instance state.
func LoadInstanceState(runtimeDirectory string) (InstanceStateV1, error) {
	if err := inspectRuntimeDirectory(runtimeDirectory); err != nil {
		return InstanceStateV1{}, err
	}
	path := InstancePath(runtimeDirectory)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return InstanceStateV1{}, ErrNotRunning
		}
		return InstanceStateV1{}, fmt.Errorf("runtime: inspect instance state: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return InstanceStateV1{}, fmt.Errorf("%w: instance state is not a regular file", ErrUnsafeState)
	}
	file, err := openStateFile(path)
	if err != nil {
		return InstanceStateV1{}, fmt.Errorf("runtime: open instance state: %w", err)
	}
	defer file.Close()
	if err := validatePrivateState(path, file, info); err != nil {
		return InstanceStateV1{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxStateBytes+1))
	if err != nil {
		return InstanceStateV1{}, fmt.Errorf("runtime: read instance state: %w", err)
	}
	if len(data) > MaxStateBytes {
		return InstanceStateV1{}, ErrInvalidState
	}
	var state InstanceStateV1
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return InstanceStateV1{}, fmt.Errorf("%w: decode instance state", ErrInvalidState)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return InstanceStateV1{}, fmt.Errorf("%w: trailing JSON", ErrInvalidState)
	}
	if err := ValidateInstanceState(state); err != nil {
		return InstanceStateV1{}, err
	}
	return state, nil
}

// RemoveInstanceState removes only a regular, private instance state file.
func RemoveInstanceState(runtimeDirectory string) error {
	if err := inspectRuntimeDirectory(runtimeDirectory); err != nil {
		if errors.Is(err, ErrNotRunning) {
			return nil
		}
		return err
	}
	path := InstancePath(runtimeDirectory)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("runtime: inspect state before removal: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: refusing to remove non-regular state", ErrUnsafeState)
	}
	file, err := openStateFile(path)
	if err != nil {
		return fmt.Errorf("runtime: open state before removal: %w", err)
	}
	validationErr := validatePrivateState(path, file, info)
	closeErr := file.Close()
	if validationErr != nil {
		return validationErr
	}
	if closeErr != nil {
		return fmt.Errorf("runtime: close state before removal: %w", closeErr)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("runtime: remove instance state: %w", err)
	}
	if err := syncRuntimeDirectory(runtimeDirectory); err != nil {
		return fmt.Errorf("runtime: sync runtime directory: %w", err)
	}
	return nil
}

func validateReplacementTarget(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("runtime: inspect replacement target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: replacement target is not a regular file", ErrUnsafeState)
	}
	file, err := openStateFile(path)
	if err != nil {
		return fmt.Errorf("runtime: open replacement target: %w", err)
	}
	defer file.Close()
	if err := validatePrivateState(path, file, info); err != nil {
		return err
	}
	return nil
}
