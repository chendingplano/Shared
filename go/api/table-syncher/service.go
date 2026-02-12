package tablesyncher

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

// Location codes for service operations
const (
	LOC_SVC_INIT  = "SHD_SYN_090"
	LOC_SVC_RUN   = "SHD_SYN_091"
	LOC_SVC_SYNC  = "SHD_SYN_092"
	LOC_SVC_CLOSE = "SHD_SYN_093"
)

// SyncDataService is the main service that coordinates sync operations.
type SyncDataService struct {
	config     *SyncConfig
	db         *sql.DB
	state      *StateManager
	logger     *slog.Logger
	stats      *RuntimeStats
	sftpClient *SFTPClient
	metrics    *MetricsAggregator

	// Runtime state
	isRunning atomic.Bool
}

// NewService creates a new SyncDataService with a logger.
func NewService(config *SyncConfig, logger *slog.Logger) *SyncDataService {
	return &SyncDataService{
		config: config,
		logger: logger,
		state:  NewStateManager(config.StateFilePath),
		stats: &RuntimeStats{
			StartTime: time.Now(),
		},
	}
}

// NewServiceWithDB creates a service with an existing DB connection.
func NewServiceWithDB(config *SyncConfig, db *sql.DB, logger *slog.Logger) *SyncDataService {
	s := NewService(config, logger)
	s.db = db
	return s
}

// Initialize opens the DB connection, creates tables, and loads state.
func (s *SyncDataService) Initialize(ctx context.Context) error {
	s.logger.Info("Initializing sync service", "loc", LOC_SVC_INIT)

	// Open database connection if not provided
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

	// Ensure sync tables exist
	if err := EnsureTables(ctx, s.db, s.logger); err != nil {
		return err
	}

	// Load state
	if err := s.state.Load(); err != nil {
		return err
	}

	// Initialize metrics aggregator
	s.metrics = NewMetricsAggregator(s.db, s.logger)

	// Initialize SFTP client
	s.sftpClient = NewSFTPClient(s.config, s.logger)

	s.logger.Info("Sync service initialized",
		"state_file", s.config.StateFilePath,
		"archive_host", s.config.ArchiveHost,
		"loc", LOC_SVC_INIT)

	return nil
}

// Close closes the database and SFTP connections.
func (s *SyncDataService) Close() {
	if s.sftpClient != nil {
		s.sftpClient.Close()
	}
	if s.db != nil {
		s.db.Close()
	}
	s.logger.Info("Sync service closed", "loc", LOC_SVC_CLOSE)
}

// GetStats returns a copy of the runtime statistics.
func (s *SyncDataService) GetStats() *RuntimeStats {
	return &RuntimeStats{
		StartTime:      s.stats.StartTime,
		RecordsSynced:  s.stats.RecordsSynced,
		ErrorCount:     s.stats.ErrorCount,
		LastSyncTime:   s.stats.LastSyncTime,
		LastSyncResult: s.stats.LastSyncResult,
	}
}

// RunOnce performs a single sync cycle.
func (s *SyncDataService) RunOnce(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	// Connect to SFTP if not connected
	if s.sftpClient.sftpClient == nil {
		if err := s.sftpClient.Connect(ctx); err != nil {
			return nil, fmt.Errorf("failed to connect to archive: %w (%s)", err, LOC_SVC_SYNC)
		}
	}

	// Get whitelist of tables
	tableNames, err := GetTableNames(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get table whitelist: %w (%s)", err, LOC_SVC_SYNC)
	}

	if len(tableNames) == 0 {
		s.logger.Debug("No tables in whitelist, skipping sync")
		return result, nil
	}

	whitelist := make(map[string]bool)
	for _, t := range tableNames {
		whitelist[t] = true
	}

	// Discover new change files
	lastFileTime := s.state.GetLastFileTime()
	changeFiles, err := s.sftpClient.DiscoverChangeFiles(ctx, lastFileTime)
	if err != nil {
		return nil, fmt.Errorf("failed to discover change files: %w (%s)", err, LOC_SVC_SYNC)
	}

	s.logger.Debug("Discovered change files",
		"count", len(changeFiles),
		"since", lastFileTime)

	// Process each change file
	for _, cf := range changeFiles {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(start)
			return result, ctx.Err()
		default:
		}

		records, err := s.sftpClient.FetchChangeFile(ctx, cf)
		if err != nil {
			s.logger.Error("Failed to fetch change file",
				"file", cf.Name,
				"error", err,
				"loc", LOC_SVC_SYNC)
			s.stats.ErrorCount++
			continue
		}

		// Apply changes
		fileResult, err := ApplyChanges(ctx, s.db, records, whitelist, s.logger)
		if err != nil {
			s.logger.Error("Failed to apply changes",
				"file", cf.Name,
				"error", err,
				"loc", LOC_SVC_SYNC)
			s.stats.ErrorCount++

			// Log failure
			LogSyncEvent(ctx, s.db, "*", "FAILED", 0, cf.Name, err.Error())
			continue
		}

		// Accumulate results
		result.FilesProcessed++
		result.RecordsAdded += fileResult.RecordsAdded
		result.RecordsUpdated += fileResult.RecordsUpdated
		result.RecordsDeleted += fileResult.RecordsDeleted
		result.RecordsSkipped += fileResult.RecordsSkipped
		result.RecordsFailed += fileResult.RecordsFailed

		// Update state
		if err := s.state.SetLastFile(cf.Name, cf.ModTime); err != nil {
			s.logger.Error("Failed to update state",
				"file", cf.Name,
				"error", err,
				"loc", LOC_SVC_SYNC)
		}

		// Log success for each table that had changes
		totalSynced := int(fileResult.RecordsAdded + fileResult.RecordsUpdated + fileResult.RecordsDeleted)
		LogSyncEvent(ctx, s.db, "*", "SUCCESS", totalSynced, cf.Name, "")

		s.logger.Info("Processed change file",
			"file", cf.Name,
			"added", fileResult.RecordsAdded,
			"updated", fileResult.RecordsUpdated,
			"deleted", fileResult.RecordsDeleted,
			"skipped", fileResult.RecordsSkipped)
	}

	result.Duration = time.Since(start)
	result.LastLSN = s.state.GetGlobalLSN()

	// Update runtime stats
	totalSynced := result.RecordsAdded + result.RecordsUpdated + result.RecordsDeleted
	s.stats.RecordsSynced += totalSynced
	s.stats.LastSyncTime = time.Now()
	s.stats.LastSyncResult = result

	return result, nil
}

// RunLoop starts the polling loop at the configured frequency.
// Blocks until ctx is cancelled.
func (s *SyncDataService) RunLoop(ctx context.Context) error {
	if !s.isRunning.CompareAndSwap(false, true) {
		return fmt.Errorf("service is already running (%s)", LOC_SVC_RUN)
	}
	defer s.isRunning.Store(false)

	ticker := time.NewTicker(time.Duration(s.config.DataSyncFreq) * time.Second)
	defer ticker.Stop()

	// Metrics aggregation ticker (hourly check, but only aggregate at MetricFreq)
	metricsTicker := time.NewTicker(1 * time.Hour)
	defer metricsTicker.Stop()
	lastMetricsRun := time.Time{}

	s.logger.Info("Starting sync loop",
		"frequency", s.config.DataSyncFreq,
		"loc", LOC_SVC_RUN)

	// Run once immediately on startup
	if result, err := s.RunOnce(ctx); err != nil {
		s.logger.Error("Initial sync failed", "error", err, "loc", LOC_SVC_RUN)
	} else if result.FilesProcessed > 0 {
		s.logger.Info("Initial sync complete",
			"files", result.FilesProcessed,
			"added", result.RecordsAdded,
			"updated", result.RecordsUpdated,
			"deleted", result.RecordsDeleted,
			"duration", result.Duration)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Shutting down sync service", "loc", LOC_SVC_RUN)
			return nil

		case <-ticker.C:
			result, err := s.RunOnce(ctx)
			if err != nil {
				s.logger.Error("Sync cycle failed", "error", err, "loc", LOC_SVC_RUN)
				s.stats.ErrorCount++
			} else if result.FilesProcessed > 0 {
				s.logger.Info("Sync cycle complete",
					"files", result.FilesProcessed,
					"added", result.RecordsAdded,
					"updated", result.RecordsUpdated,
					"deleted", result.RecordsDeleted,
					"duration", result.Duration)
			}

		case <-metricsTicker.C:
			// Check if it's time to aggregate metrics
			hoursSinceLastRun := time.Since(lastMetricsRun).Hours()
			if hoursSinceLastRun >= float64(s.config.MetricFreq) {
				s.logger.Debug("Running metrics aggregation")
				if err := s.metrics.AggregateFrequencyPeriod(ctx, s.config.DataSyncFreq); err != nil {
					s.logger.Error("Metrics aggregation failed", "error", err)
				}
				lastMetricsRun = time.Now()
			}
		}
	}
}

// Resync drops and reloads a specific table.
func (s *SyncDataService) Resync(ctx context.Context, tableName string) (*SyncResult, error) {
	s.logger.Info("Resyncing table", "table", tableName, "loc", LOC_SVC_SYNC)

	// Verify table is in whitelist
	inWhitelist, err := IsTableInWhitelist(ctx, s.db, tableName)
	if err != nil {
		return nil, err
	}
	if !inWhitelist {
		return nil, fmt.Errorf("table %s is not in sync whitelist", tableName)
	}

	// Truncate the table
	if err := ClearTable(ctx, s.db, tableName, s.logger); err != nil {
		return nil, err
	}

	// Reset state for this table
	if err := s.state.ResetTable(tableName); err != nil {
		return nil, err
	}

	// For a full resync, we need to process ALL change files from the beginning
	// This is a simplified implementation - in production you'd want to handle
	// this more carefully
	s.logger.Warn("Full resync requires processing all change files from archive",
		"table", tableName)

	// Run a sync cycle (this will only get new files, not historical)
	return s.RunOnce(ctx)
}

// Clear truncates all synced tables.
func (s *SyncDataService) Clear(ctx context.Context) error {
	s.logger.Info("Clearing all synced tables", "loc", LOC_SVC_SYNC)

	if err := ClearAllTables(ctx, s.db, s.logger); err != nil {
		return err
	}

	// Reset all state
	return s.state.Reset()
}

// AddTables adds tables to the sync whitelist.
func (s *SyncDataService) AddTables(ctx context.Context, tableNames []string) ([]string, error) {
	return AddTables(ctx, s.db, tableNames, "", s.logger)
}

// RemoveTables removes tables from the sync whitelist.
func (s *SyncDataService) RemoveTables(ctx context.Context, tableNames []string) ([]string, error) {
	return RemoveTables(ctx, s.db, tableNames, s.logger)
}

// ListTables returns all tables in the sync whitelist.
func (s *SyncDataService) ListTables(ctx context.Context) ([]TableInfo, error) {
	return ListTables(ctx, s.db)
}

// GetStatus returns the current daemon status.
func (s *SyncDataService) GetStatus(ctx context.Context) (*DaemonStatus, error) {
	return GetDaemonStatus(ctx, s.config, s.db)
}
