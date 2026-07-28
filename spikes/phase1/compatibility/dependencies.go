// Package compatibility forces the phase-one dependency set through the
// compiler. It deliberately contains no runtime behavior.
package compatibility

import (
	_ "github.com/coder/websocket"
	_ "k8s.io/api/core/v1"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/tools/clientcmd"
	_ "k8s.io/metrics/pkg/client/clientset/versioned"
	_ "modernc.org/sqlite"
)
