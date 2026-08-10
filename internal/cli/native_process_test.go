package cli

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNativeBinaryLifecycleBlackBox(t *testing.T) {
	if os.Getenv("KUBEPEEP_NATIVE_BLACKBOX") != "1" {
		t.Skip("set KUBEPEEP_NATIVE_BLACKBOX=1 on an isolated native runner")
	}
	_, source, _, ok := stdruntime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test source path")
	}
	repository := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	executable := filepath.Join(t.TempDir(), "kubePeep")
	if stdruntime.GOOS == "windows" {
		executable += ".exe"
	}
	build := exec.Command("go", "build", "-trimpath", "-o", executable, "./cmd/kubePeep")
	build.Dir = repository
	build.Env = replaceProcessEnvironment(os.Environ(), map[string]string{"CGO_ENABLED": "0", "GOTOOLCHAIN": "go1.25.12"})
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native binary: %v\n%s", err, output)
	}

	testHome := t.TempDir()
	processEnvironment := replaceProcessEnvironment(os.Environ(), map[string]string{
		"HOME":         testHome,
		"USERPROFILE":  testHome,
		"LOCALAPPDATA": testHome,
	})
	port := nativeAvailablePort(t)
	start := exec.Command(executable, "start", "--no-browser", "--port", strconv.Itoa(port))
	start.Env = processEnvironment
	stdout, err := start.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	start.Stderr = &stderr
	if err := start.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if start.Process != nil && (start.ProcessState == nil || !start.ProcessState.Exited()) {
			_ = start.Process.Kill()
			_, _ = start.Process.Wait()
		}
	})

	ready := make(chan struct{})
	scanDone := make(chan error, 1)
	var readyOnce sync.Once
	var outputMu sync.Mutex
	var observed strings.Builder
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			outputMu.Lock()
			observed.WriteString(line)
			observed.WriteByte('\n')
			outputMu.Unlock()
			if strings.HasPrefix(line, "running pid=") {
				readyOnce.Do(func() { close(ready) })
			}
		}
		scanDone <- scanner.Err()
	}()
	select {
	case <-ready:
	case <-time.After(15 * time.Second):
		outputMu.Lock()
		output := observed.String()
		outputMu.Unlock()
		t.Fatalf("native binary did not become ready; stdout=%q stderr=%q", output, stderr.String())
	}

	if code, output := runNativeCommand(t, executable, processEnvironment, "status"); code != ExitSuccess || !strings.Contains(output, "running pid=") {
		t.Fatalf("status exit=%d output=%q", code, output)
	}
	if code, output := runNativeCommand(t, executable, processEnvironment, "stop"); code != ExitSuccess || !strings.Contains(output, "stop requested pid=") {
		t.Fatalf("stop exit=%d output=%q", code, output)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- start.Wait() }()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("foreground process exit: %v stderr=%q", err, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("foreground process survived authenticated stop")
	}
	if err := <-scanDone; err != nil {
		t.Fatal(err)
	}
	if code, output := runNativeCommand(t, executable, processEnvironment, "status"); code != ExitDegraded || output != "not running\n" {
		t.Fatalf("post-stop status exit=%d output=%q", code, output)
	}
}

func runNativeCommand(t *testing.T, executable string, environment []string, arguments ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err == nil {
		return 0, string(output)
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), string(output)
	}
	t.Fatalf("run %v: %v output=%q", arguments, err, output)
	return -1, string(output)
}

func replaceProcessEnvironment(base []string, replacements map[string]string) []string {
	result := make([]string, 0, len(base)+len(replacements))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replace := replacements[strings.ToUpper(key)]; replace {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}

func nativeAvailablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}
