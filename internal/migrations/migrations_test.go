package migrations

import (
	"regexp"
	"testing"
)

func TestEmbeddedMigrationsAreOrderedAndChecksummed(t *testing.T) {
	loaded, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 5 || loaded[0].Version != 1 || loaded[0].Name != "initial" || loaded[1].Version != 2 || loaded[1].Name != "expand_preferences" || loaded[2].Version != 3 || loaded[2].Name != "namespace_scope_defaults" || loaded[3].Version != 4 || loaded[3].Name != "context_view_preferences" || loaded[4].Version != 5 || loaded[4].Name != "saved_port_forwards" {
		t.Fatalf("unexpected migrations: %#v", loaded)
	}
	for _, migration := range loaded {
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(migration.Checksum) {
			t.Fatalf("invalid checksum: %q", migration.Checksum)
		}
	}
	if loaded[0].Destructive {
		t.Fatal("initial migration must not be destructive")
	}
	if !loaded[1].Destructive {
		t.Fatal("preference table replacement requires a verified backup")
	}
	if !loaded[3].Destructive {
		t.Fatal("context preference table replacement requires a verified backup")
	}
}
