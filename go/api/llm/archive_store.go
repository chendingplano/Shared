package llm

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"time"
)

type UsageArchivePaths struct {
	DayDir         string
	AccountDir     string
	InputBodyPath  string
	OutputBodyPath string
}

func BuildUsageArchivePaths(root string, day time.Time, accountID string, eventID string) UsageArchivePaths {
	dayDir := filepath.Join(
		root,
		day.Format("2006"),
		day.Format("2006-01"),
		day.Format("2006-01-02"),
	)
	if accountID == "" {
		accountID = "unknown"
	}
	accountDir := filepath.Join(dayDir, "account-"+accountID)
	bodiesDir := filepath.Join(accountDir, "bodies")

	return UsageArchivePaths{
		DayDir:         dayDir,
		AccountDir:     accountDir,
		InputBodyPath:  filepath.Join(bodiesDir, eventID+"-input.json.gz"),
		OutputBodyPath: filepath.Join(bodiesDir, eventID+"-output.json.gz"),
	}
}

func WriteGzipFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := gzip.NewWriter(f)
	if _, err := zw.Write(body); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return nil
}

func ReadGzipFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	return io.ReadAll(zr)
}
