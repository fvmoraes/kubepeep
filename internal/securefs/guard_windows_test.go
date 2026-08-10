//go:build windows

package securefs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

type tokenOwnerInformation struct {
	Owner *windows.SID
}

func TestWindowsDACLUsesTokenUserAndRejectsTampering(t *testing.T) {
	directory := t.TempDir()
	if err := EnsurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(directory, "private.db")
	guard, err := CreateExclusive(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = guard.Close() })
	if err := validatePrivateDACL(windows.Handle(guard.File().Fd()), false, true); err != nil {
		t.Fatalf("created child DACL is not protected: %v", err)
	}

	user, err := currentTokenUserSID()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := currentTokenOwnerSID()
	if err != nil {
		t.Fatal(err)
	}
	sameOwner := windows.EqualSid(user, owner)
	if windows.GetCurrentProcessToken().IsElevated() && sameOwner {
		t.Fatal("elevated TOKEN_OWNER unexpectedly equals TokenUser; the test cannot prove the SID distinction")
	}

	if err := installBroadTestDACL(path, user); err != nil {
		t.Fatal(err)
	}
	if err := guard.Validate(); !errors.Is(err, ErrUnsafeObject) {
		t.Fatalf("Validate() after broad DACL = %v, want ErrUnsafeObject", err)
	}
	if err := guard.Protect(0o600); err != nil {
		t.Fatalf("restore private DACL: %v", err)
	}
	if err := ValidatePrivateRegular(path); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsRejectsReparsePoint(t *testing.T) {
	directory := t.TempDir()
	if err := EnsurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "target")
	guard, err := CreateExclusive(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("host does not permit an unprivileged reparse-point fixture: %v", err)
	}
	if _, err := OpenRegular(link, os.O_RDONLY); err == nil {
		t.Fatal("OpenRegular accepted a reparse point")
	}
}

func currentTokenOwnerSID() (*windows.SID, error) {
	token := windows.GetCurrentProcessToken()
	var size uint32
	_ = windows.GetTokenInformation(token, windows.TokenOwner, nil, 0, &size)
	if size == 0 {
		return nil, errors.New("TOKEN_OWNER size is unavailable")
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenOwner, &buffer[0], size, &size); err != nil {
		return nil, err
	}
	owner := (*tokenOwnerInformation)(unsafe.Pointer(&buffer[0])).Owner
	if owner == nil {
		return nil, errors.New("TOKEN_OWNER SID is unavailable")
	}
	return owner, nil
}

func installBroadTestDACL(path string, user *windows.SID) error {
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.String() + ")(A;;GR;;;WD)")
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}
