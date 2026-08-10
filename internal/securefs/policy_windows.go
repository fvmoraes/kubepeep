//go:build windows

package securefs

import (
	"fmt"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type fileAttributeTagInfo struct {
	FileAttributes uint32
	ReparseTag     uint32
}

const fileAllAccess windows.ACCESS_MASK = 0x001f01ff

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
	user, err := currentTokenUserSID()
	if err != nil {
		return err
	}
	ace := "(A;;GA;;;" + user.String() + ")"
	if directory {
		ace = "(A;OICI;GA;;;" + user.String() + ")"
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P" + ace)
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}

func validatePrivateDACL(handle windows.Handle, directory, requireProtected bool) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PRESENT == 0 || requireProtected && control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("%w: DACL is absent or not protected", ErrUnsafeObject)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	if dacl == nil || dacl.AceCount != 1 {
		return fmt.Errorf("%w: DACL is not limited to one principal", ErrUnsafeObject)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		return err
	}
	if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		return fmt.Errorf("%w: DACL does not contain one allow ACE", ErrUnsafeObject)
	}
	user, err := currentTokenUserSID()
	if err != nil {
		return err
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !windows.EqualSid(user, aceSID) {
		return fmt.Errorf("%w: DACL principal is not the current token user", ErrUnsafeObject)
	}
	if ace.Mask != windows.GENERIC_ALL && ace.Mask != fileAllAccess {
		return fmt.Errorf("%w: DACL does not grant full control", ErrUnsafeObject)
	}
	flags := ace.Header.AceFlags
	if directory {
		wanted := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
		if control&windows.SE_DACL_PROTECTED == 0 || flags != wanted {
			return fmt.Errorf("%w: directory DACL is not protected and inheritable", ErrUnsafeObject)
		}
		return nil
	}
	if control&windows.SE_DACL_PROTECTED != 0 {
		if flags != 0 {
			return fmt.Errorf("%w: protected file ACE has unexpected inheritance flags", ErrUnsafeObject)
		}
		return nil
	}
	allowedInherited := uint8(windows.INHERITED_ACE | windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
	if flags&windows.INHERITED_ACE == 0 || flags & ^allowedInherited != 0 {
		return fmt.Errorf("%w: file DACL was neither protected nor safely inherited", ErrUnsafeObject)
	}
	return nil
}

func currentTokenUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("query current token user: %w", err)
	}
	if user == nil || user.User.Sid == nil {
		return nil, fmt.Errorf("%w: current token user SID is unavailable", ErrUnsafeObject)
	}
	return user.User.Sid, nil
}
