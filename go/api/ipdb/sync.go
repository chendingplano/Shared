package ipdb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const (
	downloadURL     = "https://downloads.ip66.dev/db/ip66.mmdb"
	defaultFilePath = "/var/data/ip66.mmdb"
	syncInterval    = 24 * time.Hour
)

// service holds all runtime state for the ipdb package.
type service struct {
	filePath     string
	cacheTTLDays int
	syncing      atomic.Bool
	cancel       context.CancelFunc
}

var svc = &service{
	filePath:     defaultFilePath,
	cacheTTLDays: 7,
}

// Init initialises the ipdb service.
// Call once at application startup, after the database is ready.
//
//   - If the MMDB file already exists on disk it is loaded immediately.
//   - A background goroutine then syncs every 24 hours.
//
// Environment variables:
//   - IPDB_FILE_PATH      – where to store the MMDB file (default /var/data/ip66.mmdb)
//   - IPDB_CACHE_TTL_DAYS – lookup cache TTL in days (default 7)
func Init(logger ApiTypes.JimoLogger) {
	if p := os.Getenv("IPDB_FILE_PATH"); p != "" {
		svc.filePath = p
	}
	if d := os.Getenv("IPDB_CACHE_TTL_DAYS"); d != "" {
		var days int
		if _, err := fmt.Sscanf(d, "%d", &days); err == nil && days > 0 {
			svc.cacheTTLDays = days
		}
	}

	logger.Info("ipdb: initialising",
		"file_path", svc.filePath,
		"cache_ttl_days", svc.cacheTTLDays,
		"sync_interval", syncInterval)

	// Load existing MMDB if present
	if _, err := os.Stat(svc.filePath); err == nil {
		if err := openDB(svc.filePath); err != nil {
			logger.Warn("ipdb: could not open existing MMDB, will download on first sync",
				"error", err)
		} else {
			logger.Info("ipdb: loaded existing MMDB from disk", "path", svc.filePath)
		}
	} else {
		logger.Info("ipdb: no local MMDB found, triggering immediate download")
		if err := Sync(logger); err != nil {
			logger.Warn("ipdb: initial sync failed", "error", err)
		}
	}

	// Start background sync loop
	ctx, cancel := context.WithCancel(context.Background())
	svc.cancel = cancel
	go syncLoop(ctx, logger)
}

// Shutdown stops the background sync loop and closes the MMDB reader.
func Shutdown() {
	if svc.cancel != nil {
		svc.cancel()
	}
	closeDB()
}

// syncLoop runs Sync every syncInterval until ctx is cancelled.
func syncLoop(ctx context.Context, logger ApiTypes.JimoLogger) {
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := Sync(logger); err != nil {
				logger.Warn("ipdb: periodic sync failed", "error", err)
			}
		}
	}
}

// Sync downloads the latest MMDB from ip66.dev and hot-swaps it into the reader.
// It is safe to call concurrently; concurrent calls beyond the first are no-ops.
func Sync(logger ApiTypes.JimoLogger) error {
	if !svc.syncing.CompareAndSwap(false, true) {
		logger.Info("ipdb: sync already in progress, skipping")
		return nil
	}
	defer svc.syncing.Store(false)

	logger.Info("ipdb: starting sync", "url", downloadURL)
	start := time.Now()

	fileSize, err := download(svc.filePath)

	db := ApiTypes.ProjectDBHandle
	if db != nil {
		status := "success"
		errMsg := ""
		if err != nil {
			status = "failure"
			errMsg = err.Error()
		}
		if logErr := writeSyncLog(db, status, fileSize, errMsg); logErr != nil {
			logger.Warn("ipdb: failed to write sync log", "error", logErr)
		}

		// Purge stale cache entries so lookups reflect new data
		if err == nil {
			purged, purgeErr := purgeStaleCacheEntries(db, svc.cacheTTLDays)
			if purgeErr != nil {
				logger.Warn("ipdb: cache purge failed", "error", purgeErr)
			} else {
				logger.Info("ipdb: cache purge complete", "rows_deleted", purged)
			}
		}
	}

	if err != nil {
		logger.Warn("ipdb: sync failed", "error", err, "elapsed", time.Since(start))
		return err
	}

	// Hot-swap the MMDB reader
	if err := openDB(svc.filePath); err != nil {
		logger.Warn("ipdb: failed to open downloaded MMDB", "error", err)
		return err
	}

	logger.Info("ipdb: sync complete",
		"file_size_bytes", fileSize,
		"elapsed", time.Since(start))
	return nil
}

// download fetches the MMDB from url and writes it atomically to dest.
// Returns the number of bytes written.
func download(dest string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, fmt.Errorf("ipdb: cannot create directory (SHD_IPD_330): %w", err)
	}

	// Write to a temp file first for atomic replacement
	tmp := dest + ".tmp"

	resp, err := http.Get(downloadURL) //nolint:gosec // URL is a constant
	if err != nil {
		return 0, fmt.Errorf("ipdb: HTTP GET failed (SHD_IPD_337): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("ipdb: unexpected HTTP status (SHD_IPD_342): %d", resp.StatusCode)
	}

	f, err := os.Create(tmp)
	if err != nil {
		return 0, fmt.Errorf("ipdb: cannot create temp file (SHD_IPD_347): %w", err)
	}

	n, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return 0, fmt.Errorf("ipdb: download write failed (SHD_IPD_354): %w", err)
	}

	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return 0, fmt.Errorf("ipdb: atomic rename failed (SHD_IPD_359): %w", err)
	}

	return n, nil
}
