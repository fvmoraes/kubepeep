package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestUpdateDownloadsOneExactTagAndAtomicallyReplacesExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the native Windows post-exit transaction has a dedicated test")
	}
	target, dataSentinel := writeInstalledFixture(t)
	archiveName := "kubePeep_0.2.0_linux_amd64.tar.gz"
	archive := tarArchive(t, "kubePeep", versionScript("0.2.0", false))
	requests := []string{}
	server := releaseServer(t, archiveName, archive, validChecksums(archiveName, archive), &requests)
	service := testService(t, server.URL, target, "linux", "amd64")

	result, err := service.Update(context.Background(), Request{CurrentVersion: "0.1.0", TargetVersion: "v0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetVersion != "0.2.0" || result.Archive != archiveName || result.Scheduled {
		t.Fatalf("result=%#v", result)
	}
	if got := executableVersion(t, target); !strings.Contains(got, "version=0.2.0") {
		t.Fatalf("installed version=%q", got)
	}
	if content, err := os.ReadFile(dataSentinel); err != nil || string(content) != "preserve-me\n" {
		t.Fatalf("data sentinel content=%q err=%v", content, err)
	}
	wantRequests := []string{"/v0.2.0/checksums.txt", "/v0.2.0/" + archiveName}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Fatalf("requests=%v want=%v", requests, wantRequests)
	}
	assertNoTransactionFiles(t, filepath.Dir(target))
}

func TestUpdateRejectsInvalidChecksumBeforeReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform replacement is scheduled on Windows")
	}
	target, dataSentinel := writeInstalledFixture(t)
	archiveName := "kubePeep_0.2.0_linux_amd64.tar.gz"
	archive := tarArchive(t, "kubePeep", versionScript("0.2.0", false))
	server := releaseServer(t, archiveName, archive, strings.Repeat("0", 64)+"  "+archiveName+"\n", nil)
	service := testService(t, server.URL, target, "linux", "amd64")

	_, err := service.Update(context.Background(), Request{CurrentVersion: "0.1.0", TargetVersion: "0.2.0"})
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("error=%v", err)
	}
	if got := executableVersion(t, target); !strings.Contains(got, "version=0.1.0") {
		t.Fatalf("installed version changed: %q", got)
	}
	if content, readErr := os.ReadFile(dataSentinel); readErr != nil || string(content) != "preserve-me\n" {
		t.Fatalf("data sentinel content=%q err=%v", content, readErr)
	}
	assertNoTransactionFiles(t, filepath.Dir(target))
}

func TestUpdateRollsBackWhenReplacementFailsPostInstallVerification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the native Windows post-exit transaction has a dedicated rollback test")
	}
	target, dataSentinel := writeInstalledFixture(t)
	archiveName := "kubePeep_0.2.0_linux_amd64.tar.gz"
	archive := tarArchive(t, "kubePeep", versionScript("0.2.0", true))
	server := releaseServer(t, archiveName, archive, validChecksums(archiveName, archive), nil)
	service := testService(t, server.URL, target, "linux", "amd64")

	_, err := service.Update(context.Background(), Request{CurrentVersion: "0.1.0", TargetVersion: "0.2.0"})
	if !errors.Is(err, ErrRollback) {
		t.Fatalf("error=%v", err)
	}
	if got := executableVersion(t, target); !strings.Contains(got, "version=0.1.0") {
		t.Fatalf("rollback version=%q", got)
	}
	if content, readErr := os.ReadFile(dataSentinel); readErr != nil || string(content) != "preserve-me\n" {
		t.Fatalf("data sentinel content=%q err=%v", content, readErr)
	}
	assertNoTransactionFiles(t, filepath.Dir(target))
}

func TestUpdateRejectsMissingAndDuplicateChecksumEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform replacement is scheduled on Windows")
	}
	archiveName := "kubePeep_0.2.0_linux_amd64.tar.gz"
	archive := tarArchive(t, "kubePeep", versionScript("0.2.0", false))
	valid := validChecksums(archiveName, archive)
	for _, test := range []struct {
		name      string
		checksums string
	}{
		{name: "missing", checksums: strings.Repeat("a", 64) + "  another.tar.gz\n"},
		{name: "duplicate", checksums: valid + valid},
		{name: "malformed", checksums: "not-a-sha  " + archiveName + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			target, _ := writeInstalledFixture(t)
			server := releaseServer(t, archiveName, archive, test.checksums, nil)
			service := testService(t, server.URL, target, "linux", "amd64")
			if _, err := service.Update(context.Background(), Request{CurrentVersion: "0.1.0", TargetVersion: "0.2.0"}); err == nil {
				t.Fatal("update accepted invalid checksum list")
			}
			if got := executableVersion(t, target); !strings.Contains(got, "version=0.1.0") {
				t.Fatalf("installed version changed: %q", got)
			}
		})
	}
}

func TestUpdateRejectsUnsafeOrMissingArchiveExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform replacement is scheduled on Windows")
	}
	for _, test := range []struct {
		name    string
		archive []byte
	}{
		{name: "missing", archive: tarArchive(t, "README.md", []byte("not executable"))},
		{name: "symlink", archive: tarSymlinkArchive(t, "kubePeep", "../../outside")},
		{name: "duplicate", archive: tarDuplicateArchive(t, "kubePeep", versionScript("0.2.0", false))},
	} {
		t.Run(test.name, func(t *testing.T) {
			target, _ := writeInstalledFixture(t)
			archiveName := "kubePeep_0.2.0_linux_amd64.tar.gz"
			server := releaseServer(t, archiveName, test.archive, validChecksums(archiveName, test.archive), nil)
			service := testService(t, server.URL, target, "linux", "amd64")
			if _, err := service.Update(context.Background(), Request{CurrentVersion: "0.1.0", TargetVersion: "0.2.0"}); err == nil {
				t.Fatal("update accepted unsafe archive")
			}
		})
	}
}

func TestUpdateUnderstandsPublishedWindowsZipName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the native Windows transaction is tested separately")
	}
	target, _ := writeInstalledFixture(t)
	archiveName := "kubePeep_0.2.0_windows_arm64.zip"
	archive := zipArchive(t, "kubePeep.exe", versionScript("0.2.0", false))
	server := releaseServer(t, archiveName, archive, validChecksums(archiveName, archive), nil)
	service := testService(t, server.URL, target, "windows", "arm64")
	result, err := service.Update(context.Background(), Request{CurrentVersion: "0.1.0", TargetVersion: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Archive != archiveName {
		t.Fatalf("archive=%q", result.Archive)
	}
}

func TestUpdateRejectsMutableOrAmbiguousVersionsWithoutNetwork(t *testing.T) {
	for _, version := range []string{"", "latest", "main", "1.2", "1.2.3-rc1", "01.2.3", "1.02.3", "1.2.03", "v1.2.3/asset"} {
		t.Run(strings.ReplaceAll(version, "/", "_"), func(t *testing.T) {
			called := false
			service, err := New(Options{
				Client: HTTPDoerFunc(func(*http.Request) (*http.Response, error) {
					called = true
					return nil, errors.New("unexpected network")
				}),
				ExecutablePath: func() (string, error) { return "", errors.New("unexpected executable lookup") },
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Update(context.Background(), Request{CurrentVersion: "0.1.0", TargetVersion: version}); !errors.Is(err, ErrInvalidVersion) {
				t.Fatalf("version=%q error=%v", version, err)
			}
			if called {
				t.Fatal("invalid version caused network access")
			}
		})
	}
}

func TestUpdateNoOpsWhenExactVersionIsAlreadyInstalled(t *testing.T) {
	service, err := New(Options{ExecutablePath: func() (string, error) {
		return "", errors.New("executable must not be inspected")
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Update(context.Background(), Request{CurrentVersion: "v1.2.3", TargetVersion: "1.2.3"})
	if err != nil || !result.AlreadyCurrent {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestUpdateRejectsConcurrentTransactionBeforeNetwork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform replacement is scheduled on Windows")
	}
	target, _ := writeInstalledFixture(t)
	lock := filepath.Join(filepath.Dir(target), ".kubePeep.update.lock")
	if err := os.WriteFile(lock, []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	service, err := New(Options{
		Client: HTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected network")
		}),
		ExecutablePath:  func() (string, error) { return target, nil },
		OperatingSystem: "linux", Architecture: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), Request{CurrentVersion: "0.1.0", TargetVersion: "0.2.0"}); !errors.Is(err, ErrConcurrent) {
		t.Fatalf("error=%v", err)
	}
	if called {
		t.Fatal("concurrent transaction caused network access")
	}
}

func TestStageCandidateStripsGroupAndWorldWritePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix executable permission bits")
	}
	root := t.TempDir()
	source := filepath.Join(root, "release")
	if err := os.WriteFile(source, []byte("candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "kubePeep")
	staged, err := stageCandidate(source, target, 0o777)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(staged) })
	info, err := os.Stat(staged)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
		t.Fatalf("staged permissions=%#o want=%#o", got, want)
	}
}

type HTTPDoerFunc func(*http.Request) (*http.Response, error)

func (function HTTPDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func testService(t *testing.T, baseURL, target, operatingSystem, architecture string) *Service {
	t.Helper()
	service, err := New(Options{
		ReleaseBaseURL:  baseURL,
		AllowHTTP:       true,
		ExecutablePath:  func() (string, error) { return target, nil },
		OperatingSystem: operatingSystem,
		Architecture:    architecture,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func writeInstalledFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	installDirectory := filepath.Join(root, "bin")
	dataDirectory := filepath.Join(root, "data")
	if err := os.MkdirAll(installDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(installDirectory, "kubePeep")
	if err := os.WriteFile(target, versionScript("0.1.0", false), 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dataDirectory, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve-me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return target, sentinel
}

func versionScript(version string, failAtCanonicalTarget bool) []byte {
	failure := ""
	if failAtCanonicalTarget {
		failure = "if [ \"${0##*/}\" = kubePeep ]; then exit 9; fi\n"
	}
	return []byte("#!/bin/sh\n" + failure + "if [ \"${1:-}\" = version ]; then printf '%s\\n' 'version=" + version + " commit=fixture build_date=fixture'; exit 0; fi\nexit 1\n")
}

func executableVersion(t *testing.T, path string) string {
	t.Helper()
	output, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("execute %s: %v output=%q", path, err, output)
	}
	return string(output)
}

func releaseServer(t *testing.T, archiveName string, archive []byte, checksums string, requests *[]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if requests != nil {
			*requests = append(*requests, request.URL.Path)
		}
		switch request.URL.Path {
		case "/v0.2.0/checksums.txt":
			_, _ = io.WriteString(response, checksums)
		case "/v0.2.0/" + archiveName:
			_, _ = response.Write(archive)
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func validChecksums(archiveName string, archive []byte) string {
	hash := sha256.Sum256(archive)
	return hex.EncodeToString(hash[:]) + "  " + archiveName + "\n"
}

func tarArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func tarSymlinkArchive(t *testing.T, name, target string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Linkname: target, Mode: 0o755, Typeflag: tar.TypeSymlink}); err != nil {
		t.Fatal(err)
	}
	_ = tarWriter.Close()
	_ = gzipWriter.Close()
	return buffer.Bytes()
}

func tarDuplicateArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for range 2 {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	_ = tarWriter.Close()
	_ = gzipWriter.Close()
	return buffer.Bytes()
}

func zipArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o755)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func assertNoTransactionFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".kubePeep.update.") || strings.HasPrefix(entry.Name(), ".kubePeep.backup.") || entry.Name() == ".kubePeep.update.lock" {
			t.Fatalf("transaction file survived: %s", entry.Name())
		}
	}
}
