package runtime

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

var ErrLocked = errors.New("runtime: instance lock is held")

// FileLock owns the file handle that keeps the platform lock alive.
type FileLock struct {
	file *os.File
	once sync.Once
	err  error
}

// AcquireFileLock acquires an exclusive, non-blocking lock and keeps the
// underlying descriptor open until Close.
func AcquireFileLock(path string) (*FileLock, error) {
	if path == "" {
		return nil, errors.New("runtime: lock path is empty")
	}
	file, err := acquirePlatformLock(path)
	if err != nil {
		if errors.Is(err, ErrLocked) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("runtime: acquire instance lock: %w", err)
	}
	return &FileLock{file: file}, nil
}

// Close releases the lock exactly once.
func (lock *FileLock) Close() error {
	if lock == nil {
		return nil
	}
	lock.once.Do(func() {
		if lock.file == nil {
			return
		}
		unlockErr := releasePlatformLock(lock.file)
		closeErr := lock.file.Close()
		lock.err = errors.Join(unlockErr, closeErr)
	})
	return lock.err
}
