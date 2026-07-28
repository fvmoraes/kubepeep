package control

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const probeBinaryEnvironment = "F1_CONTROL_PROBE_BINARY"

type probeOutput struct {
	Running  bool            `json:"running"`
	Instance *PublicInstance `json:"instance,omitempty"`
}

func TestNativeControlLifecycleBlackBox(t *testing.T) {
	if os.Getenv("F1_CONTROL_UNRELATED_HELPER") == "1" {
		t.Skip("helper is selected through TestUnrelatedProcessHelper")
	}

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
		"--port-attempts", "1",
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

	instance := waitForInstance(t, runtimeDir, waitResult, stdoutPath, stderrPath)
	assertPrivateUnixModes(t, runtimeDir)

	status := runProbeCommand(t, binary, runtimeDir, "status")
	if !status.Running || status.Instance == nil || *status.Instance != instance.Public() {
		t.Fatalf("status output = %#v, want %#v", status, instance.Public())
	}
	if outputContains(t, stdoutPath, instance.Token) {
		t.Fatal("start stdout exposed control token")
	}
	if outputContains(t, stderrPath, instance.Token) {
		t.Fatal("start stderr exposed control token")
	}

	held, err := AcquireFileLock(LockPath(runtimeDir))
	if !errors.Is(err, ErrLocked) {
		if held != nil {
			_ = held.Close()
		}
		t.Fatalf("active instance lock error = %v, want ErrLocked", err)
	}

	secondContext, cancelSecond := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelSecond()
	second := exec.CommandContext(
		secondContext,
		binary,
		"--runtime-dir", runtimeDir,
		"start",
		"--port", "0",
	)
	secondOutput, secondErr := second.CombinedOutput()
	if secondErr == nil || !strings.Contains(string(secondOutput), ErrAlreadyRunning.Error()) {
		t.Fatalf("second start err=%v output=%q", secondErr, secondOutput)
	}

	assertRejectedControlRequests(t, instance)
	if got := runProbeCommand(t, binary, runtimeDir, "status"); !got.Running {
		t.Fatal("unauthorized requests stopped the active instance")
	}

	stop := runProbeCommand(t, binary, runtimeDir, "stop")
	if !stop.Running || stop.Instance == nil || *stop.Instance != instance.Public() {
		t.Fatalf("stop output = %#v, want %#v", stop, instance.Public())
	}
	select {
	case err := <-waitResult:
		if err != nil {
			t.Fatalf(
				"foreground process exit: %v\nstdout:\n%s\nstderr:\n%s",
				err,
				readOutput(t, stdoutPath),
				readOutput(t, stderrPath),
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("foreground process did not exit after authenticated stop")
	}
	if _, err := os.Stat(InstancePath(runtimeDir)); !os.IsNotExist(err) {
		t.Fatalf("instance state remains after stop: %v", err)
	}

	reacquired, err := AcquireFileLock(LockPath(runtimeDir))
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	if err := reacquired.Close(); err != nil {
		t.Fatalf("release reacquired lock: %v", err)
	}
	if got := runProbeCommand(t, binary, runtimeDir, "stop"); got.Running {
		t.Fatal("idempotent stop reported a running instance")
	}

	proveStalePIDIsNeverAuthority(t, binary, runtimeDir)
}

func TestUnrelatedProcessHelper(t *testing.T) {
	if os.Getenv("F1_CONTROL_UNRELATED_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func controlProbeBinary(t *testing.T) string {
	t.Helper()
	if configured := os.Getenv(probeBinaryEnvironment); configured != "" {
		path, err := filepath.Abs(configured)
		if err != nil {
			t.Fatalf("resolve configured probe binary: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("configured probe binary: %v", err)
		}
		return path
	}

	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate black-box test source")
	}
	moduleRoot := filepath.Dir(filepath.Dir(source))
	name := "f1-control-probe"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	output := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-o", output, "./cmd/f1-control-probe")
	command.Dir = moduleRoot
	command.Env = append(
		os.Environ(),
		"GOTOOLCHAIN=go1.25.0",
		"CGO_ENABLED=0",
	)
	if data, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build control probe: %v\n%s", err, data)
	}
	return output
}

func createOutputFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create process output: %v", err)
	}
	return file
}

func waitForInstance(
	t *testing.T,
	runtimeDir string,
	waitResult <-chan error,
	stdoutPath string,
	stderrPath string,
) Instance {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		instance, err := LoadInstance(runtimeDir)
		if err == nil {
			return instance
		}
		if !errors.Is(err, ErrNotRunning) {
			t.Fatalf("load starting instance: %v", err)
		}
		select {
		case processErr := <-waitResult:
			t.Fatalf(
				"probe exited before ready: %v\nstdout:\n%s\nstderr:\n%s",
				processErr,
				readOutput(t, stdoutPath),
				readOutput(t, stderrPath),
			)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf(
		"probe did not publish state\nstdout:\n%s\nstderr:\n%s",
		readOutput(t, stdoutPath),
		readOutput(t, stderrPath),
	)
	return Instance{}
}

func runProbeCommand(t *testing.T, binary, runtimeDir, subcommand string) probeOutput {
	t.Helper()
	command := exec.Command(binary, "--runtime-dir", runtimeDir, subcommand)
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s command: %v\n%s", subcommand, err, data)
	}
	var result probeOutput
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode %s output %q: %v", subcommand, data, err)
	}
	return result
}

func assertRejectedControlRequests(t *testing.T, instance Instance) {
	t.Helper()
	tests := []struct {
		name       string
		token      string
		host       string
		origin     string
		wantStatus int
	}{
		{
			name:       "wrong token",
			token:      "wrong-token",
			host:       net.JoinHostPort("127.0.0.1", strconv.Itoa(instance.Port)),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "foreign host",
			token:      instance.Token,
			host:       "attacker.example",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "browser origin",
			token:      instance.Token,
			host:       net.JoinHostPort("127.0.0.1", strconv.Itoa(instance.Port)),
			origin:     "https://attacker.example",
			wantStatus: http.StatusForbidden,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			url := "http://" +
				net.JoinHostPort("127.0.0.1", strconv.Itoa(instance.Port)) +
				stopPath
			request, err := http.NewRequest(http.MethodPost, url, nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			request.Host = test.host
			request.Header.Set(ControlTokenHeader, test.token)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			client := &http.Client{Timeout: 2 * time.Second}
			response, err := client.Do(request)
			if err != nil {
				t.Fatalf("control request: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.wantStatus {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("status=%d want=%d body=%q", response.StatusCode, test.wantStatus, body)
			}
		})
	}
}

func proveStalePIDIsNeverAuthority(t *testing.T, binary, runtimeDir string) {
	t.Helper()
	helper := exec.Command(os.Args[0], "-test.run=^TestUnrelatedProcessHelper$")
	helper.Env = append(os.Environ(), "F1_CONTROL_UNRELATED_HELPER=1")
	stdout, err := helper.StdoutPipe()
	if err != nil {
		t.Fatalf("unrelated helper stdout: %v", err)
	}
	stdin, err := helper.StdinPipe()
	if err != nil {
		t.Fatalf("unrelated helper stdin: %v", err)
	}
	if err := helper.Start(); err != nil {
		t.Fatalf("start unrelated helper: %v", err)
	}
	helperWait := make(chan error, 1)
	go func() {
		helperWait <- helper.Wait()
		close(helperWait)
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		select {
		case <-helperWait:
		case <-time.After(time.Second):
			_ = helper.Process.Kill()
			<-helperWait
		}
	})

	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatalf("unrelated helper did not become ready: %q err=%v", scanner.Text(), scanner.Err())
	}
	fingerprint, err := ProcessFingerprint(helper.Process.Pid)
	if err != nil {
		t.Fatalf("fingerprint unrelated process: %v", err)
	}
	port := unusedLoopbackPort(t)
	instanceID, err := randomSecret()
	if err != nil {
		t.Fatalf("generate stale instance ID: %v", err)
	}
	token, err := randomSecret()
	if err != nil {
		t.Fatalf("generate stale token: %v", err)
	}
	stale := Instance{
		Schema:      SchemaVersion,
		InstanceID:  instanceID,
		Token:       token,
		PID:         helper.Process.Pid,
		Fingerprint: fingerprint,
		Port:        port,
		Protocol:    ProtocolVersion,
	}
	if err := WriteInstanceAtomic(runtimeDir, stale); err != nil {
		t.Fatalf("write stale instance: %v", err)
	}

	result := runProbeCommand(t, binary, runtimeDir, "stop")
	if result.Running {
		t.Fatal("stale PID was reported as a verified running instance")
	}
	select {
	case err := <-helperWait:
		t.Fatalf("stop affected unrelated PID: %v", err)
	default:
	}
	if _, err := os.Stat(InstancePath(runtimeDir)); !os.IsNotExist(err) {
		t.Fatalf("safely proven stale state was not removed: %v", err)
	}
	_ = stdin.Close()
	select {
	case <-helperWait:
	case <-time.After(2 * time.Second):
		t.Fatal("unrelated helper did not exit after its own stdin was closed")
	}
}

func unusedLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve unused port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release unused port: %v", err)
	}
	return port
}

func assertPrivateUnixModes(t *testing.T, runtimeDir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Log("Windows permissions use DACLs; Unix mode-bit assertions do not apply")
		return
	}
	paths := []struct {
		path string
		want os.FileMode
	}{
		{runtimeDir, 0o700},
		{InstancePath(runtimeDir), 0o600},
		{LockPath(runtimeDir), 0o600},
	}
	for _, item := range paths {
		info, err := os.Stat(item.path)
		if err != nil {
			t.Fatalf("stat %s: %v", item.path, err)
		}
		if got := info.Mode().Perm(); got != item.want {
			t.Fatalf("%s mode=%04o want=%04o", item.path, got, item.want)
		}
	}
}

func outputContains(t *testing.T, path, secret string) bool {
	t.Helper()
	return bytes.Contains([]byte(readOutput(t, path)), []byte(secret))
}

func readOutput(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output %s: %v", path, err)
	}
	return string(data)
}
