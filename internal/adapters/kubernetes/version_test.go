package kubernetes

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestKubernetesModulesArePinnedToValidatedVersion(t *testing.T) {
	want := map[string]string{
		"k8s.io/api":          "v0.35.7",
		"k8s.io/apimachinery": "v0.35.7",
		"k8s.io/client-go":    "v0.35.7",
		"k8s.io/metrics":      "v0.35.7",
	}
	for module, version := range want {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		output, err := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Version}}", module).Output()
		cancel()
		if err != nil {
			t.Fatalf("go list -m %s: %v", module, err)
		}
		if got := strings.TrimSpace(string(output)); got != version {
			t.Fatalf("%s version = %s, want %s", module, got, version)
		}
	}
}
