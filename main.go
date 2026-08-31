// Package main is the desktop entrypoint compiled by Wails. The web binary
// remains in cmd/kubePeep; both share the same CLI and core composition.
// Desktop support is linked through the build:tags entry in wails.json, which
// selects internal/desktop/runner's Wails-backed implementation.
package main

import (
	"os"

	"github.com/fvmoraes/kubepeep/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
