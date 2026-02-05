package logs2db

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Location codes for daemon operations
const (
	LOC_DAEMON_START  = "SHD_L2D_040"
	LOC_DAEMON_STOP   = "SHD_L2D_041"
	LOC_DAEMON_STATUS = "SHD_L2D_042"
)

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
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}

	return pid, nil
}

// RemovePIDFile removes the PID file.
func RemovePIDFile(pidPath string) error {
	return os.Remove(pidPath)
}

// IsRunning checks if a process with the given PID is alive.
// Uses signal 0 which does not kill but checks process existence.
func IsRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks if process exists without actually sending a signal
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// StopProcess sends SIGTERM to the process and waits up to 10 seconds
// for it to exit.
func StopProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %w (%s)", err, LOC_DAEMON_STOP)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to PID %d: %w (%s)", pid, err, LOC_DAEMON_STOP)
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
		return fmt.Errorf("failed to send SIGKILL to PID %d: %w (%s)", pid, err, LOC_DAEMON_STOP)
	}

	return nil
}
