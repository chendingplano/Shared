package ApiUtils

import (
	"net"
	"net/http"
	"strings"
)

// ResolveRequestIP mirrors Echo's legacy RealIP fallback behavior and returns
// both the resolved IP and which request field supplied it.
func ResolveRequestIP(req *http.Request) (ip string, source string) {
	if req == nil {
		return "", ""
	}

	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexAny(xff, ","); idx > 0 {
			resolved := strings.TrimSpace(xff[:idx])
			resolved = strings.TrimPrefix(resolved, "[")
			resolved = strings.TrimSuffix(resolved, "]")
			return resolved, "x_forwarded_for"
		}
		return strings.TrimSpace(xff), "x_forwarded_for"
	}

	if realIP := req.Header.Get("X-Real-IP"); realIP != "" {
		realIP = strings.TrimSpace(realIP)
		realIP = strings.TrimPrefix(realIP, "[")
		realIP = strings.TrimSuffix(realIP, "]")
		return realIP, "x_real_ip"
	}

	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil {
		return host, "remote_addr"
	}
	if net.ParseIP(req.RemoteAddr) != nil {
		return req.RemoteAddr, "remote_addr"
	}
	return "", "remote_addr"
}
