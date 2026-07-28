//go:build !windows

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCompiledBinaryOwnsSIGINTAndSIGTERM(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "kubePeep")
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build spike binary: %v\n%s", err, output)
	}

	for _, testCase := range []struct {
		name   string
		signal os.Signal
	}{
		{name: "SIGINT", signal: os.Interrupt},
		{name: "SIGTERM", signal: syscall.SIGTERM},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			port := reserveTestPort(t)
			command := exec.Command(
				binaryPath,
				"start",
				"--no-browser",
				"--port",
				fmt.Sprintf("%d", port),
			)
			stdout, err := command.StdoutPipe()
			if err != nil {
				t.Fatalf("stdout pipe: %v", err)
			}
			var stderr bytes.Buffer
			command.Stderr = &stderr
			if err := command.Start(); err != nil {
				t.Fatalf("start spike: %v", err)
			}

			ready := make(chan string, 1)
			scanDone := make(chan struct{})
			go func() {
				defer close(scanDone)
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					line := scanner.Text()
					if strings.Contains(line, "spike ready at") {
						select {
						case ready <- line:
						default:
						}
					}
				}
			}()

			select {
			case line := <-ready:
				if !strings.Contains(line, "embedded migration applied") {
					_ = command.Process.Kill()
					t.Fatalf("readiness line did not prove migration: %q", line)
				}
			case <-time.After(10 * time.Second):
				_ = command.Process.Kill()
				_ = command.Wait()
				t.Fatalf("binary did not become ready: %s", stderr.String())
			}

			if err := command.Process.Signal(testCase.signal); err != nil {
				_ = command.Process.Kill()
				t.Fatalf("send %s: %v", testCase.name, err)
			}
			waited := make(chan error, 1)
			go func() {
				waited <- command.Wait()
			}()
			select {
			case err := <-waited:
				if err != nil {
					t.Fatalf("binary exited after %s: %v\n%s", testCase.name, err, stderr.String())
				}
			case <-time.After(10 * time.Second):
				_ = command.Process.Kill()
				t.Fatalf("binary did not stop after %s", testCase.name)
			}
			<-scanDone
		})
	}
}

func reserveTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release test port: %v", err)
	}
	return port
}
