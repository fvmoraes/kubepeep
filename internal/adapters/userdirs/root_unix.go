//go:build !windows

package userdirs

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func defaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user directories: resolve home: %w", err)
	}
	return filepath.Join(home, applicationDirectory), nil
}

func protectDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("directory permissions are not private")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("directory is not owned by the current user")
	}
	return nil
}
