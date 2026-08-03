package runtime

import (
	"errors"
	"net"
	"syscall"
	"testing"
)

func TestBindLoopbackRetriesOnlyAddressInUse(t *testing.T) {
	var addresses []string
	listener, port, err := bindLoopback(func(network, address string) (net.Listener, error) {
		addresses = append(addresses, address)
		if len(addresses) == 1 {
			return nil, &net.OpError{Op: "listen", Net: network, Err: syscall.EADDRINUSE}
		}
		return net.Listen(network, address)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if port != DefaultFirstPort+1 || len(addresses) != 2 {
		t.Fatalf("selected port %d after %d attempts", port, len(addresses))
	}
}

func TestBindLoopbackDoesNotHideOtherErrors(t *testing.T) {
	want := syscall.EACCES
	_, _, err := bindLoopback(func(string, string) (net.Listener, error) {
		return nil, want
	}, nil)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}

func TestBindLoopbackExplicitPortHasNoFallback(t *testing.T) {
	port := 43210
	attempts := 0
	_, _, err := bindLoopback(func(string, string) (net.Listener, error) {
		attempts++
		return nil, syscall.EADDRINUSE
	}, &port)
	if err == nil || attempts != 1 {
		t.Fatalf("explicit bind error = %v after %d attempts", err, attempts)
	}
}

func TestBindLoopbackRejectsPrivilegedPort(t *testing.T) {
	port := MinimumPort - 1
	if _, _, err := BindLoopback(&port); err == nil {
		t.Fatal("expected invalid explicit port to fail")
	}
}
