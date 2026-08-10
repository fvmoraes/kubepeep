package securefs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrivateRegularLifecycleAndNoReplacePublication(t *testing.T) {
	directory := t.TempDir()
	if err := EnsurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	temporary, err := CreateTemp(directory, ".state-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = temporary.Close() })
	if _, err := temporary.File().WriteString("private state"); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "state.json")
	if err := temporary.PublishNoReplace(destination); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrivateRegular(destination); err != nil {
		t.Fatal(err)
	}
	occupied := filepath.Join(directory, "occupied.json")
	if err := os.WriteFile(occupied, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := temporary.PublishNoReplace(occupied); err == nil {
		t.Fatal("no-replace publication overwrote an existing destination")
	}
	content, err := os.ReadFile(occupied)
	if err != nil || string(content) != "keep" {
		t.Fatalf("occupied destination changed: %q, %v", content, err)
	}
}

func TestGuardDetectsPathReplacement(t *testing.T) {
	directory := t.TempDir()
	if err := EnsurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "state")
	original, err := CreateExclusive(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = original.Close() })
	displaced := path + ".displaced"
	if err := os.Rename(path, displaced); err != nil {
		t.Fatal(err)
	}
	replacement, err := CreateExclusive(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replacement.Close() })
	if err := original.Validate(); !errors.Is(err, ErrUnsafeObject) {
		t.Fatalf("Validate() error = %v, want ErrUnsafeObject", err)
	}
}

func TestUnixValidationRejectsBroadModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows privacy is represented by DACLs")
	}
	directory := t.TempDir()
	if err := EnsurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "state")
	if err := os.WriteFile(path, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrivateRegular(path); !errors.Is(err, ErrUnsafeObject) {
		t.Fatalf("ValidatePrivateRegular() error = %v, want ErrUnsafeObject", err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePrivateDirectory(directory); !errors.Is(err, ErrUnsafeObject) {
		t.Fatalf("ValidatePrivateDirectory() error = %v, want ErrUnsafeObject", err)
	}
}
