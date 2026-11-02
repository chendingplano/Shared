package goose

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"testing/fstest"
)

const emojiMigrationName = "20261103000002_create_shared_emoji_library.sql"

//go:embed shared_migrations/20261103000002_create_shared_emoji_library.sql
var emojiMigration []byte

// withEmojiMigration makes the shared catalogue migration available to a
// consumer that maintains its own migration directory. A project copy must be
// identical, so two versions of the schema cannot silently diverge.
func withEmojiMigration(source fs.FS) (fs.FS, error) {
	view := emojiOverlay{source: source, embedded: fstest.MapFS{emojiMigrationName: &fstest.MapFile{Data: emojiMigration, Mode: 0444}}}
	if _, err := view.ReadDir("."); err != nil {
		return nil, err
	}
	return view, nil
}

type emojiOverlay struct {
	source   fs.FS
	embedded fstest.MapFS
}

type emojiHidden struct{ source fs.FS }

func withoutEmojiMigration(source fs.FS) fs.FS { return emojiHidden{source: source} }
func (v emojiHidden) Open(name string) (fs.File, error) {
	if name == emojiMigrationName {
		return nil, fs.ErrNotExist
	}
	return v.source.Open(name)
}
func (v emojiHidden) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(v.source, name)
	if err != nil {
		return nil, err
	}
	if name != "." {
		return entries, nil
	}
	result := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() != emojiMigrationName {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (v emojiOverlay) Open(name string) (fs.File, error) {
	if name == emojiMigrationName {
		data, err := fs.ReadFile(v.source, name)
		if err == nil && !bytes.Equal(data, emojiMigration) {
			return nil, fmt.Errorf("shared emoji migration conflicts with embedded migration")
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		return v.embedded.Open(name)
	}
	return v.source.Open(name)
}
func (v emojiOverlay) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(v.source, name)
	if err != nil && !(name == "." && errors.Is(err, fs.ErrNotExist)) {
		return nil, err
	}
	if name != "." {
		return entries, err
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == emojiMigrationName {
			found = true
			file, e := v.Open(emojiMigrationName)
			if e != nil {
				return nil, e
			}
			file.Close()
			break
		}
	}
	if !found {
		extra, _ := fs.ReadDir(v.embedded, ".")
		entries = append(entries, extra...)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}
