package updater

import (
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"
)

// The helper's filesystem completion protocol can be tested on every OS.
func TestWindowsStatusWaitsForTransactionCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root := t.TempDir()
		status := filepath.Join(root, ".kubePeep.update-result")
		lock := filepath.Join(root, ".kubePeep.update.lock")
		const expected = "installed version=0.2.0"
		if err := os.WriteFile(lock, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(status, []byte(expected), 0o600); err != nil {
			t.Fatal(err)
		}
		completed := make(chan string, 1)
		go func() { completed <- waitForWindowsStatusValue(t, status) }()
		synctest.Wait()
		select {
		case <-completed:
			t.Fatal("status was accepted before transaction cleanup released the lock")
		default:
		}
		if err := os.Remove(lock); err != nil {
			t.Fatal(err)
		}
		if got := <-completed; got != expected {
			t.Fatalf("status=%q want=%q", got, expected)
		}
	})
}

func waitForWindowsStatusValue(t *testing.T, path string) string {
	t.Helper()
	lock := filepath.Join(filepath.Dir(path), ".kubePeep.update.lock")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		// The helper writes status before its finally block removes the
		// candidate and transaction lock. Status alone is not completion.
		_, lockErr := os.Lstat(lock)
		if lockErr != nil && !os.IsNotExist(lockErr) {
			t.Fatal(lockErr)
		}
		if os.IsNotExist(lockErr) {
			content, err := os.ReadFile(path)
			if err == nil {
				return string(content)
			}
			if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status %s and transaction lock cleanup %s", path, lock)
	return ""
}
