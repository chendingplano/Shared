package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceHashDetectsChanges(t *testing.T) {
	p := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(p, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyFile(p, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyFile(p, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"); err == nil {
		t.Fatal("accepted changed input")
	}
}
