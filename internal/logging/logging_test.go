package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fvmoraes/kubepeep/internal/securefs"
)

func TestLoggerUsesAllowlistAndRedactsContent(t *testing.T) {
	t.Parallel()
	directory := privateTempDir(t)
	path := filepath.Join(directory, "kubePeep.log")
	var stdout bytes.Buffer
	logger, sink, err := New(path, &stdout, Options{Level: slog.LevelDebug})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	secret := "Bearer abcdefghijklmnopqrstuvwxyz"
	logger.Info("request\ncompleted",
		"component", "http",
		"request_id", "req_123",
		"resource", secret,
		"error", "token=must-not-leak",
		"headers", "Authorization: must-not-leak",
	)

	assertSanitizedLine(t, stdout.String(), secret)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertSanitizedLine(t, string(content), secret)
}

func TestSinkRotatesAndLimitsBackups(t *testing.T) {
	t.Parallel()
	directory := privateTempDir(t)
	path := filepath.Join(directory, "kubePeep.log")
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	logger, sink, err := New(path, &bytes.Buffer{}, Options{
		MaxBytes:   180,
		MaxBackups: 2,
		MaxAge:     time.Hour,
		Now:        func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 12; index++ {
		logger.Info("rotation", "component", "test", "resource", strings.Repeat("x", 48))
		now = now.Add(time.Nanosecond)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected current log and two backups, got %d", len(entries))
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s permissions = %o, want 600", entry.Name(), got)
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range bytes.Split(bytes.TrimSpace(content), []byte{'\n'}) {
			var value map[string]any
			if err := json.Unmarshal(line, &value); err != nil {
				t.Fatalf("%s is not JSONL: %v", entry.Name(), err)
			}
		}
	}
}

func TestSinkDefaultLimitsAndSanitizationAcrossFiveBackups(t *testing.T) {
	t.Parallel()
	directory := privateTempDir(t)
	path := filepath.Join(directory, "kubePeep.log")
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	logger, sink, err := New(path, &bytes.Buffer{}, Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if sink.maxBytes != 10<<20 || sink.maxBackups != 5 || sink.maxAge != 14*24*time.Hour {
		t.Fatalf("unexpected defaults: bytes=%d backups=%d age=%s", sink.maxBytes, sink.maxBackups, sink.maxAge)
	}
	// Keep the production defaults asserted above, then lower only the byte
	// threshold so the test can force more than five rotations economically.
	sink.maxBytes = 180
	secret := "Bearer synthetic-secret-that-must-not-survive-rotation"
	for index := 0; index < 40; index++ {
		logger.Info("default rotation", "component", "test", "resource", secret)
		now = now.Add(time.Nanosecond)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 6 {
		t.Fatalf("expected current log and five default backups, got %d", len(entries))
	}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(content, []byte(secret)) || bytes.Contains(content, []byte("synthetic-secret")) {
			t.Fatalf("sensitive value survived in %s", entry.Name())
		}
		if !bytes.Contains(content, []byte(redactedValue)) {
			t.Fatalf("redaction marker absent from %s", entry.Name())
		}
		for _, line := range bytes.Split(bytes.TrimSpace(content), []byte{'\n'}) {
			var value map[string]any
			if err := json.Unmarshal(line, &value); err != nil {
				t.Fatalf("%s is not JSONL: %v", entry.Name(), err)
			}
		}
	}
}

func TestSinkFailsClosedWhenPublishedPathIsReplaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink requires privileges on some Windows hosts")
	}
	directory := privateTempDir(t)
	path := filepath.Join(directory, "kubePeep.log")
	victim := filepath.Join(directory, "victim.log")
	if err := os.WriteFile(victim, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	logger, sink, err := New(path, &stdout, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Close() })
	logger.Info("prime", "component", "test")
	owned := path + ".owned"
	if err := os.Rename(path, owned); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}
	logger.Info("must fail closed", "component", "test")
	if sink.Healthy() {
		t.Fatal("sink remained healthy after its pathname was replaced")
	}
	content, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "unchanged" {
		t.Fatalf("replacement target was modified: %q", content)
	}
}

func TestSinkRemovesExpiredBackupsAtStartup(t *testing.T) {
	t.Parallel()
	directory := privateTempDir(t)
	path := filepath.Join(directory, "kubePeep.log")
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	oldBackup := path + ".20260701T000000.000000000Z.001"
	recentBackup := path + ".20260803T110000.000000000Z.001"
	for _, backup := range []string{oldBackup, recentBackup} {
		if err := os.WriteFile(backup, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(oldBackup, now.Add(-15*24*time.Hour), now.Add(-15*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recentBackup, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, sink, err := New(path, &bytes.Buffer{}, Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldBackup); !os.IsNotExist(err) {
		t.Fatalf("expired backup survived: %v", err)
	}
	if _, err := os.Stat(recentBackup); err != nil {
		t.Fatalf("recent backup was removed: %v", err)
	}
}

func TestRotationFailureKeepsSanitizedStdoutAndRecovers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write permissions are not represented by POSIX mode on Windows")
	}
	directory := privateTempDir(t)
	path := filepath.Join(directory, "kubePeep.log")
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	var stdout bytes.Buffer
	degraded := 0
	logger, sink, err := New(path, &stdout, Options{
		MaxBytes:     256,
		Now:          func() time.Time { return now },
		RetryBackoff: time.Second,
		OnDegraded:   func(error) { degraded++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(directory, 0o700)
		_ = sink.Close()
	})
	logger.Info("prime", "component", "test", "resource", strings.Repeat("x", 120))
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	secret := "Bearer synthetic-token-that-must-not-leak"
	logger.Info("rotate while unavailable", "component", "test", "resource", secret)
	if sink.Healthy() || degraded != 1 {
		t.Fatalf("rotation failure was not reported: healthy=%v callbacks=%d", sink.Healthy(), degraded)
	}
	if strings.Contains(stdout.String(), secret) || !strings.Contains(stdout.String(), redactedValue) {
		t.Fatalf("stdout fallback was not sanitized: %s", stdout.String())
	}
	logger.Info("rate limited retry", "component", "test")
	if degraded != 1 {
		t.Fatalf("degradation callback was not rate limited: %d", degraded)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	logger.Info("recovered", "component", "test")
	if !sink.Healthy() {
		t.Fatal("sink did not recover after backoff")
	}
}

func TestObservabilitySchemaOmitsUnavailableFields(t *testing.T) {
	t.Parallel()
	path := filepath.Join(privateTempDir(t), "kubePeep.log")
	var stdout bytes.Buffer
	logger, sink, err := New(path, &stdout, Options{})
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("resource.list",
		"component", "api",
		"request_id", "req_schema",
		"context", "dev",
		"namespace", "payments",
		"resource", "pods",
		"duration", 25*time.Millisecond,
		"error_code", "NONE",
	)
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &entry); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"timestamp", "level", "component", "operation", "request_id", "context", "namespace", "resource", "duration", "error_code"} {
		if _, ok := entry[field]; !ok {
			t.Fatalf("required applicable field %q is absent: %#v", field, entry)
		}
	}
}

func assertSanitizedLine(t *testing.T, line, secret string) {
	t.Helper()
	if strings.Contains(line, secret) || strings.Contains(line, "must-not-leak") {
		t.Fatalf("sensitive marker leaked: %s", line)
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &entry); err != nil {
		t.Fatal(err)
	}
	for key := range entry {
		if key == "timestamp" || key == "level" {
			continue
		}
		if _, ok := allowedFields[key]; !ok {
			t.Fatalf("unexpected field %q", key)
		}
	}
	if entry["resource"] != redactedValue {
		t.Fatalf("resource was not redacted: %#v", entry["resource"])
	}
	if entry["operation"] != "request completed" {
		t.Fatalf("operation was not normalized: %#v", entry["operation"])
	}
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := securefs.EnsurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	return directory
}
