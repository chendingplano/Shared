package sysdatastores

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

var emailstore_selected_field_names = "email_id, full_email, email_source, email_status, domain_name, " +
	"user_name, caller_loc, created_at, updated_at"

var emailstore_insert_field_names = "email_id, full_email, email_source, email_status, domain_name, " +
	"user_name, caller_loc, created_at, updated_at"

const (
	EmailStoreTableDesc = `Table 'EmailStore' stores email metadata. It includes the following fields:
	    Field:full_email, mandatory, primary key, email full name, such as john@acme.com.
	    Field:email_id, mandatory, a positive non-zero sequence number, serve as a unique key to identify emails, assigned by caller.
	    Field:EmailSource, mandatory, specify the source from which the email is obtained.  Possible values include: email-signup, signup-by-google, signup-by-github, sign-up-by-cellphone, crawled, loaded-from-file, manually-entered.
	    Field: status, mandatory, specify the record's status.  Possible values include: active, deleted, suspended.
	    Field: domain_name, mandatory, is the domain part of an email, such as acme.com from john@acme.com.
	    Field: user_name, mandatory, is the user name part of an email, such as john from john@acme.com.
	    Field: creator_loc, mandatory, identify the caller that added the record.  Its format is AAA_BBB_DDD, where AAA and BBB are three-letter string (capitalized) and DDD is a three-digit string
	    Field: updater_loc, mandatory, identify the caller that last updated the record.  Its format is AAA_BBB_DDD, where AAA and BBB are three-letter string (capitalized) and DDD is a three-digit string
	    Field: created_at, optional, the creation time. If not specified, it defaults to the system current time.
	    Field: updated_at, optional, the last update time. If not specified, it defaults to the system current time.`

	Prompt_CreateEmailStoreRecords = `Create a Go function to create N records for table 'EmailStore'. Below is the table description:
    {EmailStoreTableDesc}.

    Notes on record generation:
    - Generate a few percent invalid email
    - Generate a few percent empty email
    - Generate a few very long email (length > 100) 
    `
)

type EmailInfo struct {
	EmailID     int64   `json:"email_id"`
	FullEmail   string  `json:"full_email"`
	EmailSource string  `json:"email_source"`
	Status      string  `json:"email_status"`
	DomainName  string  `json:"domain_name"`
	UserName    string  `json:"user_name"`
	CreatorLoc  string  `json:"creator_loc"`
	UpdaterLoc  string  `json:"updater_loc"`
	CreatedAt   *string `json:"created_at"`
	UpdatedAt   *string `json:"updated_at"`
}

func CreateEmailStoreTable(
	db *sql.DB,
	db_type string,
	table_name string) error {
	var stmt string
	fields := fmt.Sprintf("email_id       BIGINT          NOT NULL, " +
		"full_email     VARCHAR(256)    NOT NULL PRIMARY KEY," +
		"email_source   VARCHAR(32)     NOT NULL, " +
		"email_status   VARCHAR(32)     NOT NULL, " +
		"domain_name    VARCHAR(128)    NOT NULL, " +
		"user_name      VARCHAR(128)    NOT NULL, " +
		"creator_loc    VARCHAR(32)     NOT NULL, " +
		"updater_loc    VARCHAR(32)     NOT NULL, " +
		"updated_at     TIMESTAMP       DEFAULT CURRENT_TIMESTAMP," +
		"created_at     TIMESTAMP       DEFAULT CURRENT_TIMESTAMP")

	switch db_type {
	case ApiTypes.MysqlName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields +
			", INDEX idx_created_at (created_at) " +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;"

	case ApiTypes.PgName:
		stmt = "CREATE TABLE IF NOT EXISTS " + table_name + "(" + fields + ")"

	default:
		err := fmt.Errorf("database type not supported:%s (SHD_EST_117)", db_type)
		log.Printf("***** Alarm:%s", err.Error())
		return err
	}

	err := databaseutil.ExecuteStatement(db, stmt)
	if err != nil {
		error_msg := fmt.Errorf("failed creating table (SHD_EST_045), err: %w, stmt:%s", err, stmt)
		log.Printf("***** Alarm: %s", error_msg.Error())
		return error_msg
	}

	if db_type == ApiTypes.PgName {
		idx1 := `CREATE INDEX IF NOT EXISTS idx_created_at ON ` + table_name + ` (created_at);`
		databaseutil.ExecuteStatement(db, idx1)
	}

	log.Printf("Create table '%s' success (SHD_EST_188)", table_name)

	return nil
}

func GetEmailStoreTableDesc() string {
	return EmailStoreTableDesc
}

func CheckEmailExists(rc ApiTypes.RequestContext, email string) bool {
	// This function checks whether 'user_name' is used in the users table.
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore
	var query string
	var db *sql.DB
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE full_email = ?", table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE full_email = $1", table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_EST_153): %s", db_type)
		log.Printf("***** Alarm: %s", err.Error())
		return false
	}

	var count int
	err := db.QueryRow(query, email).Scan(&count)
	if err != nil {
		error_msg := fmt.Errorf("failed to validate session (SHD_EST_288): %w", err)
		log.Printf("***** Alarm:%s", error_msg)
		return false
	}
	log.Printf("Check user name (SHD_EST_292), stmt: %s, count:%d", query, count)
	return count > 0
}

func GetEmailInfoByEmail(rc ApiTypes.RequestContext, email string) (EmailInfo, error) {
	// This function checks whether 'user_email' is used in the users table.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore
	var email_info EmailInfo
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE user_email = ? LIMIT 1", emailstore_selected_field_names, table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT %s FROM %s WHERE user_email = $1 LIMIT 1", emailstore_selected_field_names, table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err := fmt.Errorf("unsupported database type (SHD_EST_326): %s", db_type)
		log.Printf("***** Alarm: %s", err.Error())
		return email_info, err
	}

	err := db.QueryRow(query, email).Scan(
		&email_info.EmailID,
		&email_info.FullEmail,
		&email_info.EmailSource,
		&email_info.Status,
		&email_info.DomainName,
		&email_info.UserName,
		&email_info.CreatorLoc,
		&email_info.UpdaterLoc,
		&email_info.CreatedAt,
		&email_info.UpdatedAt)

	if err != nil {
		err := fmt.Errorf("failed to retrieve email (SHD_EST_345): %w", err)
		log.Printf("***** Alarm:%s", err)
		return email_info, err
	}
	log.Printf("Email info retrieved (SHD_EST_349), user: %s, status:%s", email_info.FullEmail, email_info.Status)
	return email_info, nil
}

func GetEmailStatus(rc ApiTypes.RequestContext, email string) string {
	// This function checks whether 'user_name' is used in the users table.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore
	switch db_type {
	case ApiTypes.MysqlName:
		query = fmt.Sprintf("SELECT user_status FROM %s WHERE full_email = ? LIMIT 1", table_name)
		db = ApiTypes.MySql_DB_miner

	case ApiTypes.PgName:
		query = fmt.Sprintf("SELECT user_status FROM %s WHERE full_email = $1 LIMIT 1", table_name)
		db = ApiTypes.PG_DB_miner

	default:
		err_msg := fmt.Sprintf("error: unsupported database type (SHD_EST_326): %s", db_type)
		log.Printf("***** Alarm: %s", err_msg)
		return err_msg
	}

	var email_status string
	err := db.QueryRow(query, email).Scan(&email_status)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("Email not found (SHD_EST_443): %s", email)
			return "email not found" // or handle as "not found"
		}

		error_msg := fmt.Sprintf("error: failed to retrieve user status (SHD_EST_334): %v", err)
		log.Printf("***** Alarm:%s", error_msg)
		return error_msg
	}
	log.Printf("Email status (SHD_EST_338), db_type:%s, email:%s, status:%s", db_type, email, email_status)
	return email_status
}

func AddEmail(rc ApiTypes.RequestContext, email_info EmailInfo) (bool, error) {
	// Currently, the inserted fields are:
	//  user_id, user_name, password, user_id_type, first_name,
	//  last_name, user_email, user_mobile, user_address, user_type,
	//  user_status, picture, locale, v_token
	var db *sql.DB
	var stmt string
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameEmailStore
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			table_name, emailstore_insert_field_names)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
			table_name, emailstore_insert_field_names)

	default:
		err := fmt.Errorf("unsupported database type (SHD_EST_313): %s", db_type)
		log.Printf("***** Alarm:%s", err.Error())
		return false, err
	}

	_, err := db.Exec(stmt,
		email_info.EmailID,
		email_info.FullEmail,
		email_info.EmailSource,
		email_info.Status,
		email_info.DomainName,
		email_info.UserName,
		email_info.CreatorLoc,
		email_info.UpdaterLoc,
		email_info.CreatedAt,
		email_info.UpdatedAt)

	if err != nil {
		if ApiUtils.IsDuplicateKeyError(err) {
			log.Printf("Email already exists (SHD_EST_649), email:%s", email_info.FullEmail)
			return true, nil
		}

		error_msg := fmt.Sprintf("failed to add email (SHD_EST_213): %v, stmt:%s", err, stmt)
		log.Printf("***** Alarm %s", error_msg)
		return false, fmt.Errorf("***** Alarm:%s", error_msg)
	}

	return true, nil
}
