package ApiUtils

import (
	"encoding/json"
	"fmt"
	"net/smtp"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// SendMail sends an email using SMTP.
// Example usage:
//   err := SendMail("user@example.com", "Verify your email", "<p>Click here...</p>")
func SendMail(to, subject, body string) error {
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

	fmt.Printf("Email sent successfully to %s (MID_AUT_036)\n", to)
	return nil
}

func GetRequestInfo(c echo.Context) ApiTypes.RequestInfo {
	// This function retrieves the following values from URL:
	req 		:= c.Request()
	full_url 	:= req.URL.String()
	path 		:= req.URL.Path
	scheme 		:= req.URL.Scheme
	host 		:= req.URL.Host

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
    	FullURL: 		full_url,
    	PATH:			path,
		Scheme:			scheme,
		Host:			host,
		OriginalScheme: original_scheme,
		OriginalHost:	original_host,
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
    err := json.Unmarshal([]byte(json_str), &jsonData)  // Deserialization: string -> Go object
    if err != nil {
        return nil, fmt.Errorf("deserialization error: %v", err)
    }
    return jsonData, nil
}

func ConvertToAny(str string) (interface{}, string, error) {
	// This function converts 'json_str' to an object of type 'any' (or interface{})
	// It returns the object, the type.
    var generic_obj interface{}
    err := json.Unmarshal([]byte(str), &generic_obj)  // Deserialization: string -> Go object
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