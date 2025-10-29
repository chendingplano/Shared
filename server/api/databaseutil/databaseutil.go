package databaseutil

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chendingplano/Shared/server/api/datastructures"
	_ "github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
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

type DBConfig struct {
    Host        string
    Port        int
    DBType      string
    CreateFlag  bool
    UserName    string
    Password    string
    DbName      string
}

type DatabaseInfo struct {
    DBType              string
    PGDBName            string
    MySQLDBName         string
    PGDBHandle          *sql.DB
    MySQLDBHandle       *sql.DB
    SessionsTableName   string
    UsersTableName      string
}

var CurrentDatabaseInfo DatabaseInfo
var PG_DB_miner *sql.DB
var MySql_DB_miner *sql.DB
const (
    MysqlName = "mysql"  // ✅ exported
    PgName    = "pg"     // ✅ exported
)

func InitDB(mysql_config DBConfig, pg_config DBConfig) error {
    if pg_config.DBType != "pg" {
        error_msg := fmt.Errorf("invalid PG config name (MID_DBS_056):%s", pg_config.DBType)
        log.Printf("***** Alarm %s", error_msg.Error())
        return error_msg
    }

    if pg_config.CreateFlag {
        err := CreatePGDBMiner(pg_config)
        if err != nil {
            log.Fatal("***** Alarm Failed creating PG connection (MID_DBS_026)", err)
            return err
        }
    } else {
        log.Printf("PostgreSQL not configured (MID_DBS_033)")
    }

    if mysql_config.DBType != "mysql" {
        error_msg := fmt.Errorf("invalid mysql config name (MID_DBS_072):%s", mysql_config.DBType)
        log.Printf("***** Alarm %s", error_msg.Error())
        return error_msg
    }

    if mysql_config.CreateFlag {
        err := AosCreateMySqlDBMiner(mysql_config)
        if err != nil {
            log.Fatal("***** Alarm Failed creating MySQL connection (MID_DBS_032)", err)
            return err    
        }
    } else {
        log.Printf("MySQL not configured (MID_DBS_044)")
    }
    return nil
}

func CreatePGDBMiner(config DBConfig) error {
    var err error
    host := config.Host
    port := config.Port
    username := config.UserName
    password := config.Password
    dbname := config.DbName

    connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=disable dbname=%s",
        host, port, username, password, dbname)
    
    PG_DB_miner, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal("Failed to connect to database (MID_DBS_050):", err)
    }
    
    // Test the connection
    if err = PG_DB_miner.Ping(); err != nil {
        log.Printf("***** Alarm: Failed conneting PostgreSQL (MID_DBS_055), err:%s, conn:%s, user:%s, pwd:%s, dbname:%s",
            err, connStr, username, password, dbname)
    } else {
        log.Printf("PostgreSQL created (MID_DBS_058), dbname:%s, user:%s", dbname, username)
    }

    return nil
}

func AosCreateMySqlDBMiner(config DBConfig) error {
    var err error
    host := config.Host
    port := config.Port
    username := config.UserName
    password := config.Password
    db_name := config.DbName
    options := "?tls=false&parseTime=true&loc=Local&timeout=30s&readTimeout=30s&writeTimeout=30s" 
    connStr := fmt.Sprintf("%s:%s@(%s:%d)/%s%s", username, password, host, port, db_name, options)
    
    log.Printf("To connect to MySQL with connStr (MID_DBS_081)")
    MySql_DB_miner, err = sql.Open("mysql", connStr)
    if err != nil {
        log.Fatal("***** Alarm Failed connecting MySQL (MID_DBS_084):", err)
        return err
    }
    
    // Test the connection
    if err = MySql_DB_miner.Ping(); err != nil {
        log.Printf("***** Alarm: Failed to ping MySQL (MID_DBS_090), err:%s, connStr:%s:", err, connStr)
        return err
    }
    
    log.Println("Connected to MySQL database")

    return nil
}

func HandleSelect(c echo.Context,
        base_stmt string,
        db *sql.DB,
        allowedFields map[string]bool,
        limit string) (*sql.Rows, error) {
	// Query the database for dashboard data
	log.Printf("To retrieve data for Documents (MID_DBS_024)")

	var whereClauses []string
	var args []interface{}
    var args_str string
	i := 0
	for {
		log.Printf("Processing filter index: %d (MID_DBS_178)", i)
  		field := c.QueryParam(fmt.Sprintf("field_%d", i))
  		if field == "" { 
			break 
		}

  		op := c.QueryParam(fmt.Sprintf("op_%d", i))
		logic_opr := "AND"
		if i > 0 {
        	logic_opr = c.QueryParam(fmt.Sprintf("logic_opr_%d", i))
    	}

		if !allowedFields[field] {
            error_msg := fmt.Errorf("invalid field:%s", field)
            return nil, error_msg
		}

		if i > 0 && !AllowedOps[op] {
            error_msg := fmt.Errorf("invalid operator:%s", op)
    		return nil, error_msg
		}

  		val := c.QueryParam(fmt.Sprintf("val_%d", i))
		whereClauses = append(whereClauses, fmt.Sprintf("%s %s ?", field, op))
    	args = append(args, val)
        args_str += fmt.Sprintf(", %s", val)
		log.Printf("Received filter - field: %s, op: %s, val: %s, logic_opr: %s (MID_001_035)", field, op, val, logic_opr)
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

	log.Printf("Constructed query: %s (MID_DBS_214)", query)
	rows, err := db.Query(query, args...)
	if err != nil {
        error_msg := fmt.Errorf("select failed (MID_DBS_217), err:%v, query:%s, args:%s", err, query, args_str)
		log.Printf("***** Alarm %s", error_msg.Error())
		return nil, error_msg
	}
    return rows, nil
}


func AosExecuteStatement(db_type string, stmt string) error {
    switch db_type {
    case MysqlName:
         return ExecuteStatement(MySql_DB_miner, stmt)

    case PgName:
         return ExecuteStatement(PG_DB_miner, stmt)

    default:
         return fmt.Errorf("unsupported database type (MID_DBS_153): %s", db_type)
    }
}

func ExecuteStatement(db *sql.DB, stmt string) error {
    tx, err := db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction (MID_DBS_158): %w", err);
    }

    defer func() {
        _ = tx.Rollback(); // Rollback if not committed
    } ()

    _, err1 := tx.Exec(stmt)
    if err1 != nil {
        return fmt.Errorf("failed to execute query in transaction (MID_DBS_166): %w", err1)
    }
    
    // Commit the transaction
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction (MID_DBS_171): %w", err)
    }
    
    log.Println("Statement executed successfully (MID_DBS_175)")
    return nil
}

func SaveSession(
            db_type string,
            table_name string,
            login_method string,
            session_id string, 
            user_name string, 
            user_name_type string,
            user_reg_id string,
            expiry time.Time) error {
    var stmt string
    var db *sql.DB
    switch db_type {
    case MysqlName:
         stmt = fmt.Sprintf(`INSERT INTO %s (login_method, session_id, status,
                    user_name, user_name_type, user_reg_id, expires_at)
              VALUES (?, ?, ?, ?, ?, ?, ?)
              ON DUPLICATE KEY UPDATE 
              session_id = VALUES(session_id), 
              status = "active",
              expires_at = VALUES(expires_at)`, table_name)
         db = MySql_DB_miner
    
    case PgName:
         stmt = fmt.Sprintf(`INSERT INTO %s (login_method, session_id, status,
                    user_name, user_name_type, user_reg_id, expires_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7)
            ON CONFLICT (user_name)
            DO UPDATE SET
            session_id = EXCLUDED.session_id, 
            status = EXCLUDED.status,
            expires_at = EXCLUDED.expires_at`, table_name)
         db = PG_DB_miner
    
    default:
         return fmt.Errorf("unsupported database type (MID_DBS_234): %s", db_type)
    }

    _, err := db.Exec(stmt, login_method, session_id, "active",
                user_name, user_name_type, user_reg_id, expiry)
    if err != nil {
        values := fmt.Sprintf("login_method:%s, session_id:%s, user_name:%s, name_type:%s ,reg_id:%s, expires:%s",
            login_method, session_id, user_name, user_name_type, user_reg_id, expiry)
        log.Printf("Values:%s", values)
        error_msg := fmt.Sprintf("failed to save session (MID_DBS_208): %v, stmt:%s", err, stmt)
        log.Printf("***** Alarm %s", error_msg)
        return fmt.Errorf("***** Alarm:%s", error_msg)
    }
    return nil
}

func IsValidSession(
            db_type string,
            table_name string,
            session_id string) (string, bool, error) {
    // This function checks whether 'session_id' is valid in the sessions table.
    // If valid, return user_name.
    var query string
    var db *sql.DB
    log.Printf("Check IsValidSession (MID_DBS_251), db_type:%s", db_type)
    switch db_type {
    case MysqlName:
         db = MySql_DB_miner
         query = fmt.Sprintf("SELECT user_name FROM %s WHERE session_id = ? AND expires_at > NOW() LIMIT 1", table_name)

    case PgName:
         db = PG_DB_miner
         query = fmt.Sprintf("SELECT user_name FROM %s WHERE session_id = $1 AND expires_at > NOW() LIMIT 1", table_name)

    default:
         error_msg := fmt.Errorf("unsupported database type (MID_DBS_234): %s", db_type)
         log.Printf("***** Alarm %s:", error_msg.Error())
         return "", false, error_msg
    }

    var user_name string
    err := db.QueryRow(query, session_id).Scan(&user_name)
    if err != nil {
        error_msg := fmt.Errorf("failed to validate session (MID_DBS_240): %w", err)
        log.Printf("***** Alarm:%s", error_msg)
        return "", false, error_msg
    }
    log.Printf("Check session (MID_DBS_271), stmt: %s, user_name:%s", query, user_name)
    return user_name, user_name != "", nil
}

func UserExists(db_type string,
                table_name string,
                user_name string) bool {
    // This function checks whether 'user_name' is used in the users table.
    var query string
    var db *sql.DB
    switch db_type {
    case MysqlName:
         query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE user_name = ?", table_name)
         db = MySql_DB_miner

    case PgName:
         query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE user_name = $1", table_name)
         db = PG_DB_miner

    default:
         err := fmt.Errorf("unsupported database type (MID_DBS_153): %s", db_type)
         log.Printf("***** Alarm: %s", err.Error())
         return false
    }

    var count int
    err := db.QueryRow(query, user_name).Scan(&count)
    if err != nil {
        error_msg := fmt.Errorf("failed to validate session (MID_DBS_288): %w", err)
        log.Printf("***** Alarm:%s", error_msg)
        return false
    }
    log.Printf("Check user name (MID_DBS_292), stmt: %s, count:%d", query, count)
    return count > 0
}

func GetUserByEmail(
            db_type string,
            table_name string,
            user_email string) (datastructures.UserInfo, error) {
    // This function checks whether 'user_email' is used in the users table.
    var query string
    var db *sql.DB
    var user_info datastructures.UserInfo
    selected_fields := "user_name, password, user_id_type, user_real_name, user_email, user_mobile, " +
        "user_type, user_status, v_token"
    switch db_type {
    case MysqlName:
         query = fmt.Sprintf("SELECT %s FROM %s WHERE user_email = ? LIMIT 1", selected_fields, table_name)
         db = MySql_DB_miner

    case PgName:
         query = fmt.Sprintf("SELECT %s FROM %s WHERE user_email = $1 LIMIT 1", selected_fields, table_name)
         db = PG_DB_miner

    default:
         err := fmt.Errorf("unsupported database type (MID_DBS_326): %s", db_type)
         log.Printf("***** Alarm: %s", err.Error())
         return user_info, err
    }

    err := db.QueryRow(query, user_email).Scan(
            &user_info.UserName,
            &user_info.Password,
            &user_info.UserIdType,
            &user_info.RealName,
            &user_info.Email,
            &user_info.PhoneNumber,
            &user_info.UserType,
            &user_info.Status,
            &user_info.VToken)
    if err != nil {
        err := fmt.Errorf("failed to retrieve user info (MID_DBS_345): %w", err)
        log.Printf("***** Alarm:%s", err)
        return user_info, err
    }
    log.Printf("User info retrieved (MID_DBS_349), user: %s, status:%s", user_info.UserName, user_info.Status)
    return user_info, nil
}

func GetUserStatus(
            db_type string,
            table_name string,
            user_name string) string {
    // This function checks whether 'user_name' is used in the users table.
    var query string
    var db *sql.DB
    switch db_type {
    case MysqlName:
         query = fmt.Sprintf("SELECT user_status FROM %s WHERE user_name = ? LIMIT 1", table_name)
         db = MySql_DB_miner

    case PgName:
         query = fmt.Sprintf("SELECT user_status FROM %s WHERE user_name = $1 LIMIT 1", table_name)
         db = PG_DB_miner

    default:
         err := fmt.Errorf("unsupported database type (MID_DBS_326): %s", db_type)
         log.Printf("***** Alarm: %s", err.Error())
         return ""
    }

    var user_status string
    err := db.QueryRow(query, user_name).Scan(&user_status)
    if err != nil {
        error_msg := fmt.Errorf("failed to retrieve user status (MID_DBS_334): %w", err)
        log.Printf("***** Alarm:%s", error_msg)
        return ""
    }
    log.Printf("User status (MID_DBS_338), user: %s, status:%s", user_name, user_status)
    return user_status
}

func GetUserPasswordByEmail(
            db_type string,
            table_name string,
            user_email string) string {
    // This function checks whether 'user_name' is used in the users table.
    var query string
    var db *sql.DB
    switch db_type {
    case MysqlName:
         query = fmt.Sprintf("SELECT password FROM %s WHERE user_email = ? LIMIT 1", table_name)
         db = MySql_DB_miner

    case PgName:
         query = fmt.Sprintf("SELECT password FROM %s WHERE user_email = $1 LIMIT 1", table_name)
         db = PG_DB_miner

    default:
         err := fmt.Errorf("unsupported database type (MID_DBS_358): %s", db_type)
         log.Printf("***** Alarm: %s", err.Error())
         return ""
    }

    var user_status string
    err := db.QueryRow(query, user_email).Scan(&user_status)
    if err != nil {
        error_msg := fmt.Errorf("failed to retrieve user status (MID_DBS_366): %w", err)
        log.Printf("***** Alarm:%s", error_msg)
        return ""
    }
    log.Printf("User password retrieved (MID_DBS_370), email: %s", user_email)
    return user_status
}

func AddUser(db_type string,
            table_name string,
            user_info datastructures.UserInfo) (bool, error) {
    var db *sql.DB
    var stmt string
    switch db_type {
    case MysqlName:
         db = MySql_DB_miner
         stmt = fmt.Sprintf("INSERT INTO %s (user_name, password, user_id_type, user_real_name, user_email, user_mobile, user_type, user_status, v_token) " +
        				 "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", table_name)

    case PgName:
         db = PG_DB_miner
         stmt = fmt.Sprintf("INSERT INTO %s (user_name, password, user_id_type, user_real_name, user_email, user_mobile, user_type, user_status, v_token) " +
        				 "VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)", table_name)

    default:
         err := fmt.Errorf("unsupported database type (MID_DBS_313): %s", db_type)
         log.Printf("***** Alarm:%s", err.Error())
         return false, err
    }

    _, err := db.Exec(stmt, user_info.UserName,
                    user_info.Password,
                    user_info.UserIdType,
                    user_info.RealName,
                    user_info.Email,
                    user_info.PhoneNumber,
                    user_info.UserType,
                    user_info.Status,
                    user_info.VToken)

    if err != nil {
        error_msg := fmt.Sprintf("failed to save session (pg) (MID_DBS_213): %v, stmt:%s", err, stmt)
        log.Printf("***** Alarm %s", error_msg)
        return false, fmt.Errorf("***** Alarm:%s", error_msg)
    }

    return true, nil
}

func LookupUserByToken(
            db_type string,
            table_name string,
            token string) (datastructures.UserInfo, error) {
    var db *sql.DB
    var query string
    var user_info datastructures.UserInfo
    switch db_type {
    case MysqlName:
         db = MySql_DB_miner
         query = fmt.Sprintf("SELECT user_name, user_email FROM %s WHERE v_token = ? LIMIT 1", table_name)

    case PgName:
         db = PG_DB_miner
         query = fmt.Sprintf("SELECT user_name, user_email FROM %s WHERE v_token = $1 LIMIT 1", table_name)

    default:
         err := fmt.Errorf("unsupported database type (MID_DBS_474): %s", db_type)
         log.Printf("***** Alarm:%s", err.Error())
         return user_info, err
    }

    err := db.QueryRow(query, token).Scan(&user_info.UserName, &user_info.Email)
    if err != nil {
        error_msg := fmt.Errorf("failed to retrieve user by token (MID_DBS_482), token:%s, err: %w", token, err)
        log.Printf("***** Alarm:%s", error_msg.Error())
        return user_info, error_msg
    }
    log.Printf("Lookup User by Token success (MID_DBS_382), token:%s, user_name:%s, email:%s",
            token, user_info.UserName, user_info.Email)
    return user_info, nil
}

func UpdateVTokenByEmail(
            db_type string,
            table_name string,
            user_email string, 
            token string) error {
    var db *sql.DB
    var stmt string
    switch db_type {
    case MysqlName:
         db = MySql_DB_miner
         stmt = fmt.Sprintf("UPDATE %s SET v_token = ? WHERE user_email = ?", table_name)

    case PgName:
         db = PG_DB_miner
         stmt = fmt.Sprintf("UPDATE %s SET v_token = 12 WHERE user_email = $2", table_name)

    default:
         err := fmt.Errorf("unsupported database type (MID_DBS_504): %s", db_type)
         log.Printf("***** Alarm:%s", err.Error())
         return err
    }

    _, err := db.Exec(stmt, token, user_email)
    if err != nil {
        error_msg := fmt.Errorf("failed to update table (MID_DBS_511), stmt:%s, err: %w", stmt, err)
        log.Printf("***** Alarm:%s", error_msg.Error())
        return error_msg
    }
    log.Printf("Update token success (MID_DBS_515), user_email:%s, token:%s", user_email, token)
    return nil
}

func MarkUserVerified(
            db_type string,
            table_name string,
            user_name string) error {
    var db *sql.DB
    var stmt string
    switch db_type {
    case MysqlName:
         db = MySql_DB_miner
         stmt = fmt.Sprintf("UPDATE %s SET user_status = ? WHERE user_name = ?", table_name)

    case PgName:
         db = PG_DB_miner
         stmt = fmt.Sprintf("UPDATE %s SET user_status = $1 WHERE user_name = $2", table_name)

    default:
         err := fmt.Errorf("unsupported database type (MID_DBS_401): %s", db_type)
         log.Printf("***** Alarm:%s", err.Error())
         return err
    }

    _, err := db.Exec(stmt, "active", user_name)
    if err != nil {
        error_msg := fmt.Errorf("failed to update table (MID_DBS_404), stmt:%s, err: %w", stmt, err)
        log.Printf("***** Alarm:%s", error_msg.Error())
        return error_msg
    }
    log.Printf("Mark user verified success (MID_DBS_408), user_name:%s", user_name)
    return nil
}

func UpdatePasswordByUserName(
            db_type string,
            table_name string,
            user_name string, password string) error {
    var db *sql.DB
    var stmt string
    switch db_type {
    case MysqlName:
         db = MySql_DB_miner
         stmt = fmt.Sprintf("UPDATE %s SET password = ? WHERE user_name = ?", table_name)

    case PgName:
         db = PG_DB_miner
         stmt = fmt.Sprintf("UPDATE %s SET password = $1 WHERE user_name = $2", table_name)

    default:
         err := fmt.Errorf("unsupported database type (MID_DBS_565): %s", db_type)
         log.Printf("***** Alarm:%s", err.Error())
         return err
    }

    _, err := db.Exec(stmt, password, user_name)
    if err != nil {
        error_msg := fmt.Errorf("failed to update password (MID_DBS_572), stmt:%s, err: %w", stmt, err)
        log.Printf("***** Alarm:%s", error_msg.Error())
        return error_msg
    }
    log.Printf("Update password success (MID_DBS_576), user_name:%s", user_name)
    return nil
}

func IsValidDBType(db_type string) bool {
    return db_type == MysqlName || db_type == PgName
}