package runtime

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestFileLockIsStableAndExclusive(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	if err := EnsureRuntimeDirectory(runtimeDirectory); err != nil {
		t.Fatal(err)
	}
	first, err := AcquireFileLock(LockPath(runtimeDirectory))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := AcquireFileLock(LockPath(runtimeDirectory)); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock error = %v, want ErrLocked", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireFileLock(LockPath(runtimeDirectory))
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}
