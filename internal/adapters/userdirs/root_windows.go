//go:build windows

package userdirs

import (
	"fmt"
	"path/filepath"
	"unsafe"

	"github.com/fvmoraes/kubepeep/internal/winacl"
	"golang.org/x/sys/windows"
)

func defaultRoot() (string, error) {
	localAppData, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_CREATE)
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

	if err := winacl.SetHandle(handle, true); err != nil {
		return err
	}
	if err := winacl.Validate(handle, true, true); err != nil {
		return fmt.Errorf("directory DACL is not limited to the current user: %w", err)
	}
	return nil
}

type directoryAttributeTagInformation struct {
	FileAttributes uint32
	ReparseTag     uint32
}
