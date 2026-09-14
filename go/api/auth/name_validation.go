package auth

import "strings"

// NameValidationResult contains the result of first/last name validation.
type NameValidationResult struct {
	Valid  bool
	Errors []string
}

// ValidateNameFields checks that both first and last name are present and
// non-blank. Used to enforce mandatory name entry on new-account signup
// (email and phone) - see
// ChenWeb/openspec/changes/mandatory-signup-name/design.md.
func ValidateNameFields(first, last string) NameValidationResult {
	result := NameValidationResult{Valid: true, Errors: []string{}}
	if strings.TrimSpace(first) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "first name is required")
	}
	if strings.TrimSpace(last) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "last name is required")
	}
	return result
}
