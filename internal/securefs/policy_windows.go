//go:build windows

package securefs

import (
	"fmt"
	"io/fs"
	"os"
	"unsafe"

	"github.com/fvmoraes/kubepeep/internal/winacl"
	"golang.org/x/sys/windows"
)

type fileAttributeTagInfo struct {
	FileAttributes uint32
	ReparseTag     uint32
}

// Ownership and privacy are validated from the open handle below. FileInfo on
// Windows does not expose either the token SID or the effective DACL.
func platformValidateOwner(os.FileInfo, bool) error { return nil }

func platformValidateHandle(file *os.File, directory, private bool) error {
	if err := rejectReparsePoint(file); err != nil {
		return err
	}
	if !private {
		return nil
	}
	return validatePrivateDACL(windows.Handle(file.Fd()), directory, directory)
}

func platformProtectFile(file *os.File, path string, _ fs.FileMode) error {
	secured, err := reopenSecurityHandle(file, path, false)
	if err != nil {
		return err
	}
	defer secured.Close()
	if err := setPrivateDACL(windows.Handle(secured.Fd()), false); err != nil {
		return err
	}
	return validatePrivateDACL(windows.Handle(secured.Fd()), false, true)
}

func platformProtectDirectory(directory *os.File, path string, _ fs.FileMode) error {
	secured, err := reopenSecurityHandle(directory, path, true)
	if err != nil {
		return err
	}
	defer secured.Close()
	if err := setPrivateDACL(windows.Handle(secured.Fd()), true); err != nil {
		return err
	}
	return validatePrivateDACL(windows.Handle(secured.Fd()), true, true)
}

func reopenSecurityHandle(original *os.File, path string, directory bool) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	handle, err := windows.CreateFile(
		name,
		windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		flags,
		0,
	)
	if err != nil {
		return nil, err
	}
	secured := os.NewFile(uintptr(handle), path)
	if secured == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("%w: could not own security handle", ErrUnsafeObject)
	}
	originalInfo, err := original.Stat()
	if err != nil {
		_ = secured.Close()
		return nil, err
	}
	securedInfo, err := secured.Stat()
	if err != nil {
		_ = secured.Close()
		return nil, err
	}
	if !os.SameFile(originalInfo, securedInfo) {
		_ = secured.Close()
		return nil, fmt.Errorf("%w: security handle refers to a different object", ErrUnsafeObject)
	}
	if err := rejectReparsePoint(secured); err != nil {
		_ = secured.Close()
		return nil, err
	}
	return secured, nil
}

func rejectReparsePoint(file *os.File) error {
	var info fileAttributeTagInfo
	if err := windows.GetFileInformationByHandleEx(
		windows.Handle(file.Fd()),
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%w: reparse point is not allowed", ErrUnsafeObject)
	}
	return nil
}

func setPrivateDACL(handle windows.Handle, directory bool) error {
	return winacl.SetHandle(handle, directory)
}

func validatePrivateDACL(handle windows.Handle, directory, requireProtected bool) error {
	if err := winacl.Validate(handle, directory, requireProtected); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsafeObject, err)
	}
	return nil
}

func currentTokenUserSID() (*windows.SID, error) {
	user, err := winacl.CurrentUserSID()
	if err != nil {
		return nil, fmt.Errorf("query current token user: %w", err)
	}
	return user, nil
}
