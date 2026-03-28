package pdfparser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasOperation(t *testing.T) {
	raw := `[{"operation":"parse","time":"20260325 08:00:00","status":"success","error":""}]`
	if !hasOperation(raw, "parse") {
		t.Fatalf("expected parse operation to exist")
	}
	if hasOperation(raw, "analyze") {
		t.Fatalf("did not expect analyze operation")
	}
}

func TestAppendStatusEntry(t *testing.T) {
	got, err := appendStatusEntry("[]", OperationStatus{
		Operation: "parse",
		Time:      "20260325 08:00:00",
		Status:    "success",
		Error:     "",
	})
	if err != nil {
		t.Fatalf("appendStatusEntry failed: %v", err)
	}
	if !hasOperation(got, "parse") {
		t.Fatalf("expected parse operation to exist after append")
	}
}

func TestChooseLeastUsedRepoDir(t *testing.T) {
	root := t.TempDir()
	dirA := filepath.Join(root, "repo-a")
	dirB := filepath.Join(root, "repo-b")
	if err := os.MkdirAll(dirA, 0755); err != nil {
		t.Fatalf("mkdir dirA failed: %v", err)
	}
	if err := os.MkdirAll(dirB, 0755); err != nil {
		t.Fatalf("mkdir dirB failed: %v", err)
	}

	// Make dirA heavier than dirB.
	if err := os.WriteFile(filepath.Join(dirA, "file.bin"), make([]byte, 1024), 0644); err != nil {
		t.Fatalf("write heavy file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "file.bin"), make([]byte, 10), 0644); err != nil {
		t.Fatalf("write light file failed: %v", err)
	}

	selected, err := chooseLeastUsedRepoDir([]string{dirA, dirB})
	if err != nil {
		t.Fatalf("chooseLeastUsedRepoDir failed: %v", err)
	}
	if selected != dirB {
		t.Fatalf("expected %s, got %s", dirB, selected)
	}
}

func TestIsRemoteDestination(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{in: "/tmp/backup", want: false},
		{in: "user@host:/data/backup", want: true},
		{in: "https://bucket/path", want: true},
		{in: "C:\\tmp\\backup", want: false},
	}

	for _, tc := range cases {
		got := isRemoteDestination(tc.in)
		if got != tc.want {
			t.Fatalf("isRemoteDestination(%q): got %v, want %v", tc.in, got, tc.want)
		}
	}
}
