package main

import (
	"os"

	"github.com/fvmoraes/kubepeep/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
