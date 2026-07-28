//go:build linux

package control

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestControlProbeSIGTERMCleansStateAndReleasesLock(t *testing.T) {
	binary := controlProbeBinary(t)
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	stdoutPath := filepath.Join(t.TempDir(), "start.stdout")
	stderrPath := filepath.Join(t.TempDir(), "start.stderr")
	stdout := createOutputFile(t, stdoutPath)
	stderr := createOutputFile(t, stderrPath)

	start := exec.Command(
		binary,
		"--runtime-dir", runtimeDir,
		"start",
		"--port", "0",
		"--shutdown-timeout", "1s",
	)
	start.Stdout = stdout
	start.Stderr = stderr
	if err := start.Start(); err != nil {
		t.Fatalf("start probe: %v", err)
	}
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- start.Wait()
		close(waitResult)
	}()
	t.Cleanup(func() {
		select {
		case <-waitResult:
		default:
			_ = start.Process.Kill()
			<-waitResult
		}
		_ = stdout.Close()
		_ = stderr.Close()
	})

	_ = waitForInstance(t, runtimeDir, waitResult, stdoutPath, stderrPath)
	if err := start.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	select {
	case err := <-waitResult:
		if err != nil {
			t.Fatalf(
				"probe exit after SIGTERM: %v\nstdout:\n%s\nstderr:\n%s",
				err,
				readOutput(t, stdoutPath),
				readOutput(t, stderrPath),
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not exit after SIGTERM")
	}

	if _, err := os.Stat(InstancePath(runtimeDir)); !os.IsNotExist(err) {
		t.Fatalf("instance state remains after SIGTERM: %v", err)
	}
	lock, err := AcquireFileLock(LockPath(runtimeDir))
	if err != nil {
		t.Fatalf("lock was not released after SIGTERM: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("release reacquired lock: %v", err)
	}
}
