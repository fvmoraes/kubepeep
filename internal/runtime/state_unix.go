//go:build !windows

package runtime

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func openStateFile(path string) (*os.File, error) {
	return os.Open(path)
}

func protectRuntimeDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	return validateUnixOwnerAndMode(info, 0o700)
}

func validateRuntimeDirectory(_ string, info os.FileInfo) error {
	return validateUnixOwnerAndMode(info, 0o700)
}

func protectStateFile(path string, file *os.File) error {
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	return validateUnixOwnerAndMode(info, 0o600)
}

func validatePrivateState(_ string, file *os.File, pathInfo os.FileInfo) error {
	handleInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("runtime: inspect open state: %w", err)
	}
	pathStat, pathOK := pathInfo.Sys().(*syscall.Stat_t)
	handleStat, handleOK := handleInfo.Sys().(*syscall.Stat_t)
	if !pathOK || !handleOK || pathStat.Dev != handleStat.Dev || pathStat.Ino != handleStat.Ino {
		return fmt.Errorf("%w: instance state changed while opening", ErrUnsafeState)
	}
	if err := validateUnixOwnerAndMode(handleInfo, 0o600); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsafeState, err)
	}
	return nil
}

func validateUnixOwnerAndMode(info os.FileInfo, expected os.FileMode) error {
	if info.Mode().Perm() != expected {
		return fmt.Errorf("permissions are %04o, want %04o", info.Mode().Perm(), expected)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("ownership metadata is unavailable")
	}
	if int(stat.Uid) != os.Geteuid() {
		return errors.New("path is not owned by the current user")
	}
	return nil
}

func atomicReplace(source, target string) error {
	return os.Rename(source, target)
}

func syncRuntimeDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
