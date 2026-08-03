package userdirs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForRootUsesOnlyCanonicalPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	layout, err := ForRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"config":   filepath.Join(root, "config.yaml"),
		"database": filepath.Join(root, "kubePeep.db"),
		"log":      filepath.Join(root, "logs", "kubePeep.log"),
		"lock":     filepath.Join(root, "runtime", "kubePeep.lock"),
		"instance": filepath.Join(root, "runtime", "instance.json"),
		"cache":    filepath.Join(root, "cache"),
	}
	got := map[string]string{
		"config": layout.Config, "database": layout.Database, "log": layout.Log,
		"lock": layout.Lock, "instance": layout.Instance, "cache": layout.Cache,
	}
	for name, expected := range want {
		if got[name] != expected {
			t.Fatalf("%s path = %q, want %q", name, got[name], expected)
		}
	}
}

func TestEnsureDirectoriesDoesNotInventFiles(t *testing.T) {
	layout, err := ForRoot(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}

	for _, directory := range []string{layout.Root, layout.LogsDir, layout.RuntimeDir, layout.Cache} {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			t.Fatalf("canonical directory %q was not created: %v", directory, err)
		}
	}
	for _, file := range []string{layout.Config, layout.Database, layout.Log, layout.Lock, layout.Instance} {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("file %q should be owned and created by another adapter", file)
		}
	}
}
