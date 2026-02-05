package logs2db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"time"
)

// Location codes for service operations
const (
	LOC_SVC_INIT   = "SHD_L2D_060"
	LOC_SVC_RUN    = "SHD_L2D_061"
	LOC_SVC_SCAN   = "SHD_L2D_062"
	LOC_SVC_RELOAD = "SHD_L2D_063"
)

// ScanResult summarizes one scan cycle.
type ScanResult struct {
	FilesScanned  int
	LinesInserted int
	LinesSkipped  int // already loaded
	LinesFailed   int // malformed JSON
	Duration      time.Duration
}

// RuntimeStats tracks service statistics since the service started.
type RuntimeStats struct {
	StartTime        time.Time
	EntriesSinceStart atomic.Int64
	TotalErrors       atomic.Int64
}

// Log2DBService is the main service that coordinates scanning, parsing,
// and inserting log entries.
type Log2DBService struct {
	config *Log2DBConfig
	db     *sql.DB
	state  *StateManager
	logger *slog.Logger
	stats  *RuntimeStats
}

// NewService creates a new Log2DBService with a logger.
func NewService(config *Log2DBConfig, logger *slog.Logger) *Log2DBService {
	return &Log2DBService{
		config: config,
		logger: logger,
		state:  NewStateManager(config.StateFilePath),
		stats: &RuntimeStats{
			StartTime: time.Now(),
		},
	}
}

// NewServiceWithDB creates a service with an existing DB connection.
func NewServiceWithDB(config *Log2DBConfig, db *sql.DB, logger *slog.Logger) *Log2DBService {
	s := NewService(config, logger)
	s.db = db
	return s
}

// Initialize opens the DB connection (if not provided), creates the target
// table if needed, and loads the state file.
func (s *Log2DBService) Initialize(ctx context.Context) error {
	if s.db == nil {
		db, err := sql.Open("postgres", s.config.ConnectionString())
		if err != nil {
			return fmt.Errorf("failed to open database: %w (%s)", err, LOC_SVC_INIT)
		}

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			db.Close()
			return fmt.Errorf("failed to connect to database: %w (%s)", err, LOC_SVC_INIT)
		}

		s.db = db
	}

	if err := s.EnsureTable(ctx); err != nil {
		return err
	}

	if err := s.state.Load(); err != nil {
		return err
	}

	return nil
}

// Close closes the database connection.
func (s *Log2DBService) Close() {
	if s.db != nil {
		s.db.Close()
	}
}

// GetStats returns a copy of the runtime statistics.
func (s *Log2DBService) GetStats() RuntimeStats {
	return RuntimeStats{
		StartTime: s.stats.StartTime,
	}
}

// RunOnce performs a single scan cycle: discover files, read new lines, insert.
func (s *Log2DBService) RunOnce(ctx context.Context) (*ScanResult, error) {
	start := time.Now()
	result := &ScanResult{}

	files, err := s.DiscoverLogFiles()
	if err != nil {
		return nil, err
	}

	for _, filePath := range files {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(start)
			return result, ctx.Err()
		default:
		}

		basename := filepath.Base(filePath)
		lastLine := s.state.GetLastLine(basename)

		entries, lastLineRead, err := s.ScanFile(ctx, filePath, lastLine)
		if err != nil {
			s.logger.Error("Failed to scan file",
				"file", basename,
				"error", err,
				"loc", LOC_SVC_SCAN)
			s.stats.TotalErrors.Add(1)
			continue
		}

		result.FilesScanned++
		result.LinesSkipped += lastLine

		if len(entries) == 0 {
			// Update state even if no new entries (file might have been read to end)
			if lastLineRead > lastLine {
				s.state.SetLastLine(basename, lastLineRead)
			}
			continue
		}

		// Count failed entries
		for _, e := range entries {
			if e.ErrorMsg != "" {
				result.LinesFailed++
			}
		}

		inserted, err := s.InsertBatch(ctx, entries)
		if err != nil {
			s.logger.Error("Failed to insert entries",
				"file", basename,
				"count", len(entries),
				"error", err,
				"loc", LOC_SVC_SCAN)
			s.stats.TotalErrors.Add(1)
			continue
		}

		result.LinesInserted += inserted
		s.stats.EntriesSinceStart.Add(int64(inserted))

		// Update state with the last line we read
		if err := s.state.SetLastLine(basename, lastLineRead); err != nil {
			s.logger.Error("Failed to save state",
				"file", basename,
				"error", err,
				"loc", LOC_SVC_SCAN)
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// RunLoop starts the polling loop at the configured frequency.
// Blocks until ctx is cancelled.
func (s *Log2DBService) RunLoop(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(s.config.SyncFreqSec) * time.Second)
	defer ticker.Stop()

	// Run once immediately on startup
	if result, err := s.RunOnce(ctx); err != nil {
		s.logger.Error("Initial scan failed", "error", err, "loc", LOC_SVC_RUN)
	} else if result.LinesInserted > 0 {
		s.logger.Info("Initial scan complete",
			"files", result.FilesScanned,
			"inserted", result.LinesInserted,
			"failed", result.LinesFailed,
			"duration", result.Duration)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Shutting down log2db service")
			return nil
		case <-ticker.C:
			result, err := s.RunOnce(ctx)
			if err != nil {
				s.logger.Error("Scan cycle failed", "error", err, "loc", LOC_SVC_RUN)
				s.stats.TotalErrors.Add(1)
			} else if result.LinesInserted > 0 {
				s.logger.Info("Scan cycle complete",
					"files", result.FilesScanned,
					"inserted", result.LinesInserted,
					"failed", result.LinesFailed,
					"duration", result.Duration)
			}
		}
	}
}

// Reload truncates the table, resets state, and reloads all files.
func (s *Log2DBService) Reload(ctx context.Context) (*ScanResult, error) {
	s.logger.Info("Reloading: truncating table and rescanning all files",
		"table", s.config.DBTableName,
		"loc", LOC_SVC_RELOAD)

	if err := s.TruncateTable(ctx); err != nil {
		return nil, err
	}

	if err := s.state.Reset(); err != nil {
		return nil, fmt.Errorf("failed to reset state: %w (%s)", err, LOC_SVC_RELOAD)
	}

	return s.RunOnce(ctx)
}
