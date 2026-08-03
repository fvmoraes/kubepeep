package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/fvmoraes/kubepeep/internal/securefs"
	"gopkg.in/yaml.v3"
)

var (
	ErrInvalid = errors.New("invalid kubePeep configuration")
	ErrIO      = errors.New("kubePeep configuration I/O failure")
)

// Load reads one strict YAML document. A missing file is initialized with
// private defaults through a same-directory temporary file and atomic,
// no-replace publication.
func Load(path string) (Config, error) {
	data, err := readBounded(path)
	if errors.Is(err, os.ErrNotExist) {
		defaults := Default()
		if err := writeDefault(path, defaults); err != nil {
			return Config{}, sanitizedIO("initialize defaults", err)
		}
		return defaults, nil
	}
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			return Config{}, err
		}
		return Config{}, sanitizedIO("read", err)
	}
	return Parse(data)
}

// Parse validates and decodes one bounded YAML v1 document.
func Parse(data []byte) (Config, error) {
	if len(data) > MaxFileSize {
		return Config{}, invalid("file exceeds 64 KiB")
	}
	if !utf8.Valid(data) {
		return Config{}, invalid("file is not valid UTF-8")
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return Config{}, invalid("malformed YAML")
	}
	if len(document.Content) != 1 {
		return Config{}, invalid("document is empty")
	}
	if err := validateNode(document.Content[0]); err != nil {
		return Config{}, invalid(err.Error())
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return Config{}, invalid("multiple YAML documents are not allowed")
	}

	cfg := Default()
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	if err := strict.Decode(&cfg); err != nil {
		return Config{}, invalid("unknown field or invalid value type")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, errors.Join(ErrInvalid, err)
	}
	return cfg, nil
}

func invalid(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, detail)
}

func sanitizedIO(operation string, err error) error {
	reason := "operation failed"
	switch {
	case errors.Is(err, os.ErrPermission):
		reason = "permission denied"
	case errors.Is(err, os.ErrExist):
		reason = "destination already exists"
	case errors.Is(err, os.ErrNotExist):
		reason = "parent directory is unavailable"
	}
	return fmt.Errorf("%w: %s: %s", ErrIO, operation, reason)
}

func validateNode(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode || node.Alias != nil {
		return fmt.Errorf("aliases are not allowed")
	}
	if node.Anchor != "" {
		return fmt.Errorf("anchors are not allowed")
	}
	if node.Style&yaml.TaggedStyle != 0 {
		return fmt.Errorf("explicit YAML tags are not allowed")
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%2 != 0 {
			return fmt.Errorf("invalid mapping")
		}
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("mapping keys must be strings")
			}
			if _, exists := seen[key.Value]; exists {
				return fmt.Errorf("duplicate mapping key")
			}
			seen[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := validateNode(child); err != nil {
			return err
		}
	}
	return nil
}

func readBounded(path string) ([]byte, error) {
	guard, err := securefs.OpenRegular(path, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer guard.Close()
	data, err := io.ReadAll(io.LimitReader(guard.File(), MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if err := guard.Validate(); err != nil {
		return nil, err
	}
	if len(data) > MaxFileSize {
		return nil, invalid("file exceeds 64 KiB")
	}
	return data, nil
}

func writeDefault(path string, cfg Config) (returnErr error) {
	if path == "" {
		return fmt.Errorf("empty configuration path")
	}
	parent := filepath.Dir(path)
	if err := securefs.EnsurePrivateDirectory(parent); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	temporary, err := securefs.CreateTemp(parent, ".config.yaml.tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Path()
	defer func() {
		if closeErr := temporary.Close(); returnErr == nil && closeErr != nil {
			returnErr = closeErr
		}
		_ = os.Remove(temporaryPath)
	}()
	if _, err := temporary.File().Write(data); err != nil {
		return err
	}
	if err := temporary.File().Sync(); err != nil {
		return err
	}
	if err := temporary.PublishNoReplace(path); err != nil {
		return err
	}
	return nil
}
