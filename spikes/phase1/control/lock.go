package control

import (
	"errors"
	"sync"
)

// ErrLocked reports that another process currently holds the runtime lock.
var ErrLocked = errors.New("control: runtime lock is held")

// FileLock is an exclusive, process-scoped file lock.
//
// The operating-system lock remains held until Close is called or the process
// exits. The lock file itself may remain on disk after Close; its inode must not
// be replaced while another process could still be waiting on it.
type FileLock struct {
	handle platformLock
	once   sync.Once
	err    error
}

type platformLock interface {
	close() error
}

// AcquireFileLock acquires an exclusive non-blocking lock for path.
func AcquireFileLock(path string) (*FileLock, error) {
	handle, err := acquirePlatformLock(path)
	if err != nil {
		return nil, err
	}
	return &FileLock{handle: handle}, nil
}

// Close releases the lock. It is safe to call Close more than once.
func (l *FileLock) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.handle != nil {
			l.err = l.handle.close()
		}
	})
	return l.err
}
