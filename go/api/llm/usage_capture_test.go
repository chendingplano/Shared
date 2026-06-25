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
	inputBody := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	outputBody := []byte(`{"content":"world"}`)

	record := NewUsageCaptureRecord(UsageCaptureInput{
		AccountID:             "acct_10",
		ProfileID:             "prof_20",
		Provider:              ProviderOpenAICompatible,
		ModelName:             "deepseek-v4-flash",
		PromptName:            "extract-products-v2",
		RequestStartedAt:      startedAt,
		RequestFinishedAt:     finishedAt,
		InputTokens:           123,
		OutputTokens:          45,
		PromptCacheHitTokens:  100,
		PromptCacheMissTokens: 23,
		InputBodyRef:          "archive/in.json.gz",
		OutputBodyRef:         "archive/out.json.gz",
		ErrorMessage:          "upstream timeout",
		ProviderRequestID:     "req_123",
		InputBody:             inputBody,
		OutputBody:            outputBody,
		RecordID:              77,
		CallReason:            "extract_products",
		CallLoc:               "MID-CWB-USAGE-CAPTURE",
	})

	if record.AccountID != "acct_10" {
		t.Fatalf("AccountID = %q, want acct_10", record.AccountID)
	}
	if record.ProfileID != "prof_20" {
		t.Fatalf("ProfileID = %q, want prof_20", record.ProfileID)
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
	if record.PromptCacheHitTokens != 100 || record.PromptCacheMissTokens != 23 {
		t.Fatalf("unexpected cache tokens: %+v", record)
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
	if record.RecordID != 77 || record.CallReason != "extract_products" || record.CallLoc != "MID-CWB-USAGE-CAPTURE" {
		t.Fatalf("unexpected capture metadata: %+v", record)
	}
	if string(record.InputBody) != string(inputBody) {
		t.Fatalf("InputBody = %q", string(record.InputBody))
	}
	if string(record.OutputBody) != string(outputBody) {
		t.Fatalf("OutputBody = %q", string(record.OutputBody))
	}
}
