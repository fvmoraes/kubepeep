package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const validConfig = `version: 1
server:
  port: null
  openBrowser: true
  shutdownTimeout: 10s
dashboard:
  blockTimeout: 12s
observability:
  otel:
    enabled: false
    endpoint: null
    protocol: http/protobuf
    insecure: false
`

func TestLoadCreatesPrivateDefaultsAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	first, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.Server.OpenBrowser != true || first.Server.Port != nil || first.Server.ShutdownTimeout.Duration != 10*time.Second {
		t.Fatalf("unexpected defaults: %#v", first)
	}
	if first.Dashboard.BlockTimeout.Duration != DefaultDashboardBlockTimeout {
		t.Fatalf("unexpected dashboard defaults: %#v", first.Dashboard)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
	}
	second, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if second.Server.ShutdownTimeout.Duration != first.Server.ShutdownTimeout.Duration {
		t.Fatalf("reopened config differs: %#v != %#v", second, first)
	}
}

func TestParseStrictValidDocument(t *testing.T) {
	cfg, err := Parse([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestParseRejectsUnsafeOrAmbiguousYAML(t *testing.T) {
	tests := map[string]string{
		"unknown field":  strings.Replace(validConfig, "  port: null", "  host: 0.0.0.0\n  port: null", 1),
		"duplicate key":  strings.Replace(validConfig, "  port: null", "  port: null\n  port: 2748", 1),
		"anchor":         strings.Replace(validConfig, "port: null", "port: &chosen 2748", 1),
		"alias":          strings.Replace(validConfig, "openBrowser: true", "openBrowser: &enabled true\n  portCopy: *enabled", 1),
		"explicit tag":   strings.Replace(validConfig, "port: null", "port: !!int 2748", 1),
		"multi document": validConfig + "---\nversion: 1\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(input))
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestParseRejectsInvalidUTF8AndOversize(t *testing.T) {
	for name, input := range map[string][]byte{
		"invalid UTF-8": {0xff, 0xfe},
		"over 64 KiB":   []byte(strings.Repeat("x", MaxFileSize+1)),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(input)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestLoadPreservesInvalidClassificationForOversizeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", MaxFileSize+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
	if errors.Is(err, ErrIO) {
		t.Fatalf("oversized configuration was misclassified as I/O: %v", err)
	}
}

func TestLoadRejectsSymlinkAndDefaultCreationNeverOverwrites(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.yaml")
	if err := os.WriteFile(target, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "config-link.yaml")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := Load(link); !errors.Is(err, ErrIO) {
		t.Fatalf("symlink error = %v, want sanitized ErrIO", err)
	}

	existing := filepath.Join(directory, "existing.yaml")
	const sentinel = "existing-content-must-survive"
	if err := os.WriteFile(existing, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeDefault(existing, Default()); err == nil {
		t.Fatal("default publication overwrote an existing file")
	}
	contents, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != sentinel {
		t.Fatalf("existing content changed to %q", contents)
	}
}

func TestValidationMatrix(t *testing.T) {
	ptr := func(value string) *string { return &value }
	port := func(value int) *int { return &value }
	tests := map[string]func(*Config){
		"version":           func(c *Config) { c.Version = 2 },
		"low port":          func(c *Config) { c.Server.Port = port(1023) },
		"high port":         func(c *Config) { c.Server.Port = port(65536) },
		"short shutdown":    func(c *Config) { c.Server.ShutdownTimeout.Duration = 0 },
		"long shutdown":     func(c *Config) { c.Server.ShutdownTimeout.Duration = 31 * time.Second },
		"short block":       func(c *Config) { c.Dashboard.BlockTimeout.Duration = 0 },
		"long block":        func(c *Config) { c.Dashboard.BlockTimeout.Duration = 61 * time.Second },
		"protocol":          func(c *Config) { c.Observability.OTel.Protocol = "grpc" },
		"disabled endpoint": func(c *Config) { c.Observability.OTel.Endpoint = ptr("https://localhost") },
		"missing endpoint":  func(c *Config) { c.Observability.OTel.Enabled = true },
		"userinfo": func(c *Config) {
			c.Observability.OTel.Enabled = true
			c.Observability.OTel.Endpoint = ptr("https://user@example.test")
		},
		"query": func(c *Config) {
			c.Observability.OTel.Enabled = true
			c.Observability.OTel.Endpoint = ptr("https://example.test?a=b")
		},
		"insecure remote HTTP": func(c *Config) {
			c.Observability.OTel.Enabled = true
			c.Observability.OTel.Insecure = true
			c.Observability.OTel.Endpoint = ptr("http://example.test")
		},
		"secure loopback HTTP": func(c *Config) {
			c.Observability.OTel.Enabled = true
			c.Observability.OTel.Endpoint = ptr("http://127.0.0.1:4318")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := Default()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestOTelAllowsHTTPSAndExplicitInsecureLoopbackHTTP(t *testing.T) {
	for _, endpoint := range []string{"https://collector.example.test/v1/traces", "http://[::1]:4318/v1/traces"} {
		cfg := Default()
		cfg.Observability.OTel.Enabled = true
		cfg.Observability.OTel.Endpoint = &endpoint
		cfg.Observability.OTel.Insecure = strings.HasPrefix(endpoint, "http://")
		if err := cfg.Validate(); err != nil {
			t.Fatalf("endpoint %q: %v", endpoint, err)
		}
	}
}

func TestApplyFlagsUsesSeparatePrecedenceLayer(t *testing.T) {
	filePort, flagPort := 2749, 2750
	base := Default()
	base.Server.Port = &filePort
	effective, err := ApplyFlags(base, FlagOverrides{Port: &flagPort, NoBrowser: true})
	if err != nil {
		t.Fatal(err)
	}
	if *effective.Server.Port != flagPort || effective.Server.OpenBrowser {
		t.Fatalf("unexpected effective config: %#v", effective)
	}
	if *base.Server.Port != filePort || !base.Server.OpenBrowser {
		t.Fatal("ApplyFlags mutated file configuration")
	}
}

func TestToGingerKeepsLoopbackAndFixedPool(t *testing.T) {
	port := 2797
	cfg := Default()
	cfg.Server.Port = &port
	ginger := cfg.ToGinger("/private/kubePeep.db")
	if ginger.HTTP.Host != "127.0.0.1" || ginger.HTTP.Port != port {
		t.Fatalf("unexpected HTTP config: %#v", ginger.HTTP)
	}
	if ginger.Database.MaxOpen != 4 || ginger.Database.MaxIdle != 4 || ginger.Database.DSN != "/private/kubePeep.db" {
		t.Fatalf("unexpected database config: %#v", ginger.Database)
	}
}

func TestResourcesCollectionTimeoutBounds(t *testing.T) {
	cfg := Default()
	if cfg.Resources.CollectionTimeout.Duration != DefaultResourcesCollectionTimeout {
		t.Fatalf("unexpected collection default: %#v", cfg.Resources)
	}
	cfg.Resources.CollectionTimeout.Duration = 4 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected below-minimum collectionTimeout to be rejected")
	}
	cfg.Resources.CollectionTimeout.Duration = 301 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected above-ceiling collectionTimeout to be rejected")
	}
	cfg.Resources.CollectionTimeout.Duration = 120 * time.Second
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
