package observability

import "testing"

func TestConfigFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv("OBSERVABILITY_ENABLED", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "")

	cfg := ConfigFromEnv("chenweb")

	if cfg.Enabled {
		t.Fatalf("expected observability to be disabled by default")
	}
	if cfg.ServiceName != "chenweb" {
		t.Fatalf("service name = %q, want chenweb", cfg.ServiceName)
	}
	if cfg.Environment != "local" {
		t.Fatalf("environment = %q, want local", cfg.Environment)
	}
}

func TestConfigFromEnvClickStackDefaults(t *testing.T) {
	t.Setenv("OBSERVABILITY_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	t.Setenv("APP_ENV", "development")

	cfg := ConfigFromEnv("chenweb")

	if !cfg.Enabled {
		t.Fatalf("expected observability to be enabled")
	}
	if cfg.OTLPEndpoint != "http://localhost:14318" {
		t.Fatalf("OTLP endpoint = %q, want local ClickStack endpoint", cfg.OTLPEndpoint)
	}
	if cfg.ServiceName != "chenweb" {
		t.Fatalf("service name = %q, want chenweb", cfg.ServiceName)
	}
	if cfg.Environment != "development" {
		t.Fatalf("environment = %q, want development", cfg.Environment)
	}
}

func TestConfigFromEnvParsesOTLPHeaders(t *testing.T) {
	t.Setenv("OBSERVABILITY_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=Bearer test-token,x-clickstack-source=chenweb")

	cfg := ConfigFromEnv("chenweb")

	if got := cfg.Headers["authorization"]; got != "Bearer test-token" {
		t.Fatalf("authorization header = %q, want Bearer test-token", got)
	}
	if got := cfg.Headers["x-clickstack-source"]; got != "chenweb" {
		t.Fatalf("x-clickstack-source header = %q, want chenweb", got)
	}
}
