//go:build !windows

package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func installPlatform(ctx context.Context, target, staged, targetVersion, _ string, _ string, verifier BinaryVerifier) (bool, error) {
	backup, err := copyBackup(target)
	if err != nil {
		return false, err
	}
	removeBackup := true
	defer func() {
		if removeBackup {
			_ = os.Remove(backup)
		}
	}()
	if err := os.Rename(staged, target); err != nil {
		return false, fmt.Errorf("update: atomically replace executable: %w", err)
	}
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		rollbackErr := os.Rename(backup, target)
		_ = syncDirectory(filepath.Dir(target))
		if rollbackErr != nil {
			return false, errors.Join(fmt.Errorf("update: sync executable replacement: %w", err), fmt.Errorf("update: rollback failed: %w", rollbackErr))
		}
		removeBackup = false
		return false, ErrRollback
	}
	if err := verifier.Verify(ctx, target, targetVersion); err != nil {
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			return false, errors.Join(fmt.Errorf("%w: %v", ErrRollback, err), fmt.Errorf("update: rollback failed: %w", rollbackErr))
		}
		removeBackup = false
		if syncErr := syncDirectory(filepath.Dir(target)); syncErr != nil {
			return false, errors.Join(ErrRollback, fmt.Errorf("update: sync rollback: %w", syncErr))
		}
		return false, ErrRollback
	}
	if err := os.Remove(backup); err != nil {
		return false, fmt.Errorf("update: replacement succeeded but backup cleanup failed: %w", err)
	}
	removeBackup = false
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		return false, fmt.Errorf("update: replacement succeeded but directory sync failed: %w", err)
	}
	return false, nil
}

func copyBackup(target string) (string, error) {
	input, err := os.Open(target)
	if err != nil {
		return "", fmt.Errorf("update: open installed executable for backup: %w", err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return "", fmt.Errorf("update: inspect installed executable for backup: %w", err)
	}
	output, err := os.CreateTemp(filepath.Dir(target), ".kubePeep.backup.*")
	if err != nil {
		return "", fmt.Errorf("update: create executable backup: %w", err)
	}
	backup := output.Name()
	remove := true
	defer func() {
		_ = output.Close()
		if remove {
			_ = os.Remove(backup)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return "", fmt.Errorf("update: copy executable backup: %w", err)
	}
	if err := output.Chmod(info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("update: protect executable backup: %w", err)
	}
	if err := output.Sync(); err != nil {
		return "", fmt.Errorf("update: sync executable backup: %w", err)
	}
	if err := output.Close(); err != nil {
		return "", fmt.Errorf("update: close executable backup: %w", err)
	}
	remove = false
	return backup, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
