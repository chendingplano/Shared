package databaseutil

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

var AllowedOps = map[string]bool{
	"=":  true,
	"!=": true,
	">":  true,
	"<":  true,
	"LIKE": true,
	// add others as needed
}

var AllowedLogicOps = map[string]bool{
	"AND": true,
	"OR":  true,
}

func InitDB(mysql_config ApiTypes.DBConfig, pg_config ApiTypes.DBConfig) error {
    if pg_config.DBType != "pg" {
        error_msg := fmt.Errorf("invalid PG config name (SHD_DBS_056):%s", pg_config.DBType)
        log.Printf("***** Alarm %s", error_msg.Error())
        return error_msg
    }

    if pg_config.CreateFlag {
        err := CreatePGDBMiner(pg_config)
        if err != nil {
            log.Fatal("***** Alarm Failed creating PG connection (SHD_DBS_026)", err)
            return err
        }
    } else {
        log.Printf("PostgreSQL not configured (SHD_DBS_033)")
    }

    if mysql_config.DBType != "mysql" {
        error_msg := fmt.Errorf("invalid mysql config name (SHD_DBS_072):%s", mysql_config.DBType)
        log.Printf("***** Alarm %s", error_msg.Error())
        return error_msg
    }

    if mysql_config.CreateFlag {
        err := AosCreateMySqlDBMiner(mysql_config)
        if err != nil {
            log.Fatal("***** Alarm Failed creating MySQL connection (SHD_DBS_032)", err)
            return err    
        }
    } else {
        log.Printf("MySQL not configured (SHD_DBS_044)")
    }

    return nil
}

// Helper to validate table names (prevents SQL injection)
func IsValidTableName(name string) bool {
    // To prevent SQL injection, table names should be made of alphanumerics 
    // and underscores only;
    return regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(name)
}

func CreatePGDBMiner(config ApiTypes.DBConfig) error {
    var err error
    host := config.Host
    port := config.Port
    username := config.UserName
    password := config.Password
    dbname := config.DbName

    connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=disable dbname=%s",
        host, port, username, password, dbname)
    
    ApiTypes.PG_DB_miner, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal("Failed to connect to database (SHD_DBS_050):", err)
    }
    
    // Test the connection
    if err = ApiTypes.PG_DB_miner.Ping(); err != nil {
        log.Printf("***** Alarm: Failed conneting PostgreSQL (SHD_DBS_055), err:%s, conn:%s, user:%s, pwd:%s, dbname:%s",
            err, connStr, username, password, dbname)
    } else {
        log.Printf("PostgreSQL created (SHD_DBS_058), dbname:%s, user:%s", dbname, username)
    }

    ApiTypes.DatabaseInfo.PGDBHandle = ApiTypes.PG_DB_miner

    return nil
}

func AosCreateMySqlDBMiner(config ApiTypes.DBConfig) error {
    var err error
    host := config.Host
    port := config.Port
    username := config.UserName
    password := config.Password
    db_name := config.DbName
    options := "?tls=false&parseTime=true&loc=Local&timeout=30s&readTimeout=30s&writeTimeout=30s" 
    connStr := fmt.Sprintf("%s:%s@(%s:%d)/%s%s", username, password, host, port, db_name, options)
    
    log.Printf("To connect to MySQL with connStr (SHD_DBS_081)")
    ApiTypes.MySql_DB_miner, err = sql.Open("mysql", connStr)
    if err != nil {
        log.Fatal("***** Alarm Failed connecting MySQL (SHD_DBS_084):", err)
        return err
    }
    
    // Test the connection
    if err = ApiTypes.MySql_DB_miner.Ping(); err != nil {
        log.Printf("***** Alarm: Failed to ping MySQL (SHD_DBS_090), err:%s, connStr:%s:", err, connStr)
        return err
    }
    
    log.Println("Connected to MySQL database (SHD_DBS_174)")
    ApiTypes.DatabaseInfo.MySQLDBHandle = ApiTypes.MySql_DB_miner

    return nil
}

func HandleSelect(c echo.Context,
        base_stmt string,
        db *sql.DB,
        allowedFields map[string]bool,
        whereClauses []string,
        args []interface{},
        limit string) (*sql.Rows, error) {
    // This function handles dynamic WHERE clause construction.
    // The conditions are passed as query parameters:
    // field_0, op_0, val_0, logic_opr_0    
    // field_1, op_1, val_1, logic_opr_1
    // ...
    // IMPORTANT: This function assumes the query conditions are
    // passed through the query portion of the URL (from echo.Context)

	log.Printf("To retrieve data for Documents (SHD_DBS_024)")

    var args_str string
	i := 0
	for {
		log.Printf("Processing filter index: %d (SHD_DBS_178)", i)
  		field := c.QueryParam(fmt.Sprintf("field_%d", i))
  		if field == "" { 
			break 
		}

  		op := c.QueryParam(fmt.Sprintf("op_%d", i))
		logic_opr := "AND"
		if i > 0 {
        	logic_opr = c.QueryParam(fmt.Sprintf("logic_opr_%d", i))
            if logic_opr == "" || !AllowedLogicOps[logic_opr] { 
                error_msg := fmt.Errorf("invalid logic operator (SHD_DBS_177):%s", logic_opr)
                log.Printf("***** Alarm %s", error_msg.Error())
                return nil, error_msg
            }
    	}

		if !allowedFields[field] {
            error_msg := fmt.Errorf("invalid field (SHD_DBS_183):%s", field)
            return nil, error_msg
		}

		if i > 0 && !AllowedOps[op] {
            error_msg := fmt.Errorf("invalid operator (SHD_DBS_188):%s", op)
    		return nil, error_msg
		}

  		val := c.QueryParam(fmt.Sprintf("val_%d", i))
		whereClauses = append(whereClauses, fmt.Sprintf("%s %s ?", field, op))
    	args = append(args, val)
        args_str += fmt.Sprintf(", %s", val)
		log.Printf("Received filter - field: %s, op: %s, val: %s, logic_opr: %s (SHD_001_035)", field, op, val, logic_opr)
		if i == 0 {
			base_stmt += " WHERE "
		} else {
			base_stmt += " " + logic_opr + " "
		}
		base_stmt += fmt.Sprintf(" %s %s '%s' ", field, op, val)
  		i++
	}

    query := base_stmt
    if len(whereClauses) > 0 {
        query += " WHERE " + strings.Join(whereClauses, " ")
    }

    if limit != "" {
        query += " " + limit
    }

	log.Printf("Constructed query: %s (SHD_DBS_215)", query)
	rows, err := db.Query(query, args...)
	if err != nil {
        error_msg := fmt.Errorf("select failed (SHD_DBS_217), err:%v, query:%s, args:%s", err, query, args_str)
		log.Printf("***** Alarm %s", error_msg.Error())
		return nil, error_msg
	}
    return rows, nil
}


func AosExecuteStatement(db_type string, stmt string) error {
    switch db_type {
    case ApiTypes.MysqlName:
         return ExecuteStatement(ApiTypes.MySql_DB_miner, stmt)

    case ApiTypes.PgName:
         return ExecuteStatement(ApiTypes.PG_DB_miner, stmt)

    default:
         return fmt.Errorf("unsupported database type (SHD_DBS_153): %s", db_type)
    }
}

func ExecuteStatement(db *sql.DB, stmt string) error {
    tx, err := db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction (SHD_DBS_158): %w", err);
    }

    defer func() {
        _ = tx.Rollback(); // Rollback if not committed
    } ()

    _, err1 := tx.Exec(stmt)
    if err1 != nil {
        return fmt.Errorf("failed to execute query in transaction (SHD_DBS_166): %w", err1)
    }
    
    // Commit the transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction (SHD_DBS_171): %w", err)
    }
    
    // log.Println("Statement executed successfully (SHD_DBS_175)")
    return nil
}

func StrPtr(s string) *string {
    return &s
}

func UpdateVTokenByEmail(
            db_type string,
            table_name string,
            user_email string, 
            token string) error {
    var db *sql.DB
    var stmt string
    switch db_type {
    case ApiTypes.MysqlName:
         db = ApiTypes.MySql_DB_miner
         stmt = fmt.Sprintf("UPDATE %s SET v_token = ? WHERE user_email = ?", table_name)

    case ApiTypes.PgName:
         db = ApiTypes.PG_DB_miner
         stmt = fmt.Sprintf("UPDATE %s SET v_token = $1 WHERE user_email = $2", table_name)

    default:
         err := fmt.Errorf("unsupported database type (SHD_DBS_504): %s", db_type)
         log.Printf("***** Alarm:%s", err.Error())
         return err
    }

    _, err := db.Exec(stmt, token, user_email)
    if err != nil {
        error_msg := fmt.Errorf("failed to update table (SHD_DBS_511), stmt:%s, err: %w", stmt, err)
        log.Printf("***** Alarm:%s", error_msg.Error())
        return error_msg
    }
    log.Printf("Update token success (SHD_DBS_515), user_email:%s, token:%s", user_email, token)
    return nil
}

func CloseDatabase() {
    if ApiTypes.PG_DB_miner != nil {
        ApiTypes.PG_DB_miner.Close()
    }

    if ApiTypes.MySql_DB_miner != nil {
        ApiTypes.MySql_DB_miner.Close()
    }
}

func CreateGenericTable(table_name string) error {
    db_type := ApiTypes.DatabaseInfo.DBType
    const common_fields = 
        "record_type VARCHAR(255) NOT NULL, " +
        "doc_type VARCHAR(255) NOT NULL, " +
        "doc_name VARCHAR(255) NOT NULL, " +
        "doc_desc TEXT NOT NULL, " +
        "json_doc JSON NOT NULL, " +
        "record_data TEXT NOT NULL, " +
        "remarks TEXT DEFAULT NULL, " +
        "del_flag TINYINT(1) DEFAULT 0, " +
        "created_by VARCHAR(255) NOT NULL, " +
        "updated_by VARCHAR(255) NOT NULL, " +
        "created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, " +
        "updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, "

    var stmt string
    var db *sql.DB
    switch db_type {
    case ApiTypes.MysqlName:
         db = ApiTypes.MySql_DB_miner
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + 
            "doc_id BIGINT AUTO_INCREMENT PRIMARY KEY, " + common_fields +
            ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

    case ApiTypes.PgName:
         db = ApiTypes.PG_DB_miner
         stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + common_fields + ");"

    default:
        return fmt.Errorf("database type not supported:%s (SHD_DBS_315)", db_type)
    }

    _, err := db.Exec(stmt)
    if err != nil {
        return fmt.Errorf("failed to create table (SHD_DBS_320): %w", err)
    }
    log.Printf("Table created successfully (SHD_DBS_322), table_name:%s", table_name)
    return nil
}