// Package tablesyncher provides PostgreSQL table synchronization from production
// to local instances using logical decoding change files.
package tablesyncher

import (
	"time"
)

// ChangeOperation represents the type of database operation.
type ChangeOperation string

const (
	OpInsert ChangeOperation = "INSERT"
	OpUpdate ChangeOperation = "UPDATE"
	OpDelete ChangeOperation = "DELETE"
)

// ChangeRecord represents a single change from the logical decoding output.
// JSON format: {"table": "users", "op": "INSERT", "data": {...}, "lsn": "0/16B3D40", "ts": "..."}
type ChangeRecord struct {
	Table   string                 `json:"table"`
	Op      ChangeOperation        `json:"op"`
	Data    map[string]any         `json:"data,omitempty"`    // For INSERT/UPDATE: new values
	OldKeys map[string]any         `json:"old_keys,omitempty"` // For UPDATE/DELETE: primary key values
	LSN     string                 `json:"lsn"`               // Log Sequence Number
	TS      time.Time              `json:"ts"`                // Timestamp of change
}

// SyncStatus represents the current daemon status.
type SyncStatus string

const (
	StatusActive     SyncStatus = "active"
	StatusNotStarted SyncStatus = "not-started"
)

// SyncResult summarizes a single sync cycle.
type SyncResult struct {
	FilesProcessed int
	RecordsAdded   int64
	RecordsUpdated int64
	RecordsDeleted int64
	RecordsSkipped int64 // Filtered out (not in whitelist)
	RecordsFailed  int64 // Failed to apply
	Duration       time.Duration
	LastLSN        string
}

// TableInfo represents a table in the sync whitelist.
type TableInfo struct {
	ID        int       `json:"id"`
	TableName string    `json:"table_name"`
	Creator   string    `json:"creator,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SyncLogEntry represents an entry in the data_sync_logs table.
type SyncLogEntry struct {
	ID          string    `json:"id"`
	TableName   string    `json:"table_name"`
	Status      string    `json:"status"` // SUCCESS, FAILED
	RowsSynced  int       `json:"rows_synced"`
	ArchiveRef  string    `json:"archive_ref,omitempty"` // Filename or LSN
	ErrorDetail string    `json:"error_detail,omitempty"`
	SyncTime    time.Time `json:"sync_time"`
}

// SyncMetric represents aggregated metrics in data_sync_metrics.
type SyncMetric struct {
	ID             int       `json:"id"`
	TableName      string    `json:"table_name"`
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
	PeriodType     string    `json:"period_type"` // FREQ, WEEK, MONTH
	RecordsAdded   int64     `json:"records_added"`
	RecordsUpdated int64     `json:"records_updated"`
	RecordsDeleted int64     `json:"records_deleted"`
}

// RuntimeStats tracks service statistics since startup.
type RuntimeStats struct {
	StartTime         time.Time
	RecordsSynced     int64
	ErrorCount        int64
	LastSyncTime      time.Time
	LastSyncResult    *SyncResult
}

// DaemonStatus represents the full status output for the CLI.
type DaemonStatus struct {
	Status        SyncStatus    `json:"status"`
	SyncFrequency int           `json:"sync_frequency"` // seconds
	StartTime     time.Time     `json:"start_time,omitempty"`
	RecordsSynced int64         `json:"records_synced"`
	Errors        int64         `json:"errors"`
	LastSyncTime  time.Time     `json:"last_sync_time,omitempty"`
	Tables        []TableInfo   `json:"tables,omitempty"`
}

// ChangeFile represents a discovered change file from the archive.
type ChangeFile struct {
	Name      string    // Filename
	Path      string    // Full path on remote
	Size      int64     // File size in bytes
	ModTime   time.Time // Last modification time
}
