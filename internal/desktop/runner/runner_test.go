package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/desktop"
)

func TestRunUnavailableWithoutDesktopTag(t *testing.T) {
	if Enabled {
		t.Skip("desktop build enabled")
	}
	if Run == nil {
		t.Fatal("Run must always be set")
	}
	err := Run(context.Background(), desktop.RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "desktop support") {
		t.Fatalf("unexpected error: %v", err)
	}
}
