package desktop

import (
	"io"

	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
	productconfig "github.com/fvmoraes/kubepeep/internal/config"
)

// RunOptions carries everything the desktop shell needs from the CLI without
// importing any presentation framework.
type RunOptions struct {
	Layout        userdirs.Layout
	Config        productconfig.Config
	Context       string
	ContextSet    bool
	Kubeconfig    string
	KubeconfigSet bool
	Namespace     string
	NamespaceSet  bool
	Port          *int
	LogOutput     io.Writer
}

var ErrDesktopUnavailable = errDesktopUnavailable{}

type errDesktopUnavailable struct{}

func (errDesktopUnavailable) Error() string {
	return "this kubePeep build does not include desktop support; rebuild with -tags desktop (requires Wails native dependencies) or use 'kubepeep serve'"
}
