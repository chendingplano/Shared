package auth

import (
	"regexp"
	"sort"
	"strings"
)

var roleNameRegex = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func normalizeRoles(roles []string) []string {
	seen := make(map[string]struct{}, len(roles))
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		normalized := strings.ToLower(strings.TrimSpace(role))
		if normalized == "" || !roleNameRegex.MatchString(normalized) {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeRolesFromValue(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return normalizeRoles(values)
	case []interface{}:
		roles := make([]string, 0, len(values))
		for _, value := range values {
			if role, ok := value.(string); ok {
				roles = append(roles, role)
			}
		}
		return normalizeRoles(roles)
	default:
		return nil
	}
}

func hasRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}

func ensureRole(roles []string, wanted string) []string {
	if hasRole(roles, wanted) {
		return roles
	}
	return normalizeRoles(append(append([]string{}, roles...), wanted))
}

func removeRole(roles []string, unwanted string) []string {
	filtered := make([]string, 0, len(roles))
	for _, role := range roles {
		if role != unwanted {
			filtered = append(filtered, role)
		}
	}
	return normalizeRoles(filtered)
}

func projectRolesAndAdmin(roles []string, legacyAdmin bool) ([]string, bool) {
	normalized := normalizeRoles(roles)
	if hasRole(normalized, "admin") || legacyAdmin {
		normalized = ensureRole(normalized, "admin")
		return normalized, true
	}
	return normalized, false
}
