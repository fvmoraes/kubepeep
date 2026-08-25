package lifecycle

import "errors"

// ErrServerShutdown is carried as a context cancellation cause while the
// local process is intentionally stopping. It lets long-lived transports
// distinguish shutdown from a generation replacement or client disconnect.
var ErrServerShutdown = errors.New("kubepeep: server shutdown")
