package cli

import (
	"context"
	"os"
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
	if report.Overall != DoctorPass {
		t.Fatalf("doctor did not pass: %#v", report)
	}
	wanted := map[string]bool{"strict_config": false, "integrity": false, "embedded_frontend": false, "local_permissions": false, "loopback_bind": false}
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
