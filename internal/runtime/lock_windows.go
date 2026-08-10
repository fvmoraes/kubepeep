//go:build windows

package runtime

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func acquirePlatformLock(path string) (*os.File, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("could not own lock handle")
	}
	if err := rejectHandleReparsePoint(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := setCurrentUserDACLHandle(handle, false); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := validateCurrentUserDACL(handle, false, true); err != nil {
		_ = file.Close()
		return nil, err
	}
	var overlapped windows.Overlapped
	err = windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return file, nil
}

func releasePlatformLock(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}
