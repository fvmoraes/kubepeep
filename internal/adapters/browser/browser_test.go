package browser

import (
	"context"
	"reflect"
	"testing"
)

func TestOpenUsesValidatedLoopbackURL(t *testing.T) {
	var gotName string
	var gotArgs []string
	launcher := NewLauncher(func(_ context.Context, name string, args ...string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	})
	if err := launcher.Open(context.Background(), "http://127.0.0.1:2748/"); err != nil {
		t.Fatal(err)
	}
	if gotName == "" || len(gotArgs) == 0 || gotArgs[len(gotArgs)-1] != "http://127.0.0.1:2748/" {
		t.Fatalf("unexpected opener invocation: %q %v", gotName, gotArgs)
	}
}

func TestOpenRejectsNonLoopbackAndURLCapabilities(t *testing.T) {
	launcher := NewLauncher(func(context.Context, string, ...string) error {
		t.Fatal("opener must not run")
		return nil
	})
	for _, rawURL := range []string{
		"https://127.0.0.1:2748/",
		"http://localhost:2748/",
		"http://127.0.0.1:2748/?token=secret",
		"http://user@127.0.0.1:2748/",
		"http://127.0.0.1:not-a-port/",
		"http://127.0.0.1:80/",
	} {
		if err := launcher.Open(context.Background(), rawURL); err == nil {
			t.Fatalf("expected %q to be rejected", rawURL)
		}
	}
}

func TestPlatformCommands(t *testing.T) {
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{goos: "linux", name: "xdg-open", args: []string{"http://127.0.0.1:2748/"}},
		{goos: "darwin", name: "open", args: []string{"http://127.0.0.1:2748/"}},
		{goos: "windows", name: "rundll32", args: []string{"url.dll,FileProtocolHandler", "http://127.0.0.1:2748/"}},
	}
	for _, test := range tests {
		name, args, err := commandFor(test.goos, "http://127.0.0.1:2748/")
		if err != nil {
			t.Fatal(err)
		}
		if name != test.name || !reflect.DeepEqual(args, test.args) {
			t.Fatalf("%s command = %q %v", test.goos, name, args)
		}
	}
}
