//go:build windows

package userdirs

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func defaultRoot() (string, error) {
	localAppData, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return "", fmt.Errorf("user directories: resolve LocalAppData: %w", err)
	}
	if localAppData == "" {
		return "", fmt.Errorf("user directories: LocalAppData is empty")
	}
	return filepath.Join(localAppData, "kubePeep"), nil
}

func protectDirectory(path string) error {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.READ_CONTROL|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var information directoryAttributeTagInformation
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
	); err != nil {
		return err
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("directory is a reparse point")
	}

	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sid := tokenUser.User.Sid.String()
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;OICI;GA;;;" + sid + ")")
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		return err
	}

	actual, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	sddl := actual.String()
	actualDACL, _, err := actual.DACL()
	if err != nil {
		return err
	}
	if actualDACL == nil || actualDACL.AceCount != 1 || !strings.Contains(sddl, "D:P") ||
		!strings.Contains(sddl, "(A;OICI;") || !strings.Contains(sddl, ";;;"+sid+")") {
		return fmt.Errorf("directory DACL is not limited to the current user")
	}
	return nil
}

type directoryAttributeTagInformation struct {
	FileAttributes uint32
	ReparseTag     uint32
}
