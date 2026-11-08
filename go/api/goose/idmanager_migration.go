package goose

import (
	_ "embed"
	"io/fs"
	"testing/fstest"
)

const idmanagerMigrationName = "20261108000001_id_manager.sql"

//go:embed shared_migrations/20261108000001_id_manager.sql
var idmanagerMigration []byte

// Every consumer receives the allocator migration even when its shared migration
// directory is missing or does not contain a project copy.
func withIDManagerMigration(source fs.FS) (fs.FS, error) {
	view := emojiOverlay{source: source, embedded: fstest.MapFS{idmanagerMigrationName: &fstest.MapFile{Data: idmanagerMigration, Mode: 0444}}}
	if _, err := view.ReadDir("."); err != nil {
		return nil, err
	}
	return view, nil
}
