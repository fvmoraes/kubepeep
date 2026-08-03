package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	productconfig "github.com/fvmoraes/kubepeep/internal/config"
	localruntime "github.com/fvmoraes/kubepeep/internal/runtime"
)

func TestProductionStartServesEmbeddedApplicationAndCleansUp(t *testing.T) {
	layout, err := userdirs.ForRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requestErr error
	result, err := productionStart(ctx, StartOptions{
		Layout:    layout,
		Config:    productconfig.Default(),
		LogOutput: &bytes.Buffer{},
		OnReady: func(identity localruntime.ControlIdentityDTO) {
			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, identity.URL()+"/api/v1/status", nil)
			if err != nil {
				requestErr = err
				cancel()
				return
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				requestErr = err
			} else {
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				if response.StatusCode != http.StatusOK {
					requestErr = &unexpectedStatusError{status: response.StatusCode}
				}
			}
			cancel()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	if !result.Started || result.Identity.Port < localruntime.MinimumPort || result.Identity.Port > localruntime.MaximumPort {
		t.Fatalf("unexpected run result: %#v", result)
	}
	for _, path := range []string{layout.Database, layout.Log, layout.Lock} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected runtime artifact %s: %v", filepath.Base(path), err)
		}
	}
	if _, err := os.Stat(layout.Instance); !os.IsNotExist(err) {
		t.Fatalf("instance state survived shutdown: %v", err)
	}
}

type unexpectedStatusError struct{ status int }

func (err *unexpectedStatusError) Error() string { return http.StatusText(err.status) }

func TestConfiguredBrowserPreferenceIsRespected(t *testing.T) {
	layout := cliLayout(t)
	if err := layout.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}
	configuration := []byte("version: 1\nserver:\n  port: null\n  openBrowser: false\n  shutdownTimeout: 10s\nobservability:\n  otel:\n    enabled: false\n    endpoint: null\n    protocol: http/protobuf\n    insecure: false\n")
	if err := os.WriteFile(layout.Config, configuration, 0o600); err != nil {
		t.Fatal(err)
	}
	browser := &fakeBrowser{}
	identity := cliIdentity(t)
	code := ExecuteContext(context.Background(), nil, Dependencies{
		ResolveLayout: func() (userdirs.Layout, error) { return layout, nil },
		Browser:       browser,
		Stdout:        &bytes.Buffer{},
		Stderr:        &bytes.Buffer{},
		Start: func(_ context.Context, options StartOptions) (localruntime.RunResult, error) {
			options.OnReady(identity)
			return localruntime.RunResult{Identity: identity, Existing: true}, nil
		},
	})
	if code != ExitSuccess || len(browser.urls) != 0 {
		t.Fatalf("code=%d browser URLs=%v", code, browser.urls)
	}
}

func TestProductionStartRequiresEffectiveConfiguration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := productionStart(ctx, StartOptions{})
	if err == nil {
		t.Fatal("expected missing configuration to fail")
	}
}
