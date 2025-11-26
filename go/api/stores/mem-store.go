package stores

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type InMemStore struct {
    mu          	sync.Mutex       // Ensures thread-safe access to records
    db          	*sql.DB          // Database connection
    db_type     	string
    table_name  	string
	id_start_value	int
	id_inc_value	int
    id_records		map[string]ApiTypes.IDRecordDef
    done        	chan struct{}    // Signals shutdown
    wg          	sync.WaitGroup   // Tracks background goroutine
}

var (
    in_mem_store_singleton *InMemStore
    in_mem_store_once       sync.Once   // Ensures InitCache runs once
)

// Public API
// InitCache initializes the singleton cache with a database connection
// Call this once at application startup (e.g., in main())
func InitInMemStore(db_type string,
                    table_name string,
                    db *sql.DB,
					id_start_value int,
					id_inc_value int,
					id_keys map[string]interface{}) error {
    in_mem_store_once.Do(func() {
		log.Printf("InitInMemStore (SHD_MST_035)")
        in_mem_store_singleton = newInMemStore(db_type, table_name, db)
		in_mem_store_singleton.id_start_value = id_start_value
		in_mem_store_singleton.id_inc_value = id_inc_value

		// Initialize the map
		in_mem_store_singleton.id_records = make(map[string]ApiTypes.IDRecordDef)

		var id_start_value = in_mem_store_singleton.id_start_value
		var id_inc_value = in_mem_store_singleton.id_inc_value
    	for key, value := range id_keys {
			if id_desc, ok := value.(string); ok {
				var record = ApiTypes.IDRecordDef {
					CrtLogId: 	    int64(id_start_value),
					NumLogIds:	    0,
					IdBlockSize:	id_inc_value,
					IdDesc:		    id_desc,
				}
				in_mem_store_singleton.id_records[key] = record
       			log.Printf("Add system ID (SHD_MST_075): str_value:%s", key)

				in_mem_store_singleton.UpsertSystemIDDef(key, id_desc)
       		} else {
           		log.Printf("***** Alarm:Key (SHD_MST_077): %s, Value: %v (not a string)", key, value)
			}
		}
        in_mem_store_singleton.start()
    })
    return nil
}

// Public API
func StopInMemStore() {
    in_mem_store_singleton.StopInMemStore()
}

// Public API
func NextManagedID(key string) int64 {
    return in_mem_store_singleton.nextManagedID(key)
}

// Public API
// Stop signals the cache to flush remaining records and exit
func (c *InMemStore) StopInMemStore() {
    close(c.done)
    c.wg.Wait() // Wait for flush loop to complete
}

func newInMemStore(db_type string,
            table_name string,
            db *sql.DB) *InMemStore {
    return &InMemStore {
        db:             db,
        db_type:        db_type,
        table_name:     table_name,
        done:           make(chan struct{}),
    }
}

// Start begins the background flushing loop
func (c *InMemStore) start() {
    c.wg.Add(1)
    go func() {
        defer c.wg.Done()
        c.houseKeepingLoop()
    }()
}

func (c *InMemStore) UpsertSystemIDDef(id_name string, id_desc string) error {
	// 2. Insert a record to id_mgr for activity_log id.
	field_names := "id_name, crt_value, id_desc, caller_loc"
	var stmt string
    db_type := c.db_type
    table_name := c.table_name
	switch db_type {
    case ApiTypes.MysqlName:
         stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES (?, ?, ?, ?)
              ON DUPLICATE KEY UPDATE id_name = id_name`, table_name, field_names)
    
    case ApiTypes.PgName:
         stmt = fmt.Sprintf(`INSERT INTO %s (%s) VALUES ($1, $2, $3, $4)
            ON CONFLICT (id_name)
            DO NOTHING`, table_name, field_names)

	default:
		// SHOULD NEVER HAPPEN!!!
		error_msg := fmt.Sprintf("unrecognized db_type:%s (SHD_IMG_033)", db_type)
		log.Printf("***** Alarm: %s", error_msg)
        return fmt.Errorf("%s", error_msg)
	}

	_, err := c.db.Exec(stmt, id_name, c.id_start_value, id_desc, "SHD_MST_136")
    if err != nil {
        error_msg := fmt.Sprintf("failed to insert activity_log_id record (SHD_MST_138): %v, stmt:%s", err, stmt)
        log.Printf("***** Alarm %s", error_msg)
        return fmt.Errorf("%s", error_msg)
    }

    return nil
}

func (c *InMemStore) nextManagedID(key string) int64 {
    c.mu.Lock()
    defer c.mu.Unlock()

	// Check if a key exists in the map
	var record ApiTypes.IDRecordDef
	var exists bool
	record, exists = c.id_records[key]

	if !exists {
    	// Entry does not exist
        log.Printf("***** Alarm: ID record not found:%s (SHD_MST_150)", key)
		record = ApiTypes.IDRecordDef {
			CrtLogId: 	    int64(c.id_start_value),
			NumLogIds:	    0,
			IdBlockSize:	c.id_inc_value,
		}
	}

    if record.CrtLogId <= 0 || record.NumLogIds <= 0{
        log.Printf("Retrieve ID block from DB:%s (SHD_MST_159)", key)
        new_log_id, err := c.NextIDBlock(key, record.IdBlockSize)
        if err != nil {
           eMsg := err.Error()
           errorMsg := fmt.Sprintf("failed retrieving log ID block for %s: %s (SHD_MST_201)", key, eMsg)
           log.Printf("***** Alarm:%s", errorMsg)
        }

        if new_log_id <= 0 {
           errorMsg := fmt.Sprintf("invalid new log ID (<=0) for %s (SHD_MST_206)", key)
           log.Printf("***** Alarm:%s", errorMsg)
        }

        log.Printf("Next ID block retrieved (SHD_MST_170), start_id:%d, block_size:%d", new_log_id, record.IdBlockSize)
        record.CrtLogId = new_log_id
        record.NumLogIds = record.IdBlockSize
    }

    log.Printf("Next ID for:%s, id:%d, remain:%d (SHD_MST_177)", key, record.CrtLogId, record.NumLogIds)

    next_log_id := record.CrtLogId
    record.CrtLogId += 1
    record.NumLogIds -= 1

	c.id_records[key] = record
    return next_log_id
}

// houseKeepingLoop runs indefinitely, flushing cached records to DB every 10 seconds
func (c *InMemStore) houseKeepingLoop() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop() // Ensure ticker is stopped when loop exits

    for {
        select {
        case <-ticker.C:
            // The ticker (timer) creates a channel: ticker.C.
            // When the ticker times out, it will send a value to the channel,
			// which triggers to here.
            c.mu.Lock()
			// Collect the info to do
            c.mu.Unlock()

			// If there are tasks to do, do them here

        case <-c.done:
			// This is called when the store finishes. Collect all the 
			// 'cleanup' tasks and do the tasks.
            c.mu.Lock()
			// Collect the cleanup tasks
            c.mu.Unlock()

			// If there are cleanup tasks, do them here.
            return // Exit loop
        }
    }
}

func (c *InMemStore) NextIDBlock(id_name string, inc_value int) (int64, error) {
	// This function retrieves a block of IDs and updates the record.
	// Upon success, it returns the start log ID of the ID block.
	var query string

    switch c.db_type {
    case ApiTypes.MysqlName:
		 // Support MySQL 8.0.21+
    	 query = fmt.Sprintf("UPDATE %s SET crt_value = crt_value + ? WHERE id_name = ? RETURNING crt_value", c.table_name)
    
    case ApiTypes.PgName:
    	 query = fmt.Sprintf("UPDATE %s SET crt_value = crt_value + $1 WHERE id_name = $2 RETURNING crt_value", c.table_name)
    
    default:
         error_msg := fmt.Sprintf("unsupported database type (SHD_MST_034): %s", c.db_type)
         log.Printf("***** Alarm:%s", error_msg)
         return -1, fmt.Errorf("%s", error_msg)
    }

    tx, err := c.db.Begin()
    if err != nil {
        error_msg := fmt.Sprintf("failed to start transaction: %v", err)
        log.Printf("***** Alarm:%s (SHD_MST_188)", error_msg)
        return 0, fmt.Errorf("%s", error_msg)
    }

    defer func() {
        // Rollback if the function exits with an error (e.g., query failure)
        if tx != nil && err != nil {
            tx.Rollback()
        }
    }()

    // Execute the update and scan the original value
    var originalValue int64
    err = tx.QueryRow(query, inc_value, id_name).Scan(&originalValue)
    if err != nil {
        if err == sql.ErrNoRows {
            error_msg := fmt.Sprintf("record with id_name '%s' not found, stmt:%s (SHD_MST_135)", id_name, query)
            log.Printf("***** Alarm:%s", error_msg)
            return 0, fmt.Errorf("%s", error_msg)
        }

        error_msg := fmt.Sprintf("failed to update and retrieve: %v, stmt:%s (SHD_MST_140)", err, query)
        log.Printf("***** Alarm:%s", error_msg)
        return 0, fmt.Errorf("%s", error_msg)
    }

    // Commit the transaction
    if err = tx.Commit(); err != nil {
        error_msg := fmt.Sprintf("failed to commit transaction (SHD_MST_136): %v", err)
        log.Printf("***** Alarm:%s", error_msg)
        return 0, fmt.Errorf("%s", error_msg)
    }

    return originalValue, nil
}