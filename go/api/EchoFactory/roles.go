package EchoFactory

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

func hasRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}

func resolveUpdatedRoles(existingRoles []string, requestedRoles []string, admin bool) []string {
	var base []string
	if requestedRoles != nil {
		base = normalizeRoles(requestedRoles)
	} else {
		base = normalizeRoles(existingRoles)
	}

	if admin {
		if !hasRole(base, "admin") {
			base = append(base, "admin")
		}
		return normalizeRoles(base)
	}

	filtered := make([]string, 0, len(base))
	for _, role := range base {
		if role != "admin" {
			filtered = append(filtered, role)
		}
	}
	return normalizeRoles(filtered)
}

func rolesEqual(left []string, right []string) bool {
	left = normalizeRoles(left)
	right = normalizeRoles(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
