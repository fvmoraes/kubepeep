// Package userdirs resolves and prepares kubePeep's canonical per-user paths.
package userdirs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const applicationDirectory = ".kubePeep"

// Layout contains every stable local path owned by kubePeep. SQLite sidecars,
// rotated logs and short-lived atomic-write files are the only derived paths.
type Layout struct {
	Root       string
	Config     string
	Database   string
	LogsDir    string
	Log        string
	RuntimeDir string
	Lock       string
	Instance   string
	Cache      string
}

// Resolve returns the canonical, non-configurable layout for the current user.
func Resolve() (Layout, error) {
	root, err := defaultRoot()
	if err != nil {
		return Layout{}, err
	}
	return ForRoot(root)
}

// ForRoot constructs a layout for an explicit root. It exists for composition
// and hermetic tests; the production resolver always uses Resolve.
func ForRoot(root string) (Layout, error) {
	if root == "" {
		return Layout{}, errors.New("user directories: data root is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Layout{}, fmt.Errorf("user directories: resolve data root: %w", err)
	}
	absolute = filepath.Clean(absolute)
	return Layout{
		Root:       absolute,
		Config:     filepath.Join(absolute, "config.yaml"),
		Database:   filepath.Join(absolute, "kubePeep.db"),
		LogsDir:    filepath.Join(absolute, "logs"),
		Log:        filepath.Join(absolute, "logs", "kubePeep.log"),
		RuntimeDir: filepath.Join(absolute, "runtime"),
		Lock:       filepath.Join(absolute, "runtime", "kubePeep.lock"),
		Instance:   filepath.Join(absolute, "runtime", "instance.json"),
		Cache:      filepath.Join(absolute, "cache"),
	}, nil
}

// EnsureDirectories creates only the canonical directories and protects each
// one for the current user. Files are created by their owning adapters.
func (layout Layout) EnsureDirectories() error {
	directories := []string{layout.Root, layout.LogsDir, layout.RuntimeDir, layout.Cache}
	for _, directory := range directories {
		if directory == "" {
			return errors.New("user directories: layout contains an empty path")
		}
		if err := ensureDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("user directories: create directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("user directories: inspect directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("user directories: path is not a trusted directory")
	}
	if err := protectDirectory(path); err != nil {
		return fmt.Errorf("user directories: protect directory: %w", err)
	}
	return nil
}
