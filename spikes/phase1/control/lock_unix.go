//go:build !windows

package control

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type unixFileLock struct {
	file *os.File
}

func acquirePlatformLock(path string) (platformLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("control: open lock file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("control: protect lock file: %w", err)
	}

	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("control: acquire flock: %w", err)
	}
	return &unixFileLock{file: file}, nil
}

func (l *unixFileLock) close() error {
	unlockErr := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
