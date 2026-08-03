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
	if len(loaded) != 1 || loaded[0].Version != 1 || loaded[0].Name != "initial" {
		t.Fatalf("unexpected migrations: %#v", loaded)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(loaded[0].Checksum) {
		t.Fatalf("invalid checksum: %q", loaded[0].Checksum)
	}
	if loaded[0].Destructive {
		t.Fatal("initial migration must not be destructive")
	}
}
