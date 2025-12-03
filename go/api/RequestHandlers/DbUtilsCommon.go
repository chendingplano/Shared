package RequestHandlers

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// InsertBatch inserts multiple records into the specified table
// and returns the auto-generated prompt_id values. It works for both
// single and batch inserts. The tableName and columns must be valid
// and sanitized to prevent SQL injection, as they are interpolated
// directly into the SQL string.
func InsertBatch(
	user_name string,
	db *sql.DB,
	tableName string,
	resource_name string,
	resource_store ApiTypes.ResourceStoreDef,
	fieldDefs []ApiTypes.FieldDef,
	records []map[string]interface{},
	batchSize int,
	db_type string) error {
	// This function inserts records in batch. It supports MySQL and PostgreSQL only now.
	// In the future, it may support more databases.
	if batchSize <= 0 {
		batchSize = 30
	}

	columns := []string{}
	for _, f := range fieldDefs {
		switch f.DataType {
		case "_ignore":
			 continue // Skip to next field
			
		case "_auto_inc":
			 continue // Skip to next field
			
		default:
			 columns = append(columns, f.FieldName)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	total := len(records)
	conflict_suffix := ""

	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		chunk := records[start:end]

		valueGroups := []string{}
		args := []interface{}{}


		switch db_type {
		case ApiTypes.MysqlName:
			 var err1 error
			 valueGroups, args, err1 = CreateValueGroupsMySQL(user_name, fieldDefs, chunk)
			 if err1 != nil {
				log.Printf("CreateValueGroupsMySQL failed, %d:%d", len(valueGroups), len(args))
				return err1
			 }

			 conflict_suffix, _ = CreateOnConflictMySQL(resource_store, resource_name)

		case ApiTypes.PgName:
			 var err1 error
			 valueGroups, args, err1 = CreateValueGroupsPG(user_name, fieldDefs, chunk)
			 if err1 != nil {
				log.Printf("CreateValueGroupsPG failed, %d:%d", len(valueGroups), len(args))
				return err1
			 }

			 conflict_suffix, _ = CreateOnConflictPG(resource_store, resource_name)

		default:
		 	 error_msg := fmt.Sprintf("invalid db type:%s", db_type)
		 	 log.Printf("***** Alarm:%s (SHD_DUP_059), %d:%d", error_msg, len(valueGroups), len(args))
			 return fmt.Errorf("%s", error_msg) 
		}

		if len(valueGroups) == 0 {
		 	 error_msg := fmt.Sprintf("missing values, db_type:%s, table_name:%s", db_type, tableName)
		 	 log.Printf("***** Alarm:%s (SHD_DUP_086), %d:%d", error_msg, len(valueGroups), len(args))
			 return fmt.Errorf("%s", error_msg)
		}

		sqlStr := fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES %s",
			tableName,
			strings.Join(columns, ","),
			strings.Join(valueGroups, ","),
		)

		if conflict_suffix != "" {
			sqlStr = sqlStr + " " + conflict_suffix
		}

		_, err := tx.Exec(sqlStr, args...)
		if err != nil {
			error_msg := fmt.Sprintf("failed run statement, error:%v, stmt:%s, values:%v", err, sqlStr, args)
			log.Printf("%s (SHD_UTC_103)", error_msg)
			return fmt.Errorf("%s", error_msg)
		}
	}

	return tx.Commit()
}

func InsertAutoColumns(
	db *sql.DB,
	tableName string,
	records []map[string]interface{},
	batchSize int,
) error {
	// This function infers the columns to insert automatically
	// from the input 'records'.

	if len(records) == 0 {
		return fmt.Errorf("no records")
	}

	// 1. Infer set of columns from all records
	colSet := map[string]struct{}{}
	for _, rec := range records {
		for k := range rec {
			colSet[k] = struct{}{}
		}
	}

	columns := []string{}
	for col := range colSet {
		columns = append(columns, col)
	}

	// 2. Build batch insert using ? placeholders
	if batchSize <= 0 {
		batchSize = 30
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	total := len(records)

	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		chunk := records[start:end]

		placeholders := []string{}
		args := []interface{}{}

		for _, rec := range chunk {
			row := []string{}
			for _, col := range columns {
				val, ok := rec[col]
				if !ok {
					val = nil
				}
				args = append(args, val)
				row = append(row, "?")
			}
			placeholders = append(placeholders, "("+strings.Join(row, ",")+")")
		}

		sqlStr := fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES %s",
			tableName,
			strings.Join(columns, ","),
			strings.Join(placeholders, ","),
		)

		if _, err := tx.Exec(sqlStr, args...); err != nil {
			return err
		}
	}

	return tx.Commit()
}
