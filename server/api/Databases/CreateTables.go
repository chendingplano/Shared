package Databases

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

/*
func AosCreateCustomerTable(db *sql.DB, db_type string) error {
    stmt := "CREATE TABLE IF NOT EXISTS customers (" +
            "id SERIAL PRIMARY KEY, " +
            "user_name VARCHAR(255) NOT NULL, " +
            "date_of_birth DATE, " +
            "email VARCHAR(255) NOT NULL UNIQUE, " +
            "phone_number VARCHAR(20), " +
            "education VARCHAR(100), " +
            "is_married BOOLEAN DEFAULT FALSE, " +
            "number_of_kids INTEGER DEFAULT 0, " +
            "created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP)"

    err := ExecuteStatement(db, stmt)
    if err != nil {
        return fmt.Errorf("***** Alarm: Failed creating table (MID_001_025), err: %w, stmt:%s", err, stmt)
    }

    log.Printf("=== Creating table success:%s", stmt)

    return nil
}
*/


func AosCreateProcessStatusTable(
        db *sql.DB, 
        db_type string,
        table_name string) error {
    var stmt string
    switch db_type {
    case sg_mysql_name:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + " (" +
            "id SERIAL PRIMARY KEY, " +
            "status_name VARCHAR(32) NOT NULL, " +
            "status_value VARCHAR(32) NOT NULL, " +
            "rcd_count INTEGER DEFAULT 0, " +
            "tags VARCHAR(255) NOT NULL, " +
            "created_at DATETIME DEFAULT CURRENT_TIMESTAMP)"

    case sg_pg_name:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + " (" +
            "id SERIAL PRIMARY KEY, " +
            "status_name VARCHAR(32) NOT NULL, " +
            "status_value VARCHAR(32) NOT NULL, " +
            "rcd_count INTEGER DEFAULT 0, " +
            "tags VARCHAR(255) NOT NULL, " +
            "created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW())"

    default:
        err := fmt.Errorf("database type not supported:%s (MID_CTB_061)", db_type)
        log.Printf("***** Alarm:%s", err.Error())
        return err
    }

    err := ExecuteStatement(db, stmt)
    if err != nil {
        err1 := fmt.Errorf("Failed creating table '%s' (MID_CTB_048), err: %w, stmt:%s", table_name, err, stmt)
        log.Printf("***** Alarm: %s", err1.Error())
        return err1
    }

    log.Printf("Create table '%s' success (MID_CTB_060):%s", table_name)
    return nil
}


func AosCreateLoginSessionsTable(
            db *sql.DB, 
            db_type string,
            table_name string) error {
    var stmt string
    switch db_type {
    case sg_mysql_name:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
            "login_method VARCHAR(32), " +
            "session_id VARCHAR(128), " +
            "status VARCHAR(32) DEFAULT NULL, " +
            "user_name VARCHAR(64) NOT NULL PRIMARY KEY, " +
            "user_name_type VARCHAR(32) DEFAULT NULL, " +
            "user_reg_id VARCHAR(255) DEFAULT NULL, " +
            "expires_at TIMESTAMP NOT NULL, " +
            "created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, " +
            "INDEX idx_expires (expires_at), " +
            "INDEX idx_session_id (session_id) " +
            ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

    case sg_pg_name:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
            "login_method VARCHAR(32), " +
            "session_id VARCHAR(128), " +
            "status VARCHAR(32) DEFAULT NULL, " +
            "user_name VARCHAR(64) NOT NULL PRIMARY KEY, " +
            "user_name_type VARCHAR(32) DEFAULT NULL, " +
            "user_reg_id VARCHAR(255) DEFAULT NULL, " +
            "expires_at TIMESTAMP NOT NULL, " +
            "created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW())"

    default:
        err := fmt.Errorf("database type not supported:%s (MID_CTB_117)", db_type)
        log.Printf("***** Alarm:%s", err.Error())
        return err
    }

    err := ExecuteStatement(db, stmt)
    if err != nil {
        err1 := fmt.Errorf("Failed creating table '%s' (MID_CTB_124), err: %w, stmt:%s", table_name, err, stmt)
        log.Printf("***** Alarm: %s", err1.Error())
        return err1
    }

    if db_type == sg_mysql_name {
        idx1 := `CREATE INDEX IF NOT EXISTS idx_expires ON ` + table_name + ` (expires_at);`
        ExecuteStatement(db, idx1)

        idx2 := `CREATE INDEX IF NOT EXISTS idx_session_id ON ` + table_name + ` (session_id);`
        ExecuteStatement(db, idx2)
    }

    log.Printf("Create table '%s' success (MID_CTB_129):%s", table_name)
    return nil
}


func AosCreateUsersTable(
            db *sql.DB, 
            db_type string,
            table_name string) error {
    var stmt string
    switch db_type {
    case sg_mysql_name:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
            "user_name VARCHAR(128) NOT NULL PRIMARY KEY, " +
            "password VARCHAR(128) DEFAULT NULL, " +
            "user_id_type VARCHAR(32), " +
            "user_real_name VARCHAR(128) DEFAULT NULL, " +
            "user_email VARCHAR(255) DEFAULT NULL, " +
            "user_mobile VARCHAR(64) DEFAULT NULL, " +
            "user_type VARCHAR(32) DEFAULT NULL, " +
            "user_status VARCHAR(32) DEFAULT NULL, " +
            "v_token VARCHAR(40) DEFAULT NULL, " +
            "created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, " +
            "INDEX idx_created_at (created_at) " +
            ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

    case sg_pg_name:
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" +
            "user_name VARCHAR(128) NOT NULL PRIMARY KEY, " +
            "password VARCHAR(128) DEFAULT NULL, " +
            "user_id_type VARCHAR(32), " +
            "user_real_name VARCHAR(128) DEFAULT NULL, " +
            "user_email VARCHAR(255) DEFAULT NULL, " +
            "user_mobile VARCHAR(64) DEFAULT NULL, " +
            "user_type VARCHAR(32) DEFAULT NULL, " +
            "user_status VARCHAR(32) DEFAULT NULL, " +
            "v_token VARCHAR(40) DEFAULT NULL, " +
            "created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)"

    default:
        err := fmt.Errorf("database type not supported:%s (MID_CTB_117)", db_type)
        log.Printf("***** Alarm:%s", err.Error())
        return err
    }

    err := ExecuteStatement(db, stmt)
    if err != nil {
        error_msg := fmt.Errorf("failed creating table (MID_001_045), err: %w, stmt:%s", err, stmt)
        log.Printf("***** Alarm: %s", error_msg.Error())
        return error_msg
    }

    if db_type == sg_mysql_name {
        idx1 := `CREATE INDEX IF NOT EXISTS idx_created_at ON ` + table_name + ` (created_at);`
        ExecuteStatement(db, idx1)
    }

    log.Printf("Creating '%s' table success (MID_001_188)", table_name)

    return nil
}