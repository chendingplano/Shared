package databaseutil

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// TableDefinition represents how a table is defined in your JSON docs.
type TableDefinition struct {
	DBType 			string 					`json:"db_type,omitempty"`
	DBName 			string 					`json:"db_name,omitempty"`
    TableName 		string         			`json:"table_name"`
	TableType 		string         			`json:"table_type,omitempty"`
    TableDesc 		string         			`json:"table_desc,omitempty"`
    TableDef 		[]ApiTypes.FieldDef 	`json:"table_def"`
    Remarks 		string         			`json:"remarks,omitempty"`
}

// ManagedTable is what we cache in memory.
type ManagedTable struct {
    DbType      string
    Name        string
    TableDesc   *string
    Definition  *TableDefinition
    RawJSON     string  // raw JSON from table_def
    Remarks     *string
}

// TableManager manages the `table_manager` table and in-memory cache.
type TableManager struct {
    mu     sync.RWMutex
    tables map[string]*ManagedTable
    db     *sql.DB
    dbType string
}

// GlobalTableManager optional singleton.
var GlobalTableManager *TableManager

// InitTableManager should be called on system startup after DB init.
func InitTableManager() error {
    db, dbType, err := getCurrentDB()
    if err != nil {
        return err
    }

    tm := &TableManager{
        tables: make(map[string]*ManagedTable),
        db:     db,
        dbType: dbType,
    }

    if err := tm.loadAll(); err != nil {
        return err
    }

    GlobalTableManager = tm
    log.Printf("TableManager initialized with %d tables cached", len(tm.tables))
    return nil
}

func getCurrentDB() (*sql.DB, string, error) {
    dbType := ApiTypes.DatabaseInfo.DBType
    switch dbType {
    case ApiTypes.MysqlName:
         if ApiTypes.MySql_DB_miner == nil {
            return nil, "", fmt.Errorf("MySQL DB handle is nil")
         }
         return ApiTypes.MySql_DB_miner, dbType, nil

    case ApiTypes.PgName:
         if ApiTypes.PG_DB_miner == nil {
            return nil, "", fmt.Errorf("PostgreSQL DB handle is nil")
         }
         return ApiTypes.PG_DB_miner, dbType, nil

    default:
        return nil, "", fmt.Errorf("unsupported database type for TableManager: %s", dbType)
    }
}

// loadAll reads all non-deleted rows from table_manager and caches them.
func (tm *TableManager) loadAll() error {
    // Adjust WHERE as needed (e.g. add del_flag if you later add it).
    const query = `
        SELECT db_type, table_name, table_desc, table_def, remarks
        FROM table_manager
    `

    rows, err := tm.db.Query(query)
    if err != nil {
        return fmt.Errorf("failed to load table_manager records: %w", err)
    }
    defer rows.Close()

    tm.mu.Lock()
    defer tm.mu.Unlock()

    tm.tables = make(map[string]*ManagedTable)

    for rows.Next() {
        var (
            db_type     string
            name        string
            desc        *string
            rawJSON     string
            remarks     *string
        )

        if err := rows.Scan(&db_type, &name, &desc, &rawJSON, &remarks); err != nil {
            return fmt.Errorf("failed to scan table_manager row: %w", err)
        }

        if !IsValidTableName(name) {
            log.Printf("Skipping invalid table_name in table_manager: %s", name)
            continue
        }

        mt := &ManagedTable{
            DbType:      db_type,
            Name:        name,
            TableDesc:   desc,
            RawJSON:     rawJSON,
            Remarks:     remarks,
        }

        var def TableDefinition
        if err := json.Unmarshal([]byte(rawJSON), &def); err == nil {
            if def.TableName == "" {
                def.TableName = name
            }

			if def.TableDesc == "" && desc != nil {
				def.TableDesc = *desc
			}

            mt.Definition = &def
        } else {
            log.Printf("Warning: failed to parse table_def JSON for table %s: %v", name, err)
        }

        tm.tables[name] = mt
    }

    if err := rows.Err(); err != nil {
        return fmt.Errorf("error iterating table_manager rows: %w", err)
    }

    return nil
}

// RegisterTable is called when a new physical table is created.
// tableDefJSON is the JSON definition to store in table_def.
func (tm *TableManager) RegisterTable(
    tableName string,
    tableDesc *string,
    tableDefJSON string,
    remarks *string,
) error {
    if !IsValidTableName(tableName) {
        return fmt.Errorf("invalid table name: %s", tableName)
    }

    // We assume created_at/updated_at have default values in the schema,
    // so we don't explicitly set them here.
    const field_names = "db_type, table_name, table_type, table_desc, table_def"
    db_type := tm.dbType 
    table_name := ApiTypes.LibConfig.SystemTableNames.TableNameTableManager
    var stmt string
    switch tm.dbType {
    case ApiTypes.MysqlName:
         stmt = "INSERT INTO " + table_name + " (" + field_names + 
               ") VALUES (?, ?, ?, ?, ?)"
        
    case ApiTypes.PgName:
         stmt = "INSERT INTO " + table_name + " (" + field_names + 
                ") VALUES ($1, $2, $3, $4, $5)"

    default:
        return fmt.Errorf("unsupported db type in TableManager.RegisterTable: %s", tm.dbType)
    }

    if _, err := tm.db.Exec(stmt, db_type, tableName, tableDesc, tableDefJSON, remarks); err != nil {
        return fmt.Errorf("failed to insert into table_manager: %w", err)
    }

    // Update in‑memory cache.
    mt := &ManagedTable{
        DbType:      db_type,
        Name:        tableName,
        TableDesc:   tableDesc,
        RawJSON:     tableDefJSON,
        Remarks:     remarks,
    }

    var def TableDefinition
    if err := json.Unmarshal([]byte(tableDefJSON), &def); err == nil {
        if def.TableName == "" {
            def.TableName = tableName
        }
        mt.Definition = &def
    } else {
        error_msg := fmt.Sprintf("failed to parse table_def JSON for the new table:%s: %v", tableName, err)
        log.Printf("***** Alarm:%s (SHD_TMG_221)", error_msg)
        return fmt.Errorf("%s", error_msg)
    }

    tm.mu.Lock()
    tm.tables[tableName] = mt
    tm.mu.Unlock()

    log.Printf("Registered table in table_manager: %s", tableName)
    return nil
}

// GetTable returns the cached ManagedTable.
func (tm *TableManager) GetTable(tableName string) (*ManagedTable, bool) {
    tm.mu.RLock()
    defer tm.mu.RUnlock()
    mt, ok := tm.tables[tableName]
    return mt, ok
}

// ListTables returns all managed table names.
func (tm *TableManager) ListTables() []string {
    tm.mu.RLock()
    defer tm.mu.RUnlock()

    names := make([]string, 0, len(tm.tables))
    for name := range tm.tables {
        names = append(names, name)
    }
    return names
}