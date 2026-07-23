package llm

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type recordingLogger struct {
	mu       sync.Mutex
	warnings []logEntry
}

type logEntry struct {
	message string
	args    []any
}

func (l *recordingLogger) Debug(string, ...any) {}
func (l *recordingLogger) Line(string, ...any)  {}
func (l *recordingLogger) Info(string, ...any)  {}
func (l *recordingLogger) Trace(string)         {}
func (l *recordingLogger) Close()               {}

func (l *recordingLogger) Warn(message string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnings = append(l.warnings, logEntry{message: message, args: append([]any(nil), args...)})
}

func (l *recordingLogger) Error(string, ...any) {}

func (l *recordingLogger) Warnings() []logEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]logEntry, len(l.warnings))
	copy(out, l.warnings)
	return out
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

	captureUsageRecord(context.Background(), req, UsageCaptureInput{ModelName: "deepseek-chat"}, &recordingLogger{})

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

func TestPromptWarningLogFieldsIncludesPromptEnvContext(t *testing.T) {
	fields := promptWarningLogFields(UsageCaptureInput{
		Provider:   ProviderOpenAICompatible,
		ModelName:  "deepseek-v4-flash",
		CallReason: "review_metrics",
		CallLoc:    "MID-20260706-011",
		Metadata: map[string]any{
			"prompt_ref":         "prompt-review-metrics-v3.md",
			"prompt_dir_env_var": "PROMPT_DIR",
			"prompt_dir":         "/Users/cding/Workspace/ChenWeb/prompts",
		},
	})

	got := map[string]any{}
	for i := 0; i+1 < len(fields); i += 2 {
		got[fields[i].(string)] = fields[i+1]
	}
	if got["prompt_ref"] != "prompt-review-metrics-v3.md" {
		t.Fatalf("prompt_ref=%v", got["prompt_ref"])
	}
	if got["prompt_dir_env_var"] != "PROMPT_DIR" {
		t.Fatalf("prompt_dir_env_var=%v", got["prompt_dir_env_var"])
	}
	if got["prompt_dir"] != "/Users/cding/Workspace/ChenWeb/prompts" {
		t.Fatalf("prompt_dir=%v", got["prompt_dir"])
	}
}

func TestCaptureUsageRecordWarnsWhenCallLocOrCallReasonMissing(t *testing.T) {
	logger := &recordingLogger{}
	sink := &testUsageCaptureSink{}

	captureUsageRecord(context.Background(), Request{Capture: &RequestCapture{Sink: sink}}, UsageCaptureInput{
		ModelName:  "deepseek-chat",
		PromptName: "extract-products-v2",
	}, logger)

	warnings := logger.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("warning count = %d, want 1", len(warnings))
	}
}

func TestCaptureUsageRecordDoesNotWarnWhenCallLocAndCallReasonSet(t *testing.T) {
	logger := &recordingLogger{}
	sink := &testUsageCaptureSink{}

	captureUsageRecord(context.Background(), Request{Capture: &RequestCapture{Sink: sink}}, UsageCaptureInput{
		ModelName:  "deepseek-chat",
		PromptName: "review-provision",
		CallReason: "review-provision",
		CallLoc:    "MID-20260706-0001",
	}, logger)

	if got := logger.Warnings(); len(got) != 0 {
		t.Fatalf("warning count = %d, want 0; got %+v", len(got), got)
	}
}

type contextCheckingUsageCaptureSink struct {
	ctxErr error
}

func (s *contextCheckingUsageCaptureSink) Capture(ctx context.Context, _ UsageCaptureRecord) (string, error) {
	s.ctxErr = ctx.Err()
	if s.ctxErr != nil {
		return "", s.ctxErr
	}
	return "evt-test", nil
}

func TestCaptureUsageRecordDetachesSinkFromCanceledCallerContext(t *testing.T) {
	sink := &contextCheckingUsageCaptureSink{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	eventID := captureUsageRecord(ctx, Request{Capture: &RequestCapture{Sink: sink}}, UsageCaptureInput{
		ModelName:  "deepseek-chat",
		CallReason: "review-provision",
		CallLoc:    "MID-20260706-0001",
	}, &recordingLogger{})

	if eventID != "evt-test" {
		t.Fatalf("eventID = %q, want evt-test", eventID)
	}
	if errors.Is(sink.ctxErr, context.Canceled) {
		t.Fatalf("sink received canceled context")
	}
}

var _ ApiTypes.JimoLogger = (*recordingLogger)(nil)
