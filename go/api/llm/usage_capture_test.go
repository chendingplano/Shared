package llm

import (
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndReadGzipFileRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "archive", "body.json.gz")
	body := []byte(`{"prompt":"hello","response":"world"}`)

	if err := WriteGzipFile(path, body); err != nil {
		t.Fatalf("WriteGzipFile() error = %v", err)
	}

	got, err := ReadGzipFile(path)
	if err != nil {
		t.Fatalf("ReadGzipFile() error = %v", err)
	}

	if string(got) != string(body) {
		t.Fatalf("ReadGzipFile() = %q, want %q", string(got), string(body))
	}
}

func TestNewUsageCaptureRecordPreservesPromptTokensRefsAndErrors(t *testing.T) {
	startedAt := time.Date(2026, time.June, 19, 14, 5, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Second)

	record := NewUsageCaptureRecord(UsageCaptureInput{
		AccountID:         10,
		ProfileID:         20,
		Provider:          ProviderOpenAICompatible,
		ModelName:         "deepseek-v4-flash",
		PromptName:        "extract-products-v2",
		RequestStartedAt:  startedAt,
		RequestFinishedAt: finishedAt,
		InputTokens:       123,
		OutputTokens:      45,
		InputBodyRef:      "archive/in.json.gz",
		OutputBodyRef:     "archive/out.json.gz",
		ErrorMessage:      "upstream timeout",
		ProviderRequestID: "req_123",
	})

	if record.AccountID != 10 {
		t.Fatalf("AccountID = %d, want 10", record.AccountID)
	}
	if record.ProfileID != 20 {
		t.Fatalf("ProfileID = %d, want 20", record.ProfileID)
	}
	if record.Provider != ProviderOpenAICompatible {
		t.Fatalf("Provider = %q, want %q", record.Provider, ProviderOpenAICompatible)
	}
	if record.ModelName != "deepseek-v4-flash" {
		t.Fatalf("ModelName = %q", record.ModelName)
	}
	if record.PromptName != "extract-products-v2" {
		t.Fatalf("PromptName = %q", record.PromptName)
	}
	if record.RequestStartedAt != startedAt || record.RequestFinishedAt != finishedAt {
		t.Fatalf("unexpected request times: %#v", record)
	}
	if record.InputTokens != 123 || record.OutputTokens != 45 || record.TotalTokens != 168 {
		t.Fatalf("unexpected tokens: %+v", record)
	}
	if record.InputBodyRef != "archive/in.json.gz" || record.OutputBodyRef != "archive/out.json.gz" {
		t.Fatalf("unexpected refs: %+v", record)
	}
	if record.ErrorMessage != "upstream timeout" {
		t.Fatalf("ErrorMessage = %q", record.ErrorMessage)
	}
	if record.ProviderRequestID != "req_123" {
		t.Fatalf("ProviderRequestID = %q", record.ProviderRequestID)
	}
}
