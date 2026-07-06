package llm

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

// recordingHandler is a minimal slog.Handler that captures emitted records so
// tests can assert on warnings without depending on log output formatting.
type recordingHandler struct {
	records *[]slog.Record
}

func (h recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h recordingHandler) Handle(_ context.Context, r slog.Record) error {
	*h.records = append(*h.records, r)
	return nil
}
func (h recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h recordingHandler) WithGroup(_ string) slog.Handler      { return h }

func withCapturedWarnings(t *testing.T) *[]slog.Record {
	t.Helper()
	records := &[]slog.Record{}
	original := captureLogger
	captureLogger = slog.New(recordingHandler{records: records})
	t.Cleanup(func() { captureLogger = original })
	return records
}

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

func TestNewUsageCaptureRecordPreservesMetadata(t *testing.T) {
	record := NewUsageCaptureRecord(UsageCaptureInput{
		CallReason: "review-provision",
		CallLoc:    "MID-20260706-0001",
		Metadata:   map[string]any{"run_id": int64(123), "provision_id": "244-prv-2"},
	})

	if record.Metadata["run_id"] != int64(123) || record.Metadata["provision_id"] != "244-prv-2" {
		t.Fatalf("Metadata = %+v, want run_id/provision_id preserved", record.Metadata)
	}
}

func TestCaptureUsageRecordFallsBackToRequestCallFieldsAndMetadata(t *testing.T) {
	sink := &testUsageCaptureSink{}
	req := Request{
		CallReason: "review-provision",
		CallLoc:    "MID-20260706-0001",
		Metadata:   map[string]any{"run_id": int64(123)},
		Capture:    &RequestCapture{Sink: sink},
	}

	captureUsageRecord(context.Background(), req, UsageCaptureInput{ModelName: "deepseek-chat"})

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	got := records[0]
	if got.CallReason != "review-provision" || got.CallLoc != "MID-20260706-0001" {
		t.Fatalf("unexpected call fields: %+v", got)
	}
	if got.Metadata["run_id"] != int64(123) {
		t.Fatalf("Metadata = %+v, want run_id fallback from request", got.Metadata)
	}
}

func TestCaptureUsageRecordWarnsWhenCallLocOrCallReasonMissing(t *testing.T) {
	records := withCapturedWarnings(t)
	sink := &testUsageCaptureSink{}

	captureUsageRecord(context.Background(), Request{Capture: &RequestCapture{Sink: sink}}, UsageCaptureInput{
		ModelName: "deepseek-chat",
	})

	if len(*records) != 1 {
		t.Fatalf("warning count = %d, want 1", len(*records))
	}
	if (*records)[0].Level != slog.LevelWarn {
		t.Fatalf("level = %v, want Warn", (*records)[0].Level)
	}
}

func TestCaptureUsageRecordDoesNotWarnWhenCallLocAndCallReasonSet(t *testing.T) {
	records := withCapturedWarnings(t)
	sink := &testUsageCaptureSink{}

	captureUsageRecord(context.Background(), Request{Capture: &RequestCapture{Sink: sink}}, UsageCaptureInput{
		ModelName:  "deepseek-chat",
		CallReason: "review-provision",
		CallLoc:    "MID-20260706-0001",
	})

	if len(*records) != 0 {
		t.Fatalf("warning count = %d, want 0; got %+v", len(*records), *records)
	}
}
