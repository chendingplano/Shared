package llm

import (
	"encoding/json"
	"strings"
)

// repairLLMJSON applies conservative cleanup to malformed LLM JSON output.
// It only returns true when the repaired candidate is syntactically valid JSON.
func repairLLMJSON(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	candidates := []string{raw}
	if cleaned := cleanMarkdownJSONFence(raw); cleaned != raw {
		candidates = append(candidates, cleaned)
	}
	if firstObj, ok := extractFirstJSONObjectFromArray(raw); ok {
		candidates = append(candidates, firstObj)
	}
	if extracted, ok := extractJSONObject(raw); ok {
		candidates = append(candidates, extracted)
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if json.Valid([]byte(candidate)) {
			return candidate, true
		}
		if repaired, ok := repairUnescapedInnerQuotes(candidate); ok && json.Valid([]byte(repaired)) {
			return repaired, true
		}
	}

	return "", false
}

func repairUnescapedInnerQuotes(s string) (string, bool) {
	if strings.TrimSpace(s) == "" {
		return "", false
	}

	var b strings.Builder
	b.Grow(len(s) + 8)
	inString := false
	escaped := false
	changed := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				b.WriteByte(c)
				escaped = true
				continue
			case '"':
				if looksLikeStringTerminator(s, i+1) {
					b.WriteByte(c)
					inString = false
					continue
				}
				b.WriteByte('\\')
				b.WriteByte('"')
				changed = true
				continue
			}
			b.WriteByte(c)
			continue
		}

		if c == '"' {
			inString = true
		}
		b.WriteByte(c)
	}

	if inString {
		return "", false
	}
	if !changed {
		return "", false
	}
	return b.String(), true
}

func looksLikeStringTerminator(s string, idx int) bool {
	for idx < len(s) {
		switch s[idx] {
		case ' ', '\n', '\r', '\t':
			idx++
			continue
		case ',', '}', ']', ':':
			return true
		default:
			return false
		}
	}
	return true
}

