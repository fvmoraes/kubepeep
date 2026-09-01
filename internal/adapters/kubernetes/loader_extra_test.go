package kubernetes

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestResolutionContextsReturnsSortedCredentialFreeView(t *testing.T) {
	if got := (*Resolution)(nil).Contexts(); len(got) != 0 {
		t.Fatalf("nil resolution contexts = %#v", got)
	}
	if got := (&Resolution{}).Contexts(); len(got) != 0 {
		t.Fatalf("empty resolution contexts = %#v", got)
	}
	raw := clientcmdapi.NewConfig()
	raw.Contexts["beta"] = &clientcmdapi.Context{Cluster: "cluster-beta"}
	raw.Contexts["alpha"] = &clientcmdapi.Context{Cluster: "cluster-alpha"}
	raw.Contexts["void"] = nil
	resolution := &Resolution{raw: raw}
	contexts := resolution.Contexts()
	want := []ContextDescriptor{
		{Name: "alpha", Cluster: "cluster-alpha"},
		{Name: "beta", Cluster: "cluster-beta"},
		{Name: "void", Cluster: ""},
	}
	if len(contexts) != len(want) {
		t.Fatalf("contexts = %#v, want %#v", contexts, want)
	}
	for position, descriptor := range want {
		if contexts[position] != descriptor {
			t.Fatalf("contexts = %#v, want %#v", contexts, want)
		}
	}
}

func TestResolutionSelectedContextGuards(t *testing.T) {
	raw := clientcmdapi.NewConfig()
	raw.Contexts["current"] = &clientcmdapi.Context{Cluster: "cluster-current"}
	tests := []struct {
		name       string
		resolution *Resolution
		want       bool
		cluster    string
	}{
		{name: "nil", resolution: nil, want: false},
		{name: "no raw", resolution: &Resolution{}, want: false},
		{name: "no selection", resolution: &Resolution{raw: raw}, want: false},
		{name: "selected", resolution: &Resolution{raw: raw, descriptor: Descriptor{Context: "current"}}, want: true, cluster: "cluster-current"},
		{name: "missing context", resolution: &Resolution{raw: raw, descriptor: Descriptor{Context: "elsewhere"}}, want: false},
		{name: "nil context entry", resolution: &Resolution{raw: raw, descriptor: Descriptor{Context: "void"}}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := test.resolution.SelectedContext()
			if ok != test.want {
				t.Fatalf("selected ok = %v, want %v", ok, test.want)
			}
			if ok && got.Cluster != test.cluster {
				t.Fatalf("selected cluster = %q, want %q", got.Cluster, test.cluster)
			}
		})
	}
}

func TestResolutionExecPluginDiagnosticExposesOnlyBooleans(t *testing.T) {
	zero := ExecPluginDiagnostic{}
	guards := []struct {
		name       string
		resolution *Resolution
	}{
		{name: "nil", resolution: nil},
		{name: "no raw", resolution: &Resolution{}},
		{name: "no selection", resolution: &Resolution{raw: clientcmdapi.NewConfig()}},
		{name: "missing context", resolution: &Resolution{raw: clientcmdapi.NewConfig(), descriptor: Descriptor{Context: "current"}}},
		{name: "missing authinfo", resolution: &Resolution{raw: contextWithoutAuthInfo(), descriptor: Descriptor{Context: "current"}}},
		{name: "no exec", resolution: &Resolution{raw: plainAuthInfoConfig(), descriptor: Descriptor{Context: "current"}}},
	}
	for _, test := range guards {
		t.Run(test.name, func(t *testing.T) {
			if got := test.resolution.ExecPlugin(); got != zero {
				t.Fatalf("diagnostic = %#v, want zero", got)
			}
		})
	}

	compatible := execPluginResolution(t, "sh", "client.authentication.k8s.io/v1", clientcmdapi.NeverExecInteractiveMode)
	want := ExecPluginDiagnostic{Configured: true, CommandAvailable: true, NonInteractive: true, VersionCompatible: true}
	if got := compatible.ExecPlugin(); got != want {
		t.Fatalf("compatible diagnostic = %#v, want %#v", got, want)
	}
	if missing := execPluginResolution(t, "kubepeep-missing-plugin-binary", "client.authentication.k8s.io/v1", clientcmdapi.NeverExecInteractiveMode).ExecPlugin(); missing.CommandAvailable || !missing.Configured || !missing.NonInteractive || !missing.VersionCompatible {
		t.Fatalf("missing command diagnostic = %#v", missing)
	}
	if incompatible := execPluginResolution(t, "sh", "client.authentication.k8s.io/v9", clientcmdapi.NeverExecInteractiveMode).ExecPlugin(); incompatible.VersionCompatible {
		t.Fatalf("incompatible diagnostic = %#v", incompatible)
	}
	if interactive := execPluginResolution(t, "sh", "client.authentication.k8s.io/v1", clientcmdapi.AlwaysExecInteractiveMode).ExecPlugin(); interactive.NonInteractive {
		t.Fatalf("interactive diagnostic = %#v", interactive)
	}
}

func contextWithoutAuthInfo() *clientcmdapi.Config {
	raw := clientcmdapi.NewConfig()
	raw.Contexts["current"] = &clientcmdapi.Context{Cluster: "cluster", AuthInfo: "ghost"}
	return raw
}

func plainAuthInfoConfig() *clientcmdapi.Config {
	raw := contextWithoutAuthInfo()
	raw.AuthInfos["user"] = &clientcmdapi.AuthInfo{}
	raw.Contexts["current"].AuthInfo = "user"
	return raw
}

func execPluginResolution(t *testing.T, command, apiVersion string, interactiveMode clientcmdapi.ExecInteractiveMode) *Resolution {
	t.Helper()
	raw := plainAuthInfoConfig()
	raw.AuthInfos["user"].Exec = &clientcmdapi.ExecConfig{
		Command:         command,
		APIVersion:      apiVersion,
		InteractiveMode: interactiveMode,
	}
	return &Resolution{raw: raw, descriptor: Descriptor{Context: "current"}}
}

func TestValidContextNameRejectsUnsafeNames(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: false},
		{name: "normal", value: "prod-eu-1", want: true},
		{name: "too long", value: strings.Repeat("a", 1025), want: false},
		{name: "boundary length", value: strings.Repeat("a", 1024), want: true},
		{name: "invalid utf8", value: "\xff\xfe", want: false},
		{name: "control character", value: "prod\n-eu", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validContextName(test.value); got != test.want {
				t.Fatalf("validContextName(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestResolutionConfigCopyRejectsMissingAndImpersonatedConfigs(t *testing.T) {
	_, err := (*Resolution)(nil).configCopy()
	if code := safeCode(t, err); code != CodeContextRequired {
		t.Fatalf("nil resolution code = %q", code)
	}
	_, err = (&Resolution{}).configCopy()
	if code := safeCode(t, err); code != CodeContextRequired {
		t.Fatalf("empty resolution code = %q", code)
	}
	impersonated := &Resolution{restConfig: &rest.Config{Host: "https://cluster.invalid", Impersonate: rest.ImpersonationConfig{UserName: "forbidden-admin"}}}
	_, err = impersonated.configCopy()
	if code := safeCode(t, err); code != CodeKubeconfigInvalid {
		t.Fatalf("impersonated code = %q", code)
	}
	resolution := &Resolution{restConfig: &rest.Config{Host: "https://cluster.invalid"}}
	copied, err := resolution.configCopy()
	if err != nil {
		t.Fatal(err)
	}
	if copied == resolution.restConfig || copied.Host != "https://cluster.invalid" {
		t.Fatalf("config copy = %#v", copied)
	}
}

func TestResolutionNilAccessorsStaySafe(t *testing.T) {
	if got := (*Resolution)(nil).Descriptor(); got.Context != "" || got.Paths != nil {
		t.Fatalf("nil descriptor = %#v", got)
	}
	if got := (*Resolution)(nil).Fingerprint(); got != (Fingerprint{}) {
		t.Fatalf("nil fingerprint = %s", got)
	}
	_, err := (*Resolution)(nil).CurrentFingerprint(context.Background())
	if code := safeCode(t, err); code != CodeKubeconfigInvalid {
		t.Fatalf("nil current fingerprint code = %q", code)
	}
}

func TestLoaderResolveRejectsInvalidRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeTestKubeconfig(t, path, testKubeconfig("https://cluster.invalid", "current"))
	emptyRecommended := ""
	loader := NewLoader(LoaderOptions{
		LookupEnv:       func(string) (string, bool) { return "", false },
		RecommendedPath: func() string { return emptyRecommended },
	})
	explicitEmpty := ""
	emptyContext := ""
	controlContext := "prod\n-context"

	tests := []struct {
		name    string
		request ResolveRequest
		want    ErrorCode
	}{
		{name: "profile only without persisted", request: ResolveRequest{ProfileOnly: true}, want: CodeKubeconfigInvalid},
		{name: "profile only without paths", request: ResolveRequest{ProfileOnly: true, Persisted: &ProfileReference{}}, want: CodeKubeconfigInvalid},
		{name: "empty explicit path", request: ResolveRequest{ExplicitPath: &explicitEmpty}, want: CodeKubeconfigInvalid},
		{name: "persisted without paths", request: ResolveRequest{Persisted: &ProfileReference{}}, want: CodeKubeconfigInvalid},
		{name: "no sources at all", request: ResolveRequest{FirstReconcile: true}, want: CodeKubeconfigNotFound},
		{name: "empty explicit context", request: ResolveRequest{ExplicitPath: &path, ExplicitContext: &emptyContext}, want: CodeContextNotFound},
		{name: "control character context", request: ResolveRequest{ExplicitPath: &path, ExplicitContext: &controlContext}, want: CodeContextNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loader.Resolve(context.Background(), test.request)
			if code := safeCode(t, err); code != test.want {
				t.Fatalf("code = %q, want %q", code, test.want)
			}
		})
	}

	_, err := loader.Resolve(nil, ResolveRequest{ExplicitPath: &path})
	if code := safeCode(t, err); code != CodeKubeconfigInvalid {
		t.Fatalf("nil context code = %q", code)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = loader.Resolve(canceled, ResolveRequest{ExplicitPath: &path})
	if code := safeCode(t, err); code != CodeRequestCanceled {
		t.Fatalf("canceled context code = %q", code)
	}

	separatorLoader := NewLoader(LoaderOptions{
		LookupEnv: func(string) (string, bool) {
			return string(filepath.ListSeparator) + string(filepath.ListSeparator), true
		},
	})
	_, err = separatorLoader.Resolve(context.Background(), ResolveRequest{FirstReconcile: true})
	if code := safeCode(t, err); code != CodeKubeconfigInvalid {
		t.Fatalf("separator-only environment code = %q", code)
	}
}

func TestNormalizePathsSkipsEmptiesAndDuplicates(t *testing.T) {
	directory := t.TempDir()
	relative := "relative.yaml"
	normalized, err := normalizePaths([]string{"", filepath.Join(directory, relative), relative, filepath.Join(directory, relative)})
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != 2 || normalized[0] != filepath.Join(directory, relative) {
		t.Fatalf("normalized = %#v", normalized)
	}
	_, err = normalizePaths([]string{""})
	if code := safeCode(t, err); code != CodeKubeconfigInvalid {
		t.Fatalf("empty normalization code = %q", code)
	}
}
