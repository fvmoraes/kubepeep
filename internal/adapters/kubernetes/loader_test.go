package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderAppliesSourcePrecedenceWithoutSilentFallback(t *testing.T) {
	directory := t.TempDir()
	explicit := filepath.Join(directory, "explicit.yaml")
	environment := filepath.Join(directory, "environment.yaml")
	persisted := filepath.Join(directory, "persisted.yaml")
	recommended := filepath.Join(directory, "recommended.yaml")
	writeTestKubeconfig(t, explicit, testKubeconfig("https://explicit.invalid", "current"))
	writeTestKubeconfig(t, environment, testKubeconfig("https://environment.invalid", "current"))
	writeTestKubeconfig(t, persisted, testKubeconfig("https://persisted.invalid", "current"))
	writeTestKubeconfig(t, recommended, testKubeconfig("https://recommended.invalid", "current"))

	environmentValue := environment
	loader := NewLoader(LoaderOptions{
		LookupEnv:       func(key string) (string, bool) { return environmentValue, environmentValue != "" },
		RecommendedPath: func() string { return recommended },
	})
	resolution, err := loader.Resolve(context.Background(), ResolveRequest{
		ExplicitPath:   &explicit,
		Persisted:      &ProfileReference{Paths: []string{persisted}},
		FirstReconcile: true,
	})
	if err != nil {
		t.Fatalf("resolve explicit source: %v", err)
	}
	if got := resolution.Descriptor().Source; got != SourceExplicit {
		t.Fatalf("source = %q, want %q", got, SourceExplicit)
	}
	if got := resolution.restConfig.Host; got != "https://explicit.invalid" {
		t.Fatalf("host = %q", got)
	}

	resolution, err = loader.Resolve(context.Background(), ResolveRequest{
		Persisted:      &ProfileReference{Paths: []string{persisted}},
		FirstReconcile: true,
		ProfileOnly:    true,
	})
	if err != nil || resolution.Descriptor().Source != SourceProfile || resolution.restConfig.Host != "https://persisted.invalid" {
		t.Fatalf("profile-only source must ignore ambient KUBECONFIG: descriptor=%+v err=%v", resolution.Descriptor(), err)
	}

	missing := filepath.Join(directory, "SECRET_PATH_SHOULD_NOT_LEAK")
	_, err = loader.Resolve(context.Background(), ResolveRequest{
		ExplicitPath:   &missing,
		Persisted:      &ProfileReference{Paths: []string{persisted}},
		FirstReconcile: true,
	})
	if code := safeCode(t, err); code != CodeKubeconfigNotFound {
		t.Fatalf("code = %q", code)
	}
	if strings.Contains(err.Error(), missing) || strings.Contains(err.Error(), "SECRET_PATH") {
		t.Fatalf("error leaked selected path: %v", err)
	}

	invalidEnvironment := filepath.Join(directory, "environment-invalid.yaml")
	if err := os.WriteFile(invalidEnvironment, []byte("token: TOKEN_SHOULD_NOT_LEAK: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	environmentValue = invalidEnvironment
	_, err = loader.Resolve(context.Background(), ResolveRequest{
		Persisted:      &ProfileReference{Paths: []string{persisted}},
		FirstReconcile: true,
	})
	if code := safeCode(t, err); code != CodeKubeconfigInvalid {
		t.Fatalf("code = %q", code)
	}
	if strings.Contains(err.Error(), "TOKEN_SHOULD_NOT_LEAK") {
		t.Fatalf("parse error leaked content: %v", err)
	}

	environmentValue = ""
	resolution, err = loader.Resolve(context.Background(), ResolveRequest{
		Persisted:      &ProfileReference{Paths: []string{persisted}},
		FirstReconcile: true,
	})
	if err != nil || resolution.Descriptor().Source != SourceProfile {
		t.Fatalf("resolve profile: source=%q err=%v", resolution.Descriptor().Source, err)
	}
	resolution, err = loader.Resolve(context.Background(), ResolveRequest{FirstReconcile: true})
	if err != nil || resolution.Descriptor().Source != SourceRecommended {
		t.Fatalf("resolve recommended: source=%q err=%v", resolution.Descriptor().Source, err)
	}
}

func TestLoaderAppliesContextPrecedenceAndAllowsUnselectedValidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeTestKubeconfig(t, path, testKubeconfig("https://cluster.invalid", "current"))
	loader := NewLoader(LoaderOptions{LookupEnv: func(string) (string, bool) { return "", false }})

	resolution, err := loader.Resolve(context.Background(), ResolveRequest{
		ExplicitPath:    &path,
		ExplicitContext: stringPointer("explicit"),
		Persisted:       &ProfileReference{Context: "persisted"},
		FirstReconcile:  true,
	})
	if err != nil || resolution.Descriptor().Context != "explicit" {
		t.Fatalf("explicit context: descriptor=%+v err=%v", resolution.Descriptor(), err)
	}
	resolution, err = loader.Resolve(context.Background(), ResolveRequest{
		ExplicitPath:   &path,
		Persisted:      &ProfileReference{Context: "persisted"},
		FirstReconcile: true,
	})
	if err != nil || resolution.Descriptor().Context != "persisted" {
		t.Fatalf("persisted context: descriptor=%+v err=%v", resolution.Descriptor(), err)
	}
	resolution, err = loader.Resolve(context.Background(), ResolveRequest{
		ExplicitPath:   &path,
		FirstReconcile: true,
	})
	if err != nil || resolution.Descriptor().Context != "current" {
		t.Fatalf("current context: descriptor=%+v err=%v", resolution.Descriptor(), err)
	}
	resolution, err = loader.Resolve(context.Background(), ResolveRequest{
		ExplicitPath:   &path,
		FirstReconcile: false,
	})
	if err != nil || resolution.Descriptor().Context != "" || resolution.restConfig != nil {
		t.Fatalf("unselected config: descriptor=%+v config=%v err=%v", resolution.Descriptor(), resolution.restConfig, err)
	}

	_, err = loader.Resolve(context.Background(), ResolveRequest{
		ExplicitPath:    &path,
		ExplicitContext: stringPointer("missing-secret-context"),
		Persisted:       &ProfileReference{Context: "persisted"},
		FirstReconcile:  true,
	})
	if code := safeCode(t, err); code != CodeContextNotFound {
		t.Fatalf("code = %q", code)
	}
	if strings.Contains(err.Error(), "missing-secret-context") {
		t.Fatalf("context name leaked: %v", err)
	}
}

func TestLoaderMergesOrderedFilesAndDeduplicatesPaths(t *testing.T) {
	directory := t.TempDir()
	firstPath := filepath.Join(directory, "first.yaml")
	secondPath := filepath.Join(directory, "second.yaml")
	first := testKubeconfig("https://first.invalid", "current")
	first.AuthInfos = nil
	second := testKubeconfig("https://second.invalid", "")
	second.Clusters = nil
	second.Contexts = nil
	writeTestKubeconfig(t, firstPath, first)
	writeTestKubeconfig(t, secondPath, second)
	environment := strings.Join([]string{firstPath, secondPath, firstPath}, string(os.PathListSeparator))
	loader := NewLoader(LoaderOptions{LookupEnv: func(string) (string, bool) { return environment, true }})
	resolution, err := loader.Resolve(context.Background(), ResolveRequest{FirstReconcile: true})
	if err != nil {
		t.Fatalf("resolve merged config: %v", err)
	}
	descriptor := resolution.Descriptor()
	if len(descriptor.Paths) != 2 || descriptor.Paths[0] != firstPath || descriptor.Paths[1] != secondPath {
		t.Fatalf("ordered paths = %#v", descriptor.Paths)
	}
	if resolution.restConfig.Host != "https://first.invalid" {
		t.Fatalf("merge precedence host = %q", resolution.restConfig.Host)
	}

	explicitList := strings.Join([]string{firstPath, secondPath}, string(os.PathListSeparator))
	resolution, err = loader.Resolve(context.Background(), ResolveRequest{ExplicitPath: &explicitList, FirstReconcile: true})
	if err != nil {
		t.Fatalf("resolve explicit ordered list: %v", err)
	}
	if descriptor := resolution.Descriptor(); descriptor.Source != SourceExplicit || len(descriptor.Paths) != 2 {
		t.Fatalf("explicit descriptor = %#v", descriptor)
	}
}
