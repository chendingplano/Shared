package ApiUtils

import (
	"net/http"
	"testing"
)

func TestResolveRequestIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xRealIP    string
		wantIP     string
		wantSource string
	}{
		{
			name:       "uses first xff hop",
			remoteAddr: "10.0.0.2:8080",
			xff:        "192.168.29.1, 10.0.0.2",
			wantIP:     "192.168.29.1",
			wantSource: "x_forwarded_for",
		},
		{
			name:       "falls back to x real ip",
			remoteAddr: "10.0.0.2:8080",
			xRealIP:    "192.168.29.1",
			wantIP:     "192.168.29.1",
			wantSource: "x_real_ip",
		},
		{
			name:       "falls back to remote addr",
			remoteAddr: "10.0.0.2:8080",
			wantIP:     "10.0.0.2",
			wantSource: "remote_addr",
		},
		{
			name:       "supports bare remote ip",
			remoteAddr: "10.0.0.2",
			wantIP:     "10.0.0.2",
			wantSource: "remote_addr",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.xRealIP != "" {
				req.Header.Set("X-Real-IP", tc.xRealIP)
			}

			gotIP, gotSource := ResolveRequestIP(req)
			if gotIP != tc.wantIP || gotSource != tc.wantSource {
				t.Fatalf("ResolveRequestIP() = (%q, %q), want (%q, %q)", gotIP, gotSource, tc.wantIP, tc.wantSource)
			}
		})
	}
}
