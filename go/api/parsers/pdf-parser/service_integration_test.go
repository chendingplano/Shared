package pdfparser

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type testLogger struct{}

func (l *testLogger) Debug(string, ...any) {}
func (l *testLogger) Line(string, ...any)  {}
func (l *testLogger) Info(string, ...any)  {}
func (l *testLogger) Warn(string, ...any)  {}
func (l *testLogger) Error(string, ...any) {}
func (l *testLogger) Trace(string)         {}
func (l *testLogger) Close()               {}

func TestProcessOnce_Integration_MockedOCRAndDB(t *testing.T) {
	root := t.TempDir()
	stagingDir := filepath.Join(root, "staging")
	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	backupDir := filepath.Join(root, "backup")
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		t.Fatalf("mkdir staging failed: %v", err)
	}
	if err := os.MkdirAll(repoA, 0755); err != nil {
		t.Fatalf("mkdir repoA failed: %v", err)
	}
	if err := os.MkdirAll(repoB, 0755); err != nil {
		t.Fatalf("mkdir repoB failed: %v", err)
	}

	// Make repoA heavier, so repoB should be selected.
	if err := os.WriteFile(filepath.Join(repoA, "heavy.bin"), make([]byte, 2048), 0644); err != nil {
		t.Fatalf("write heavy file failed: %v", err)
	}

	pdfName := "doc-1001.pdf"
	sourcePDF := filepath.Join(stagingDir, pdfName)
	if err := os.WriteFile(sourcePDF, []byte("%PDF-1.4 fake"), 0644); err != nil {
		t.Fatalf("write source pdf failed: %v", err)
	}

	scriptPath := filepath.Join(root, "fake_ocr.sh")
	script := `#!/usr/bin/env bash
set -euo pipefail
out_dir=""
for ((i=1; i<=$#; i++)); do
  if [ "${!i}" = "-o" ]; then
    j=$((i+1))
    out_dir="${!j}"
  fi
done
mkdir -p "$out_dir"
printf '{"page":1,"text":"hello"}' > "$out_dir/page_1.json"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake OCR script failed: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	svc, err := NewService(Config{
		PollInterval:      1 * time.Second,
		BatchSize:         10,
		StagingDir:        stagingDir,
		RepoDirs:          []string{repoA, repoB},
		BackupDir:         backupDir,
		PythonBin:         "bash",
		PaddleOCRScript:   scriptPath,
		DeleteFromStaging: true,
		WorkDir:           root,
	})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	rows := sqlmock.NewRows([]string{"id", "file_name", "status"}).AddRow(int64(1001), pdfName, "[]")
	mock.ExpectQuery(regexp.QuoteMeta(svc.queryPendingSQL)).WithArgs(10).WillReturnRows(rows)
	mock.ExpectExec(regexp.QuoteMeta(svc.updateSuccessSQL)).
		WithArgs(sqlmock.AnyArg(), "ocr_rslt_1001.json", sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.ProcessOnce(context.Background(), &testLogger{}); err != nil {
		t.Fatalf("ProcessOnce failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}

	resultJSON := filepath.Join(repoB, "ocr_rslt_1001.json")
	if _, err := os.Stat(resultJSON); err != nil {
		t.Fatalf("expected result JSON in selected repo dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoB, pdfName)); err != nil {
		t.Fatalf("expected copied PDF in selected repo dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, pdfName)); err != nil {
		t.Fatalf("expected copied PDF in backup dir: %v", err)
	}
	if _, err := os.Stat(sourcePDF); !os.IsNotExist(err) {
		t.Fatalf("expected source PDF removed from staging, got err=%v", err)
	}
}
