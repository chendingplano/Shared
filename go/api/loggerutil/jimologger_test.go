package loggerutil

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestCreateLoggerFromContextUsesRequestID(t *testing.T) {
	ctx := context.WithValue(context.Background(), ApiTypes.RequestIDKey, "req-fixed")

	logger := CreateLoggerFromContext(ctx, "SHD_JLG_TEST")
	impl, ok := logger.(*JimoLoggerImpl)
	if !ok {
		t.Fatalf("logger type = %T, want *JimoLoggerImpl", logger)
	}
	if impl.reqID != "req-fixed" {
		t.Fatalf("reqID = %q, want req-fixed", impl.reqID)
	}
}

func TestShouldUseJSONLoggerFromEnv(t *testing.T) {
	t.Setenv("JIMO_LOG_FORMAT", "json")

	if !shouldUseJSONLogger() {
		t.Fatalf("expected JIMO_LOG_FORMAT=json to enable JSON logger")
	}
}
