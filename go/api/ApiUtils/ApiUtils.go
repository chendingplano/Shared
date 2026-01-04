package ApiUtils

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"net/url"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

func GenerateSecureToken(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}

// EmailSenderFunc is the signature for custom email sender functions.
// Apps can register their own email sender to use their preferred email service and styling.
// Parameters: reqID (for logging), to (recipient), subject, htmlBody, loc (caller location for logging)
type EmailSenderFunc func(reqID, to, subject, htmlBody, loc string) error

// customEmailSender holds the registered custom email sender function.
// If nil, the default SMTP sender is used.
var customEmailSender EmailSenderFunc

// SetEmailSender registers a custom email sender function.
// Call this during app initialization to use your own email service (e.g., Resend).
func SetEmailSender(sender EmailSenderFunc) {
	customEmailSender = sender
	log.Println("Custom email sender registered")
}

// SendMail sends an email using either the custom sender (if registered) or default SMTP.
// Example usage:
//
//	err := SendMail("user@example.com", "Verify your email", "<p>Click here...</p>")
func SendMail(reqID, to, subject, body string, loc string) error {
	// Use custom sender if registered
	if customEmailSender != nil {
		return customEmailSender(reqID, to, subject, body, loc)
	}

	// Fall back to default SMTP sender
	return sendMailSMTP(reqID, to, subject, body, loc)
}

// sendMailSMTP is the default SMTP-based email sender using Gmail.
func sendMailSMTP(reqID, to, subject, body string, loc string) error {
	// ⚙️ SMTP server configuration
	from := "chending1111@gmail.com"
	password := "fonn wwrr jthy ylph"
	smtpHost := "smtp.gmail.com"
	smtpPort := "587"

	// 📩 Message headers and body
	msg := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-version: 1.0;\r\n"+
			"Content-Type: text/html; charset=\"UTF-8\";\r\n\r\n"+
			"%s\r\n", from, to, subject, body))

	// 🔐 Authentication
	auth := smtp.PlainAuth("", from, password, smtpHost)

	// 🚀 Send email
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, []string{to}, msg)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	fmt.Printf("[req=%s] (SHD_AUT_036:%s) Email sent successfully to %s, subject:%s, content:%s\n",
		reqID, loc, to, subject, body)
	return nil
}

func GetRequestInfo(c echo.Context) ApiTypes.RequestInfo {
	// This function retrieves the following values from URL:
	req := c.Request()
	full_url := req.URL.String()
	path := req.URL.Path
	scheme := req.URL.Scheme
	host := req.URL.Host

	// Get original scheme (if proxy sets X-Forwarded-Proto)
	forwardedProto := req.Header.Get(echo.HeaderXForwardedProto)
	original_scheme := scheme
	if forwardedProto != "" {
		original_scheme = forwardedProto
	}

	// Get original host (if proxy sets X-Forwarded-Host)
	forwardedHost := req.Header.Get("X-Forwarded-Host")
	original_host := host
	if forwardedHost != "" {
		original_host = forwardedHost
	}

	return ApiTypes.RequestInfo{
		FullURL:        full_url,
		PATH:           path,
		Scheme:         scheme,
		Host:           host,
		OriginalScheme: original_scheme,
		OriginalHost:   original_host,
	}
}

func IsDuplicateKeyError(err error) bool {
	// PostgreSQL (lib/pq)
	if pqErr, ok := err.(*pq.Error); ok {
		return pqErr.Code == "23505"
	}
	// MySQL
	if mysqlErr, ok := err.(*mysql.MySQLError); ok {
		return mysqlErr.Number == 1062
	}
	// pgx via database/sql might need different check...
	return false
}

func ConvertToJSON(json_str string) (map[string]interface{}, error) {
	var jsonData map[string]interface{}
	err := json.Unmarshal([]byte(json_str), &jsonData) // Deserialization: string -> Go object
	if err != nil {
		return nil, fmt.Errorf("deserialization error: %v", err)
	}
	return jsonData, nil
}

func ConvertToAny(str string) (interface{}, string, error) {
	// This function converts 'json_str' to an object of type 'any' (or interface{})
	// It returns the object, the type.
	var generic_obj interface{}
	err := json.Unmarshal([]byte(str), &generic_obj) // Deserialization: string -> Go object
	if err != nil {
		return nil, "", fmt.Errorf("deserialization error: %v", err)
	}

	switch generic_obj.(type) {
	case map[string]interface{}:
		return generic_obj, "map", nil

	case []interface{}:
		return generic_obj, "array", nil

	case nil:
		return generic_obj, "nil", nil

	case bool:
		return generic_obj, "bool", nil

	case float64:
		return generic_obj, "float64", nil

	case string:
		return generic_obj, "string", nil

	default:
		return generic_obj, "error", nil
	}
}

func CheckFileExists(filename string) (bool, error) {
	info, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // File doesn't exist, but that's expected
		}
		return false, err // Some other error occurred
	}
	return !info.IsDir(), nil
}

func IsValidSessionPG(reqID string, session_id string) (ApiTypes.UserInfo, bool, error) {
	// This function checks whether 'session_id' is valid in the sessions table.
	// If valid, return user_name.
	var query string
	var db *sql.DB
	db_type := ApiTypes.DatabaseInfo.DBType
	table_name := ApiTypes.LibConfig.SystemTableNames.TableNameLoginSessions

	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		query = fmt.Sprintf("SELECT user_name FROM %s WHERE session_id = ? AND expires_at > NOW() LIMIT 1", table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("SELECT user_name FROM %s WHERE session_id = $1 AND expires_at > NOW() LIMIT 1", table_name)

	default:
		error_msg := fmt.Errorf("unsupported database type (SHD_DBS_234): %s", db_type)
		log.Printf("[req=%s] ***** Alarm %s:", reqID, error_msg.Error())
		return ApiTypes.UserInfo{}, false, error_msg
	}

	var user_name sql.NullString
	err := db.QueryRow(query, session_id).Scan(&user_name)
	if err != nil || !user_name.Valid {
		if err == sql.ErrNoRows {
			error_msg := fmt.Sprintf("user not found:%s (SHD_DBS_333)", user_name.String)
			log.Printf("[req=%s] %s", reqID, error_msg)
			return ApiTypes.UserInfo{}, false, nil

		}

		error_msg := fmt.Errorf("failed to validate session (SHD_DBS_240): %w", err)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)
		return ApiTypes.UserInfo{}, false, error_msg
	}

	log.Printf("[req=%s] Check session (SHD_DBS_271), stmt: %s, user_name:%s",
		reqID, query, user_name.String)

	const selected_fields = "user_id, user_name, user_id_type, firstName, lastName," +
		"email, user_mobile, user_address, verified, is_admin, " +
		"emailVisibility, user_type, user_status, avatar, locale"

	table_name = ApiTypes.LibConfig.SystemTableNames.TableNameUsers
	switch db_type {
	case ApiTypes.MysqlName:
		db = ApiTypes.MySql_DB_miner
		query = fmt.Sprintf("SELECT %s FROM %s WHERE user_name = ? LIMIT 1",
			selected_fields, table_name)

	case ApiTypes.PgName:
		db = ApiTypes.PG_DB_miner
		query = fmt.Sprintf("SELECT %s FROM %s WHERE user_name = $1 LIMIT 1",
			selected_fields, table_name)

	default:
		error_msg := fmt.Errorf("unsupported database type (SHD_DBS_234): %s", db_type)
		log.Printf("[req=%s] ***** Alarm %s:", reqID, error_msg.Error())
		return ApiTypes.UserInfo{}, false, error_msg
	}

	var user_info ApiTypes.UserInfo
	var user_mobile, user_address, user_id, user_id_type,
		first_name, last_name, avatar, locale,
		email, verified, admin, emailVisibility,
		user_type, user_status sql.NullString
	err = db.QueryRow(query, user_name).Scan(
		&user_id,
		&user_name,
		&user_id_type,
		&first_name,
		&last_name,
		&email,
		&user_mobile,
		&user_address,
		&verified,
		&admin,
		&emailVisibility,
		&user_type,
		&user_status,
		&avatar,
		&locale)

	if err != nil {
		if err == sql.ErrNoRows {
			error_msg := fmt.Sprintf("user not found:%s (SHD_DBS_243)", user_name.String)
			log.Printf("[req=%s] %s", reqID, error_msg)
			return ApiTypes.UserInfo{}, false, nil
		}

		error_msg := fmt.Errorf("failed to validate session (SHD_DBS_248): %w", err)
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)
		return ApiTypes.UserInfo{}, false, error_msg
	}

	if user_id.Valid {
		user_info.UserId = user_id.String
	}

	if user_name.Valid {
		user_info.UserName = user_name.String
	}

	if user_id_type.Valid {
		user_info.UserIdType = user_id_type.String
	}

	if first_name.Valid {
		user_info.FirstName = first_name.String
	}

	if last_name.Valid {
		user_info.LastName = last_name.String
	}

	if user_mobile.Valid {
		user_info.UserMobile = user_mobile.String
	}

	if user_address.Valid {
		user_info.UserAddress = user_address.String
	}

	if avatar.Valid {
		user_info.Avatar = avatar.String
	}

	if locale.Valid {
		user_info.Locale = locale.String
	}

	if email.Valid {
		user_info.Email = email.String
	}

	if verified.Valid {
		/*
			if b, err := strconv.ParseBool(verified.String); err == nil {
				user_info.Verified = b
			} else {
				// Handle invalid boolean string
				log.Printf("invalid boolean string %q in verified column", verified.String)
				user_info.Verified = false // or return error
			}
		*/
		user_info.Verified = verified.String == "true"
	}

	if admin.Valid {
		user_info.Admin = admin.String == "true"
	}

	if emailVisibility.Valid {
		user_info.EmailVisibility = emailVisibility.String == "true"
	}

	if user_type.Valid {
		user_info.AuthType = user_type.String
	}

	if user_status.Valid {
		user_info.UserStatus = user_status.String
	}

	return user_info, true, nil
}

func IsSecure() bool {
	// Adjust based on your deployment
	return os.Getenv("ENV") == "production"
}

func GetRedirectURL(
	reqID string,
	token string,
	user_name string) string {
	// Redirect to port 8090 (backend) instead of 5173 (vite dev server)
	// This ensures the pb_auth cookie is set on the correct domain
	home_domain := os.Getenv("APP_DOMAIN_NAME")
	if home_domain == "" {
		error_msg := "missing APP_DOMAIN_NAME env var, default to localhost:5173 (SHD_RCP_092)"
		home_domain = "http://localhost:5173"
		log.Printf("[req=%s] ***** Alarm:%s", reqID, error_msg)
	}

	// Ensure home_domain has a scheme (http:// or https://)
	// APP_DOMAIN_NAME should include the scheme, but add legacy support
	if !strings.HasPrefix(home_domain, "http://") && !strings.HasPrefix(home_domain, "https://") {
		if strings.HasPrefix(home_domain, "localhost") {
			home_domain = "http://" + home_domain
		} else {
			home_domain = "https://" + home_domain
		}
	}

	redirect_url := fmt.Sprintf("%s/oauth/callback", home_domain)
	redirectURL := fmt.Sprintf("%s?token=%s&name=%s",
		redirect_url,
		url.QueryEscape(token),
		url.QueryEscape(user_name))
	return redirectURL
}
