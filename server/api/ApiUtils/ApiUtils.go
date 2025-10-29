package ApiUtils

import (
	"fmt"
	"net/smtp"
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

	fmt.Printf("Email sent successfully to %s\n", to)
	return nil
}
