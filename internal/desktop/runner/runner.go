// Package runner selects the desktop implementation at build time. Without
// the desktop tag, both Enabled and Run stay at their safe defaults so the
// web binary and all CI test jobs never link Wails.
package runner

import (
	"context"

	"github.com/fvmoraes/kubepeep/internal/desktop"
)

// Enabled reports whether this binary was built with desktop support.
var Enabled = false

// Run launches the desktop window. It is replaced by the Wails-backed
// implementation when the binary is built with -tags desktop.
var Run = func(context.Context, desktop.RunOptions) error {
	return desktop.ErrDesktopUnavailable
}
