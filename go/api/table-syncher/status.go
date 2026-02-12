package tablesyncher

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Location codes for status operations
const (
	LOC_STATUS_CHECK = "SHD_SYN_080"
	LOC_STATUS_PID   = "SHD_SYN_081"
)

// GetDaemonStatus returns the current daemon status.
func GetDaemonStatus(ctx context.Context, config *SyncConfig, db *sql.DB) (*DaemonStatus, error) {
	status := &DaemonStatus{
		Status:        StatusNotStarted,
		SyncFrequency: config.DataSyncFreq,
	}

	// Check if daemon is running via PID file
	pid, running := checkDaemonRunning(config.PIDFilePath)
	if running {
		status.Status = StatusActive

		// Try to get start time from state file
		state := NewStateManager(config.StateFilePath)
		if err := state.Load(); err == nil {
			// Estimate start time from first sync cycle minus frequency
			if !state.GetLastSyncCycle().IsZero() {
				// This is an approximation
				status.StartTime = state.GetLastSyncCycle()
			}
			status.RecordsSynced = state.GetTotalSynced()
		}
	}

	// Get error count from database
	if db != nil {
		// Get errors since start time (or last 24 hours if unknown)
		since := status.StartTime
		if since.IsZero() {
			since = time.Now().Add(-24 * time.Hour)
		}

		errorCount, err := GetErrorCount(ctx, db, since)
		if err == nil {
			status.Errors = errorCount
		}

		// Get tables
		tables, err := ListTables(ctx, db)
		if err == nil {
			status.Tables = tables
		}
	}

	// Get last sync time from state
	state := NewStateManager(config.StateFilePath)
	if err := state.Load(); err == nil {
		status.LastSyncTime = state.GetLastSyncCycle()
	}

	_ = pid // unused but available for future use
	return status, nil
}

// checkDaemonRunning checks if the daemon is running by reading the PID file.
func checkDaemonRunning(pidPath string) (int, bool) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}

	// Check if process is actually running
	process, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}

	// Signal 0 checks if process exists
	err = process.Signal(syscall.Signal(0))
	return pid, err == nil
}

// FormatStatus formats the daemon status for CLI output.
func FormatStatus(status *DaemonStatus) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("status: %s\n", status.Status))
	sb.WriteString(fmt.Sprintf("sync frequency: %d seconds\n", status.SyncFrequency))

	if status.Status == StatusActive {
		if !status.StartTime.IsZero() {
			sb.WriteString(fmt.Sprintf("start time: %s\n", status.StartTime.Format(time.RFC3339)))
		}
		if !status.LastSyncTime.IsZero() {
			sb.WriteString(fmt.Sprintf("last sync: %s\n", status.LastSyncTime.Format(time.RFC3339)))
		}
	}

	sb.WriteString(fmt.Sprintf("records synced: %d\n", status.RecordsSynced))
	sb.WriteString(fmt.Sprintf("errors: %d\n", status.Errors))

	if len(status.Tables) > 0 {
		sb.WriteString(fmt.Sprintf("\nsynced tables (%d):\n", len(status.Tables)))
		for _, t := range status.Tables {
			sb.WriteString(fmt.Sprintf("  - %s\n", t.TableName))
		}
	}

	return sb.String()
}

// PrintStatusTable prints a formatted status table.
func PrintStatusTable(ctx context.Context, config *SyncConfig, db *sql.DB, logger *slog.Logger) error {
	status, err := GetDaemonStatus(ctx, config, db)
	if err != nil {
		return err
	}

	fmt.Print(FormatStatus(status))
	return nil
}

// WritePIDFile writes the current process PID to the PID file.
func WritePIDFile(pidPath string) error {
	pid := os.Getpid()
	return os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644)
}

// ReadPIDFile reads the PID from the PID file.
func ReadPIDFile(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w (%s)", err, LOC_STATUS_PID)
	}

	return pid, nil
}

// RemovePIDFile removes the PID file.
func RemovePIDFile(pidPath string) error {
	return os.Remove(pidPath)
}

// IsRunning checks if a process with the given PID is alive.
func IsRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// StopProcess sends SIGTERM to the process and waits for it to exit.
func StopProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %w (%s)", err, LOC_STATUS_PID)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to PID %d: %w (%s)", pid, err, LOC_STATUS_PID)
	}

	// Wait for process to exit (poll every 200ms, up to 10 seconds)
	for i := 0; i < 50; i++ {
		time.Sleep(200 * time.Millisecond)
		if !IsRunning(pid) {
			return nil
		}
	}

	// Process didn't exit gracefully, send SIGKILL
	if err := process.Signal(syscall.SIGKILL); err != nil {
		if !IsRunning(pid) {
			return nil // Already exited
		}
		return fmt.Errorf("failed to send SIGKILL to PID %d: %w (%s)", pid, err, LOC_STATUS_PID)
	}

	return nil
}
