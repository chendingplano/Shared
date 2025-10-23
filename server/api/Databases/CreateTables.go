package Databases

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	_ "github.com/lib/pq"
)


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


func AosCreateProcessStatusTable(db *sql.DB, db_type string) error {
    stmt := "CREATE TABLE IF NOT EXISTS process_status (" +
            "id SERIAL PRIMARY KEY, " +
            "status_name VARCHAR(32) NOT NULL, " +
            "status_value VARCHAR(32) NOT NULL, " +
            "rcd_count INTEGER DEFAULT 0, " +
            "tags VARCHAR(255) NOT NULL, " +
            "created_at DATETIME DEFAULT CURRENT_TIMESTAMP)"

    log.Printf("Create process_status table with stmt (MID_001_043):%s", stmt)
    err := ExecuteStatement(db, stmt)
    if err != nil {
        log.Printf("***** Alarm: Failed creating process table (MID_001_048), err: %s, stmt:%s", err, stmt)
        return fmt.Errorf("***** Alarm: Failed creating table (MID_001_048), err: %w, stmt:%s", err, stmt)
    }

    log.Printf("Creating 'process_status' table success (MID_001_048)")

    return nil
}


func AosCreateLoginSessionsTable(db *sql.DB, db_type string) error {
    // Assuming the syntax for mysql and pg is the same
    stmt := "CREATE TABLE IF NOT EXISTS " + config.AosGetLoginSessionsTableName() + "(" +
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

    log.Printf("Create login sessions table with stmt (MID_001_072):%s", stmt)
    err := ExecuteStatement(db, stmt)
    if err != nil {
        log.Printf("***** Alarm: Failed creating table (MID_001_075), err: %s, stmt:%s", err, stmt)
        return fmt.Errorf("***** Alarm: Failed creating sessions table (MID_001_045), err: %w, stmt:%s", err, stmt)
    }

    log.Printf("Creating 'process_status' table success (MID_001_048)")

    return nil
}


func AosCreateUsersTable(db *sql.DB, db_type string) error {
    // Assuming the syntax for mysql and pg is the same
    stmt := "CREATE TABLE IF NOT EXISTS " + config.AosGetUsersTableName() + "(" +
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

    log.Printf("Create users table (MID_001_100):%s", stmt)
    err := ExecuteStatement(db, stmt)
    if err != nil {
        error_msg := fmt.Errorf("failed creating table (MID_001_045), err: %w, stmt:%s", err, stmt)
        log.Printf("***** Alarm: %s", error_msg.Error())
        return error_msg
    }

    log.Printf("Creating '%s' table success (MID_001_048)", config.AosGetUsersTableName())

    return nil
}



