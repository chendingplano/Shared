package pdfparser

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const (
	parseOperationName = "parse"
	statusSuccess      = "success"
	statusFail         = "fail"
	timeLayoutStatus   = "20060102 15:04:05"
)

type Config struct {
	PollInterval time.Duration
	BatchSize    int

	StagingDir string
	RepoDirs   []string
	BackupDir  string

	PythonBin         string
	PaddleOCRScript   string
	UsePaddleOCRVL    bool
	WorkDir           string
	DeleteFromStaging bool
}

type Service struct {
	cfg Config

	queryPendingSQL  string
	updateSuccessSQL string
	updateFailureSQL string
}

type InputRecord struct {
	ID       int64
	Name     string
	FileName string
	Status   string
}

func NewService(cfg Config) (*Service, error) {
	cfg = applyDefaults(cfg)
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &Service{
		cfg: cfg,
		queryPendingSQL: `
SELECT id,
       COALESCE(name, ''),
	   COALESCE(file_name, ''),
       COALESCE(status::text, '[]')
FROM kb.inputs
WHERE type = 'pdf'
ORDER BY id ASC
LIMIT $1`,
		updateSuccessSQL: `
UPDATE kb.inputs
SET status = $1,
    result_filename = $2,
    file_name = $3,
    backup_filename = $4,
    error_msg = NULL,
    modify_time = NOW()
WHERE id = $5`,
		updateFailureSQL: `
UPDATE kb.inputs
SET status = $1,
    error_msg = $2,
    modify_time = NOW()
WHERE id = $3`,
	}, nil
}

func (s *Service) Run(ctx context.Context, logger ApiTypes.JimoLogger) error {
	if logger == nil {
		return errors.New("logger is required")
	}
	if ApiTypes.ProjectDBHandle == nil {
		return errors.New("ApiTypes.ProjectDBHandle is nil")
	}

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	logger.Info("PDF parser service started",
		"poll_interval", s.cfg.PollInterval.String(),
		"batch_size", s.cfg.BatchSize,
		"repo_dirs", strings.Join(s.cfg.RepoDirs, ","),
		"backup_dir", s.cfg.BackupDir,
	)

	if err := s.ProcessOnce(ctx, logger); err != nil {
		logger.Error("initial processing cycle failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("PDF parser service stopping", "reason", ctx.Err())
			return nil
		case <-ticker.C:
			if err := s.ProcessOnce(ctx, logger); err != nil {
				logger.Error("processing cycle failed", "error", err)
			}
		}
	}
}

func (s *Service) ProcessOnce(ctx context.Context, logger ApiTypes.JimoLogger) error {
	if logger == nil {
		return errors.New("(MID_26032640) logger is required")
	}
	if ApiTypes.ProjectDBHandle == nil {
		return errors.New("(MID_26032640) ApiTypes.ProjectDBHandle is nil")
	}

	records, err := s.fetchCandidates(ctx, ApiTypes.ProjectDBHandle, s.cfg.BatchSize)
	if err != nil {
		return fmt.Errorf("(MID_26032650) failed fetching candidates, error:%w", err)
	}

	if len(records) == 0 {
		logger.Info("Nothing to process")
		return nil
	}

	for _, rec := range records {
		if hasOperation(rec.Status, parseOperationName) {
			continue
		}

		if err := s.processRecord(ctx, ApiTypes.ProjectDBHandle, logger, rec); err != nil {
			logger.Error("failed to process PDF input",
				"input_id", rec.ID,
				"file_name", rec.FileName,
				"error", err,
			)
		}

		logger.Info("Processed record", "record_id", rec.ID, "filename", rec.FileName)
	}
	return nil
}

func (s *Service) fetchCandidates(ctx context.Context, db *sql.DB, limit int) ([]InputRecord, error) {
	rows, err := db.QueryContext(ctx, s.queryPendingSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("(MID_26032610) query pending pdf inputs failed: %w", err)
	}
	defer rows.Close()

	out := make([]InputRecord, 0, limit)
	for rows.Next() {
		var rec InputRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.FileName, &rec.Status); err != nil {
			return nil, fmt.Errorf("(MID_26032611) scan pending pdf input failed: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("(MID_26032612) iterate pending pdf inputs failed: %w", err)
	}
	return out, nil
}

func (s *Service) processRecord(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, rec InputRecord) error {
	sourcePath, err := s.resolveSourcePath(rec.FileName)
	if err != nil {
		return s.recordFailure(ctx, db, rec, err, "MID_26032601")
	}

	repoDir, err := chooseLeastUsedRepoDir(s.cfg.RepoDirs)
	if err != nil {
		return s.recordFailure(ctx, db, rec, err, "MID_26032602")
	}

	resultFilename := fmt.Sprintf("ocr_rslt_%d.json", rec.ID)
	resultPath := filepath.Join(repoDir, resultFilename)

	if err := s.runPaddleOCR(ctx, sourcePath, resultPath, rec.ID); err != nil {
		return s.recordFailure(ctx, db, rec, err, "MID_26032603")
	}

	repoPDFPath := filepath.Join(repoDir, filepath.Base(sourcePath))
	if err := copyFile(sourcePath, repoPDFPath); err != nil {
		return s.recordFailure(ctx, db, rec, fmt.Errorf("(MID_26032613) copy file to repo failed: %w", err), "MID_26032604")
	}

	backupPDFPath, err := copyToBackup(sourcePath, s.cfg.BackupDir)
	if err != nil {
		return s.recordFailure(ctx, db, rec, fmt.Errorf("(MID_26032614) copy file to backup failed: %w", err), "MID_26032605")
	}

	if s.cfg.DeleteFromStaging {
		if inside, chkErr := isWithinDir(sourcePath, s.cfg.StagingDir); chkErr == nil && inside {
			if rmErr := os.Remove(sourcePath); rmErr != nil {
				logger.Warn("failed to remove staged PDF after processing",
					"input_id", rec.ID,
					"path", sourcePath,
					"error", rmErr,
				)
			}
		}
	}

	statusJSON, err := appendStatusEntry(rec.Status, OperationStatus{
		Operation: parseOperationName,
		Time:      time.Now().Format(timeLayoutStatus),
		Status:    statusSuccess,
		Error:     "",
	})
	if err != nil {
		return s.recordFailure(ctx, db, rec, fmt.Errorf("(MID_26032615) build success status failed: %w", err), "MID_26032606")
	}

	_, err = db.ExecContext(ctx, s.updateSuccessSQL,
		statusJSON,
		resultFilename,
		repoPDFPath,
		backupPDFPath,
		rec.ID,
	)
	if err != nil {
		return fmt.Errorf("(MID_26032616) update success state failed: %w", err)
	}

	logger.Info("processed PDF input successfully",
		"input_id", rec.ID,
		"source", sourcePath,
		"repo_file", repoPDFPath,
		"backup_file", backupPDFPath,
		"result_file", resultPath,
	)
	return nil
}

func (s *Service) recordFailure(ctx context.Context, db *sql.DB, rec InputRecord, processErr error, loc string) error {
	error_msg := fmt.Sprintf("%s (%s)", processErr.Error(), loc)
	statusJSON, statusErr := appendStatusEntry(rec.Status, OperationStatus{
		Operation: parseOperationName,
		Time:      time.Now().Format(timeLayoutStatus),
		Status:    statusFail,
		Error:     error_msg,
	})

	if statusErr != nil {
		return fmt.Errorf("(MID_26032617) processing error: %v; status update build error: %w", error_msg, statusErr)
	}

	_, err := db.ExecContext(ctx, s.updateFailureSQL, statusJSON, error_msg, rec.ID)
	if err != nil {
		return fmt.Errorf("(MID_26032618) processing error: %v; db update error: %w", error_msg, err)
	}
	return processErr
}

func (s *Service) resolveSourcePath(fileName string) (string, error) {
	if strings.TrimSpace(fileName) == "" {
		return "", errors.New("input file_name is empty")
	}

	if filepath.IsAbs(fileName) {
		if _, err := os.Stat(fileName); err != nil {
			return "", fmt.Errorf("(MID_26032619) source file not accessible: %w", err)
		}
		return fileName, nil
	}

	candidate := filepath.Join(s.cfg.StagingDir, fileName)
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("(MID_26032620) source file not accessible under staging dir: %w", err)
	}
	return candidate, nil
}

func (s *Service) runPaddleOCR(ctx context.Context, pdfPath string, outputJSONPath string, inputID int64) error {
	baseWorkDir := strings.TrimSpace(s.cfg.WorkDir)
	if baseWorkDir == "" {
		baseWorkDir = os.TempDir()
	}
	if !filepath.IsAbs(baseWorkDir) {
		if stagingBase := strings.TrimSpace(os.Getenv("DATA_STAGING_DIR")); stagingBase != "" {
			baseWorkDir = filepath.Join(stagingBase, baseWorkDir)
		} else {
			baseWorkDir = filepath.Join(s.cfg.StagingDir, baseWorkDir)
		}
	}
	if absWorkDir, err := filepath.Abs(baseWorkDir); err == nil {
		baseWorkDir = absWorkDir
	}
	if err := os.MkdirAll(baseWorkDir, 0755); err != nil {
		return fmt.Errorf("(MID_26032621) ensure OCR work dir failed (%s): %w", baseWorkDir, err)
	}

	workDir, err := os.MkdirTemp(baseWorkDir, "pdf-parser-ocr-*")
	if err != nil {
		return fmt.Errorf("(MID_26032621) create OCR temp dir failed: %w", err)
	}
	defer os.RemoveAll(workDir)

	args := []string{s.cfg.PaddleOCRScript, pdfPath, "-o", workDir}
	if s.cfg.UsePaddleOCRVL {
		args = append(args, "--vl")
	}

	cmd := exec.CommandContext(ctx, s.cfg.PythonBin, args...)
	cmd.Dir = filepath.Dir(s.cfg.PaddleOCRScript)
	cmd.Env = buildPythonEnv(s.cfg.PythonBin)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("(MID_26032622) PaddleOCR command failed: %w (output: %s)", err, strings.TrimSpace(output.String()))
	}

	pageFiles, err := filepath.Glob(filepath.Join(workDir, "page_*.json"))
	if err != nil {
		return fmt.Errorf("(MID_26032623) scan OCR page json files failed: %w", err)
	}
	if len(pageFiles) == 0 {
		return fmt.Errorf("(MID_26032624) PaddleOCR produced no page json files (output: %s)", strings.TrimSpace(output.String()))
	}
	sort.Strings(pageFiles)

	pages := make([]any, 0, len(pageFiles))
	for _, path := range pageFiles {
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("(MID_26032625) read OCR page file failed (%s): %w", path, err)
		}
		var decoded any
		if err := json.Unmarshal(b, &decoded); err != nil {
			return fmt.Errorf("(MID_26032626) parse OCR page file failed (%s): %w", path, err)
		}
		pages = append(pages, decoded)
	}

	result := map[string]any{
		"input_id":     inputID,
		"source_pdf":   pdfPath,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"engine":       "paddleocr",
		"pages":        pages,
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("(MID_26032627) marshal OCR result failed: %w", err)
	}

	if err := os.WriteFile(outputJSONPath, encoded, 0644); err != nil {
		return fmt.Errorf("(MID_26032628) write OCR result json failed: %w", err)
	}

	return nil
}

func applyDefaults(cfg Config) Config {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 25
	}
	if cfg.PythonBin == "" {
		cfg.PythonBin = "python3"
	}
	if cfg.PaddleOCRScript == "" {
		cfg.PaddleOCRScript = "/Users/cding/Workspace/ThirdParty/paddleocr/parse_pdf.py"
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = os.TempDir()
	}
	if !cfg.DeleteFromStaging {
		cfg.DeleteFromStaging = true
	}
	return cfg
}

func (cfg Config) validate() error {
	if strings.TrimSpace(cfg.StagingDir) == "" {
		return errors.New("staging dir is required")
	}
	if len(cfg.RepoDirs) == 0 {
		return errors.New("at least one repo dir is required")
	}
	if strings.TrimSpace(cfg.BackupDir) == "" {
		return errors.New("backup dir is required")
	}
	if strings.TrimSpace(cfg.PaddleOCRScript) == "" {
		return errors.New("PaddleOCR script path is required")
	}
	if _, err := os.Stat(cfg.PaddleOCRScript); err != nil {
		return fmt.Errorf("(MID_26032629) PaddleOCR script path is not accessible: %w", err)
	}
	for _, repoDir := range cfg.RepoDirs {
		if strings.TrimSpace(repoDir) == "" {
			return errors.New("repo dir cannot be empty")
		}
	}
	return nil
}

func chooseLeastUsedRepoDir(repoDirs []string) (string, error) {
	var selected string
	var minBytes int64 = -1

	for _, dir := range repoDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("(MID_26032630) ensure repo dir failed (%s): %w", dir, err)
		}
		sz, err := dirSizeBytes(dir)
		if err != nil {
			return "", err
		}
		if minBytes < 0 || sz < minBytes {
			selected = dir
			minBytes = sz
		}
	}
	if selected == "" {
		return "", errors.New("no repo directory selected")
	}
	return selected, nil
}

func dirSizeBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("(MID_26032631) walk repo dir failed (%s): %w", root, err)
	}
	return total, nil
}

func copyToBackup(srcPath string, backupDir string) (string, error) {
	dstName := filepath.Base(srcPath)

	if isRemoteDestination(backupDir) {
		destination := strings.TrimRight(backupDir, "/") + "/" + dstName
		cmd := exec.Command("scp", "-q", srcPath, destination)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("(MID_26032632) scp backup failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return destination, nil
	}

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("(MID_26032633) create backup dir failed: %w", err)
	}
	dstPath := filepath.Join(backupDir, dstName)
	if err := copyFile(srcPath, dstPath); err != nil {
		return "", err
	}
	return dstPath, nil
}

func isRemoteDestination(dest string) bool {
	if strings.Contains(dest, "://") {
		return true
	}
	idx := strings.Index(dest, ":")
	if idx <= 0 {
		return false
	}
	// Windows drive (e.g. C:\)
	if len(dest) >= 2 && ((dest[0] >= 'A' && dest[0] <= 'Z') || (dest[0] >= 'a' && dest[0] <= 'z')) && dest[1] == ':' {
		return false
	}
	return true
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("(MID_26032634) open source file failed: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("(MID_26032635) create target dir failed: %w", err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("(MID_26032636) create target file failed: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("(MID_26032637) copy file content failed: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("(MID_26032638) sync target file failed: %w", err)
	}
	return nil
}

func isWithinDir(path, dir string) (bool, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..", nil
}

// buildPythonEnv returns the current environment cleaned up for running a
// venv Python binary directly. It removes PYTHONHOME and PYTHONPATH (which
// can redirect package lookups away from the venv's site-packages when set
// by nix or other environment managers) and sets VIRTUAL_ENV to the venv
// directory inferred from pythonBin (.venv/bin/python → .venv).
func buildPythonEnv(pythonBin string) []string {
	venvDir := filepath.Dir(filepath.Dir(pythonBin)) // .venv/bin/python → .venv
	env := os.Environ()
	result := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "VIRTUAL_ENV=") ||
			strings.HasPrefix(e, "PYTHONHOME=") ||
			strings.HasPrefix(e, "PYTHONPATH=") {
			continue
		}
		result = append(result, e)
	}
	result = append(result, "VIRTUAL_ENV="+venvDir)
	return result
}
