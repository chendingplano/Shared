// logs2db is a CLI tool that monitors log files and loads their contents
// into a PostgreSQL database table.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/chendingplano/shared/go/api/logs2db"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

var (
	verbose bool
)

func createLogger() *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

var rootCmd = &cobra.Command{
	Use:   "log2db",
	Short: "Monitor log files and load entries into PostgreSQL",
	Long: `log2db monitors a directory of JSON log files and continuously
loads new entries into a PostgreSQL table.

Configuration via TOML file specified by LOG2DB_CONFIG environment variable.
Database connection via: PG_USER_NAME, PG_PASSWORD, PG_DB_NAME, PG_HOST, PG_PORT`,
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the log monitoring service",
	Long: `Starts the log monitoring service in the foreground.
Use nohup, tmux, or systemd to run in the background.

The service writes a PID file for stop/status commands.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()

		config, err := logs2db.LoadConfig()
		if err != nil {
			return err
		}

		// Check if already running
		if pid, err := logs2db.ReadPIDFile(config.PIDFilePath); err == nil {
			if logs2db.IsRunning(pid) {
				return fmt.Errorf("log2db is already running (PID %d)", pid)
			}
			// Stale PID file, clean up
			logs2db.RemovePIDFile(config.PIDFilePath)
		}

		service := logs2db.NewService(config, logger)
		if err := service.Initialize(context.Background()); err != nil {
			return err
		}
		defer service.Close()

		// Write PID file
		if err := logs2db.WritePIDFile(config.PIDFilePath); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer logs2db.RemovePIDFile(config.PIDFilePath)

		// Set up signal handling for graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			sig := <-sigCh
			logger.Info("Received signal, shutting down", "signal", sig)
			cancel()
		}()

		logger.Info("log2db service started",
			"log_dir", config.LogFileDir,
			"table", config.DBTableName,
			"poll_interval_sec", config.SyncFreqSec)

		return service.RunLoop(ctx)
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the log monitoring service",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := logs2db.LoadConfig()
		if err != nil {
			return err
		}

		pid, err := logs2db.ReadPIDFile(config.PIDFilePath)
		if err != nil {
			return fmt.Errorf("log2db is not running (no PID file found)")
		}

		if !logs2db.IsRunning(pid) {
			logs2db.RemovePIDFile(config.PIDFilePath)
			return fmt.Errorf("log2db is not running (stale PID %d, cleaned up)", pid)
		}

		fmt.Printf("Stopping log2db (PID %d)...\n", pid)
		if err := logs2db.StopProcess(pid); err != nil {
			return err
		}

		logs2db.RemovePIDFile(config.PIDFilePath)
		fmt.Println("log2db service stopped")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show service status and statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := logs2db.LoadConfig()
		if err != nil {
			return err
		}

		// Check if running
		pid, pidErr := logs2db.ReadPIDFile(config.PIDFilePath)
		isActive := pidErr == nil && logs2db.IsRunning(pid)

		if isActive {
			fmt.Println("Service Status: active")
		} else {
			fmt.Println("Service Status: not started")
		}

		// Try to get stats from DB
		logger := createLogger()
		service := logs2db.NewService(config, logger)
		if err := service.Initialize(context.Background()); err != nil {
			// Can't connect to DB, just show status
			fmt.Println("Start Time: N/A")
			fmt.Println("Total Log Entries: N/A (database unavailable)")
			fmt.Println("Entries Since Start: N/A")
			fmt.Println("Total Errors: N/A")
			return nil
		}
		defer service.Close()

		totalEntries, err := service.CountEntries(context.Background())
		if err != nil {
			fmt.Printf("Total Log Entries: error (%v)\n", err)
		} else {
			fmt.Printf("Total Log Entries: %d\n", totalEntries)
		}

		// Runtime stats are only available when the service is running in-process.
		// When checking status externally, these are not accessible.
		if isActive {
			fmt.Println("Start Time: (check service logs)")
			fmt.Println("Entries Since Start: (check service logs)")
			fmt.Println("Total Errors: (check service logs)")
		} else {
			fmt.Println("Start Time: N/A")
			fmt.Println("Entries Since Start: N/A")
			fmt.Println("Total Errors: N/A")
		}

		return nil
	},
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Clear table and reload all log files from scratch",
	Long: `Truncates the database table, resets the state file, and reloads
all log files from the configured directory.

WARNING: This deletes all existing log entries from the table.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()

		config, err := logs2db.LoadConfig()
		if err != nil {
			return err
		}

		// Interactive confirmation
		fmt.Printf("WARNING: This will DELETE ALL rows from table '%s' and reload all log files.\n",
			config.DBTableName)
		fmt.Print("Type 'yes' to confirm: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Aborted.")
			return nil
		}

		service := logs2db.NewService(config, logger)
		if err := service.Initialize(context.Background()); err != nil {
			return err
		}
		defer service.Close()

		result, err := service.Reload(context.Background())
		if err != nil {
			return err
		}

		fmt.Printf("\nReload complete:\n")
		fmt.Printf("  Files scanned:  %d\n", result.FilesScanned)
		fmt.Printf("  Lines inserted: %d\n", result.LinesInserted)
		fmt.Printf("  Lines failed:   %d\n", result.LinesFailed)
		fmt.Printf("  Duration:       %v\n", result.Duration)
		return nil
	},
}

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete old log files that have been loaded to DB",
	Long: `Keeps the specified number of most recent log files and deletes
older ones, provided they have been fully loaded into the database.

Files that have not been fully loaded will be skipped.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()

		maxFiles, _ := cmd.Flags().GetInt("maxfiles")

		config, err := logs2db.LoadConfig()
		if err != nil {
			return err
		}

		service := logs2db.NewService(config, logger)
		if err := service.Initialize(context.Background()); err != nil {
			return err
		}
		defer service.Close()

		result, err := service.Purge(context.Background(), maxFiles)
		if err != nil {
			return err
		}

		fmt.Printf("Purge complete:\n")
		fmt.Printf("  Files kept:    %d %v\n", len(result.FilesKept), result.FilesKept)
		fmt.Printf("  Files deleted: %d %v\n", len(result.FilesDeleted), result.FilesDeleted)
		if len(result.FilesSkipped) > 0 {
			fmt.Printf("  Files skipped: %d %v (not fully loaded)\n", len(result.FilesSkipped), result.FilesSkipped)
		}
		if result.FreedBytes > 0 {
			fmt.Printf("  Space freed:   %s\n", formatBytes(result.FreedBytes))
		}
		if len(result.Errors) > 0 {
			fmt.Printf("  Errors:        %d\n", len(result.Errors))
			for _, e := range result.Errors {
				fmt.Printf("    - %s\n", e)
			}
		}
		return nil
	},
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", b)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	purgeCmd.Flags().IntP("maxfiles", "n", 5, "Number of most recent log files to keep")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(reloadCmd)
	rootCmd.AddCommand(purgeCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
