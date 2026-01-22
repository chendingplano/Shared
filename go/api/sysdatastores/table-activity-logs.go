// Description
// ActivityLog
package sysdatastores

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// Define the Cache.
// Cache manages buffered records and periodic DB insertion
type ActivityLogCache struct {
	records                        []ApiTypes.ActivityLogDef // Holds cached records
	mu                             sync.Mutex                // Ensures thread-safe access to records
	db                             *sql.DB                   // Database connection
	db_type                        string
	table_name                     string
	id_name                        string
	crt_log_id                     int64
	num_log_ids                    int
	activity_log_insert_fieldnames string
	done                           chan struct{}  // Signals shutdown
	wg                             sync.WaitGroup // Tracks background goroutine
	logger                         *loggerutil.JimoLogger
}

// Global singleton instance and initialization guard
var (
	activity_log_singleton *ActivityLogCache
	activity_log_once      sync.Once // Ensures InitCache runs once
)

func CreateActivityLogTable(
	db *sql.DB,
	db_type string,
	table_name string) error {
	var stmt string
	fields :=
		"log_id             int NOT NULL PRIMARY KEY, " +
			"activity_name      VARCHAR(64) NOT NULL, " +
			"activity_type      VARCHAR(64) NOT NULL, " +
			"app_name           VARCHAR(128) NOT NULL, " +
			"module_name        VARCHAR(128) NOT NULL, " +
			"activity_msg       TEXT DEFAULT NULL, " +
			"activity_notes     TEXT DEFAULT NULL, " +
			"caller_loc         VARCHAR(20) NOT NULL, " +
			"created_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP"

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields +
			", INDEX idx_created_at (created_at) " +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields + ")"

	default:
		err := fmt.Errorf("database type not supported:%s (SHD_ALG_117)", db_type)
		log.Printf("***** Alarm:%s", err.Error())
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		error_msg := fmt.Errorf("failed creating table (SHD_ALG_045), err: %w, stmt:%s", err, stmt)
		log.Printf("***** Alarm: %s", error_msg.Error())
		return error_msg
	}

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_created_at ON ` + table_name + ` (created_at);`
		databaseutil.ExecuteStatement(db, idx1)
	}

	log.Printf("Create table '%s' success (SHD_ALG_188)", table_name)

	return nil
}

// Public API
// InitCache initializes the singleton cache with a database connection
// Call this once at application startup (e.g., in main())
func InitActivityLogCache(db_type string,
	table_name string,
	db *sql.DB) error {
	activity_log_once.Do(func() {
		activity_log_singleton = newActivityLogCache(db_type, table_name, db)
		activity_log_singleton.start()
	})
	return nil
}

// Public API
func StopActivityLogCache() {
	activity_log_singleton.StopActivityLogCache()
}

// Public API
func NextActivityLogID() int64 {
	return activity_log_singleton.nextLogID()
}

// AddActivityLog adds an activity log record to the cache.
// This is a non-blocking public API call. Records are added to the cache
// and flushed to the database in the background.
func AddActivityLog(record ApiTypes.ActivityLogDef) error {
	c := activity_log_singleton
	if c == nil {
		error_msg := "cache not initialized; call InitCache first (SHD_ALG_077)"
		log.Printf("***** Alarm:%s", error_msg)
		return fmt.Errorf("%s", error_msg)
	}
	c.addToCache(record)
	return nil
}

// Public API
// Stop signals the cache to flush remaining records and exit
func (c *ActivityLogCache) StopActivityLogCache() {
	close(c.done)
	c.wg.Wait() // Wait for flush loop to complete
}

func newActivityLogCache(db_type string,
	table_name string,
	db *sql.DB) *ActivityLogCache {
	logger := loggerutil.CreateLogger2(
		loggerutil.ContextTypeBackground,
		loggerutil.LogHandlerTypeDefault,
		10000)
	return &ActivityLogCache{
		db:                             db,
		db_type:                        db_type,
		table_name:                     table_name,
		done:                           make(chan struct{}),
		crt_log_id:                     -1,
		num_log_ids:                    0,
		id_name:                        "activity_log_id",
		logger:                         logger,
		activity_log_insert_fieldnames: "log_id, activity_name, activity_type, app_name, module_name, activity_msg, activity_notes, caller_loc",
	}
}

// Start begins the background flushing loop
func (c *ActivityLogCache) start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.flushLoop()
	}()
}

func (c *ActivityLogCache) nextLogID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.num_log_ids <= 0 {
		// Need to fetch a new block of IDs
		block_size := 1000

		start_id, err := NextIDBlock(c.id_name, block_size)
		if err != nil {
			c.logger.Error("failed to get next ID block for activity_log_id", "error", err)
			return -1
		}
		c.crt_log_id = start_id - 1
		c.num_log_ids = block_size
		c.logger.Info("Fetched new activity_log_id block",
			"start_id", start_id,
			"size", block_size)
	}
	id := c.crt_log_id
	c.crt_log_id++
	c.num_log_ids--
	return id
}

// flushLoop runs indefinitely, flushing cached records to DB every 10 seconds
func (c *ActivityLogCache) flushLoop() {
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
					c.logger.Error("flush failed (ticker). Records may be lost.", "error", err)
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
					c.logger.Error("Final flush failed. Records may be lost.", "error", err)
				}
			}
			return // Exit loop
		}
	}
}

func (c *ActivityLogCache) addToCache(record ApiTypes.ActivityLogDef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, record)
}

// insertRecords inserts records into the database using a transaction
func (c *ActivityLogCache) insertRecords(records []ApiTypes.ActivityLogDef) error {
	if len(records) == 0 {
		return nil
	}

	// c.logger.Info("Flush records", "len", len(records))

	// Start transaction
	tx, err := c.db.Begin()
	if err != nil {
		error_msg := fmt.Sprintf("failed to begin transaction: %v (SHD_ALG_163)", err)
		c.logger.Error("failed to begin transaction", "error", err)
		return fmt.Errorf("%s", error_msg)
	}

	defer func() {
		// Rollback on error (if transaction not committed)
		if tx != nil && err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				error_msg := fmt.Sprintf("original error: %v; rollback failed: %v (SHD_ALG_169)", err, rollbackErr)
				c.logger.Error("rollback error", "error", rollbackErr)
				err = fmt.Errorf("%s", error_msg)
			}
		}
	}()

	var stmt string
	switch c.db_type {
	case ApiTypes.MysqlName:
		stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, c.table_name, c.activity_log_insert_fieldnames)

	case ApiTypes.PgName:
		stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, c.table_name, c.activity_log_insert_fieldnames)

	default:
		c.logger.Error("unrecognized database type (SHD_ALG_220)", "db_type", c.db_type)
		stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, c.table_name, c.activity_log_insert_fieldnames)
	}

	stmt1, err := tx.Prepare(stmt)
	if err != nil {
		error_msg := fmt.Sprintf("failed to prepare statement: %v, stmt:%s (SHD_ALG_189)", err, stmt)
		c.logger.Error("failed to prepare statement", "error", err, "stmt", stmt)
		return fmt.Errorf("%s", error_msg)
	}

	defer stmt1.Close()

	// Insert each record
	for i, record := range records {
		if record.LogID <= 0 {
			record.LogID = c.nextLogID()
		}

		_, err := stmt1.Exec(
			record.LogID,
			record.ActivityName,
			record.ActivityType,
			record.AppName,        // *string (nil → NULL)
			record.ModuleName,     // *string (nil → NULL)
			record.ActivityMsg,    // *string (nil → NULL)
			record.Activity_notes, // *string (nil → NULL)
			record.CallerLoc)
		if err != nil {
			error_msg := fmt.Sprintf("record %d (log_id=%d) insert failed: %v (SHD_ALG_230)", i, record.LogID, err)
			c.logger.Error("database error", "error", err, "stmt", stmt)
			return fmt.Errorf("%s", error_msg)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		error_msg := fmt.Sprintf("failed to commit transaction: %v (SHD_ALG_236)", err)
		c.logger.Error("failed to commit", "error", err)
		return fmt.Errorf("%s", error_msg)
	}
	return nil
}
