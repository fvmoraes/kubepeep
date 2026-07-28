//go:build windows

package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsPrivateDACLAndTamperRejection(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := EnsureRuntimeDir(runtimeDir); err != nil {
		t.Fatalf("ensure runtime directory: %v", err)
	}
	if err := validateWindowsPrivateDACL(runtimeDir, true); err != nil {
		t.Fatalf("validate runtime directory DACL: %v", err)
	}

	inheritedPath := filepath.Join(runtimeDir, "inherited.tmp")
	if err := os.WriteFile(inheritedPath, []byte("inheritance probe"), 0o600); err != nil {
		t.Fatalf("create inherited child: %v", err)
	}
	if err := validateWindowsInheritedDACL(inheritedPath); err != nil {
		t.Fatalf("validate inherited child DACL: %v", err)
	}
	if err := os.Remove(inheritedPath); err != nil {
		t.Fatalf("remove inherited child: %v", err)
	}

	lock, err := AcquireFileLock(LockPath(runtimeDir))
	if err != nil {
		t.Fatalf("acquire protected lock: %v", err)
	}
	if err := validateWindowsPrivateDACL(LockPath(runtimeDir), false); err != nil {
		_ = lock.Close()
		t.Fatalf("validate lock DACL: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("release protected lock: %v", err)
	}

	instance := testInstance(t, 43210)
	if err := WriteInstanceAtomic(runtimeDir, instance); err != nil {
		t.Fatalf("write instance: %v", err)
	}
	path := InstancePath(runtimeDir)
	if err := validateWindowsPrivateDACL(path, false); err != nil {
		t.Fatalf("validate instance DACL: %v", err)
	}
	if _, err := LoadInstance(runtimeDir); err != nil {
		t.Fatalf("load private instance: %v", err)
	}

	replacement := instance
	replacement.InstanceID = "replacement-0123456789abcdef0123456789abcdef"
	replacement.Token = "replacement-token-0123456789abcdef0123456789abcdef"
	if err := WriteInstanceAtomic(runtimeDir, replacement); err != nil {
		t.Fatalf("replace instance: %v", err)
	}
	loaded, err := LoadInstance(runtimeDir)
	if err != nil {
		t.Fatalf("load replaced instance: %v", err)
	}
	if loaded != replacement {
		t.Fatalf("replaced instance = %#v; want %#v", loaded, replacement)
	}

	worldSID, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		t.Fatalf("create Everyone SID: %v", err)
	}
	publicACL, err := windows.ACLFromEntries(
		[]windows.EXPLICIT_ACCESS{{
			AccessPermissions: windows.GENERIC_READ,
			AccessMode:        windows.SET_ACCESS,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(worldSID),
			},
		}},
		nil,
	)
	if err != nil {
		t.Fatalf("create public DACL: %v", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|
			windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		publicACL,
		nil,
	); err != nil {
		t.Fatalf("tamper instance DACL: %v", err)
	}
	t.Cleanup(func() {
		if err := protectWindowsPath(path, windows.NO_INHERITANCE, false); err != nil {
			t.Errorf("restore private instance DACL: %v", err)
		}
	})

	if err := validateWindowsPrivateDACL(path, false); err == nil {
		t.Fatal("expected public DACL to be rejected")
	}
	if _, err := LoadInstance(runtimeDir); err == nil ||
		!strings.Contains(err.Error(), "insecure Windows instance permissions") {
		t.Fatalf("load with public DACL error = %v; want private-permission rejection", err)
	}
}
