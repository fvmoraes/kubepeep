package control

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestFileLockExcludesAnotherOwnerAndCanBeReacquired(t *testing.T) {
	path := filepath.Join(t.TempDir(), lockFileName)

	first, err := AcquireFileLock(path)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	defer first.Close()

	if _, err := AcquireFileLock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second acquire error = %v, want ErrLocked", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first lock: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first lock again: %v", err)
	}

	second, err := AcquireFileLock(path)
	if err != nil {
		t.Fatalf("reacquire lock: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close reacquired lock: %v", err)
	}
}
