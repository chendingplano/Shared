package llm

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBuildUsageArchivePathsUsesDayAndAccountPartitions(t *testing.T) {
	day := time.Date(2026, time.June, 19, 2, 30, 0, 0, time.UTC)

	paths := BuildUsageArchivePaths("Data/llm-logs", day, "acct_42", "evt-123")

	wantDayDir := filepath.Join("Data/llm-logs", "2026", "2026-06", "2026-06-19")
	if paths.DayDir != wantDayDir {
		t.Fatalf("DayDir = %q, want %q", paths.DayDir, wantDayDir)
	}

	wantAccountDir := filepath.Join(wantDayDir, "account-acct_42")
	if paths.AccountDir != wantAccountDir {
		t.Fatalf("AccountDir = %q, want %q", paths.AccountDir, wantAccountDir)
	}

	wantInput := filepath.Join(wantAccountDir, "bodies", "evt-123-input.json.gz")
	if paths.InputBodyPath != wantInput {
		t.Fatalf("InputBodyPath = %q, want %q", paths.InputBodyPath, wantInput)
	}

	wantOutput := filepath.Join(wantAccountDir, "bodies", "evt-123-output.json.gz")
	if paths.OutputBodyPath != wantOutput {
		t.Fatalf("OutputBodyPath = %q, want %q", paths.OutputBodyPath, wantOutput)
	}
}
