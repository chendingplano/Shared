package observability

import (
	"os"
	"strings"
)

const defaultClickStackOTLPEndpoint = "http://localhost:14318"

type Config struct {
	Enabled      bool
	ServiceName  string
	Environment  string
	OTLPEndpoint string
	Headers      map[string]string
}

func ConfigFromEnv(defaultServiceName string) Config {
	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = strings.TrimSpace(defaultServiceName)
	}

	environment := strings.TrimSpace(os.Getenv("APP_ENV"))
	if environment == "" {
		environment = strings.TrimSpace(os.Getenv("ENV"))
	}
	if environment == "" {
		environment = "local"
	}

	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	enabled := readBoolEnv("OBSERVABILITY_ENABLED") || readBoolEnv("OTEL_ENABLED")
	if endpoint == "" && enabled {
		endpoint = defaultClickStackOTLPEndpoint
	}

	return Config{
		Enabled:      enabled,
		ServiceName:  serviceName,
		Environment:  environment,
		OTLPEndpoint: endpoint,
		Headers:      parseHeaders(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")),
	}
}

func readBoolEnv(name string) bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseHeaders(raw string) map[string]string {
	headers := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		headers[key] = value
	}
	return headers
}
