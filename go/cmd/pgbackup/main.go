// pgbackup is a CLI tool for PostgreSQL WAL archiving and Point-in-Time Recovery (PITR)
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/chendingplano/shared/go/api/pgbackup"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

var (
	// Flags
	verbose bool
)

// createLogger creates a slog logger for CLI output
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

// connectDB creates a database connection for PostgreSQL operations
func connectDB(config *pgbackup.BackupConfig) (*sql.DB, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.PGHost, config.PGPort, config.PGUser, config.PGPassword, config.PGDatabase)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

var rootCmd = &cobra.Command{
	Use:   "pgbackup",
	Short: "PostgreSQL WAL archiving and PITR backup tool",
	Long: `pgbackup provides PostgreSQL backup management with WAL archiving
and Point-in-Time Recovery (PITR) capabilities.

Environment variables:
  PG_USER_NAME              PostgreSQL username
  PG_PASSWORD               PostgreSQL password
  PG_DB_NAME                PostgreSQL database name
  PG_HOST                   PostgreSQL host (default: 127.0.0.1)
  PG_PORT                   PostgreSQL port (default: 5432)
  PG_BACKUP_DIR             Base directory for backups (required)
  PGDATA                    PostgreSQL data directory (for restore)
  PG_BACKUP_RETAIN_DAYS     Days to keep backups (default: 7)
  PG_BACKUP_RETAIN_COUNT    Minimum backups to keep (default: 3)
`,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize backup environment",
	Long: `Creates backup directories, installs the WAL archive script,
and verifies PostgreSQL configuration for WAL archiving.

This command should be run once before starting backups.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := pgbackup.LoadConfig()
		if err != nil {
			return err
		}

		// Try to connect to verify PG config
		db, err := connectDB(config)
		if err != nil {
			logger.Warn("Could not connect to PostgreSQL - skipping config verification", "error", err)
		}
		defer func() {
			if db != nil {
				db.Close()
			}
		}()

		service := pgbackup.NewBackupServiceWithDB(config, db)
		if err := service.Initialize(ctx, logger); err != nil {
			return err
		}

		fmt.Println()
		fmt.Println("Initialization complete!")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("1. Configure PostgreSQL for WAL archiving (if not already done):")
		fmt.Println()
		fmt.Printf("   ALTER SYSTEM SET wal_level = 'logical';\n")
		fmt.Printf("   ALTER SYSTEM SET archive_mode = 'on';\n")
		fmt.Printf("   ALTER SYSTEM SET archive_command = '%s %%p %%f';\n", config.ArchiveScriptPath)
		fmt.Printf("   ALTER SYSTEM SET archive_timeout = 300;\n")
		fmt.Println()
		fmt.Println("2. Restart PostgreSQL to apply changes")
		fmt.Println("3. Run 'pgbackup backup' to create your first backup")
		fmt.Println()

		return nil
	},
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Perform a base backup",
	Long: `Creates a full base backup using pg_basebackup.

The backup includes all database files compressed with gzip.
WAL files are streamed during the backup to ensure consistency.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := pgbackup.LoadConfig()
		if err != nil {
			return err
		}

		service := pgbackup.NewBackupService(config)

		// Check disk space first
		if err := service.CheckDiskSpace(ctx, logger); err != nil {
			return fmt.Errorf("disk space check failed: %w", err)
		}

		result, err := service.PerformBaseBackup(ctx, logger)
		if err != nil {
			return err
		}

		fmt.Println()
		fmt.Println("Backup completed successfully!")
		fmt.Printf("  Backup ID:   %s\n", result.BackupID)
		fmt.Printf("  Path:        %s\n", result.BackupPath)
		fmt.Printf("  Size:        %.2f MB\n", float64(result.SizeBytes)/(1024*1024))
		fmt.Printf("  Duration:    %s\n", result.EndTime.Sub(result.StartTime).Round(time.Second))
		fmt.Println()

		return nil
	},
}

var restoreCmd = &cobra.Command{
	Use:   "restore [backup-id]",
	Short: "Restore from a backup",
	Long: `Restores PostgreSQL from a backup with optional point-in-time recovery.

IMPORTANT: PostgreSQL must be STOPPED before running restore.

The restore process:
1. Extracts the base backup to the target directory
2. Configures recovery parameters (recovery.signal, postgresql.auto.conf)
3. When PostgreSQL starts, it automatically replays WAL files to the target time

Examples:
  pgbackup restore 20260202_100000
  pgbackup restore 20260202_100000 --target-time "2026-02-02 12:00:00"
  pgbackup restore 20260202_100000 --dry-run
  pgbackup restore 20260202_100000 --target-dir /path/to/new/data`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := pgbackup.LoadConfig()
		if err != nil {
			return err
		}

		targetTimeStr, _ := cmd.Flags().GetString("target-time")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		targetDir, _ := cmd.Flags().GetString("target-dir")

		opts := pgbackup.RestoreOptions{
			BackupID:        args[0],
			TargetDirectory: targetDir,
			DryRun:          dryRun,
		}

		if targetTimeStr != "" {
			t, err := time.ParseInLocation("2006-01-02 15:04:05", targetTimeStr, time.Local)
			if err != nil {
				return fmt.Errorf("invalid target-time format (use: 2006-01-02 15:04:05): %w", err)
			}
			opts.TargetTime = &t
		}

		service := pgbackup.NewBackupService(config)
		result, err := service.Restore(ctx, logger, opts)
		if err != nil {
			return err
		}

		fmt.Println()
		if dryRun {
			fmt.Println("Dry run completed - restore is valid")
			fmt.Printf("  Backup:      %s\n", result.BackupUsed)
			fmt.Printf("  Target Dir:  %s\n", result.TargetDir)
		} else {
			fmt.Println("Restore completed!")
			fmt.Printf("  Backup:      %s\n", result.BackupUsed)
			fmt.Printf("  Target Dir:  %s\n", result.TargetDir)
			if opts.TargetTime != nil {
				fmt.Printf("  Target Time: %s\n", opts.TargetTime.Format(time.RFC3339))
			}
			fmt.Println()
			fmt.Println("Next steps:")
			fmt.Println("1. Start PostgreSQL")
			fmt.Println("2. Recovery will happen automatically")
			fmt.Println("3. PostgreSQL will promote to normal operation when recovery is complete")
		}
		fmt.Println()

		return nil
	},
}

var verifyCmd = &cobra.Command{
	Use:   "verify [backup-id]",
	Short: "Verify backup integrity",
	Long: `Verifies the integrity of backup files.

Checks:
- Tar file integrity (gzip -t and tar -tf)
- Presence of required files (base.tar.gz)
- WAL archive status

If no backup-id is specified, verifies the latest backup.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := pgbackup.LoadConfig()
		if err != nil {
			return err
		}

		service := pgbackup.NewBackupService(config)

		var backupID string
		if len(args) > 0 {
			backupID = args[0]
		}

		all, _ := cmd.Flags().GetBool("all")

		if all {
			results, err := service.VerifyAll(ctx, logger)
			if err != nil {
				return err
			}

			fmt.Println()
			fmt.Println("Verification Results:")
			allOK := true
			for _, result := range results {
				status := "OK"
				if !result.Success {
					status = "FAILED"
					allOK = false
				}
				fmt.Printf("  %s: %s\n", result.BackupID, status)
				for _, issue := range result.Issues {
					fmt.Printf("    - %s\n", issue)
				}
			}

			if !allOK {
				return fmt.Errorf("some backups failed verification")
			}
		} else {
			result, err := service.Verify(ctx, logger, backupID)
			if err != nil {
				return err
			}

			fmt.Println()
			if result.Success {
				fmt.Printf("Backup %s verified successfully!\n", result.BackupID)
			} else {
				fmt.Printf("Backup %s verification FAILED\n", result.BackupID)
				for _, issue := range result.Issues {
					fmt.Printf("  - %s\n", issue)
				}
				return fmt.Errorf("backup verification failed")
			}
		}

		fmt.Println()
		return nil
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Apply retention policy and remove old backups",
	Long: `Removes old backups according to the retention policy.

Retention rules:
- Keep at least PG_BACKUP_RETAIN_COUNT backups (default: 3)
- Delete backups older than PG_BACKUP_RETAIN_DAYS (default: 7 days)
- Clean WAL files no longer needed for recovery`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := pgbackup.LoadConfig()
		if err != nil {
			return err
		}

		service := pgbackup.NewBackupService(config)
		result, err := service.ApplyRetention(ctx, logger)
		if err != nil {
			return err
		}

		fmt.Println()
		fmt.Println("Cleanup completed!")
		fmt.Printf("  Deleted backups:    %d\n", len(result.DeletedBackups))
		fmt.Printf("  Retained backups:   %d\n", len(result.RetainedBackups))
		fmt.Printf("  Deleted WAL files:  %d\n", result.DeletedWALFiles)
		fmt.Printf("  Freed space:        %.2f MB\n", float64(result.FreedSpaceBytes)/(1024*1024))
		fmt.Println()

		if len(result.DeletedBackups) > 0 {
			fmt.Println("Deleted backups:")
			for _, id := range result.DeletedBackups {
				fmt.Printf("  - %s\n", id)
			}
			fmt.Println()
		}

		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show backup status and history",
	Long:  `Displays comprehensive backup status information including all available backups, WAL archive status, and PostgreSQL configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := pgbackup.LoadConfig()
		if err != nil {
			return err
		}

		// Try to connect to get PG config info
		db, err := connectDB(config)
		if err != nil {
			logger.Info("Could not connect to PostgreSQL - some info may be unavailable")
		}
		defer func() {
			if db != nil {
				db.Close()
			}
		}()

		service := pgbackup.NewBackupServiceWithDB(config, db)
		return service.PrintStatus(ctx, logger)
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available backups",
	Long:  `Lists all available backups with their IDs, timestamps, and sizes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()

		config, err := pgbackup.LoadConfig()
		if err != nil {
			return err
		}

		service := pgbackup.NewBackupService(config)
		backups, err := service.ListBackups()
		if err != nil {
			return err
		}

		if len(backups) == 0 {
			fmt.Println("No backups found.")
			fmt.Println()
			fmt.Println("Run 'pgbackup backup' to create your first backup.")
			return nil
		}

		fmt.Println()
		fmt.Println("Available Backups:")
		fmt.Println()
		fmt.Printf("%-20s %-25s %12s  %s\n", "BACKUP ID", "TIMESTAMP", "SIZE", "STATUS")
		fmt.Printf("%-20s %-25s %12s  %s\n", "---------", "---------", "----", "------")

		for _, b := range backups {
			status := "OK"
			if !b.Success {
				status = "FAILED"
			}
			fmt.Printf("%-20s %-25s %10.2f MB  %s\n",
				b.BackupID,
				b.StartTime.Format("2006-01-02 15:04:05 MST"),
				float64(b.SizeBytes)/(1024*1024),
				status)
		}

		fmt.Println()
		logger.Debug("Listed backups", "count", len(backups))

		return nil
	},
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync all backups to remote host",
	Long: `Syncs all base backups and WAL archive files to a remote host using rsync over SSH.

Requires PG_BACKUP_REMOTE_HOST to be set. Optional:
  PG_BACKUP_REMOTE_USER    SSH username (default: current user)
  PG_BACKUP_REMOTE_DIR     Remote directory (default: same as PG_BACKUP_DIR)
  PG_BACKUP_REMOTE_PORT    SSH port (default: 22)

Requires SSH key-based authentication to the remote host.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := createLogger()
		ctx := context.Background()

		config, err := pgbackup.LoadConfig()
		if err != nil {
			return err
		}

		if !config.RemoteEnabled() {
			return fmt.Errorf("remote sync not configured: set PG_BACKUP_REMOTE_HOST environment variable")
		}

		service := pgbackup.NewBackupService(config)
		result, err := service.SyncAll(ctx, logger)
		if err != nil {
			return err
		}

		fmt.Println()
		if result.Success {
			fmt.Println("Remote sync completed successfully!")
		} else {
			fmt.Println("Remote sync failed!")
			fmt.Printf("  Error: %s\n", result.ErrorMsg)
		}
		fmt.Printf("  Destination: %s\n", result.Destination)
		fmt.Println()

		if !result.Success {
			return fmt.Errorf("remote sync failed")
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	restoreCmd.Flags().String("target-time", "", "Point-in-time recovery target (format: 2006-01-02 15:04:05)")
	restoreCmd.Flags().String("target-dir", "", "Target directory for restore (defaults to PGDATA)")
	restoreCmd.Flags().Bool("dry-run", false, "Validate restore without executing")

	verifyCmd.Flags().Bool("all", false, "Verify all backups")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(syncCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
