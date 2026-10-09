package kubeconfigimport

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/services/clusterprofiles"
	"k8s.io/client-go/tools/clientcmd"
	clientapi "k8s.io/client-go/tools/clientcmd/api"
)

func fixture(t *testing.T, name string, customize func(*clientapi.Config)) string {
	t.Helper()
	c := clientapi.NewConfig()
	c.Clusters[name] = &clientapi.Cluster{Server: "https://example.invalid"}
	c.AuthInfos[name] = &clientapi.AuthInfo{Token: "test-only-credential"}
	c.Contexts[name] = &clientapi.Context{Cluster: name, AuthInfo: name}
	c.CurrentContext = name
	if customize != nil {
		customize(c)
	}
	data, err := clientcmd.Write(*c)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestImportMergesPrivatelyWithoutChangingCurrentContextOrCredentials(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	i := &Importer{Home: home}
	for _, name := range []string{"production", "development", "development"} {
		path, err := i.Import(t.Context(), clusterprofiles.ImportRequest{Content: fixture(t, name, nil)})
		if err != nil {
			t.Fatal(err)
		}
		if path != filepath.Join(home, ".kube", "config") {
			t.Fatal("unexpected destination")
		}
	}
	path := filepath.Join(home, ".kube", "config")
	config, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Contexts) != 2 || config.CurrentContext != "production" || config.AuthInfos["production"].Token != "test-only-credential" {
		t.Fatal("existing context was changed")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o", info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal("temporary files were left behind")
	}
}

func TestImportConflictAndInvalidInputPreserveOriginalBytes(t *testing.T) {
	t.Parallel()
	i := &Importer{Home: t.TempDir()}
	path, err := i.Import(t.Context(), clusterprofiles.ImportRequest{Content: fixture(t, "production", nil)})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, content string
		want          error
	}{
		{"conflict", fixture(t, "production", func(c *clientapi.Config) { c.Clusters["production"].Server = "https://different.invalid" }), clusterprofiles.ErrImportConflict},
		{"invalid", "not a kubeconfig: [test-only-credential", clusterprofiles.ErrImportInvalid},
		{"relative certificate", fixture(t, "development", func(c *clientapi.Config) { c.Clusters["development"].CertificateAuthority = "certs/ca.pem" }), clusterprofiles.ErrImportRelativePaths},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := i.Import(t.Context(), clusterprofiles.ImportRequest{Content: tc.content})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v", err)
			}
			if bytes.Contains([]byte(err.Error()), []byte("test-only-credential")) {
				t.Fatal("credential leaked in error")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("existing kubeconfig was modified")
			}
		})
	}
}

func TestLocalPathKeepsRelativeReferencesWithoutCopyingOrExecutingPlugins(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, "original.yaml")
	content := fixture(t, "development", func(c *clientapi.Config) {
		c.Clusters["development"].CertificateAuthority = "certs/ca.pem"
		c.AuthInfos["development"] = &clientapi.AuthInfo{Exec: &clientapi.ExecConfig{Command: "nonexistent-command-must-not-execute", APIVersion: "client.authentication.k8s.io/v1", InteractiveMode: clientapi.NeverExecInteractiveMode}}
	})
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	i := &Importer{Home: home}
	got, err := i.Import(t.Context(), clusterprofiles.ImportRequest{Path: "~/original.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatal("original source was not retained")
	}
	if _, err := os.Stat(filepath.Join(home, ".kube", "config")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("path registration copied the file")
	}
}

func TestImportRejectsDestinationSymlinkAndOversize(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(outside, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, ".kube"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".kube", "config")); err != nil {
		t.Skip("symlinks unavailable")
	}
	i := &Importer{Home: home}
	if _, err := i.Import(t.Context(), clusterprofiles.ImportRequest{Content: fixture(t, "development", nil)}); err == nil {
		t.Fatal("accepted destination symlink")
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "untouched" {
		t.Fatal("symlink target changed")
	}
	if _, err := i.Import(t.Context(), clusterprofiles.ImportRequest{Content: string(bytes.Repeat([]byte("x"), maxBytes+1))}); !errors.Is(err, clusterprofiles.ErrImportInvalid) {
		t.Fatalf("oversize: %v", err)
	}
}
