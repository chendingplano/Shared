package llm

import (
	"encoding/json"
	"regexp"
	"strings"
)

// RepairLLMJSON applies conservative cleanup to malformed LLM JSON output.
// It only returns true when the repaired candidate is syntactically valid JSON.
func RepairLLMJSON(raw string) (string, bool) {
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
	// Bare inclusive ranges such as [1598-1617] are not JSON; quote them.
	if quoted, ok := quoteBareSpanRanges(raw); ok {
		candidates = append(candidates, quoted)
	}
	// Span arrays with unbalanced quotes, e.g. [492,"493,"494,505], are not
	// JSON either; rebuild them from their numeric tokens.
	if rebuilt, ok := rebuildBrokenNumericArrays(raw); ok {
		candidates = append(candidates, rebuilt)
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

// numericArrayBody matches a flat array whose body holds only digits, commas,
// hyphens, whitespace and quote characters. Arrays containing any other
// character (text values, nesting) never match.
var numericArrayBody = regexp.MustCompile(`\[([0-9,"\-\s]*"[0-9,"\-\s]*)\]`)

var numericSpanToken = regexp.MustCompile(`^\d+(?:-\d+)?$`)

// rebuildBrokenNumericArrays repairs line-span arrays the model emitted with
// unbalanced quotes (`[492,"493,"494,"495,"496","497,505]`). Only arrays that
// are not already valid JSON are touched. Quotes are discarded, then each
// token is re-emitted: plain integers bare, `N-M` ranges as strings. If any
// token is not a line number or range, that array is left alone. Returns false
// when nothing changed.
func rebuildBrokenNumericArrays(s string) (string, bool) {
	changed := false
	out := numericArrayBody.ReplaceAllStringFunc(s, func(arr string) string {
		if json.Valid([]byte(arr)) {
			return arr
		}
		body := strings.ReplaceAll(arr[1:len(arr)-1], `"`, "")
		var parts []string
		for _, tok := range strings.Split(body, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			if !numericSpanToken.MatchString(tok) {
				return arr
			}
			if strings.Contains(tok, "-") {
				tok = `"` + tok + `"`
			}
			parts = append(parts, tok)
		}
		if len(parts) == 0 {
			return arr
		}
		changed = true
		return "[" + strings.Join(parts, ",") + "]"
	})
	return out, changed
}

// bareSpanRange matches an inclusive line range written as a bare token inside
// an array (e.g. `[1598-1617]` or `[143,1598-1617]`). The surrounding brackets
// or commas are captured so the match can be rewritten as a quoted string.
var bareSpanRange = regexp.MustCompile(`([\[,]\s*)(\d+)-(\d+)(\s*[\],])`)

// quoteBareSpanRanges rewrites bare N-M range tokens to "N-M" strings. Repeats
// until stable because adjacent ranges share a separator. Returns false when
// nothing changed.
func quoteBareSpanRanges(s string) (string, bool) {
	out := s
	for {
		next := bareSpanRange.ReplaceAllString(out, `$1"$2-$3"$4`)
		if next == out {
			break
		}
		out = next
	}
	return out, out != s
}
