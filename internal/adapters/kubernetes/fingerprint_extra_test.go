package kubernetes

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveReferenceResolvesRelativeAndAbsolutePaths(t *testing.T) {
	directory := t.TempDir()
	absolute := filepath.Join(directory, "ca.pem")
	origin := filepath.Join(directory, "nested", "config.yaml")
	tests := []struct {
		name      string
		origin    string
		reference string
		want      string
	}{
		{name: "absolute", origin: origin, reference: absolute, want: absolute},
		{name: "relative with origin", origin: origin, reference: "ca.pem", want: filepath.Join(directory, "nested", "ca.pem")},
		{name: "relative with parent traversal", origin: origin, reference: filepath.Join("..", "ca.pem"), want: filepath.Join(directory, "ca.pem")},
		{name: "relative without origin", origin: "", reference: "ca.pem", want: mustAbs(t, "ca.pem")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveReference(test.origin, test.reference); got != test.want {
				t.Fatalf("resolveReference(%q, %q) = %q, want %q", test.origin, test.reference, got, test.want)
			}
		})
	}
}

func mustAbs(t *testing.T, relative string) string {
	t.Helper()
	absolute, err := filepath.Abs(relative)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func TestHashFileRejectsUnusableFiles(t *testing.T) {
	directory := t.TempDir()
	validPath := filepath.Join(directory, "valid.yaml")
	if err := os.WriteFile(validPath, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oversizedPath := filepath.Join(directory, "oversized.yaml")
	if err := os.WriteFile(oversizedPath, []byte(strings.Repeat("a", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(directory, "SECRET_MISSING_FILE")
	directoryPath := filepath.Join(directory, "directory-as-file")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(directory, "link.yaml")
	if err := os.Symlink(validPath, linkPath); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	tests := []struct {
		name    string
		tracked trackedFile
		maxSize int64
		want    ErrorCode
	}{
		{name: "missing source", tracked: trackedFile{path: missingPath, source: true}, want: CodeKubeconfigNotFound},
		{name: "missing reference", tracked: trackedFile{path: missingPath}, want: CodeKubeconfigInvalid},
		{name: "directory reference", tracked: trackedFile{path: directoryPath}, want: CodeKubeconfigInvalid},
		{name: "oversized reference", tracked: trackedFile{path: oversizedPath}, maxSize: 32, want: CodeKubeconfigInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			maxSize := test.maxSize
			if maxSize == 0 {
				maxSize = 4096
			}
			_, err := fingerprintFiles(context.Background(), []trackedFile{test.tracked}, maxSize)
			if code := safeCode(t, err); code != test.want {
				t.Fatalf("code = %q, want %q", code, test.want)
			}
		})
	}

	t.Run("symlink reference", func(t *testing.T) {
		fingerprint, err := fingerprintFiles(context.Background(), []trackedFile{{path: linkPath}}, 4096)
		if err != nil {
			t.Fatal(err)
		}
		if fingerprint == (Fingerprint{}) {
			t.Fatal("symlink fingerprint is empty")
		}
	})

	t.Run("canceled during read", func(t *testing.T) {
		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		hasher := sha256.New()
		err := hashFile(canceled, hasher, trackedFile{path: validPath, source: true}, 4096)
		if code := safeCode(t, err); code != CodeRequestCanceled {
			t.Fatalf("canceled hash code = %q", code)
		}
	})
}

func TestFingerprintFilesGuardsContext(t *testing.T) {
	_, err := fingerprintFiles(nil, nil, 4096)
	if code := safeCode(t, err); code != CodeKubeconfigInvalid {
		t.Fatalf("nil context code = %q", code)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	validPath := filepath.Join(t.TempDir(), "valid.yaml")
	if err := os.WriteFile(validPath, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = fingerprintFiles(canceled, []trackedFile{{path: validPath, source: true}}, 4096)
	if code := safeCode(t, err); code != CodeRequestCanceled {
		t.Fatalf("canceled context code = %q", code)
	}
}

func TestReferencedFilesTrackOnlyFileBasedReferences(t *testing.T) {
	directory := t.TempDir()
	config := testKubeconfig("https://cluster.invalid", "current")
	config.Clusters["cluster"].CertificateAuthorityData = []byte("embedded")
	config.Clusters["cluster"].CertificateAuthority = "ignored-embedded-ca.pem"
	raw := config
	tracked := referencedFiles([]string{filepath.Join(directory, "config.yaml")}, raw)
	if len(tracked) != 1 || !tracked[0].source {
		t.Fatalf("tracked files = %#v", tracked)
	}
	config.AuthInfos["user"].TokenFile = ""
	config.Clusters["cluster"].CertificateAuthorityData = nil
	config.Clusters["cluster"].CertificateAuthority = "ca.pem"
	tracked = referencedFiles([]string{filepath.Join(directory, "config.yaml")}, config)
	if len(tracked) != 2 || tracked[1].label != "cluster-ca:cluster" {
		t.Fatalf("tracked files = %#v", tracked)
	}
}
