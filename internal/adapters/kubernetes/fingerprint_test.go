package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintTracksReferencedCertificatesTokensAndAtomicReplacement(t *testing.T) {
	directory := t.TempDir()
	caPath := filepath.Join(directory, "ca.pem")
	certificatePath := filepath.Join(directory, "client.pem")
	keyPath := filepath.Join(directory, "client.key")
	tokenPath := filepath.Join(directory, "token")
	for path, contents := range map[string]string{
		caPath:          "synthetic-ca-v1",
		certificatePath: "synthetic-cert-v1",
		keyPath:         "synthetic-key-v1",
		tokenPath:       "synthetic-token-v1",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := testKubeconfig("https://cluster.invalid", "current")
	config.Clusters["cluster"].CertificateAuthority = filepath.Base(caPath)
	config.AuthInfos["user"].ClientCertificate = filepath.Base(certificatePath)
	config.AuthInfos["user"].ClientKey = filepath.Base(keyPath)
	config.AuthInfos["user"].TokenFile = filepath.Base(tokenPath)
	configPath := filepath.Join(directory, "config.yaml")
	writeTestKubeconfig(t, configPath, config)
	loader := NewLoader(LoaderOptions{LookupEnv: func(string) (string, bool) { return "", false }})
	resolution, err := loader.Resolve(context.Background(), ResolveRequest{ExplicitPath: &configPath, FirstReconcile: true})
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	initial := resolution.Fingerprint()

	if err := os.WriteFile(caPath, []byte("synthetic-ca-v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterCA, err := resolution.CurrentFingerprint(context.Background())
	if err != nil || afterCA == initial {
		t.Fatalf("CA change fingerprint=%s initial=%s err=%v", afterCA, initial, err)
	}
	resolution, err = loader.Resolve(context.Background(), ResolveRequest{ExplicitPath: &configPath, FirstReconcile: true})
	if err != nil {
		t.Fatal(err)
	}
	beforeToken := resolution.Fingerprint()
	if err := os.WriteFile(tokenPath, []byte("synthetic-token-v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterToken, err := resolution.CurrentFingerprint(context.Background())
	if err != nil || afterToken == beforeToken {
		t.Fatalf("token change fingerprint=%s initial=%s err=%v", afterToken, beforeToken, err)
	}

	resolution, err = loader.Resolve(context.Background(), ResolveRequest{ExplicitPath: &configPath, FirstReconcile: true})
	if err != nil {
		t.Fatal(err)
	}
	beforeReplace := resolution.Fingerprint()
	replacement := filepath.Join(directory, "replacement.yaml")
	replacementConfig := testKubeconfig("https://replacement.invalid", "current")
	writeTestKubeconfig(t, replacement, replacementConfig)
	if err := os.Rename(replacement, configPath); err != nil {
		t.Skipf("atomic replacement is unavailable on this filesystem: %v", err)
	}
	afterReplace, err := resolution.CurrentFingerprint(context.Background())
	if err != nil || afterReplace == beforeReplace {
		t.Fatalf("atomic replace fingerprint=%s initial=%s err=%v", afterReplace, beforeReplace, err)
	}
}

func TestFingerprintTracksSymlinkRetargetAndSourceOrder(t *testing.T) {
	directory := t.TempDir()
	firstPath := filepath.Join(directory, "first.yaml")
	secondPath := filepath.Join(directory, "second.yaml")
	writeTestKubeconfig(t, firstPath, testKubeconfig("https://first.invalid", "current"))
	writeTestKubeconfig(t, secondPath, testKubeconfig("https://second.invalid", "current"))
	symlinkPath := filepath.Join(directory, "selected.yaml")
	if err := os.Symlink(firstPath, symlinkPath); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	loader := NewLoader(LoaderOptions{LookupEnv: func(string) (string, bool) { return "", false }})
	resolution, err := loader.Resolve(context.Background(), ResolveRequest{ExplicitPath: &symlinkPath, FirstReconcile: true})
	if err != nil {
		t.Fatalf("resolve symlink: %v", err)
	}
	beforeRetarget := resolution.Fingerprint()
	if err := os.Remove(symlinkPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secondPath, symlinkPath); err != nil {
		t.Fatal(err)
	}
	afterRetarget, err := resolution.CurrentFingerprint(context.Background())
	if err != nil || afterRetarget == beforeRetarget {
		t.Fatalf("symlink retarget fingerprint=%s initial=%s err=%v", afterRetarget, beforeRetarget, err)
	}

	separator := string(os.PathListSeparator)
	environment := firstPath + separator + secondPath
	environmentLoader := NewLoader(LoaderOptions{LookupEnv: func(string) (string, bool) { return environment, true }})
	forward, err := environmentLoader.Resolve(context.Background(), ResolveRequest{FirstReconcile: true})
	if err != nil {
		t.Fatal(err)
	}
	environment = secondPath + separator + firstPath
	reverse, err := environmentLoader.Resolve(context.Background(), ResolveRequest{FirstReconcile: true})
	if err != nil {
		t.Fatal(err)
	}
	if forward.Fingerprint() == reverse.Fingerprint() {
		t.Fatal("reordered KUBECONFIG produced the same transient fingerprint")
	}
	if forward.Descriptor().CacheKey() == reverse.Descriptor().CacheKey() {
		t.Fatal("reordered KUBECONFIG produced the same logical key")
	}
}
