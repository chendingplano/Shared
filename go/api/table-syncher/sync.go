package tablesyncher

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Location codes for sync operations
const (
	LOC_SYNC_CONNECT  = "SHD_SYN_060"
	LOC_SYNC_DISCOVER = "SHD_SYN_061"
	LOC_SYNC_FETCH    = "SHD_SYN_062"
	LOC_SYNC_PARSE    = "SHD_SYN_063"
	LOC_SYNC_APPLY    = "SHD_SYN_064"
)

// SFTPClient wraps SSH/SFTP connections to the remote archive machine.
type SFTPClient struct {
	config     *SyncConfig
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	logger     *slog.Logger
}

// NewSFTPClient creates a new SFTP client for the archive machine.
func NewSFTPClient(config *SyncConfig, logger *slog.Logger) *SFTPClient {
	return &SFTPClient{
		config: config,
		logger: logger,
	}
}

// Connect establishes SSH and SFTP connections.
func (c *SFTPClient) Connect(ctx context.Context) error {
	// Try to get SSH agent authentication
	var authMethods []ssh.AuthMethod

	// Try SSH agent first
	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		authMethods = append(authMethods, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
	}

	// Try default SSH keys
	home, _ := os.UserHomeDir()
	keyPaths := []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
	}

	for _, keyPath := range keyPaths {
		if key, err := os.ReadFile(keyPath); err == nil {
			if signer, err := ssh.ParsePrivateKey(key); err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	}

	if len(authMethods) == 0 {
		return fmt.Errorf("no SSH authentication methods available (%s)", LOC_SYNC_CONNECT)
	}

	sshConfig := &ssh.ClientConfig{
		User:            c.config.ArchiveUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: use known_hosts
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", c.config.ArchiveHost, c.config.ArchivePort)
	c.logger.Debug("Connecting to SSH", "address", addr, "user", c.config.ArchiveUser)

	var err error
	c.sshClient, err = ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SSH %s: %w (%s)", addr, err, LOC_SYNC_CONNECT)
	}

	c.sftpClient, err = sftp.NewClient(c.sshClient)
	if err != nil {
		c.sshClient.Close()
		return fmt.Errorf("failed to create SFTP client: %w (%s)", err, LOC_SYNC_CONNECT)
	}

	c.logger.Info("Connected to archive machine", "address", addr, "loc", LOC_SYNC_CONNECT)
	return nil
}

// Close closes the SFTP and SSH connections.
func (c *SFTPClient) Close() {
	if c.sftpClient != nil {
		c.sftpClient.Close()
	}
	if c.sshClient != nil {
		c.sshClient.Close()
	}
}

// DiscoverChangeFiles lists change files in the archive directory newer than the given time.
func (c *SFTPClient) DiscoverChangeFiles(ctx context.Context, sinceTime time.Time) ([]ChangeFile, error) {
	if c.sftpClient == nil {
		return nil, fmt.Errorf("SFTP client not connected (%s)", LOC_SYNC_DISCOVER)
	}

	files, err := c.sftpClient.ReadDir(c.config.ArchiveDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive directory %s: %w (%s)",
			c.config.ArchiveDir, err, LOC_SYNC_DISCOVER)
	}

	var changeFiles []ChangeFile
	for _, f := range files {
		// Only process .json files
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		// Filter by modification time
		if !sinceTime.IsZero() && !f.ModTime().After(sinceTime) {
			continue
		}

		changeFiles = append(changeFiles, ChangeFile{
			Name:    f.Name(),
			Path:    filepath.Join(c.config.ArchiveDir, f.Name()),
			Size:    f.Size(),
			ModTime: f.ModTime(),
		})
	}

	// Sort by modification time (oldest first)
	sort.Slice(changeFiles, func(i, j int) bool {
		return changeFiles[i].ModTime.Before(changeFiles[j].ModTime)
	})

	c.logger.Debug("Discovered change files",
		"count", len(changeFiles),
		"since", sinceTime,
		"loc", LOC_SYNC_DISCOVER)

	return changeFiles, nil
}

// FetchChangeFile downloads a change file from the archive.
func (c *SFTPClient) FetchChangeFile(ctx context.Context, cf ChangeFile) ([]ChangeRecord, error) {
	if c.sftpClient == nil {
		return nil, fmt.Errorf("SFTP client not connected (%s)", LOC_SYNC_FETCH)
	}

	f, err := c.sftpClient.Open(cf.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open remote file %s: %w (%s)", cf.Path, err, LOC_SYNC_FETCH)
	}
	defer f.Close()

	return ParseChangeFile(ctx, f, c.logger)
}

// ParseChangeFile parses change records from a reader (one JSON per line).
func ParseChangeFile(ctx context.Context, r io.Reader, logger *slog.Logger) ([]ChangeRecord, error) {
	var records []ChangeRecord
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return records, ctx.Err()
		default:
		}

		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var record ChangeRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			logger.Warn("Failed to parse change record",
				"line", lineNum,
				"error", err,
				"loc", LOC_SYNC_PARSE)
			continue
		}

		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return records, fmt.Errorf("error reading change file: %w (%s)", err, LOC_SYNC_PARSE)
	}

	return records, nil
}

// ApplyChanges applies change records to the local database.
func ApplyChanges(ctx context.Context, db *sql.DB, records []ChangeRecord, whitelist map[string]bool, logger *slog.Logger) (*SyncResult, error) {
	result := &SyncResult{}
	start := time.Now()

	// Group records by table for batch processing
	byTable := make(map[string][]ChangeRecord)
	for _, r := range records {
		// Filter by whitelist
		if !whitelist[r.Table] {
			result.RecordsSkipped++
			continue
		}
		byTable[r.Table] = append(byTable[r.Table], r)
	}

	// Process each table's changes in a transaction
	for tableName, tableRecords := range byTable {
		if err := applyTableChanges(ctx, db, tableName, tableRecords, result, logger); err != nil {
			// Log error but continue with other tables
			logger.Error("Failed to apply changes to table",
				"table", tableName,
				"error", err,
				"loc", LOC_SYNC_APPLY)
		}
	}

	result.Duration = time.Since(start)
	if len(records) > 0 {
		result.LastLSN = records[len(records)-1].LSN
	}

	return result, nil
}

// applyTableChanges applies changes for a single table in a transaction.
func applyTableChanges(ctx context.Context, db *sql.DB, tableName string, records []ChangeRecord, result *SyncResult, logger *slog.Logger) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, r := range records {
		var applyErr error
		switch r.Op {
		case OpInsert:
			applyErr = applyInsert(ctx, tx, tableName, r, logger)
			if applyErr == nil {
				result.RecordsAdded++
			}
		case OpUpdate:
			applyErr = applyUpdate(ctx, tx, tableName, r, logger)
			if applyErr == nil {
				result.RecordsUpdated++
			}
		case OpDelete:
			applyErr = applyDelete(ctx, tx, tableName, r, logger)
			if applyErr == nil {
				result.RecordsDeleted++
			}
		default:
			logger.Warn("Unknown operation", "op", r.Op, "table", tableName)
			result.RecordsFailed++
			continue
		}

		if applyErr != nil {
			logger.Warn("Failed to apply change",
				"table", tableName,
				"op", r.Op,
				"error", applyErr,
				"loc", LOC_SYNC_APPLY)
			result.RecordsFailed++
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// applyInsert applies an INSERT operation (with UPSERT semantics).
func applyInsert(ctx context.Context, tx *sql.Tx, tableName string, r ChangeRecord, _ *slog.Logger) error {
	if len(r.Data) == 0 {
		return fmt.Errorf("INSERT record has no data")
	}

	// Build INSERT ... ON CONFLICT DO UPDATE statement
	columns := make([]string, 0, len(r.Data))
	placeholders := make([]string, 0, len(r.Data))
	values := make([]any, 0, len(r.Data))
	updateClauses := make([]string, 0, len(r.Data))

	i := 1
	for col, val := range r.Data {
		columns = append(columns, quoteIdentifier(col))
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		values = append(values, val)
		updateClauses = append(updateClauses, fmt.Sprintf("%s = EXCLUDED.%s", quoteIdentifier(col), quoteIdentifier(col)))
		i++
	}

	// Assume first column is the primary key for ON CONFLICT
	// This is a simplification - production code should introspect the schema
	pkCol := columns[0]

	query := fmt.Sprintf(
		`INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s`,
		quoteIdentifier(tableName),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
		pkCol,
		strings.Join(updateClauses, ", "),
	)

	_, err := tx.ExecContext(ctx, query, values...)
	return err
}

// applyUpdate applies an UPDATE operation.
func applyUpdate(ctx context.Context, tx *sql.Tx, tableName string, r ChangeRecord, logger *slog.Logger) error {
	if len(r.Data) == 0 {
		return fmt.Errorf("UPDATE record has no data")
	}
	if len(r.OldKeys) == 0 {
		return fmt.Errorf("UPDATE record has no old_keys")
	}

	// Build SET clause
	setClauses := make([]string, 0, len(r.Data))
	values := make([]any, 0, len(r.Data)+len(r.OldKeys))

	i := 1
	for col, val := range r.Data {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", quoteIdentifier(col), i))
		values = append(values, val)
		i++
	}

	// Build WHERE clause
	whereClauses := make([]string, 0, len(r.OldKeys))
	for col, val := range r.OldKeys {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", quoteIdentifier(col), i))
		values = append(values, val)
		i++
	}

	query := fmt.Sprintf(
		`UPDATE %s SET %s WHERE %s`,
		quoteIdentifier(tableName),
		strings.Join(setClauses, ", "),
		strings.Join(whereClauses, " AND "),
	)

	result, err := tx.ExecContext(ctx, query, values...)
	if err != nil {
		return err
	}

	// Log warning if no rows affected (row doesn't exist locally)
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		logger.Debug("UPDATE affected no rows (row may not exist locally)",
			"table", tableName,
			"keys", r.OldKeys)
	}

	return nil
}

// applyDelete applies a DELETE operation.
func applyDelete(ctx context.Context, tx *sql.Tx, tableName string, r ChangeRecord, logger *slog.Logger) error {
	if len(r.OldKeys) == 0 {
		return fmt.Errorf("DELETE record has no old_keys")
	}

	// Build WHERE clause
	whereClauses := make([]string, 0, len(r.OldKeys))
	values := make([]any, 0, len(r.OldKeys))

	i := 1
	for col, val := range r.OldKeys {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", quoteIdentifier(col), i))
		values = append(values, val)
		i++
	}

	query := fmt.Sprintf(
		`DELETE FROM %s WHERE %s`,
		quoteIdentifier(tableName),
		strings.Join(whereClauses, " AND "),
	)

	result, err := tx.ExecContext(ctx, query, values...)
	if err != nil {
		return err
	}

	// Log warning if no rows affected
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		logger.Debug("DELETE affected no rows (row may not exist locally)",
			"table", tableName,
			"keys", r.OldKeys)
	}

	return nil
}
