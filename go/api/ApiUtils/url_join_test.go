package ApiUtils

import "testing"

func TestJoinBaseURLAndRoute(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		route   string
		want    string
	}{
		{
			name:    "route with leading slash",
			baseURL: "http://192.168.29.96:8080",
			route:   "/dashboard",
			want:    "http://192.168.29.96:8080/dashboard",
		},
		{
			name:    "route without leading slash",
			baseURL: "http://192.168.29.96:8080",
			route:   "dashboard",
			want:    "http://192.168.29.96:8080/dashboard",
		},
		{
			name:    "base url with trailing slash",
			baseURL: "http://192.168.29.96:8080/",
			route:   "/dashboard",
			want:    "http://192.168.29.96:8080/dashboard",
		},
		{
			name:    "empty route",
			baseURL: "http://192.168.29.96:8080/",
			route:   "",
			want:    "http://192.168.29.96:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JoinBaseURLAndRoute(tt.baseURL, tt.route); got != tt.want {
				t.Fatalf("JoinBaseURLAndRoute(%q, %q) = %q, want %q", tt.baseURL, tt.route, got, tt.want)
			}
		})
	}
}
