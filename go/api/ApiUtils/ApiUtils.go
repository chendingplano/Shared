package ApiUtils

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/smtp"
	"net/url"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func GenerateSecureToken(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}

// Email type constants for identifying email templates
const (
	EmailTypeGeneric       = "generic"        // Default, wrapped in basic layout
	EmailTypeVerification  = "verification"   // Email verification with CTA button
	EmailTypePasswordReset = "password_reset" // Password reset with CTA button
)

// EmailSenderFunc is the signature for custom email sender functions.
// Apps can register their own email sender to use their preferred email service and styling.
// Parameters: reqID (for logging), to (recipient), subject, textBody, htmlBody, loc (caller location), emailType (template type)
type EmailSenderFunc func(reqID, to, subject, textBody, htmlBody, loc, emailType string) error

// customEmailSender holds the registered custom email sender function.
// If nil, the default SMTP sender is used.
var customEmailSender EmailSenderFunc

// SetEmailSender registers a custom email sender function.
// Call this during app initialization to use your own email service (e.g., Resend).
func SetEmailSender(sender EmailSenderFunc) {
	customEmailSender = sender
	logger.Info("Custom email sender registered")
}

// SendMail sends an email using either the custom sender (if registered) or default SMTP.
// The emailType parameter identifies the template type (use EmailType* constants).
// Example usage:
//
//	err := SendMail(reqID, "user@example.com", "Verify your email", "Plain text", "<p>HTML body</p>", "CALLER_LOC", EmailTypeVerification)
func SendMail(reqID, to, subject, textBody, htmlBody, loc, emailType string) error {
	// Use custom sender if registered
	if customEmailSender != nil {
		return customEmailSender(reqID, to, subject, textBody, htmlBody, loc, emailType)
	}

	// Fall back to default SMTP sender
	return sendMailSMTP(reqID, to, subject, textBody, htmlBody, loc)
}

// sendMailSMTP is the default SMTP-based email sender using Gmail.
func sendMailSMTP(reqID, to, subject, textBody, htmlBody string, loc string) error {
	// ⚙️ SMTP server configuration from environment variables
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		error_msg := "Missing SMTP_FROM environment variable"
		logger.Error("***** Alarm", "error", error_msg)
		from = "chending1111@gmail.com" // fallback
	}

	password := os.Getenv("SMTP_PASSWORD")
	if password == "" {
		password = "fonn wwrr jthy ylph" // fallback
	}

	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "smtp.gmail.com" // fallback
	}

	smtpPort := os.Getenv("SMTP_PORT")
	if smtpPort == "" {
		smtpPort = "587" // fallback
	}

	// Generate MIME boundary
	boundary := "boundary-" + GenerateSecureToken(16)

	// 📩 Build multipart message with both text and HTML versions
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
	msg.WriteString("\r\n")

	// Plain text part
	if textBody != "" {
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(textBody)
		msg.WriteString("\r\n\r\n")
	}

	// HTML part
	if htmlBody != "" {
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(htmlBody)
		msg.WriteString("\r\n\r\n")
	}

	// Closing boundary
	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	// 🔐 Authentication
	auth := smtp.PlainAuth("", from, password, smtpHost)

	// 🚀 Send email
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, []string{to}, []byte(msg.String()))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	fmt.Printf("[req=%s] (SHD_AUT_036:%s) Email sent successfully to %s, subject:%s\n",
		reqID, loc, to, subject)
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
		logger.Error("***** Alarm", "req", reqID, "error", error_msg.Error())
		return ApiTypes.UserInfo{}, false, error_msg
	}

	var user_name sql.NullString
	err := db.QueryRow(query, session_id).Scan(&user_name)
	if err != nil || !user_name.Valid {
		if err == sql.ErrNoRows {
			error_msg := fmt.Sprintf("user not found:%s (SHD_DBS_333)", user_name.String)
			logger.Warn("+++++ WARN", "req", reqID, "error", error_msg)
			return ApiTypes.UserInfo{}, false, nil

		}

		error_msg := fmt.Errorf("failed to validate session (SHD_DBS_240): %w", err)
		logger.Error("***** Alarm", "req", reqID, "error", error_msg)
		return ApiTypes.UserInfo{}, false, error_msg
	}

	logger.Info("Check session (SHD_DBS_271)", "req", reqID, "stmt", query, "user_name", user_name.String)

	const selected_fields = "user_id, user_name, user_id_type, first_name, last_name," +
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
		logger.Error("***** Alarm", "req", reqID, "error", error_msg.Error())
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
			logger.Warn("+++++ WARN", "req", reqID, "error", error_msg)
			return ApiTypes.UserInfo{}, false, nil
		}

		error_msg := fmt.Errorf("failed to validate session (SHD_DBS_248): %w", err)
		logger.Error("***** Alarm", "req", reqID, "error", error_msg)
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

func GetOAuthRedirectURL(
	reqID string,
	token string,
	user_name string) string {
	// Redirect to port 8090 (backend) 5173 (vite dev server)
	// This ensures the pb_auth cookie is set on the correct domain
	home_domain := os.Getenv("APP_DOMAIN_NAME")
	if home_domain == "" {
		error_msg := fmt.Sprintf("missing APP_DOMAIN_NAME env var, set to:%s", home_domain)
		logger.Error("***** Alarm", "req", reqID, "error", error_msg)
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

func AddCallFlow(ctx context.Context, current_flow string) context.Context {
	parent_flow := ctx.Value(ApiTypes.CallFlowKey).(string)
	new_flow := fmt.Sprintf("%s->%s", parent_flow, current_flow)
	return context.WithValue(ctx, ApiTypes.CallFlowKey, new_flow)
}

// Helper to generate a short, random request ID
func GenerateRequestID(key string) string {
	bytes := make([]byte, 4) // 4 bytes = 8 hex chars
	if _, err := rand.Read(bytes); err != nil {
		// Fallback if crypto/rand fails (very rare)
		return "fallback-req-id"
	}
	return key + "-" + hex.EncodeToString(bytes)
}

func GetDefahotHomeURL() string {
	var url = fmt.Sprintf("%s%s", os.Getenv("APP_DOMAIN_NAME"), os.Getenv("APP_DEFAULT_ENDPOINT"))
	return url
}

// GeneratePassword creates a cryptographically secure random password
// with the specified length using letters, numbers, and special characters
func GeneratePassword(length int) string {
	if length <= 0 {
		logger.Error("***** Alarm", "invalid length", length,
			"default to", "12", "loc", " (SHD_UTL_419)")
		length = 12
	}

	// Character set with all printable ASCII characters (excluding ambiguous ones)
	charset := "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"0123456789" +
		"@#$%&-=+"

	// Alternative charset without ambiguous characters (0, O, l, 1, etc.)
	// charset := "abcdefghijkmnpqrstuvwxyz" +
	//            "ABCDEFGHJKMNPQRSTUVWXYZ" +
	//            "23456789" +
	//            "!@#$%^&*()-_=+[]{}|;:,.<>?"

	charsetLength := len(charset)
	password := make([]byte, length)

	for i := 0; i < length; i++ {
		// Generate a random index within the charset range
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(charsetLength)))
		if err != nil {
			logger.Error("***** Alarm", "failed to generate random number", err)
			// Create a new *big.Int from the fallback value
			randomIndex = big.NewInt(int64(i % charsetLength))
		}
		password[i] = charset[randomIndex.Int64()]
	}

	return string(password)
}

// GeneratePasswordCustom allows custom character sets and length
func GeneratePasswordCustom(length int, charset string) string {
	if length <= 0 {
		logger.Error("***** Alarm", "invalid length", length,
			"default to", "12", "loc", "SHD_UTL_453")
		length = 12
	}

	if len(charset) == 0 {
		logger.Error("***** Alarm", "invalid charset", length,
			"default to", "12", "loc", "SHD_UTL_458")
		length = 12
		return GeneratePassword(length)
	}

	charsetLength := len(charset)
	password := make([]byte, length)

	for i := 0; i < length; i++ {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(charsetLength)))
		if err != nil {
			logger.Error("***** Alarm", "failed to generate random number", err,
				"loc", "SHD_UTL_468")
			// Create a new *big.Int from the fallback value
			randomIndex = big.NewInt(int64(i % charsetLength))
		}
		password[i] = charset[randomIndex.Int64()]
	}

	return string(password)
}
