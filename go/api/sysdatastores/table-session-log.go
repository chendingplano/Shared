// Description
package sysdatastores

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type SessionLogDef struct {
	LoginMethod  string  `json:"login_method"`
	SessionID    string  `json:"session_id"`
	AuthToken    string  `json:"auth_token"`
	Status       string  `json:"status"`
	UserName     string  `json:"user_name"`
	UserNameType string  `json:"user_name_type"`
	UserRegID    string  `json:"user_reg_id"`
	UserEmail    *string `json:"user_email"`
	CallerLoc    string  `json:"caller_loc"`
	ExpiresAt    *string `json:"expires_at"`
	CreatedAt    *string `json:"created_at"`
}

// Assume these are defined elsewhere (adjust as needed)
const (
	session_log_insert_fieldnames = "login_method, " +
		"session_id, auth_token, status, user_name, user_name_type," +
		"user_reg_id, user_email, caller_loc, expires_at, created_at"
)

// Define the Cache.
// Cache manages buffered records and periodic DB insertion
type SessionLogCache struct {
	records    []SessionLogDef    // Holds cached records
	mu         sync.Mutex         // Ensures thread-safe access to records
	db         *sql.DB            // Database connection
	db_type    string
	table_name string
	done       chan struct{}       // Signals shutdown
	wg         sync.WaitGroup     // Tracks background goroutine
	logger     ApiTypes.JimoLogger // Structured logger for background operations
}

// Global singleton instance and initialization guard
var (
	session_log_singleton *SessionLogCache
	session_log_once      sync.Once // Ensures InitCache runs once
)

func CreateSessionLogTable(
	logger ApiTypes.JimoLogger,
	db *sql.DB,
	db_type string,
	table_name string) error {
	logger.Info("Create table", "table_name", table_name)
	var stmt string
	const common_fields = "login_method 	VARCHAR(32) 	NOT NULL, " +
		"session_id 	VARCHAR(256) 	NOT NULL, " +
		"auth_token     VARCHAR(64) 	NOT NULL, " +
		"status 		VARCHAR(32) 	NOT NULL, " +
		"user_name 		VARCHAR(64) 	NOT NULL, " +
		"user_name_type VARCHAR(32) 	NOT NULL, " +
		"user_reg_id 	VARCHAR(255) 	NOT NULL, " +
		"user_email 	VARCHAR(255) 	DEFAULT NULL, " +
		"caller_loc		VARCHAR(32) 	NOT NULL, " +
		"expires_at 	TIMESTAMP 		NOT NULL, "

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + common_fields +
			"created_at 	TIMESTAMP 		DEFAULT CURRENT_TIMESTAMP, " +
			"INDEX idx_expires (expires_at), " +
			"INDEX idx_session_id (session_id) " +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + common_fields +
			"created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW())"

	default:
		err := fmt.Errorf("database type not supported:%s (SHD_SLG_117)", db_type)
		logger.Error("database type not supported", "db_type", db_type)
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		err1 := fmt.Errorf("failed creating table '%s' (SHD_SLG_124), err: %w, stmt:%s", table_name, err, stmt)
		logger.Error("failed creating table", "table_name", table_name, "error", err)
		return err1
	}

	// Ensure columns added after initial table creation exist (safe for both new and existing tables).
	switch db_type {
	case ApiTypes.PgName:
		alterStmts := []string{
			`ALTER TABLE ` + table_name + ` ADD COLUMN IF NOT EXISTS auth_token VARCHAR(64) NOT NULL DEFAULT ''`,
		}
		for _, s := range alterStmts {
			if err := databaseutil.ExecuteStatement(db, s); err != nil {
				logger.Warn("ALTER TABLE warning (non-fatal)", "stmt", s, "error", err)
			}
		}

		idx1 := `CREATE INDEX IF NOT EXISTS idx_expires ON ` + table_name + ` (expires_at);`
		databaseutil.ExecuteStatement(db, idx1)

		idx2 := `CREATE INDEX IF NOT EXISTS idx_session_id ON ` + table_name + ` (session_id);`
		databaseutil.ExecuteStatement(db, idx2)

	case ApiTypes.MysqlName:
		// MySQL doesn't support ADD COLUMN IF NOT EXISTS natively; use a procedure-style workaround.
		alterStmt := `ALTER TABLE ` + table_name + ` ADD COLUMN auth_token VARCHAR(64) NOT NULL DEFAULT ''`
		if err := databaseutil.ExecuteStatement(db, alterStmt); err != nil {
			// "Duplicate column name" error is expected if column already exists — ignore it.
			logger.Info("ALTER TABLE auth_token (may already exist)", "error", err)
		}
	}

	logger.Info("Create table success", "table_name", table_name)
	return nil
}

// Public API
// InitCache initializes the singleton cache with a database connection
// Call this once at application startup (e.g., in main())
func InitSessionLogCache(db_type string,
	table_name string,
	db *sql.DB) error {
	session_log_once.Do(func() {
		session_log_singleton = newSessionLogCache(db_type, table_name, db)
		session_log_singleton.start()
	})
	return nil
}

// Public API
func StopSessionLogCache() {
	session_log_singleton.StopSessionLogCache()
}

// Public API
// AddActivityLog for applications to add logs to the cache
func AddSessionLog(record SessionLogDef) error {
	c := session_log_singleton
	if c == nil {
		error_msg := "cache not initialized; call InitCache first (SHD_SLG_077)"
		// Use a temporary logger since the singleton isn't initialized yet
		tmpLogger := loggerutil.CreateDefaultLogger("SHD_SLG_153")
		tmpLogger.Error(error_msg)
		return fmt.Errorf("%s", error_msg)
	}
	c.addToCache(record)
	return nil
}

// Public API
// Stop signals the cache to flush remaining records and exit
func (c *SessionLogCache) StopSessionLogCache() {
	close(c.done)
	c.wg.Wait() // Wait for flush loop to complete
}

func newSessionLogCache(db_type string,
	table_name string,
	db *sql.DB) *SessionLogCache {
	return &SessionLogCache{
		db:         db,
		db_type:    db_type,
		table_name: table_name,
		done:       make(chan struct{}),
		logger:     loggerutil.CreateDefaultLogger("SHD_SLG_173"),
	}
}

// Start begins the background flushing loop
func (c *SessionLogCache) start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.flushLoop()
	}()
}

// flushLoop runs indefinitely, flushing cached records to DB every 10 seconds
func (c *SessionLogCache) flushLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop() // Ensure ticker is stopped when loop exits

	for {
		select {
		case <-ticker.C:
			// When creating a ticker, the ticker creates a channel: ticker.C.
			// When the ticker times out, it will send a value to the channel.
			// Collect records and reset cache (under mutex)
			c.mu.Lock()
			records := c.records
			c.records = nil // Reset to collect new records
			c.mu.Unlock()

			if len(records) > 0 {
				if err := c.insertRecords(records); err != nil {
					c.logger.Error("Flush failed (ticker), records may be lost", "error", err, "record_count", len(records))
				}
			}
		case <-c.done:
			// Flush remaining records on shutdown
			c.mu.Lock()
			records := c.records
			c.records = nil
			c.mu.Unlock()

			if len(records) > 0 {
				if err := c.insertRecords(records); err != nil {
					c.logger.Error("Final flush failed, records may be lost", "error", err, "record_count", len(records))
				}
			}
			return // Exit loop
		}
	}
}

func (c *SessionLogCache) addToCache(record SessionLogDef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, record)
}

// insertRecords inserts records into the database using a transaction
func (c *SessionLogCache) insertRecords(records []SessionLogDef) error {
	if len(records) == 0 {
		return nil
	}

	// log.Printf("Flush records, len:%d (SHD_SLG_162)", len(records))

	// Start transaction
	tx, err := c.db.Begin()
	if err != nil {
		c.logger.Error("failed to begin transaction", "error", err)
		return fmt.Errorf("failed to begin transaction: %v (SHD_SLG_163)", err)
	}

	defer func() {
		// Rollback on error (if transaction not committed)
		if tx != nil && err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				c.logger.Error("rollback failed", "original_error", err, "rollback_error", rollbackErr)
				err = fmt.Errorf("original error: %v; rollback failed: %v (SHD_SLG_169)", err, rollbackErr)
			}
		}
	}()

	var stmt string
	switch c.db_type {
	case ApiTypes.MysqlName:
		stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, c.table_name, session_log_insert_fieldnames)

	case ApiTypes.PgName:
		stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, c.table_name, session_log_insert_fieldnames)

	default:
		c.logger.Error("unrecognized database type", "db_type", c.db_type)
		stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, c.table_name, session_log_insert_fieldnames)
	}

	stmt1, err := tx.Prepare(stmt)
	if err != nil {
		c.logger.Error("failed to prepare statement", "error", err, "stmt", stmt)
		return fmt.Errorf("failed to prepare statement: %v, stmt:%s (SHD_SLG_189)", err, stmt)
	}

	defer stmt1.Close()

	// Insert each record
	for i, record := range records {
		_, err := stmt1.Exec(
			record.LoginMethod,
			record.SessionID,
			record.AuthToken,
			record.Status,
			record.UserName,
			record.UserNameType,
			record.UserRegID,
			record.UserEmail, // *string (nil → NULL)
			record.CallerLoc,
			record.ExpiresAt,
			record.CreatedAt)

		if err != nil {
			c.logger.Error("record insert failed", "record_index", i, "session_id", record.SessionID, "error", err)
			return fmt.Errorf("record %d (session_id=%s) insert failed: %v (SHD_SLG_230)", i, record.SessionID, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		c.logger.Error("failed to commit transaction", "error", err)
		return fmt.Errorf("failed to commit transaction: %v (SHD_SLG_236)", err)
	}
	return nil
}
