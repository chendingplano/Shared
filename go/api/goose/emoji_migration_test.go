package goose

import (
	"errors"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
)

func TestSharedEmojiMigrationAvailableWithoutProjectCopy(t *testing.T) {
	merged, err := withEmojiMigration(fstest.MapFS{"20200101000000_other.sql": {Data: []byte("-- +goose Up\nSELECT 1;")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fs.ReadFile(merged, emojiMigrationName); err != nil {
		t.Fatal(err)
	}
	if _, err = fs.ReadFile(merged, "20200101000000_other.sql"); err != nil {
		t.Fatal(err)
	}
}

func TestSharedEmojiMigrationCreatesMissingDirectoryView(t *testing.T) {
	merged, err := withEmojiMigration(os.DirFS(t.TempDir() + "/missing"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fs.ReadFile(merged, emojiMigrationName); err != nil {
		t.Fatal(err)
	}
}

func TestSharedEmojiMigrationSeesFilesCreatedLater(t *testing.T) {
	dir := t.TempDir()
	merged, err := withEmojiMigration(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/20200101000001_new.sql", []byte("-- +goose Up\nSELECT 1;"), 0644); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(merged, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected live migration and embedded migration, got %v", entries)
	}
}

func TestSharedEmojiMigrationRejectsConflictingCopy(t *testing.T) {
	_, err := withEmojiMigration(fstest.MapFS{emojiMigrationName: {Data: []byte("different")}})
	if err == nil {
		t.Fatal("accepted conflicting migration")
	}
}

func TestUnavailableEmojiMigrationDoesNotBlockOtherMigrations(t *testing.T) {
	source := fstest.MapFS{emojiMigrationName: {Data: emojiMigration}, "20200101000000_other.sql": {Data: []byte("-- +goose Up\nSELECT 1;")}}
	view := withoutEmojiMigration(source)
	entries, err := fs.ReadDir(view, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "20200101000000_other.sql" {
		t.Fatalf("unexpected migrations: %v", entries)
	}
	if _, err = fs.ReadFile(view, emojiMigrationName); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected hidden migration: %v", err)
	}
}
