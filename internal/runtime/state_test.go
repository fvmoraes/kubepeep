package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testState(t *testing.T, port int) InstanceStateV1 {
	t.Helper()
	fingerprint, err := CurrentProcessFingerprint()
	if err != nil {
		fingerprint = "test-process-fingerprint"
	}
	state, err := NewInstanceState(os.Getpid(), port, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestInstanceStateAtomicRoundTrip(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "runtime")
	want := testState(t, DefaultFirstPort)
	if err := WriteInstanceStateAtomic(directory, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadInstanceState(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("state = %#v, want %#v", got, want)
	}
	if strings.Contains(string(mustReadFile(t, InstancePath(directory))), ".tmp") {
		t.Fatal("published state contains temporary metadata")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(InstancePath(directory))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("instance mode = %04o", info.Mode().Perm())
		}
	}
}

func TestInstanceStateFormattingNeverDisclosesControlToken(t *testing.T) {
	state := testState(t, DefaultFirstPort)
	formatted := fmt.Sprint(state)
	if strings.Contains(formatted, state.ControlToken) || !strings.Contains(formatted, "<redacted>") {
		t.Fatalf("unsafe state formatting: %q", formatted)
	}
}

func TestLoadInstanceStateRejectsUnknownAndTrailingJSON(t *testing.T) {
	state := testState(t, DefaultFirstPort)
	base := strings.TrimSpace(string(mustMarshalState(t, state)))
	for _, contents := range []string{
		strings.TrimSuffix(base, "}") + `,"extra":true}`,
		base + ` {}`,
	} {
		directory := filepath.Join(t.TempDir(), "runtime")
		if err := EnsureRuntimeDirectory(directory); err != nil {
			t.Fatal(err)
		}
		path := InstancePath(directory)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadInstanceState(directory); !errors.Is(err, ErrInvalidState) {
			t.Fatalf("load error = %v, want ErrInvalidState", err)
		}
	}
}

func TestLoadInstanceStateRejectsInsecureMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not the Windows security boundary")
	}
	directory := filepath.Join(t.TempDir(), "runtime")
	if err := WriteInstanceStateAtomic(directory, testState(t, DefaultFirstPort)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(InstancePath(directory), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadInstanceState(directory); !errors.Is(err, ErrUnsafeState) {
		t.Fatalf("load error = %v, want ErrUnsafeState", err)
	}
}

func TestLoadInstanceStateRejectsInsecureRuntimeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not the Windows security boundary")
	}
	directory := filepath.Join(t.TempDir(), "runtime")
	if err := WriteInstanceStateAtomic(directory, testState(t, DefaultFirstPort)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadInstanceState(directory); !errors.Is(err, ErrUnsafeState) {
		t.Fatalf("load error = %v, want ErrUnsafeState", err)
	}
}

func TestLoadInstanceStateRejectsSymlinkRuntimeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows hosts")
	}
	directory := filepath.Join(t.TempDir(), "runtime")
	if err := WriteInstanceStateAtomic(directory, testState(t, DefaultFirstPort)); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "runtime-link")
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadInstanceState(alias); !errors.Is(err, ErrUnsafeState) {
		t.Fatalf("load error = %v, want ErrUnsafeState", err)
	}
}

func TestWriteInstanceStateRefusesUnsafeReplacementTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not the Windows security boundary")
	}
	directory := filepath.Join(t.TempDir(), "runtime")
	if err := EnsureRuntimeDirectory(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(InstancePath(directory), []byte("untrusted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteInstanceStateAtomic(directory, testState(t, DefaultFirstPort)); !errors.Is(err, ErrUnsafeState) {
		t.Fatalf("write error = %v, want ErrUnsafeState", err)
	}
	contents := mustReadFile(t, InstancePath(directory))
	if string(contents) != "untrusted" {
		t.Fatalf("unsafe replacement target changed: %q", contents)
	}
}

func TestRemoveInstanceStateRefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows hosts")
	}
	directory := filepath.Join(t.TempDir(), "runtime")
	if err := EnsureRuntimeDirectory(directory); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("do not remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, InstancePath(directory)); err != nil {
		t.Fatal(err)
	}
	if err := RemoveInstanceState(directory); !errors.Is(err, ErrUnsafeState) {
		t.Fatalf("remove error = %v, want ErrUnsafeState", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target was affected: %v", err)
	}
}

func mustMarshalState(t *testing.T, state InstanceStateV1) []byte {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "runtime")
	if err := WriteInstanceStateAtomic(directory, state); err != nil {
		t.Fatal(err)
	}
	return mustReadFile(t, InstancePath(directory))
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}
