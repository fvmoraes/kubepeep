// Package browser opens the published local kubePeep URL with the platform's
// default browser.
package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
)

// Launcher starts a browser opener. The zero value uses the platform command.
type Launcher struct {
	start func(context.Context, string, ...string) error
}

// NewLauncher allows composition tests to replace process creation.
func NewLauncher(start func(context.Context, string, ...string) error) Launcher {
	return Launcher{start: start}
}

// Open validates and opens a loopback HTTP URL.
func (launcher Launcher) Open(ctx context.Context, rawURL string) error {
	if ctx == nil {
		return errors.New("browser: context is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("browser: URL must be a local HTTP URL")
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil || host != "127.0.0.1" || port == "" {
		return errors.New("browser: URL must target the published loopback listener")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1024 || portNumber > 65535 || strconv.Itoa(portNumber) != port {
		return errors.New("browser: URL contains an invalid published port")
	}
	name, args, err := commandFor(runtime.GOOS, rawURL)
	if err != nil {
		return err
	}
	start := launcher.start
	if start == nil {
		start = startCommand
	}
	if err := start(ctx, name, args...); err != nil {
		return fmt.Errorf("browser: open URL: %w", err)
	}
	return nil
}

func commandFor(goos, rawURL string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{rawURL}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{rawURL}, nil
	default:
		return "", nil, fmt.Errorf("browser: unsupported platform %q", goos)
	}
}

func startCommand(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	if err := command.Start(); err != nil {
		return err
	}
	go func() {
		_ = command.Wait()
	}()
	return nil
}
