package observability

import (
	"context"
	"testing"
)

func TestInitDisabledReturnsNoopShutdown(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{
		Enabled:     false,
		ServiceName: "chenweb",
		Environment: "test",
	})
	if err != nil {
		t.Fatalf("Init disabled returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatalf("Init disabled returned nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("disabled shutdown returned error: %v", err)
	}
}
