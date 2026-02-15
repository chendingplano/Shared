package ApiUtils

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"github.com/spf13/viper"
)

func GenerateSecureToken(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}

// Email type constants for identifying email templates
const (
	EmailTypeGeneric      = "generic"      // Default, wrapped in basic layout
	EmailTypeVerification = "verification" // Email verification with CTA button
)

// EmailSenderFunc is the signature for custom email sender functions.
// Apps can register their own email sender to use their preferred email service and styling.
// Parameters: to (recipient), subject, textBody, htmlBody, loc (caller location for logging)
type EmailSenderFunc func(
	rc ApiTypes.RequestContext,
	to string,
	subject string,
	textBody string,
	htmlBody string,
	emailType string) error

// customEmailSender holds the registered custom email sender function.
// If nil, the default SMTP sender is used.
var customEmailSender EmailSenderFunc

// SetEmailSender registers a custom email sender function.
// Call this during app initialization to use your own email service (e.g., Resend).
func SetEmailSender(sender EmailSenderFunc) {
	customEmailSender = sender
}

// SendMail sends an email using either the custom sender (if registered) or default SMTP.
// The emailType parameter identifies the template type (use EmailType* constants).
func SendMail(rc ApiTypes.RequestContext, to, subject, textBody, htmlBody string, emailType string) error {
	// Use custom sender if registered
	if customEmailSender != nil {
		return customEmailSender(rc, to, subject, textBody, htmlBody, emailType)
	}

	// Fall back to default SMTP sender
	return sendMailSMTP(rc, to, subject, textBody, htmlBody)
}

// sendMailSMTP is the default SMTP-based email sender using Gmail.
func sendMailSMTP(
	rc ApiTypes.RequestContext,
	to string,
	subject string,
	textBody string,
	htmlBody string) error {
	// ⚙️ SMTP server configuration from environment variables
	// SECURITY: All credentials MUST come from environment variables - no fallbacks
	from := os.Getenv("SMTP_FROM")
	logger := rc.GetLogger()
	if from == "" {
		logger.Error("Missing required SMTP_FROM environment variable")
		return fmt.Errorf("SMTP configuration error: SMTP_FROM not set")
	}

	password := os.Getenv("SMTP_PASSWORD")
	if password == "" {
		logger.Error("Missing required SMTP_PASSWORD environment variable")
		return fmt.Errorf("SMTP configuration error: SMTP_PASSWORD not set")
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

	logger.Info("Email sent successfully",
		"to", to,
		"subject", subject)
	return nil
}

func GetRequestInfo(c echo.Context) ApiTypes.RequestInfo {
	req := c.Request()
	fullURL := req.URL.String()
	path := req.URL.Path
	scheme := req.URL.Scheme
	host := req.URL.Host

	// Get original scheme (if proxy sets X-Forwarded-Proto)
	forwardedProto := req.Header.Get(echo.HeaderXForwardedProto)
	originalScheme := scheme
	if forwardedProto != "" {
		originalScheme = forwardedProto
	}

	// Get original host (if proxy sets X-Forwarded-Host)
	forwardedHost := req.Header.Get("X-Forwarded-Host")
	originalHost := host
	if forwardedHost != "" {
		originalHost = forwardedHost
	}

	return ApiTypes.RequestInfo{
		FullURL:        fullURL,
		PATH:           path,
		Scheme:         scheme,
		Host:           host,
		OriginalScheme: originalScheme,
		OriginalHost:   originalHost,
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

func ConvertToJSON(jsonStr string) (map[string]interface{}, error) {
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonData); err != nil {
		return nil, fmt.Errorf("deserialization error: %v", err)
	}
	return jsonData, nil
}

// ConvertToAny deserializes a JSON string into an interface{} and returns the object and its type.
func ConvertToAny(str string) (interface{}, string, error) {
	var genericObj interface{}
	if err := json.Unmarshal([]byte(str), &genericObj); err != nil {
		return nil, "", fmt.Errorf("deserialization error: %v", err)
	}

	switch genericObj.(type) {
	case map[string]interface{}:
		return genericObj, "map", nil
	case []interface{}:
		return genericObj, "array", nil
	case nil:
		return genericObj, "nil", nil
	case bool:
		return genericObj, "bool", nil
	case float64:
		return genericObj, "float64", nil
	case string:
		return genericObj, "string", nil
	default:
		return genericObj, "error", nil
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

func ComposeMsg(reqID string, msg string) string {
	return fmt.Sprintf("[req=%s] %s", reqID, msg)
}

func IsSecure() bool {
	// Adjust based on your deployment
	return os.Getenv("ENV") == "production"
}

func GetOAuthRedirectURL(
	rc ApiTypes.RequestContext,
	token string,
	userName string) string {
	// Redirect to backend (vite dev server)
	// This ensures the pb_auth cookie is set on the correct domain
	homeDomain := os.Getenv("APP_BASE_URL")
	logger := rc.GetLogger()
	if homeDomain == "" {
		logger.Error("missing APP_BASE_URL env var")
	}

	// Ensure homeDomain has a scheme — APP_BASE_URL should include it, but add legacy support
	if !strings.HasPrefix(homeDomain, "http://") && !strings.HasPrefix(homeDomain, "https://") {
		if strings.HasPrefix(homeDomain, "localhost") {
			homeDomain = "http://" + homeDomain
		} else {
			homeDomain = "https://" + homeDomain
		}
	}

	callbackURL := fmt.Sprintf("%s/oauth/callback", homeDomain)
	return fmt.Sprintf("%s?token=%s&name=%s",
		callbackURL,
		url.QueryEscape(token),
		url.QueryEscape(userName))
}

func AddCallFlow(ctx context.Context, currentFlow string) context.Context {
	parentFlow, _ := ctx.Value(ApiTypes.CallFlowKey).(string)
	newFlow := fmt.Sprintf("%s->%s", parentFlow, currentFlow)
	return context.WithValue(ctx, ApiTypes.CallFlowKey, newFlow)
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

func GetDefaultHomeURL() string {
	return fmt.Sprintf("%s/%s", os.Getenv("APP_BASE_URL"), os.Getenv("VITE_DEFAULT_NORM_ROUTE"))
}

// GeneratePassword creates a cryptographically secure random password
// with the specified length using letters, numbers, and special characters
func GeneratePassword(
	rc ApiTypes.RequestContext,
	length int) string {
	logger := rc.GetLogger()
	if length <= 0 {
		logger.Error("invalid length",
			"length", length,
			"action", "default to 12")
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
			logger.Error(
				"failed to generate random number",
				"error", err)
			// Create a new *big.Int from the fallback value
			randomIndex = big.NewInt(int64(i % charsetLength))
		}
		password[i] = charset[randomIndex.Int64()]
	}

	return string(password)
}

// GeneratePasswordCustom allows custom character sets and length
func GeneratePasswordCustom(
	rc ApiTypes.RequestContext,
	length int,
	charset string) string {
	logger := rc.GetLogger()
	if length <= 0 {
		logger.Error(
			"invalid length",
			"length", length,
			"action", "default to 12")
		length = 12
	}

	if len(charset) == 0 {
		logger.Error(
			"***** Alarm", "invalid charset", charset,
			"default to", "12", "loc", "SHD_UTL_458")
		length = 12
		return GeneratePassword(rc, length)
	}

	charsetLength := len(charset)
	password := make([]byte, length)

	for i := 0; i < length; i++ {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(charsetLength)))
		if err != nil {
			logger.Error(
				"***** Alarm", "failed to generate random number", err,
				"loc", "SHD_UTL_468")
			// Create a new *big.Int from the fallback value
			randomIndex = big.NewInt(int64(i % charsetLength))
		}
		password[i] = charset[randomIndex.Int64()]
	}

	return string(password)
}

func ParseTimestamp(s string) (time.Time, error) {
	// Code generated by LLM on 2026/01/14 by Chen Ding
	// IMPORTANT: It is highly discouraged to use this function unless
	// you have to. When reading data from databases, for instance,
	// you should use a variable with type time.Time and scan it
	// into this time.Time variable. This can avoid parsing the string.
	// This code is only a best effort. Different string formats may
	// break it!
	//
	// Note: this function is kind of specific to PostgreSQL. If you
	// are using other databases, please verify it before using it!
	if s == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	// Try common PostgreSQL formats
	layouts := []string{
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05.999999-07",
		"2006-01-02 15:04:05.999999 MST",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", s)
}

func GenerateUUID() string {
	return uuid.NewString()
}

// IsSafeReturnURL validates that a return URL is safe for redirection.
// It prevents open redirect attacks by ensuring the URL is a same-origin relative path.
// Returns true only if the URL:
// - Starts with "/" (relative path)
// - Does not start with "//" (protocol-relative URL)
// - Does not contain backslashes (browser interpretation varies)
// - Does not contain "javascript:" or "data:" schemes
// - Does not contain "://" (absolute URL)
//
// NOTE: For OAuth flows that require absolute URLs, use IsSafeAbsoluteReturnURL instead.
func IsSafeReturnURL(returnURL string) bool {
	if returnURL == "" {
		return false
	}

	// Must start with / (relative path)
	if !strings.HasPrefix(returnURL, "/") {
		return false
	}

	// Reject protocol-relative URLs (//evil.com)
	if strings.HasPrefix(returnURL, "//") {
		return false
	}

	// Reject backslash URLs (/\evil.com) - browsers may interpret backslashes as forward slashes
	if strings.Contains(returnURL, "\\") {
		return false
	}

	// Reject javascript:, data:, etc.
	lowerURL := strings.ToLower(returnURL)
	if strings.Contains(lowerURL, "javascript:") || strings.Contains(lowerURL, "data:") {
		return false
	}

	// Reject absolute URLs embedded in path
	if strings.Contains(returnURL, "://") {
		return false
	}

	// Parse to catch any edge cases
	parsed, err := url.Parse(returnURL)
	if err != nil {
		return false
	}

	// Ensure the parsed URL is still a relative path
	if parsed.Host != "" || parsed.Scheme != "" {
		return false
	}

	return true
}

// IsSafeAbsoluteReturnURL validates that an absolute return URL is safe for redirection.
// This is used for OAuth flows where absolute URLs are required.
// The URL must be an allowed origin (from ALLOWED_OAUTH_ORIGINS env var or defaults).
//
// Returns true only if the URL:
// - Is a valid URL
// - Uses http or https scheme
// - Host matches one of the allowed origins
// - Does not contain dangerous payloads (javascript:, data:, etc.)
func IsSafeAbsoluteReturnURL(returnURL string) bool {
	if returnURL == "" {
		return false
	}

	// Reject dangerous schemes before parsing
	lowerURL := strings.ToLower(returnURL)
	if strings.Contains(lowerURL, "javascript:") || strings.Contains(lowerURL, "data:") {
		return false
	}

	// Parse the URL
	parsed, err := url.Parse(returnURL)
	if err != nil {
		return false
	}

	// Must be http or https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	// Get allowed origins from environment or use defaults
	allowedOrigins := getOAuthAllowedOrigins()

	// Extract the origin (scheme + host) from the return URL
	returnOrigin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)

	// Check if the origin is allowed
	for _, allowed := range allowedOrigins {
		if strings.EqualFold(returnOrigin, allowed) {
			return true
		}
	}

	return false
}

// getOAuthAllowedOrigins returns the list of allowed origins for OAuth redirects.
// Reads from ALLOWED_OAUTH_ORIGINS env var (comma-separated) or uses defaults.
func getOAuthAllowedOrigins() []string {
	// Check for explicit configuration
	originsEnv := os.Getenv("ALLOWED_OAUTH_ORIGINS")
	if originsEnv != "" {
		origins := strings.Split(originsEnv, ",")
		var result []string
		for _, o := range origins {
			o = strings.TrimSpace(o)
			if o != "" {
				result = append(result, o)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	// Build default list from common environment variables
	var defaults []string

	if appDomain := os.Getenv("APP_BASE_URL"); appDomain != "" {
		defaults = append(defaults, appDomain)
	}

	// Add common localhost origins for development
	devOrigins := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://localhost:4455",
		"http://127.0.0.1:4455",
	}

	for _, dev := range devOrigins {
		found := false
		for _, d := range defaults {
			if d == dev {
				found = true
				break
			}
		}
		if !found {
			defaults = append(defaults, dev)
		}
	}

	return defaults
}

// GetSafeAbsoluteReturnURL returns the returnURL if it's safe for OAuth, otherwise returns the fallback.
func GetSafeAbsoluteReturnURL(returnURL string, fallback string) string {
	if IsSafeAbsoluteReturnURL(returnURL) {
		return returnURL
	}
	return fallback
}

// GetSafeReturnURL returns the returnURL if it's safe, otherwise returns the fallback.
// This is the backend equivalent of the frontend getSafeReturnUrl function.
func GetSafeReturnURL(returnURL string, fallback string) string {
	if IsSafeReturnURL(returnURL) {
		return returnURL
	}
	return fallback
}

// MaskToken masks a sensitive token for safe logging.
// Shows only the first 4 and last 4 characters with asterisks in between.
// For tokens shorter than 12 characters, shows only first 2 and last 2.
// SECURITY: Use this function when logging tokens, session IDs, or other secrets.
func MaskToken(token string) string {
	if token == "" {
		return "[empty]"
	}

	length := len(token)
	if length < 8 {
		return "****"
	}

	if length < 12 {
		return token[:2] + "****" + token[length-2:]
	}

	return token[:4] + "****" + token[length-4:]
}

var libConfigOnce sync.Once

func LoadLibConfig(loc string) {
	libConfigOnce.Do(func() {
		configPath := os.Getenv("SHARED_LIB_CONFIG_DIR")
		if configPath == "" {
			slog.Error("missing SHARED_LIB_CONFIG_DIR env variable (SHD_LMG_024)")
			return
		}

		// config_path should be "~/Workspace/Shared/libconfig.toml"
		// 1. DB Must be initialized properly

		slog.Info("Loading config (SHD_LMG_542)", "config_path", configPath)
		viper.SetConfigFile(configPath)
		viper.SetConfigType("toml")

		// Read config file
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				slog.Error("config file not found (SHD_LMG_054)", "config_path", configPath)
				os.Exit(1)
			}
			slog.Error("error reading config (SHD_LMG_056)", "error", err)
			os.Exit(1)
		}

		// Override with environment variables (e.g., DATABASE_URL)
		viper.AutomaticEnv()

		// Unmarshal into struct
		if err := viper.Unmarshal(&ApiTypes.LibConfig); err != nil {
			slog.Error("unable to decode config (SHD_LMG_064)", "error", err)
			os.Exit(1)
		}
		slog.Info("Loading config success (SHD_LMG_564)")
	})
}

func GetSafeString(mapObj map[string]interface{}, key string) (string, bool) {
	if mapObj == nil {
		return "", false
	}

	val, exists := mapObj[key]
	if !exists {
		return "", false
	}

	switch v := val.(type) {
	case string:
		return v, true
	case int:
		return strconv.Itoa(v), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	default:
		return "", false
	}
}

func GetSafeSubObj(mapObj map[string]interface{}, key string) (map[string]interface{}, bool) {
	if mapObj == nil {
		return nil, false
	}

	val, exists := mapObj[key]
	if !exists {
		return nil, false
	}

	switch v := val.(type) {
	case map[string]interface{}:
		return v, true

	case map[string]string:
		result := make(map[string]interface{})
		for k, c := range v {
			result[k] = c
		}
		return result, true

	default:
		return nil, false
	}
}

func MaskString(s string, prefixLen, suffixLen int, maskChar rune) string {
	if s == "" || prefixLen < 0 || suffixLen < 0 {
		return s
	}

	runes := []rune(s)
	totalLen := len(runes)

	// If string is too short to mask, return original
	if totalLen <= prefixLen+suffixLen {
		return s
	}

	prefix := string(runes[:prefixLen])
	suffix := string(runes[totalLen-suffixLen:])
	maskCount := totalLen - prefixLen - suffixLen

	// Build mask efficiently
	mask := strings.Repeat(string(maskChar), maskCount)

	return prefix + mask + suffix
}
