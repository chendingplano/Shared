// syncdata is a CLI tool for synchronizing PostgreSQL tables from production
// to a local instance using logical decoding change files.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tablesyncher "github.com/chendingplano/shared/go/api/table-syncher"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

var (
	verbose bool
)

// createLogger creates a slog logger for CLI output.
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

// connectDB creates a database connection.
func connectDB(config *tablesyncher.SyncConfig) (*sql.DB, error) {
	db, err := sql.Open("postgres", config.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

var rootCmd = &cobra.Command{
	Use:   "syncdata",
	Short: "PostgreSQL table synchronization tool",
	Long: `syncdata synchronizes specific PostgreSQL tables from production to local
using logical decoding change files from the backup archive.

Environment variables:
  DATA_SYNC_CONFIG          Path to TOML configuration file (required)
  PG_PASSWORD               Database password (can override config)
  PG_DB_NAME                Database name (can override config)
  PG_USER_NAME              Database user (can override config)
  DATA_SYNC_FREQ            Sync frequency in seconds (can override config)
  METRIC_FREQ               Metrics aggregation frequency in hours (can override config)
`,
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the sync daemon",
	Long: `Starts the sync daemon in foreground mode.

The daemon will:
1. Connect to the local database
2. Connect to the remote archive via SFTP
3. Poll for new change files at the configured frequency
4. Apply changes to whitelisted tables
5. Log results to data_sync_logs table`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		config, err := tablesyncher.LoadConfig()
		if err != nil {
			return err
		}

		// Check if already running
		if pid, err := tablesyncher.ReadPIDFile(config.PIDFilePath); err == nil {
			if tablesyncher.IsRunning(pid) {
				return fmt.Errorf("daemon is already running (PID %d)", pid)
			}
		}

		// Write PID file
		if err := tablesyncher.WritePIDFile(config.PIDFilePath); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer tablesyncher.RemovePIDFile(config.PIDFilePath)

		// Setup signal handling
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			sig := <-sigCh
			logger.Info("Received signal, shutting down", "signal", sig)
			cancel()
		}()

		// Create and initialize service
		service := tablesyncher.NewService(config, logger)
		if err := service.Initialize(ctx); err != nil {
			return err
		}
		defer service.Close()

		fmt.Println("Sync daemon started")
		fmt.Printf("  PID file: %s\n", config.PIDFilePath)
		fmt.Printf("  Archive: %s\n", config.SSHAddress())
		fmt.Printf("  Frequency: %d seconds\n", config.DataSyncFreq)
		fmt.Println()

		// Run the sync loop
		return service.RunLoop(ctx)
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the sync daemon",
	Long:  `Gracefully stops the running sync daemon by sending SIGTERM.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()

		config, err := tablesyncher.LoadConfig()
		if err != nil {
			return err
		}

		pid, err := tablesyncher.ReadPIDFile(config.PIDFilePath)
		if err != nil {
			return fmt.Errorf("daemon is not running (no PID file)")
		}

		if !tablesyncher.IsRunning(pid) {
			// PID file exists but process is dead - clean up
			tablesyncher.RemovePIDFile(config.PIDFilePath)
			return fmt.Errorf("daemon is not running (stale PID file removed)")
		}

		logger.Info("Stopping daemon", "pid", pid)

		if err := tablesyncher.StopProcess(pid); err != nil {
			return err
		}

		// Remove PID file after successful stop
		tablesyncher.RemovePIDFile(config.PIDFilePath)

		fmt.Println("Daemon stopped")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	Long:  `Shows the current status of the sync daemon including uptime, sync stats, and errors.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		config, err := tablesyncher.LoadConfig()
		if err != nil {
			return err
		}

		// Try to connect to database for additional info
		db, dbErr := connectDB(config)
		if dbErr != nil {
			// Continue without DB - we can still show basic status
			db = nil
		}
		defer func() {
			if db != nil {
				db.Close()
			}
		}()

		status, err := tablesyncher.GetDaemonStatus(ctx, config, db)
		if err != nil {
			return err
		}

		fmt.Print(tablesyncher.FormatStatus(status))
		return nil
	},
}

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all synced tables",
	Long: `Truncates all tables in the sync whitelist and resets the sync state.

WARNING: This will delete all data in the synced tables!`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := tablesyncher.LoadConfig()
		if err != nil {
			return err
		}

		db, err := connectDB(config)
		if err != nil {
			return err
		}
		defer db.Close()

		service := tablesyncher.NewServiceWithDB(config, db, logger)
		if err := service.Initialize(ctx); err != nil {
			return err
		}

		fmt.Print("Are you sure you want to clear all synced tables? [y/N] ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Cancelled")
			return nil
		}

		if err := service.Clear(ctx); err != nil {
			return err
		}

		fmt.Println("All synced tables cleared")
		return nil
	},
}

var resyncCmd = &cobra.Command{
	Use:   "resync <table_name>",
	Short: "Resync a specific table",
	Long: `Drops and recreates a specific table's data from scratch.

This will:
1. Truncate the specified table
2. Reset the sync state for that table
3. Re-apply all changes from the archive`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		tableName := args[0]

		config, err := tablesyncher.LoadConfig()
		if err != nil {
			return err
		}

		db, err := connectDB(config)
		if err != nil {
			return err
		}
		defer db.Close()

		service := tablesyncher.NewServiceWithDB(config, db, logger)
		if err := service.Initialize(ctx); err != nil {
			return err
		}

		fmt.Printf("Resyncing table: %s\n", tableName)

		result, err := service.Resync(ctx, tableName)
		if err != nil {
			return err
		}

		fmt.Println()
		fmt.Println("Resync complete!")
		fmt.Printf("  Added: %d\n", result.RecordsAdded)
		fmt.Printf("  Updated: %d\n", result.RecordsUpdated)
		fmt.Printf("  Deleted: %d\n", result.RecordsDeleted)
		fmt.Println()

		return nil
	},
}

var addTablesCmd = &cobra.Command{
	Use:   "add-tables <name1> [name2] ...",
	Short: "Add tables to sync whitelist",
	Long: `Adds one or more tables to the synchronization whitelist.

Only tables in the whitelist will be synced from the archive.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := tablesyncher.LoadConfig()
		if err != nil {
			return err
		}

		db, err := connectDB(config)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := tablesyncher.EnsureTables(ctx, db, logger); err != nil {
			return err
		}

		added, err := tablesyncher.AddTables(ctx, db, args, "", logger)
		if err != nil {
			return err
		}

		if len(added) == 0 {
			fmt.Println("No new tables added (already in whitelist)")
		} else {
			fmt.Println("Added tables to sync whitelist:")
			for _, t := range added {
				fmt.Printf("  - %s\n", t)
			}
		}

		return nil
	},
}

var removeTablesCmd = &cobra.Command{
	Use:   "remove-tables <name1> [name2] ...",
	Short: "Remove tables from sync whitelist",
	Long: `Removes one or more tables from the synchronization whitelist.

Removed tables will no longer be synced. Their local data is NOT deleted.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := tablesyncher.LoadConfig()
		if err != nil {
			return err
		}

		db, err := connectDB(config)
		if err != nil {
			return err
		}
		defer db.Close()

		removed, err := tablesyncher.RemoveTables(ctx, db, args, logger)
		if err != nil {
			return err
		}

		if len(removed) == 0 {
			fmt.Println("No tables removed (not in whitelist)")
		} else {
			fmt.Println("Removed tables from sync whitelist:")
			for _, t := range removed {
				fmt.Printf("  - %s\n", t)
			}
		}

		return nil
	},
}

var listTablesCmd = &cobra.Command{
	Use:   "list-tables",
	Short: "List tables in sync whitelist",
	Long:  `Shows all tables currently in the synchronization whitelist.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := tablesyncher.LoadConfig()
		if err != nil {
			return err
		}

		db, err := connectDB(config)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := tablesyncher.EnsureTables(ctx, db, logger); err != nil {
			return err
		}

		tables, err := tablesyncher.ListTables(ctx, db)
		if err != nil {
			return err
		}

		if len(tables) == 0 {
			fmt.Println("No tables in sync whitelist")
			fmt.Println()
			fmt.Println("Use 'syncdata add-tables <name>' to add tables")
		} else {
			fmt.Printf("Tables in sync whitelist (%d):\n", len(tables))
			fmt.Println()
			fmt.Printf("%-30s %-20s %s\n", "TABLE NAME", "CREATOR", "CREATED AT")
			fmt.Printf("%-30s %-20s %s\n", "----------", "-------", "----------")
			for _, t := range tables {
				creator := t.Creator
				if creator == "" {
					creator = "-"
				}
				fmt.Printf("%-30s %-20s %s\n", t.TableName, creator, t.CreatedAt.Format("2006-01-02 15:04"))
			}
		}
		fmt.Println()

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(clearCmd)
	rootCmd.AddCommand(resyncCmd)
	rootCmd.AddCommand(addTablesCmd)
	rootCmd.AddCommand(removeTablesCmd)
	rootCmd.AddCommand(listTablesCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
