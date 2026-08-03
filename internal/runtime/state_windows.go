//go:build windows

package runtime

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type fileAttributeTagInformation struct {
	FileAttributes uint32
	ReparseTag     uint32
}

func openStateFile(path string) (*os.File, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("could not own state handle")
	}
	return file, nil
}

func protectRuntimeDirectory(path string) error {
	handle, err := openDirectoryHandle(path, windows.READ_CONTROL|windows.WRITE_DAC)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	if err := rejectWindowsHandleReparsePoint(handle); err != nil {
		return err
	}
	if err := setCurrentUserDACLHandle(handle, true); err != nil {
		return err
	}
	return validateCurrentUserDACL(handle, true)
}

func validateRuntimeDirectory(path string, _ os.FileInfo) error {
	handle, err := openDirectoryHandle(path, windows.READ_CONTROL)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	if err := rejectWindowsHandleReparsePoint(handle); err != nil {
		return err
	}
	return validateCurrentUserDACL(handle, true)
}

func protectStateFile(path string, file *os.File) error {
	if err := rejectHandleReparsePoint(file); err != nil {
		return err
	}
	if err := setCurrentUserDACL(path, false); err != nil {
		return err
	}
	return validateCurrentUserDACL(windows.Handle(file.Fd()), false)
}

func validatePrivateState(_ string, file *os.File, _ os.FileInfo) error {
	if err := rejectHandleReparsePoint(file); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsafeState, err)
	}
	return validateCurrentUserDACL(windows.Handle(file.Fd()), false)
}

func rejectHandleReparsePoint(file *os.File) error {
	return rejectWindowsHandleReparsePoint(windows.Handle(file.Fd()))
}

func rejectWindowsHandleReparsePoint(handle windows.Handle) error {
	var information fileAttributeTagInformation
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
	); err != nil {
		return err
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("path is a reparse point")
	}
	return nil
}

func openDirectoryHandle(path string, access uint32) (windows.Handle, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(
		pathPointer,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
}

func setCurrentUserDACL(path string, directory bool) error {
	dacl, err := currentUserDACL(directory)
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

func setCurrentUserDACLHandle(handle windows.Handle, directory bool) error {
	dacl, err := currentUserDACL(directory)
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

func currentUserDACL(directory bool) (*windows.ACL, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sid := tokenUser.User.Sid.String()
	ace := "(A;;GA;;;" + sid + ")"
	if directory {
		ace = "(A;OICI;GA;;;" + sid + ")"
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P" + ace)
	if err != nil {
		return nil, err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return nil, err
	}
	return dacl, nil
}

func validateCurrentUserDACL(handle windows.Handle, directory bool) error {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("runtime: query current user SID: %w", err)
	}
	sid := tokenUser.User.Sid.String()
	descriptor, err := windows.GetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("runtime: inspect DACL: %w", err)
	}
	sddl := descriptor.String()
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("runtime: inspect DACL entries: %w", err)
	}
	wantedACE := "(A;;"
	if directory {
		wantedACE = "(A;OICI;"
	}
	if dacl == nil || dacl.AceCount != 1 || !strings.Contains(sddl, "D:P") || !strings.Contains(sddl, wantedACE) || !strings.Contains(sddl, ";;;"+sid+")") {
		return fmt.Errorf("%w: DACL is not limited to the current user", ErrUnsafeState)
	}
	return nil
}

func atomicReplace(source, target string) error {
	sourcePointer, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetPointer, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		sourcePointer,
		targetPointer,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

func syncRuntimeDirectory(string) error {
	// MoveFileEx with MOVEFILE_WRITE_THROUGH is the durable publication
	// primitive on Windows; directories cannot be fsynced through os.File.
	return nil
}
