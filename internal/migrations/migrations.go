// Package migrations exposes the immutable SQL migrations embedded in the
// kubePeep binary.
package migrations

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed sql/*.sql
var files embed.FS

var filenamePattern = regexp.MustCompile(`^([0-9]{4})_([a-z0-9_]+)\.sql$`)

// Migration contains one immutable, ordered migration. Destructive migrations
// opt into the backup/restore path with the first-line directive documented by
// destructiveDirective.
type Migration struct {
	Version     int
	Name        string
	SQL         string
	Checksum    string
	Destructive bool
}

const destructiveDirective = "-- kubepeep:destructive"

// Embedded returns a validated, version-ordered copy of the migrations built
// into the executable.
func Embedded() ([]Migration, error) {
	entries, err := fs.ReadDir(files, "sql")
	if err != nil {
		return nil, fmt.Errorf("migrations: read embedded files: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("migrations: embedded directory is not allowed")
		}
		matches := filenamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("migrations: invalid embedded filename")
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("migrations: invalid embedded version")
		}
		contents, err := fs.ReadFile(files, "sql/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("migrations: read embedded migration: %w", err)
		}
		if len(strings.TrimSpace(string(contents))) == 0 {
			return nil, fmt.Errorf("migrations: empty embedded migration")
		}
		digest := sha256.Sum256(contents)
		migrations = append(migrations, Migration{
			Version:     version,
			Name:        matches[2],
			SQL:         string(contents),
			Checksum:    hex.EncodeToString(digest[:]),
			Destructive: strings.HasPrefix(string(contents), destructiveDirective+"\n"),
		})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	for index, migration := range migrations {
		if index > 0 && migration.Version == migrations[index-1].Version {
			return nil, fmt.Errorf("migrations: duplicate embedded version")
		}
	}
	return migrations, nil
}
