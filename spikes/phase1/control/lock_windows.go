//go:build windows

package control

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

type windowsFileLock struct {
	file *os.File
}

func acquirePlatformLock(path string) (platformLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("control: open lock file: %w", err)
	}
	if err := protectWindowsPath(path, windows.NO_INHERITANCE, false); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("control: protect lock file: %w", err)
	}

	var overlapped windows.Overlapped
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
			errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("control: acquire LockFileEx: %w", err)
	}
	return &windowsFileLock{file: file}, nil
}

func (l *windowsFileLock) close() error {
	var overlapped windows.Overlapped
	unlockErr := windows.UnlockFileEx(
		windows.Handle(l.file.Fd()),
		0,
		1,
		0,
		&overlapped,
	)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
