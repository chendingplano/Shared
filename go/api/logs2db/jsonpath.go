package logs2db

import (
	"fmt"
	"strings"
)

// extractJSONPath traverses a nested map using a dot-separated path
// and returns the value at that path.
//
// Examples:
//
//	extractJSONPath(data, "_meta.logLevelName") -> "DEBUG"
//	extractJSONPath(data, "1.lines") -> []any{...}
//	extractJSONPath(data, "2") -> "some message"
func extractJSONPath(data map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = data

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		val, exists := m[part]
		if !exists {
			return nil, false
		}
		current = val
	}

	return current, true
}

// extractString extracts a string value at the given JSON path.
// If the value is an array of strings, it joins them with newlines.
// Non-string values are converted via fmt.Sprintf.
func extractString(data map[string]any, path string) string {
	val, ok := extractJSONPath(data, path)
	if !ok {
		return ""
	}

	switch v := val.(type) {
	case string:
		return v
	case []any:
		// Join array elements as strings (e.g., "1.lines" is an array of strings)
		parts := make([]string, 0, len(v))
		for _, elem := range v {
			parts = append(parts, fmt.Sprintf("%v", elem))
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// extractInt extracts an integer value at the given JSON path.
// Returns 0 if the path doesn't exist or isn't numeric.
func extractInt(data map[string]any, path string) int {
	val, ok := extractJSONPath(data, path)
	if !ok {
		return 0
	}

	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

// applyMapping uses the config's JSONMapping to populate a LogEntry
// from the parsed JSON data.
func applyMapping(mapping map[string]string, data map[string]any, entry *LogEntry) {
	if path, ok := mapping["entry_type"]; ok {
		entry.EntryType = extractString(data, path)
	}
	if path, ok := mapping["message"]; ok {
		entry.Message = extractString(data, path)
	}
	if path, ok := mapping["sys_prompt"]; ok {
		entry.SysPrompt = extractString(data, path)
	}
	if path, ok := mapping["sys_prompt_nlines"]; ok {
		entry.SysPromptNLines = extractInt(data, path)
	}
	if path, ok := mapping["caller_filename"]; ok {
		entry.CallerFilename = extractString(data, path)
	}
	if path, ok := mapping["caller_line"]; ok {
		entry.CallerLine = extractInt(data, path)
	}
	if path, ok := mapping["created_at"]; ok {
		entry.CreatedAtRaw = extractString(data, path)
	}
}
