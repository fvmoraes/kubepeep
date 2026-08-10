//go:build windows

package securefs

import (
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func platformOpenRegular(path string, flag int, _ fs.FileMode) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(0)
	switch flag & (os.O_RDONLY | os.O_WRONLY | os.O_RDWR) {
	case os.O_WRONLY:
		access = windows.GENERIC_WRITE
	case os.O_RDWR:
		access = windows.GENERIC_READ | windows.GENERIC_WRITE
	default:
		access = windows.GENERIC_READ
	}
	if flag&os.O_APPEND != 0 {
		access &^= windows.GENERIC_WRITE
		access |= windows.FILE_APPEND_DATA | windows.FILE_WRITE_ATTRIBUTES | windows.SYNCHRONIZE
	}
	creation := uint32(windows.OPEN_EXISTING)
	switch {
	case flag&os.O_CREATE != 0 && flag&os.O_EXCL != 0:
		creation = windows.CREATE_NEW
	case flag&os.O_CREATE != 0 && flag&os.O_TRUNC != 0:
		creation = windows.CREATE_ALWAYS
	case flag&os.O_CREATE != 0:
		creation = windows.OPEN_ALWAYS
	case flag&os.O_TRUNC != 0:
		creation = windows.TRUNCATE_EXISTING
	}
	handle, err := windows.CreateFile(
		name,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		creation,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("securefs: could not own regular file handle")
	}
	return file, nil
}

// os.CreateTemp opens Windows files without FILE_SHARE_DELETE. Reopen the
// freshly created object with the securefs sharing policy and prove that its
// identity did not change across that transition.
func platformCreateTemp(directory, pattern string) (*os.File, error) {
	temporary, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return nil, err
	}
	path := temporary.Name()
	expected, statErr := temporary.Stat()
	closeErr := temporary.Close()
	if statErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if statErr != nil {
			return nil, statErr
		}
		return nil, closeErr
	}
	reopened, err := platformOpenRegular(path, os.O_RDWR, 0o600)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	actual, err := reopened.Stat()
	if err != nil || !os.SameFile(expected, actual) {
		_ = reopened.Close()
		_ = os.Remove(path)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: temporary file changed before secure reopen", ErrUnsafeObject)
	}
	return reopened, nil
}
