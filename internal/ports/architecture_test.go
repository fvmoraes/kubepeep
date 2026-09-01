package ports

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// forbiddenHandlerImports lists the package path fragments that HTTP handlers
// must never import. Handlers depend on service interfaces and DTOs only;
// concrete adapter and integration implementations are wired exclusively by
// internal/application (Compose) and the CLI runtimes.
var forbiddenHandlerImports = []string{
	"/internal/adapters/",
	"/internal/integration/",
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate ports sources")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
}

func TestHandlerPackagesDependOnlyOnServiceInterfaces(t *testing.T) {
	root := repositoryRoot(t)
	handlersDirectory := filepath.Join(root, "internal", "api", "handlers")
	entries, err := os.ReadDir(handlersDirectory)
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, filepath.Join(handlersDirectory, entry.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, imported := range file.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			for _, forbidden := range forbiddenHandlerImports {
				if strings.Contains(path, forbidden) {
					t.Fatalf("%s imports concrete implementation %q; handlers must depend on service interfaces only", entry.Name(), path)
				}
			}
		}
	}
}

func TestPortsPackageHasNoImplementationDependencies(t *testing.T) {
	root := repositoryRoot(t)
	portsDirectory := filepath.Join(root, "internal", "ports")
	entries, err := os.ReadDir(portsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, filepath.Join(portsDirectory, entry.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, imported := range file.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			if strings.Contains(path, "fvmoraes/kubepeep/internal/") {
				t.Fatalf("ports package must stay dependency-free; found %q in %s", path, entry.Name())
			}
		}
	}
}
