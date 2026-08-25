//go:build windows

package updater

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestWindowsUpdateTransfersStagedCandidateOwnershipToHelper(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "kubePeep.exe")
	candidate := filepath.Join(root, "candidate.exe")
	buildWindowsVersionFixture(t, target, "0.1.0", false)
	buildWindowsVersionFixture(t, candidate, "0.2.0", false)
	candidateBytes, err := os.ReadFile(candidate)
	if err != nil {
		t.Fatal(err)
	}
	archiveName := "kubePeep_0.2.0_windows_amd64.zip"
	archive := zipArchive(t, "kubePeep.exe", candidateBytes)
	server := releaseServer(t, archiveName, archive, validChecksums(archiveName, archive), nil)

	process := exec.Command(os.Args[0], "-test.run=^TestWindowsUpdateProcessHelper$")
	process.Env = append(os.Environ(),
		"KUBEPEEP_WINDOWS_UPDATE_HELPER=1",
		"KUBEPEEP_WINDOWS_UPDATE_TARGET="+target,
		"KUBEPEEP_WINDOWS_UPDATE_SERVER="+server.URL,
	)
	if output, runErr := process.CombinedOutput(); runErr != nil {
		t.Fatalf("update subprocess: %v output=%s", runErr, output)
	}

	status := filepath.Join(root, ".kubePeep.update-result")
	waitForWindowsStatus(t, status, "installed version=0.2.0")
	if got := executableVersion(t, target); !strings.Contains(got, "version=0.2.0") {
		t.Fatalf("installed version=%q", got)
	}
}

func TestWindowsUpdateProcessHelper(t *testing.T) {
	if os.Getenv("KUBEPEEP_WINDOWS_UPDATE_HELPER") != "1" {
		return
	}
	service, err := New(Options{
		ReleaseBaseURL:  os.Getenv("KUBEPEEP_WINDOWS_UPDATE_SERVER"),
		AllowHTTP:       true,
		ExecutablePath:  func() (string, error) { return os.Getenv("KUBEPEEP_WINDOWS_UPDATE_TARGET"), nil },
		OperatingSystem: "windows",
		Architecture:    "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Update(context.Background(), Request{CurrentVersion: "0.1.0", TargetVersion: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Scheduled {
		t.Fatal("Windows replacement was not scheduled")
	}
}

func TestWindowsHelperWaitsForParentThenAtomicallyReplaces(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "kubePeep.exe")
	candidate := filepath.Join(root, ".kubePeep.update.fixture.exe")
	buildWindowsVersionFixture(t, target, "0.1.0", false)
	buildWindowsVersionFixture(t, candidate, "0.2.0", false)
	backup := filepath.Join(root, ".kubePeep.backup.fixture.exe")
	failed := backup + ".failed"
	status := filepath.Join(root, ".kubePeep.update-result")
	lock := filepath.Join(root, ".kubePeep.update.lock")
	if err := os.WriteFile(status, []byte("installed version=stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	currentHash := hashHex(t, target)
	candidateHash := hashHex(t, candidate)
	parent := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Milliseconds 700")
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	if err := launchWindowsReplacement(target, candidate, backup, failed, status, lock, "0.2.0", currentHash, candidateHash, parent.Process.Pid); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Lstat(status); !os.IsNotExist(err) {
		t.Fatalf("stale update status survived helper launch: %v", err)
	}
	if got := executableVersion(t, target); !strings.Contains(got, "version=0.1.0") {
		t.Fatalf("replacement ran before parent exit: %q", got)
	}
	_ = parent.Wait()
	waitForWindowsStatus(t, status, "installed version=0.2.0")
	if got := executableVersion(t, target); !strings.Contains(got, "version=0.2.0") {
		t.Fatalf("installed version=%q", got)
	}
	for _, path := range []string{candidate, backup, failed, lock} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("transaction path survived: %s err=%v", path, err)
		}
	}
}

func TestWindowsHelperRollsBackFailedPostReplacementVerification(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "kubePeep.exe")
	candidate := filepath.Join(root, ".kubePeep.update.fixture.exe")
	buildWindowsVersionFixture(t, target, "0.1.0", false)
	buildWindowsVersionFixture(t, candidate, "0.2.0", true)
	backup := filepath.Join(root, ".kubePeep.backup.fixture.exe")
	failed := backup + ".failed"
	status := filepath.Join(root, ".kubePeep.update-result")
	lock := filepath.Join(root, ".kubePeep.update.lock")
	if err := os.WriteFile(lock, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	currentHash := hashHex(t, target)
	candidateHash := hashHex(t, candidate)
	if err := launchWindowsReplacement(target, candidate, backup, failed, status, lock, "0.2.0", currentHash, candidateHash, 0); err != nil {
		t.Fatal(err)
	}
	waitForWindowsStatus(t, status, "rolled-back version=0.2.0")
	if got := executableVersion(t, target); !strings.Contains(got, "version=0.1.0") {
		t.Fatalf("rollback version=%q", got)
	}
}

func TestWindowsHelperRejectsCandidateChangedWhileWaitingForParent(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "kubePeep.exe")
	candidate := filepath.Join(root, ".kubePeep.update.fixture.exe")
	buildWindowsVersionFixture(t, target, "0.1.0", false)
	buildWindowsVersionFixture(t, candidate, "0.2.0", false)
	backup := filepath.Join(root, ".kubePeep.backup.fixture.exe")
	failed := backup + ".failed"
	status := filepath.Join(root, ".kubePeep.update-result")
	lock := filepath.Join(root, ".kubePeep.update.lock")
	if err := os.WriteFile(lock, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	currentHash := hashHex(t, target)
	candidateHash := hashHex(t, candidate)
	parent := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Milliseconds 700")
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	if err := launchWindowsReplacement(target, candidate, backup, failed, status, lock, "0.2.0", currentHash, candidateHash, parent.Process.Pid); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if err := os.WriteFile(candidate, []byte("tampered candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = parent.Wait()
	waitForWindowsFailureStatus(t, status, "0.2.0", "replace-source-hash")
	if got := executableVersion(t, target); !strings.Contains(got, "version=0.1.0") {
		t.Fatalf("candidate mutation changed installed version=%q", got)
	}
	for _, path := range []string{candidate, backup, failed, lock} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("transaction path survived: %s err=%v", path, err)
		}
	}
}

func TestWindowsHelperTimesOutVersionCheckAndRollsBack(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "kubePeep.exe")
	candidate := filepath.Join(root, ".kubePeep.update.fixture.exe")
	buildWindowsVersionFixture(t, target, "0.1.0", false)
	buildWindowsHangingVersionFixture(t, candidate)
	backup := filepath.Join(root, ".kubePeep.backup.fixture.exe")
	failed := backup + ".failed"
	status := filepath.Join(root, ".kubePeep.update-result")
	lock := filepath.Join(root, ".kubePeep.update.lock")
	if err := os.WriteFile(lock, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := launchWindowsReplacement(target, candidate, backup, failed, status, lock, "0.2.0", hashHex(t, target), hashHex(t, candidate), 0); err != nil {
		t.Fatal(err)
	}
	waitForWindowsStatus(t, status, "rolled-back version=0.2.0")
	if got := executableVersion(t, target); !strings.Contains(got, "version=0.1.0") {
		t.Fatalf("timeout rollback version=%q", got)
	}
}

func buildWindowsVersionFixture(t *testing.T, destination, version string, fail bool) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "main.go")
	program := `package main
import (
    "fmt"
    "os"
)
func main() {
	if ` + fmt.Sprint(fail) + ` {
        os.Exit(9)
    }
    if len(os.Args) == 2 && os.Args[1] == "version" {
        fmt.Println("version=` + version + ` commit=fixture build_date=fixture")
        return
    }
    os.Exit(2)
}
`
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "build", "-trimpath", "-o", destination, source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Windows fixture: %v output=%s", err, output)
	}
}

func buildWindowsHangingVersionFixture(t *testing.T, destination string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "main.go")
	program := `package main
import "time"
func main() { time.Sleep(time.Minute) }
`
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "build", "-trimpath", "-o", destination, source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build hanging Windows fixture: %v output=%s", err, output)
	}
}

func hashHex(t *testing.T, path string) string {
	t.Helper()
	hash, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash)
}

func waitForWindowsStatus(t *testing.T, path, expected string) {
	t.Helper()
	content := waitForWindowsStatusValue(t, path)
	if content != expected {
		t.Fatalf("status=%q want=%q", content, expected)
	}
}

func waitForWindowsFailureStatus(t *testing.T, path, version, stage string) {
	t.Helper()
	content := waitForWindowsStatusValue(t, path)
	pattern := regexp.MustCompile("^failed version=" + regexp.QuoteMeta(version) +
		" stage=" + regexp.QuoteMeta(stage) +
		` type=[A-Za-z][A-Za-z0-9]{0,63} hresult=0x[0-9A-F]{8}$`)
	if !pattern.MatchString(content) {
		t.Fatalf("unsafe or unexpected failure status=%q", content)
	}
}

func waitForWindowsStatusValue(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil {
			return string(content)
		}
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return ""
}
