package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fvmoraes/kubepeep/internal/adapters/userdirs"
)

func TestProductionDoctorCoversLocalFoundation(t *testing.T) {
	layout, err := userdirs.ForRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	checks, err := (ProductionDoctor{}).Check(context.Background(), layout)
	if err != nil {
		t.Fatal(err)
	}
	report, err := buildDoctorReport(checks)
	if err != nil {
		t.Fatal(err)
	}
	if report.Overall == DoctorFail {
		t.Fatalf("local doctor reported a critical failure: %#v", report)
	}
	wanted := map[string]bool{
		"strict_config": false, "integrity": false, "embedded_frontend": false,
		"local_permissions": false, "loopback_bind": false, "source": false,
		"context": false, "exec_plugin": false, "connectivity": false, "basic_capability": false,
	}
	for _, check := range report.Checks {
		if _, ok := wanted[check.Name]; ok {
			wanted[check.Name] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Fatalf("doctor check %s is missing", name)
		}
	}
}

func TestKubernetesDoctorSanitizesUnavailableExecPlugin(t *testing.T) {
	layout, err := userdirs.ForRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}
	marker := "SYNTHETIC_SECRET_PLUGIN_COMMAND"
	kubeconfig := filepath.Join(t.TempDir(), "config.yaml")
	content := `apiVersion: v1
kind: Config
clusters:
- name: cluster
  cluster:
    server: https://127.0.0.1:1
    insecure-skip-tls-verify: true
contexts:
- name: local
  context:
    cluster: cluster
    user: user
current-context: local
users:
- name: user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: ` + marker + `
      interactiveMode: Never
`
	if err := os.WriteFile(kubeconfig, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	checks := checkKubernetes(context.Background(), layout)
	var pluginFound bool
	for _, check := range checks {
		if strings.Contains(check.Message, marker) || strings.Contains(check.Message, kubeconfig) {
			t.Fatalf("diagnostic leaked plugin/path detail: %#v", check)
		}
		if check.Name == "exec_plugin" {
			pluginFound = true
			if check.Status != DoctorWarn || check.Code != "EXEC_PLUGIN_UNAVAILABLE" {
				t.Fatalf("unexpected plugin check: %#v", check)
			}
		}
	}
	if !pluginFound {
		t.Fatal("exec plugin check missing")
	}
}

func TestProductionDoctorSanitizesInvalidConfig(t *testing.T) {
	layout, err := userdirs.ForRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}
	marker := "SYNTHETIC_SECRET_MUST_NOT_LEAK"
	if err := os.WriteFile(layout.Config, []byte("token: "+marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	checks, err := (ProductionDoctor{}).Check(context.Background(), layout)
	if err != nil {
		t.Fatal(err)
	}
	var configFailed bool
	for _, check := range checks {
		if strings.Contains(check.Message, marker) {
			t.Fatalf("sensitive config content leaked: %q", check.Message)
		}
		if check.Name == "strict_config" && check.Status == DoctorFail {
			configFailed = true
		}
	}
	if !configFailed {
		t.Fatal("invalid config was not reported")
	}
}

func TestPermissionDoctorInspectsExistingObjects(t *testing.T) {
	layout, err := userdirs.ForRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.EnsureDirectories(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(layout.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	check := checkPermissions(layout)
	if check.Status != DoctorFail || check.Code != "LOCAL_PERMISSIONS_INVALID" {
		t.Fatalf("permission check accepted an unsafe object: %#v", check)
	}
}
