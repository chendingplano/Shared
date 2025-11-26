package stores

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
)

type ResourceStore struct {
    mu          	sync.Mutex       // Ensures thread-safe access to records
    db          	*sql.DB          // Database connection
    db_type     	string
    table_name  	string
    resource_map    map[string]ApiTypes.ResourceDef
    done        	chan struct{}    // Signals shutdown
    wg          	sync.WaitGroup   // Tracks background goroutine
}

var (
    resource_store_singleton *ResourceStore
    resource_store_once       sync.Once   // Ensures InitCache runs once
)

// Public API
// InitCache initializes the singleton cache with a database connection
// Call this once at application startup (e.g., in main())
func InitResourceStore(db_type string,
                    table_name string,
                    db *sql.DB) error {
    resource_store_once.Do(func() {
		log.Printf("InitResourceStore (SHD_RST_035)")
        resource_store_singleton = newResourceStore(db_type, table_name, db)

		// Initialize the map
		resource_store_singleton.resource_map = make(map[string]ApiTypes.ResourceDef)
        resource_store_singleton.start()
    })
    return nil
}

// Public API
func GetResourceDef(resource_name string, resource_opr string) (ApiTypes.ResourceDef, error) {
    key := resource_name + "_" + resource_opr
    resource_def, exists := resource_store_singleton.resource_map[key]
    if exists {
        return resource_def, nil
    }
    return resource_def, fmt.Errorf("resource not exist, resource_name:%s, resource_opr:%s (SHD_RST_055)", 
            resource_name, resource_opr)
}

// Public API
func StopResourceStore() {
    resource_store_singleton.StopResourceStore()
}

// Public API
// Stop signals the cache to flush remaining records and exit
func (c *ResourceStore) StopResourceStore() {
    close(c.done)
    c.wg.Wait() // Wait for flush loop to complete
}

func newResourceStore(db_type string,
            table_name string,
            db *sql.DB) *ResourceStore {
    return &ResourceStore {
        db:             db,
        db_type:        db_type,
        table_name:     table_name,
        done:           make(chan struct{}),
    }
}

// Start begins the background flushing loop
func (c *ResourceStore) start() {
    c.wg.Add(1)
    go func() {
        log.Printf("Start Resource Store (SHD_RST_082)")
        c.LoadResourcesFromDB()
        defer c.wg.Done()
        c.houseKeepingLoop()
    }()
}

func (c *ResourceStore) LoadResourcesFromDB() {
	// 2. Insert a record to id_mgr for activity_log id.
    log.Printf("Load Resources from DB (SHD_RST_091)")
	field_names := "resource_id, resource_name, resource_opr, resource_type, resource_def, query_conditions"
	var query string
    db_type := c.db_type
    table_name := c.table_name
	switch db_type {
    case ApiTypes.MysqlName:
         query = fmt.Sprintf("SELECT %s FROM %s", field_names, table_name)
    
    case ApiTypes.PgName:
         query = fmt.Sprintf("SELECT %s FROM %s", field_names, table_name)

	default:
		// SHOULD NEVER HAPPEN!!!
		error_msg := fmt.Sprintf("unrecognized db_type:%s (SHD_RST_033)", db_type)
		log.Printf("***** Alarm: %s", error_msg)
        return 
	}

    rows, err := c.db.Query(query) 
	if err != nil {
		error_msg := fmt.Sprintf("database error:%v (SHD_RST_133)", err)
		log.Printf("***** Alarm:%s", error_msg)
		return
	}

	defer rows.Close()

    var resource_def_sql, resource_cond_sql sql.NullString 
    var num_resources = 0
	for rows.Next() {
		var row ApiTypes.ResourceDef
		if err := rows.Scan(&row.ResourceID, 
						&row.ResourceName, 
						&row.ResourceOpr, 
						&row.ResourceType,
						&resource_def_sql,
                        &resource_cond_sql); err != nil {
			log.Printf("***** Row scan error: %v (SHD_RST_121)", err)
			continue
		}

        if resource_def_sql.Valid {
            resource_def_str := resource_def_sql.String
            def_obj, err1 := ApiUtils.ConvertToJSON(resource_def_str)
            if err1 != nil {
                row.ErrorMsg = fmt.Sprintf("incorrect resource_def JSON string:%s", resource_def_str)
                row.ResourceDef = nil
                row.LOC = "SHD_RST_131"
                log.Printf("***** Alarm:%s", row.ErrorMsg)
                continue
            } else {
                row.ResourceDef = def_obj
            }
        }

        if resource_cond_sql.Valid {
            resource_cond_str := resource_cond_sql.String
            def_obj, err1 := ApiUtils.ConvertToJSON(resource_cond_str)
            if err1 != nil {
                row.ErrorMsg = fmt.Sprintf("incorrect resource_cond JSON string:%s", resource_cond_str)
                row.ResourceDef = nil
                row.LOC = "SHD_RST_131"
                log.Printf("***** Alarm:%s", row.ErrorMsg)
                continue
            } else {
                row.QueryConditions = def_obj
            }
        }

        row.LOC = "SHD_RST_135"

        log.Printf("Load resource, resource_name:%s, resource_opr:%s (SHD_RST_136)", 
                row.ResourceName, row.ResourceOpr)
        key := row.ResourceName + "_" + row.ResourceOpr
        c.resource_map[key] = row
        num_resources += 1
	}

	if err := rows.Err(); err != nil {
		error_msg := fmt.Sprintf("Rows error: %v (SHD_RST_128)", err)
        log.Printf("***** Alarm:%s", error_msg)
        return
	}

    log.Printf("Load Resource success, num_resources:%d (SHD_RST_158)", num_resources)
}

// houseKeepingLoop runs indefinitely, flushing cached records to DB every 10 seconds
func (c *ResourceStore) houseKeepingLoop() {
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
