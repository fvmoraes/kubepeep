package control

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func testInstance(t *testing.T, port int) Instance {
	t.Helper()
	fingerprint, err := CurrentProcessFingerprint()
	if err != nil {
		t.Fatalf("current process fingerprint: %v", err)
	}
	return Instance{
		Schema:      SchemaVersion,
		InstanceID:  "instance-0123456789abcdef0123456789abcdef",
		Token:       "token-0123456789abcdef0123456789abcdef",
		PID:         os.Getpid(),
		Fingerprint: fingerprint,
		Port:        port,
		Protocol:    ProtocolVersion,
	}
}

func TestInstanceStateRoundTripAndPermissions(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	instance := testInstance(t, 2748)
	if err := WriteInstanceAtomic(runtimeDir, instance); err != nil {
		t.Fatalf("write state: %v", err)
	}

	got, err := LoadInstance(runtimeDir)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if !reflect.DeepEqual(got, instance) {
		t.Fatalf("loaded state = %#v, want %#v", got, instance)
	}
	if got.Public().InstanceID != instance.InstanceID {
		t.Fatalf("public identity = %#v", got.Public())
	}

	if runtime.GOOS != "windows" {
		dirInfo, err := os.Stat(runtimeDir)
		if err != nil {
			t.Fatalf("stat runtime directory: %v", err)
		}
		if got := dirInfo.Mode().Perm(); got != 0o700 {
			t.Fatalf("runtime permissions = %04o, want 0700", got)
		}
		stateInfo, err := os.Stat(InstancePath(runtimeDir))
		if err != nil {
			t.Fatalf("stat state: %v", err)
		}
		if got := stateInfo.Mode().Perm(); got != 0o600 {
			t.Fatalf("state permissions = %04o, want 0600", got)
		}
	}
}

func TestLoadInstanceRejectsUnknownFieldsAndInsecureUnixMode(t *testing.T) {
	runtimeDir := t.TempDir()
	path := InstancePath(runtimeDir)
	data := []byte(`{"schema":1,"instance_id":"instance-0123456789abcdef0123456789abcdef","token":"token-0123456789abcdef0123456789abcdef","pid":1,"fingerprint":"fingerprint-0123456789abcdef0123456789abcdef","port":2748,"protocol":"f1-control/v1","unexpected":true}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write invalid state: %v", err)
	}
	if _, err := LoadInstance(runtimeDir); err == nil {
		t.Fatal("LoadInstance accepted unknown JSON field")
	}

	if runtime.GOOS != "windows" {
		instance := testInstance(t, 2748)
		if err := WriteInstanceAtomic(runtimeDir, instance); err != nil {
			t.Fatalf("replace state: %v", err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("make state insecure: %v", err)
		}
		if _, err := LoadInstance(runtimeDir); err == nil {
			t.Fatal("LoadInstance accepted world-readable state")
		}
	}
}
