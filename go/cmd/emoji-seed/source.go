package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

//go:embed source-manifest.json
var manifest []byte

func verifyFile(path, expected string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != expected {
		return fmt.Errorf("source checksum mismatch: %s", path)
	}
	return nil
}

func verifySources(root string) error {
	var m struct {
		Files   map[string]string `json:"files_sha256"`
		Twemoji struct {
			Tree string `json:"svg_tree_sha256"`
		} `json:"twemoji"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		return err
	}
	for name, hash := range m.Files {
		if err := verifyFile(filepath.Join(root, name), hash); err != nil {
			return err
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "twemoji/assets/svg/*.svg"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, p := range files {
		h.Write([]byte(filepath.Base(p)))
		h.Write([]byte{0})
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		h.Write(b)
	}
	if hex.EncodeToString(h.Sum(nil)) != m.Twemoji.Tree {
		return fmt.Errorf("Twemoji SVG source checksum mismatch")
	}
	return nil
}
