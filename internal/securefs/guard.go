// Package securefs provides small fail-closed filesystem primitives for
// private local state. A Guard keeps the validated object open so later reads
// and writes do not depend solely on a pathname that may have been replaced.
package securefs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

var ErrUnsafeObject = errors.New("unsafe filesystem object")

// Guard binds an open file to the identity observed at its pathname.
type Guard struct {
	file     *os.File
	path     string
	identity os.FileInfo
}

// OpenRegular opens an existing regular file without accepting a symlink or
// an object replaced between inspection and open.
func OpenRegular(path string, flag int) (*Guard, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if err := validateRegular(before); err != nil {
		return nil, err
	}
	file, err := platformOpenRegular(path, flag, 0)
	if err != nil {
		return nil, err
	}
	guard, err := guardOpenFile(file, path, before, true)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return guard, nil
}

// CreateExclusive creates one private regular file and fails if any object is
// already present at path.
func CreateExclusive(path string) (*Guard, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	guard, err := guardOpenFile(file, path, nil, false)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := guard.Protect(0o600); err != nil {
		_ = guard.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return guard, nil
}

// CreateTemp creates a private same-directory temporary file and returns it
// already guarded.
func CreateTemp(directory, pattern string) (*Guard, error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return nil, err
	}
	path := file.Name()
	guard, err := guardOpenFile(file, path, nil, false)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := guard.Protect(0o600); err != nil {
		_ = guard.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return guard, nil
}

func guardOpenFile(file *os.File, path string, expected os.FileInfo, requirePrivate bool) (*Guard, error) {
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := validateRegular(opened); err != nil {
		return nil, err
	}
	if expected != nil && !os.SameFile(expected, opened) {
		return nil, fmt.Errorf("%w: object changed before open", ErrUnsafeObject)
	}
	after, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if err := validateRegular(after); err != nil {
		return nil, err
	}
	if !os.SameFile(opened, after) {
		return nil, fmt.Errorf("%w: pathname and handle differ", ErrUnsafeObject)
	}
	if err := platformValidateHandle(file, false, requirePrivate); err != nil {
		return nil, err
	}
	return &Guard{file: file, path: path, identity: opened}, nil
}

func validateRegular(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: expected regular file", ErrUnsafeObject)
	}
	if err := platformValidateOwner(info, false); err != nil {
		return err
	}
	return nil
}

// File returns the still-open validated file.
func (guard *Guard) File() *os.File { return guard.file }

// Path returns the currently published pathname for the guarded object.
func (guard *Guard) Path() string { return guard.path }

// Validate proves that the open handle and current pathname still identify the
// original regular file.
func (guard *Guard) Validate() error {
	if guard == nil || guard.file == nil {
		return fmt.Errorf("%w: guard is closed", ErrUnsafeObject)
	}
	opened, err := guard.file.Stat()
	if err != nil {
		return err
	}
	if err := validateRegular(opened); err != nil {
		return err
	}
	if !os.SameFile(guard.identity, opened) {
		return fmt.Errorf("%w: open object identity changed", ErrUnsafeObject)
	}
	current, err := os.Lstat(guard.path)
	if err != nil {
		return err
	}
	if err := validateRegular(current); err != nil {
		return err
	}
	if !os.SameFile(opened, current) {
		return fmt.Errorf("%w: pathname was replaced", ErrUnsafeObject)
	}
	return platformValidateHandle(guard.file, false, true)
}

// Protect applies and verifies a private file mode where the platform exposes
// POSIX modes. Windows confidentiality is inherited from the protected parent
// DACL and is validated by the platform directory adapter.
func (guard *Guard) Protect(mode fs.FileMode) error {
	if err := platformProtectFile(guard.file, guard.path, mode); err != nil {
		return err
	}
	info, err := guard.file.Stat()
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != mode.Perm() {
		return fmt.Errorf("%w: private mode was not applied", ErrUnsafeObject)
	}
	return guard.Validate()
}

// ValidatePrivateFile opens an existing regular file without following a
// symlink and verifies the platform privacy boundary. Unix requires mode 0600;
// Windows accepts either a protected direct DACL or a safely inherited DACL,
// provided the sole full-control ACE belongs to the current token user.
func ValidatePrivateFile(path string) error {
	guard, err := OpenRegular(path, os.O_RDONLY)
	if err != nil {
		return err
	}
	defer guard.Close()
	info, err := guard.file.Stat()
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return fmt.Errorf("%w: file mode is not private", ErrUnsafeObject)
	}
	return guard.Validate()
}

// Same reports whether two guards refer to the same open filesystem object.
func Same(first, second *Guard) (bool, error) {
	if err := first.Validate(); err != nil {
		return false, err
	}
	if err := second.Validate(); err != nil {
		return false, err
	}
	firstInfo, err := first.file.Stat()
	if err != nil {
		return false, err
	}
	secondInfo, err := second.file.Stat()
	if err != nil {
		return false, err
	}
	return os.SameFile(firstInfo, secondInfo), nil
}

// PublishNoReplace atomically publishes a completed same-directory temporary
// file without ever replacing an existing destination. Hard-link publication
// makes existence checking part of the filesystem operation itself.
func (guard *Guard) PublishNoReplace(destination string) error {
	if err := guard.Validate(); err != nil {
		return err
	}
	if filepath.Clean(filepath.Dir(guard.path)) != filepath.Clean(filepath.Dir(destination)) {
		return fmt.Errorf("%w: publication must remain in one directory", ErrUnsafeObject)
	}
	if err := ValidatePrivateDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	if err := os.Link(guard.path, destination); err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			_ = os.Remove(destination)
		}
	}()
	destinationInfo, err := os.Lstat(destination)
	if err != nil {
		return err
	}
	openedInfo, err := guard.file.Stat()
	if err != nil {
		return err
	}
	if destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.Mode().IsRegular() || !os.SameFile(openedInfo, destinationInfo) {
		return fmt.Errorf("%w: published object differs from temporary", ErrUnsafeObject)
	}
	if err := os.Remove(guard.path); err != nil {
		return err
	}
	guard.path = destination
	published = true
	if err := guard.Validate(); err != nil {
		return err
	}
	return SyncDirectory(filepath.Dir(destination))
}

func (guard *Guard) Close() error {
	if guard == nil || guard.file == nil {
		return nil
	}
	err := guard.file.Close()
	guard.file = nil
	return err
}

// EnsurePrivateDirectory creates and protects the final directory component,
// then validates its identity through an open handle.
func EnsurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	directory, err := openDirectory(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := validateDirectoryHandle(directory, path, false); err != nil {
		return err
	}
	if err := platformProtectDirectory(directory, path, 0o700); err != nil {
		return err
	}
	return validateDirectoryHandle(directory, path, true)
}

// ValidatePrivateDirectory rejects a symlink, foreign owner, non-directory, or
// non-private Unix mode for the final directory component.
func ValidatePrivateDirectory(path string) error {
	directory, err := openDirectory(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return validateDirectoryHandle(directory, path, true)
}

func openDirectory(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, fmt.Errorf("%w: expected directory", ErrUnsafeObject)
	}
	directory, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := directory.Stat()
	if err != nil {
		_ = directory.Close()
		return nil, err
	}
	after, err := os.Lstat(path)
	if err != nil {
		_ = directory.Close()
		return nil, err
	}
	if !opened.IsDir() || after.Mode()&os.ModeSymlink != 0 || !after.IsDir() || !os.SameFile(before, opened) || !os.SameFile(opened, after) {
		_ = directory.Close()
		return nil, fmt.Errorf("%w: directory identity changed", ErrUnsafeObject)
	}
	return directory, nil
}

func validateDirectoryHandle(directory *os.File, path string, private bool) error {
	opened, err := directory.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !opened.IsDir() || current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(opened, current) {
		return fmt.Errorf("%w: directory pathname was replaced", ErrUnsafeObject)
	}
	if err := platformValidateOwner(opened, true); err != nil {
		return err
	}
	if private && runtime.GOOS != "windows" && opened.Mode().Perm() != 0o700 {
		return fmt.Errorf("%w: directory mode is not private", ErrUnsafeObject)
	}
	return platformValidateHandle(directory, true, private)
}

// SyncDirectory persists a same-directory publication where supported.
func SyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		if runtime.GOOS == "windows" || errors.Is(err, os.ErrInvalid) {
			return nil
		}
		return err
	}
	return nil
}
