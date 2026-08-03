package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCleanupRegistryRunsLIFOAndAggregatesFailures(t *testing.T) {
	var registry CleanupRegistry
	var order []string
	wantFirst := errors.New("first failed")
	wantThird := errors.New("third failed")
	for _, hook := range []struct {
		name string
		err  error
	}{
		{name: "first", err: wantFirst},
		{name: "second"},
		{name: "third", err: wantThird},
	} {
		hook := hook
		if err := registry.Add(hook.name, func(context.Context) error {
			order = append(order, hook.name)
			return hook.err
		}); err != nil {
			t.Fatal(err)
		}
	}
	err := registry.Run(context.Background())
	if !errors.Is(err, wantFirst) || !errors.Is(err, wantThird) {
		t.Fatalf("cleanup error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"third", "second", "first"}) {
		t.Fatalf("cleanup order = %v", order)
	}
	if second := registry.Run(context.Background()); second != err {
		t.Fatalf("second Run returned a different aggregate: %v", second)
	}
}

func TestCleanupRegistryContinuesAfterPanic(t *testing.T) {
	var registry CleanupRegistry
	run := false
	_ = registry.Add("after", func(context.Context) error { run = true; return nil })
	_ = registry.Add("panic", func(context.Context) error { panic("boom") })
	if err := registry.Run(context.Background()); err == nil {
		t.Fatal("expected panic to become an error")
	}
	if !run {
		t.Fatal("remaining cleanup hook did not run")
	}
}
