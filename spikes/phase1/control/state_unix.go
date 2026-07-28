//go:build !windows

package control

import (
	"fmt"
	"os"
)

func protectRuntimeDir(path string, _ os.FileInfo) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("control: protect runtime directory: %w", err)
	}
	return nil
}

func validatePrivateState(_ string, info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("control: insecure instance permissions %04o", info.Mode().Perm())
	}
	return nil
}

func protectTemporaryState(string) error {
	return nil
}

func atomicReplace(source, target string) error {
	return os.Rename(source, target)
}

func syncRuntimeDir(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
