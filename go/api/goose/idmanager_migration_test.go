package goose

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestIDManagerMigrationAlwaysAvailable(t *testing.T) {
	for _, source := range []fs.FS{fstest.MapFS{}, withoutEmojiMigration(fstest.MapFS{})} {
		merged, err := withIDManagerMigration(source)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fs.ReadFile(merged, idmanagerMigrationName); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := withIDManagerMigration(fstest.MapFS{idmanagerMigrationName: {Data: []byte("different")}}); err == nil {
		t.Fatal("accepted conflicting migration")
	}
}
