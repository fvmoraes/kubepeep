package runtime

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
)

const (
	DefaultFirstPort = 2748
	DefaultLastPort  = 2797
	MinimumPort      = 1024
	MaximumPort      = 65535
)

type listenFunc func(network, address string) (net.Listener, error)

// BindLoopback acquires the definitive listener. A nil explicitPort tries the
// exact 50-port default range; an explicit port is attempted once.
func BindLoopback(explicitPort *int) (net.Listener, int, error) {
	return bindLoopback(net.Listen, explicitPort)
}

func bindLoopback(listen listenFunc, explicitPort *int) (net.Listener, int, error) {
	first := DefaultFirstPort
	last := DefaultLastPort
	if explicitPort != nil {
		if *explicitPort < MinimumPort || *explicitPort > MaximumPort {
			return nil, 0, fmt.Errorf("runtime: port must be between %d and %d", MinimumPort, MaximumPort)
		}
		first, last = *explicitPort, *explicitPort
	}

	var occupied []error
	for port := first; port <= last; port++ {
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		listener, err := listen("tcp", address)
		if err == nil {
			actual, ok := listener.Addr().(*net.TCPAddr)
			if !ok || actual.IP.String() != "127.0.0.1" || actual.Port != port {
				_ = listener.Close()
				return nil, 0, errors.New("runtime: listener did not bind the requested loopback address")
			}
			return listener, port, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, 0, fmt.Errorf("runtime: bind loopback port %d: %w", port, err)
		}
		occupied = append(occupied, err)
		if explicitPort != nil {
			return nil, 0, fmt.Errorf("runtime: explicit port %d is already in use: %w", port, err)
		}
	}
	return nil, 0, fmt.Errorf("runtime: no port available in %d-%d: %w", first, last, errors.Join(occupied...))
}
