// Package auth provides password validation for secure authentication.
// This file implements password strength requirements following OWASP guidelines.
package auth

import (
	"strings"
	"unicode"
)

// PasswordRequirements defines the minimum requirements for a secure password
type PasswordRequirements struct {
	MinLength        int  // Minimum password length
	RequireUppercase bool // Require at least one uppercase letter
	RequireLowercase bool // Require at least one lowercase letter
	RequireDigit     bool // Require at least one digit
	RequireSpecial   bool // Require at least one special character
}

// DefaultPasswordRequirements returns secure defaults based on OWASP guidelines
func DefaultPasswordRequirements() PasswordRequirements {
	return PasswordRequirements{
		MinLength:        12,   // OWASP recommends 12+ characters
		RequireUppercase: true, // Mixed case increases entropy
		RequireLowercase: true,
		RequireDigit:     true,  // Numbers increase entropy
		RequireSpecial:   false, // Optional - length matters more than complexity
	}
}

// PasswordValidationResult contains the result of password validation
type PasswordValidationResult struct {
	Valid    bool     // Whether the password meets all requirements
	Errors   []string // List of requirement failures
	Strength string   // Password strength rating: "weak", "fair", "strong"
}

// ValidatePassword checks if a password meets security requirements
func ValidatePassword(password string, reqs PasswordRequirements) PasswordValidationResult {
	result := PasswordValidationResult{
		Valid:  true,
		Errors: []string{},
	}

	// Check minimum length
	if len(password) < reqs.MinLength {
		result.Valid = false
		result.Errors = append(result.Errors,
			"Password must be at least "+string(rune('0'+reqs.MinLength/10))+string(rune('0'+reqs.MinLength%10))+" characters")
	}

	// Check for uppercase if required
	if reqs.RequireUppercase && !hasUppercase(password) {
		result.Valid = false
		result.Errors = append(result.Errors, "Password must contain at least one uppercase letter")
	}

	// Check for lowercase if required
	if reqs.RequireLowercase && !hasLowercase(password) {
		result.Valid = false
		result.Errors = append(result.Errors, "Password must contain at least one lowercase letter")
	}

	// Check for digit if required
	if reqs.RequireDigit && !hasDigit(password) {
		result.Valid = false
		result.Errors = append(result.Errors, "Password must contain at least one number")
	}

	// Check for special character if required
	if reqs.RequireSpecial && !hasSpecialChar(password) {
		result.Valid = false
		result.Errors = append(result.Errors, "Password must contain at least one special character")
	}

	// Check for common weak patterns
	if isCommonPassword(password) {
		result.Valid = false
		result.Errors = append(result.Errors, "Password is too common or easily guessable")
	}

	// Calculate strength
	result.Strength = calculateStrength(password)

	return result
}

// ValidatePasswordDefault validates with default requirements
func ValidatePasswordDefault(password string) PasswordValidationResult {
	return ValidatePassword(password, DefaultPasswordRequirements())
}

// hasUppercase checks if password contains at least one uppercase letter
func hasUppercase(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

// hasLowercase checks if password contains at least one lowercase letter
func hasLowercase(s string) bool {
	for _, r := range s {
		if unicode.IsLower(r) {
			return true
		}
	}
	return false
}

// hasDigit checks if password contains at least one digit
func hasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// hasSpecialChar checks if password contains at least one special character
func hasSpecialChar(s string) bool {
	specialChars := "!@#$%^&*()_+-=[]{}|;:',.<>?/~`\"\\"
	for _, r := range s {
		if strings.ContainsRune(specialChars, r) {
			return true
		}
	}
	return false
}

// isCommonPassword checks against a list of common passwords
func isCommonPassword(password string) bool {
	// Lowercase for comparison
	lower := strings.ToLower(password)

	// Common passwords list (subset - in production, use a larger list)
	commonPasswords := []string{
		"password", "123456", "12345678", "qwerty", "abc123",
		"monkey", "1234567", "letmein", "trustno1", "dragon",
		"baseball", "iloveyou", "master", "sunshine", "ashley",
		"bailey", "shadow", "123123", "654321", "superman",
		"qazwsx", "michael", "football", "password1", "password123",
		"welcome", "welcome1", "admin", "login", "passw0rd",
		"password!", "p@ssword", "p@ssw0rd", "changeme", "test123",
	}

	for _, common := range commonPasswords {
		if lower == common {
			return true
		}
	}

	// Check for simple patterns
	if isSequentialPattern(password) {
		return true
	}

	if isRepeatingPattern(password) {
		return true
	}

	return false
}

// isSequentialPattern checks for sequential characters like "abcd" or "1234"
func isSequentialPattern(s string) bool {
	if len(s) < 4 {
		return false
	}

	// Check for ascending or descending sequence
	ascending := 0
	descending := 0

	for i := 1; i < len(s); i++ {
		if s[i] == s[i-1]+1 {
			ascending++
		} else {
			ascending = 0
		}

		if s[i] == s[i-1]-1 {
			descending++
		} else {
			descending = 0
		}

		// 3+ consecutive sequential chars is a pattern
		if ascending >= 3 || descending >= 3 {
			return true
		}
	}

	return false
}

// isRepeatingPattern checks for repeating characters like "aaaa" or "abab"
func isRepeatingPattern(s string) bool {
	if len(s) < 4 {
		return false
	}

	// Check for single char repetition (aaaa)
	sameCount := 1
	for i := 1; i < len(s); i++ {
		if s[i] == s[i-1] {
			sameCount++
			if sameCount >= 4 {
				return true
			}
		} else {
			sameCount = 1
		}
	}

	// Check for two-char pattern repetition (abab)
	if len(s) >= 4 {
		for patLen := 1; patLen <= 2; patLen++ {
			pattern := s[:patLen]
			matches := 0
			for i := 0; i+patLen <= len(s); i += patLen {
				if s[i:i+patLen] == pattern {
					matches++
				}
			}
			if matches >= 4 {
				return true
			}
		}
	}

	return false
}

// calculateStrength estimates password strength
func calculateStrength(password string) string {
	score := 0

	// Length scoring
	if len(password) >= 8 {
		score++
	}
	if len(password) >= 12 {
		score++
	}
	if len(password) >= 16 {
		score++
	}

	// Character class scoring
	if hasUppercase(password) {
		score++
	}
	if hasLowercase(password) {
		score++
	}
	if hasDigit(password) {
		score++
	}
	if hasSpecialChar(password) {
		score++
	}

	// Penalize common patterns
	if isCommonPassword(password) {
		score -= 3
	}

	switch {
	case score <= 2:
		return "weak"
	case score <= 4:
		return "fair"
	default:
		return "strong"
	}
}
