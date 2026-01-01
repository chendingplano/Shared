package stores

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/sysdatastores"
)

type ResourceStore struct {
	mu           sync.Mutex // Ensures thread-safe access to records
	db           *sql.DB    // Database connection
	db_type      string
	table_name   string
	resource_map map[string]ApiTypes.ResourceStoreDef
	done         chan struct{}  // Signals shutdown
	wg           sync.WaitGroup // Tracks background goroutine
}

var (
	resource_store_singleton *ResourceStore
	resource_store_once      sync.Once // Ensures InitCache runs once
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
		resource_store_singleton.resource_map = make(map[string]ApiTypes.ResourceStoreDef)
		resource_store_singleton.start()
	})
	return nil
}

// Public API
// GetResourceDef retrieves the resource def by resource_name and resource_opr.
// If not found, it returns error.
func GetResourceDef(resource_name string, resource_opr string) (ApiTypes.ResourceStoreDef, error) {
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
	return &ResourceStore{
		db:         db,
		db_type:    db_type,
		table_name: table_name,
		done:       make(chan struct{}),
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
	field_names := "resource_id, resource_name, resource_opr, resource_desc,  resource_type, " +
		"db_name, table_name, resource_status, resource_def, query_conds"
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
		error_msg := fmt.Sprintf("(SHD_RST_120) database error:%v, query:%s", err, query)
		log.Printf("***** Alarm:%s", error_msg)
		return
	}

	defer rows.Close()

	var resource_def_sql, resource_cond_sql sql.NullString
	var num_resources = 0
	var resource_name, resource_opr string
	for rows.Next() {
		var row ApiTypes.ResourceDef
		if err := rows.Scan(&row.ResourceID,
			&resource_name,
			&resource_opr,
			&row.ResourceDesc,
			&row.ResourceType,
			&row.DBName,
			&row.TableName,
			&row.ResourceStatus,
			&resource_def_sql,
			&resource_cond_sql); err != nil {
			log.Printf("***** Row scan error: %v (SHD_RST_121)", err)
			continue
		}

		if resource_cond_sql.Valid {
			resource_cond_str := resource_cond_sql.String
			def_obj, err1 := ApiUtils.ConvertToJSON(resource_cond_str)
			if err1 != nil {
				row.ErrorMsg = fmt.Sprintf("incorrect resource_cond JSON string:%s (SHD_RST_131)", resource_cond_str)
				row.ResourceJSON = nil
				log.Printf("***** Alarm:%s (SHD_RST_156)", row.ErrorMsg)
				continue
			} else {
				row.QueryCondsJSON = def_obj
			}
		}

		key := resource_name + "_" + resource_opr
		var c_row ApiTypes.ResourceStoreDef
		c_row.ResourceDef = row

		if resource_def_sql.Valid {
			err := parseResourceDef(&row, resource_def_sql)
			if err != nil {
				row.ErrorMsg = fmt.Sprintf("Failed parsing resource_def json, err:%v, json_str:%s", err, resource_def_sql.String)
				log.Printf("***** Alarm:%s (SHD_RST_151)", row.ErrorMsg)
				sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
					ActivityName: ApiTypes.ActivityName_LoadResourceStore,
					ActivityType: ApiTypes.ActivityType_Failed,
					AppName:      ApiTypes.AppName_Stores,
					ModuleName:   ApiTypes.ModuleName_ResourceStore,
					ActivityMsg:  &row.ErrorMsg,
					CallerLoc:    "SHD_RST_175"})
			} else {
				// It successfully parsed resource_def into JSON object
				// and sets it to row.ResourceJSON.
				// Now construct structs from it.
				msg := fmt.Sprintf("Parsed resource_def JSON, resource:%s:%s",
					resource_name, resource_opr)
				log.Printf("%s (SHD_RST_153)", msg)

				sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
					ActivityName: ApiTypes.ActivityName_LoadResourceStore,
					ActivityType: ApiTypes.ActivityType_Success,
					AppName:      ApiTypes.AppName_Stores,
					ModuleName:   ApiTypes.ModuleName_ResourceStore,
					ActivityMsg:  &msg,
					CallerLoc:    "SHD_RST_188"})

				field_defs, err := ConstructFieldDefs(row.ResourceJSON, resource_name)
				if err == nil {
					msg := fmt.Sprintf("Constructed FieldDefs, resource:%s:%s",
						resource_name, resource_opr)
					log.Printf("%s (SHD_RST_153)", msg)

					sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
						ActivityName: ApiTypes.ActivityName_LoadResourceStore,
						ActivityType: ApiTypes.ActivityType_Success,
						AppName:      ApiTypes.AppName_Stores,
						ModuleName:   ApiTypes.ModuleName_ResourceStore,
						ActivityMsg:  &msg,
						CallerLoc:    "SHD_RST_200"})

					c_row.FieldDefs = field_defs
				}

				selected_defs, err := ConstructSelectedFields(row.ResourceJSON, resource_name)
				if err == nil {
					msg := fmt.Sprintf("Constructed SelectedFields, resource:%s:%s",
						resource_name, resource_opr)
					log.Printf("%s (SHD_RST_210)", msg)

					sysdatastores.AddActivityLog(ApiTypes.ActivityLogDef{
						ActivityName: ApiTypes.ActivityName_LoadResourceStore,
						ActivityType: ApiTypes.ActivityType_Success,
						AppName:      ApiTypes.AppName_Stores,
						ModuleName:   ApiTypes.ModuleName_ResourceStore,
						ActivityMsg:  &msg,
						CallerLoc:    "SHD_RST_218"})

					c_row.SelectedFields = selected_defs
				}
			}
		}

		log.Printf("Add resource, resource_name:%s, resource_opr:%s (SHD_RST_136)",
			resource_name, resource_opr)

		c.resource_map[key] = c_row
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

// parseResourceDef parses the resource definition from SQL result (string)
// converts it into a JSON object
// and populates the ResourceJSON field of the ResourceDef struct.
// It expects resource_def_sql to be a valid JSON string representing
// a JSON object (map).
// Upon success, it returns nil. If any error occurs, it returns a non-nil error.
func parseResourceDef(row *ApiTypes.ResourceDef, resource_def_sql sql.NullString) error {
	if !resource_def_sql.Valid {
		error_msg := "missing resource_def (SHD_RST_231)"
		log.Printf("***** Alarm:%s", error_msg)
		return fmt.Errorf("%s", error_msg)
	}

	resource_def_str := resource_def_sql.String
	def_obj, data_type, err1 := ApiUtils.ConvertToAny(resource_def_str)
	if err1 != nil {
		error_msg := fmt.Sprintf("incorrect resource_def, error:%v, JSON string:%s", err1, resource_def_str)
		log.Printf("***** Alarm:%s", error_msg)
		return fmt.Errorf("%s", error_msg)
	}

	if data_type != "map" {
		error_msg := fmt.Sprintf("resource_def MUST be a valid JSON (not array or other type):%s", resource_def_str)
		log.Printf("***** Alarm:%s", error_msg)
		return fmt.Errorf("%s", error_msg)
	}

	row.ResourceJSON = def_obj.(map[string]interface{})
	log.Printf("Load Resource, resource_name:%s (SHD_RST_250)", row.ResourceName)
	return nil
}

// ConstructFieldDefs constructs the following from ResourceJSON:
//
//  1. FieldDefs ([]AosTypes.FieldDef). It expects:
//     "field_defs": [
//     {
//     "field_name": "name1",
//     "data_type": "string",
//     "required": true,
//     "desc": "description1"
//     },
//     ...
//     ]
//
//  2. SelectedFields ([]AosTypes.FieldDef). It expects:
//     "selected_fields": [
//     {
//     "field_name": "name1",
//     "data_type": "string",
//     "required": true,
//     "desc": "description1"
//     },
//     ...
//     ]
//
// returns a non-nil error.
// This function is called when resource store loads resources from DB.
func ConstructFieldDefs(resource_json map[string]interface{},
	resource_name string) ([]ApiTypes.FieldDef, error) {
	// This function assumes 'field_defs' is an attribute in 'resource_json':
	const field_name = "field_defs"
	value_obj, ok := resource_json[field_name]
	if !ok {
		// It is OK if 'field_defs' is not present. Return empty slice.
		log.Printf("Field '%s' not present, resource_name:%s (SHD_RST_274)",
			field_name, resource_name)
		return nil, nil
	}

	value_slice, ok := value_obj.([]interface{})
	if !ok {
		error_msg := fmt.Sprintf("field 'field_defs' is not a valid JSON, resource_name:%s (SHD_RHD_777)", resource_name)
		log.Printf("***** Alarm:%s", error_msg)
		return nil, fmt.Errorf("%s", error_msg)
	}

	field_defs := make([]ApiTypes.FieldDef, len(value_slice))
	for i, v := range value_slice {
		jsonData, err := json.Marshal(v)
		if err != nil {
			error_msg := fmt.Sprintf("invalid field_def:%v, resource_name:%s (SHD_RHD_787)", v, resource_name)
			log.Printf("***** Alarm:%s", error_msg)
			return nil, fmt.Errorf("%s", error_msg)
		}

		// Unmarshal into the struct
		var field_def ApiTypes.FieldDef
		if err := json.Unmarshal(jsonData, &field_def); err != nil {
			error_msg := fmt.Sprintf("invalid field_def:%v, resource_name:%s (SHD_RHD_787)", v, resource_name)
			log.Printf("***** Alarm:%s", error_msg)
			return nil, fmt.Errorf("%s", error_msg)
		}
		field_defs[i] = field_def
	}

	return field_defs, nil
}

// ConstructSelectedFields constructs SelectedFields from ResourceJSON:
// It expects:
//
//	"selected_fields": [
//	    {
//	        "field_name": "name1",
//	        "data_type": "string",
//	        "required": true,
//	        "desc": "description1"
//	    },
//	    ...
//	]
//
// returns a non-nil error.
// This function is called when resource store loads resources from DB.
func ConstructSelectedFields(resource_json map[string]interface{},
	resource_name string) ([]ApiTypes.FieldDef, error) {
	// This function assumes 'selected_fields' is an attribute in 'resource_json':
	const field_name = "selected_fields"
	value_obj, ok := resource_json[field_name]
	if !ok {
		// It is OK if 'selected_fields' is not present. Return empty slice.
		log.Printf("Field '%s' not present, resource_name:%s (SHD_RST_274)",
			field_name, resource_name)
		return nil, nil
	}

	value_slice, ok := value_obj.([]interface{})
	if !ok {
		error_msg := fmt.Sprintf("field %s is not a valid JSON, resource_name:%s (SHD_RHD_777)",
			field_name, resource_name)
		log.Printf("***** Alarm:%s", error_msg)
		return nil, fmt.Errorf("%s", error_msg)
	}

	selected_fields := make([]ApiTypes.FieldDef, len(value_slice))
	for i, v := range value_slice {
		jsonData, err := json.Marshal(v)
		if err != nil {
			error_msg := fmt.Sprintf("invalid field_def:%v, resource_name:%s (SHD_RHD_787)", v, resource_name)
			log.Printf("***** Alarm:%s", error_msg)
			return nil, fmt.Errorf("%s", error_msg)
		}

		// Unmarshal into the struct
		var field_def ApiTypes.FieldDef
		if err := json.Unmarshal(jsonData, &field_def); err != nil {
			error_msg := fmt.Sprintf("invalid field_def:%v, resource_name:%s (SHD_RHD_787)", v, resource_name)
			log.Printf("***** Alarm:%s", error_msg)
			return nil, fmt.Errorf("%s", error_msg)
		}
		selected_fields[i] = field_def
	}

	return selected_fields, nil
}
