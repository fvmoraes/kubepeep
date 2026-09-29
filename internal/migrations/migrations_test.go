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
	if len(loaded) != 2 || loaded[0].Version != 1 || loaded[0].Name != "initial" || loaded[1].Version != 2 || loaded[1].Name != "expand_preferences" {
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
}
