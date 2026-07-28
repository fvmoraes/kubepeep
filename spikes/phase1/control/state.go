package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// SchemaVersion versions the on-disk instance contract.
	SchemaVersion = 1
	// ProtocolVersion versions the authenticated loopback control protocol.
	ProtocolVersion = "f1-control/v1"

	instanceFileName = "instance.json"
	lockFileName     = "kubePeep.lock"
	maxStateBytes    = 64 << 10
)

var (
	// ErrNotRunning reports that no live, verified instance is available.
	ErrNotRunning = errors.New("control: instance is not running")
	// ErrIdentityMismatch reports that state and authenticated endpoint disagree.
	ErrIdentityMismatch = errors.New("control: instance identity mismatch")
)

// Instance is the private, per-user state required to authenticate a control
// request. Token must never be printed or returned by the HTTP endpoint.
type Instance struct {
	Schema      int    `json:"schema"`
	InstanceID  string `json:"instance_id"`
	Token       string `json:"token"`
	PID         int    `json:"pid"`
	Fingerprint string `json:"fingerprint"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
}

// PublicInstance is the identity proof returned by status/stop.
type PublicInstance struct {
	Schema      int    `json:"schema"`
	InstanceID  string `json:"instance_id"`
	PID         int    `json:"pid"`
	Fingerprint string `json:"fingerprint"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
}

// Public removes the control token from an instance.
func (i Instance) Public() PublicInstance {
	return PublicInstance{
		Schema:      i.Schema,
		InstanceID:  i.InstanceID,
		PID:         i.PID,
		Fingerprint: i.Fingerprint,
		Port:        i.Port,
		Protocol:    i.Protocol,
	}
}

// InstancePath returns the private state file path for runtimeDir.
func InstancePath(runtimeDir string) string {
	return filepath.Join(runtimeDir, instanceFileName)
}

// LockPath returns the stable lock file path for runtimeDir.
func LockPath(runtimeDir string) string {
	return filepath.Join(runtimeDir, lockFileName)
}

// EnsureRuntimeDir creates and validates an isolated runtime directory.
func EnsureRuntimeDir(runtimeDir string) error {
	if strings.TrimSpace(runtimeDir) == "" {
		return errors.New("control: runtime directory is required")
	}
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return fmt.Errorf("control: create runtime directory: %w", err)
	}
	info, err := os.Lstat(runtimeDir)
	if err != nil {
		return fmt.Errorf("control: inspect runtime directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("control: runtime path must be a real directory")
	}
	if err := protectRuntimeDir(runtimeDir, info); err != nil {
		return err
	}
	return nil
}

// WriteInstanceAtomic durably replaces instance.json.
func WriteInstanceAtomic(runtimeDir string, instance Instance) error {
	if err := validateInstance(instance); err != nil {
		return err
	}
	if err := EnsureRuntimeDir(runtimeDir); err != nil {
		return err
	}

	data, err := json.Marshal(instance)
	if err != nil {
		return fmt.Errorf("control: encode instance state: %w", err)
	}
	data = append(data, '\n')

	temp, err := os.CreateTemp(runtimeDir, ".instance-*.tmp")
	if err != nil {
		return fmt.Errorf("control: create temporary state: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("control: protect temporary state: %w", err)
	}
	if err := protectTemporaryState(tempPath); err != nil {
		return fmt.Errorf("control: protect temporary state: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("control: write temporary state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("control: sync temporary state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("control: close temporary state: %w", err)
	}

	target := InstancePath(runtimeDir)
	if err := atomicReplace(tempPath, target); err != nil {
		return fmt.Errorf("control: publish instance state: %w", err)
	}
	removeTemp = false
	if err := syncRuntimeDir(runtimeDir); err != nil {
		return fmt.Errorf("control: sync runtime directory: %w", err)
	}
	return nil
}

// LoadInstance reads and strictly validates private instance state.
func LoadInstance(runtimeDir string) (Instance, error) {
	path := InstancePath(runtimeDir)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Instance{}, ErrNotRunning
		}
		return Instance{}, fmt.Errorf("control: inspect instance state: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Instance{}, errors.New("control: instance state must be a regular file")
	}
	if err := validatePrivateState(path, info); err != nil {
		return Instance{}, err
	}

	file, err := os.Open(path)
	if err != nil {
		return Instance{}, fmt.Errorf("control: open instance state: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, maxStateBytes))
	decoder.DisallowUnknownFields()
	var instance Instance
	if err := decoder.Decode(&instance); err != nil {
		return Instance{}, fmt.Errorf("control: decode instance state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Instance{}, errors.New("control: instance state contains trailing JSON")
		}
		return Instance{}, fmt.Errorf("control: decode trailing state: %w", err)
	}
	if err := validateInstance(instance); err != nil {
		return Instance{}, err
	}
	return instance, nil
}

// RemoveInstance removes the transient identity file. A missing file is fine.
func RemoveInstance(runtimeDir string) error {
	err := os.Remove(InstancePath(runtimeDir))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("control: remove instance state: %w", err)
}

func validateInstance(instance Instance) error {
	switch {
	case instance.Schema != SchemaVersion:
		return fmt.Errorf("control: unsupported instance schema %d", instance.Schema)
	case len(instance.InstanceID) < 32:
		return errors.New("control: invalid instance ID")
	case len(instance.Token) < 32:
		return errors.New("control: invalid control token")
	case instance.PID <= 0:
		return errors.New("control: invalid PID")
	case len(instance.Fingerprint) < 32:
		return errors.New("control: invalid process fingerprint")
	case instance.Port < 1 || instance.Port > 65535:
		return errors.New("control: invalid control port")
	case instance.Protocol != ProtocolVersion:
		return fmt.Errorf("control: unsupported protocol %q", instance.Protocol)
	default:
		return nil
	}
}
