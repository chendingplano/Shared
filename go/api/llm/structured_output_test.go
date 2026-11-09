package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractStructuredJSON_Success(t *testing.T) {
	srv := newStructuredOutputTestServer(t, []string{
		`{"topics":[{"topic_desc":"alarm requirements","lines":["1-2"]}]}`,
	})
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL(),
		APIKey:     "test-key",
		ModelName:  "gpt-test",
		HTTPClient: srv.Client(),
	}

	result, err := client.ExtractStructuredJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	}, testTopicContract())
	if err != nil {
		t.Fatalf("ExtractStructuredJSON() error = %v", err)
	}
	if result == nil {
		t.Fatalf("ExtractStructuredJSON() returned nil result")
	}
	topics, ok := result.Parsed["topics"].([]any)
	if !ok || len(topics) != 1 {
		t.Fatalf("topics = %#v", result.Parsed["topics"])
	}
}

func TestExtractStructuredJSON_RetryAfterParseFailure(t *testing.T) {
	srv := newStructuredOutputTestServer(t, []string{
		`{"topics":[{"topic_desc":`,
		`{"topics":[{"topic_desc":"alarm requirements","lines":["1-2"]}]}`,
	})
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL(),
		APIKey:     "test-key",
		ModelName:  "gpt-test",
		HTTPClient: srv.Client(),
	}

	result, err := client.ExtractStructuredJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	}, StructuredOutputContract{
		Name:       testTopicContract().Name,
		Schema:     testTopicContract().Schema,
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("ExtractStructuredJSON() error = %v", err)
	}
	if result == nil {
		t.Fatalf("ExtractStructuredJSON() returned nil result")
	}
	if srv.requests != 2 {
		t.Fatalf("requests = %d, want 2", srv.requests)
	}
}

func TestExtractStructuredJSON_RetryAfterSchemaValidationFailure(t *testing.T) {
	srv := newStructuredOutputTestServer(t, []string{
		`{"topics":[{"lines":["1-2"]}]}`,
		`{"topics":[{"topic_desc":"alarm requirements","lines":["1-2"]}]}`,
	})
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL(),
		APIKey:     "test-key",
		ModelName:  "gpt-test",
		HTTPClient: srv.Client(),
	}

	result, err := client.ExtractStructuredJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	}, StructuredOutputContract{
		Name:       testTopicContract().Name,
		Schema:     testTopicContract().Schema,
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("ExtractStructuredJSON() error = %v", err)
	}
	if result == nil {
		t.Fatalf("ExtractStructuredJSON() returned nil result")
	}
	if srv.requests != 2 {
		t.Fatalf("requests = %d, want 2", srv.requests)
	}
}

func TestExtractStructuredJSON_FailsAfterRetriesExhausted(t *testing.T) {
	srv := newStructuredOutputTestServer(t, []string{
		`{"topics":[{"lines":["1-2"]}]}`,
		`{"topics":[{"lines":["1-2"]}]}`,
	})
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL(),
		APIKey:     "test-key",
		ModelName:  "gpt-test",
		HTTPClient: srv.Client(),
	}

	_, err := client.ExtractStructuredJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	}, StructuredOutputContract{
		Name:       testTopicContract().Name,
		Schema:     testTopicContract().Schema,
		MaxRetries: 1,
	})
	if !strings.Contains(fmt.Sprint(err), ErrStructuredOutputRetriesExhausted.Error()) {
		t.Fatalf("error = %v, want retries exhausted", err)
	}
}

func TestExtractStructuredJSON_RepairSucceedsWithoutRetry(t *testing.T) {
	srv := newStructuredOutputTestServer(t, []string{
		`Here is the JSON:
{"topics":[{"topic_desc":"alarm requirements","lines":["1-2"]}]}`,
	})
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL(),
		APIKey:     "test-key",
		ModelName:  "gpt-test",
		HTTPClient: srv.Client(),
	}

	result, err := client.ExtractStructuredJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	}, StructuredOutputContract{
		Name:        testTopicContract().Name,
		Schema:      testTopicContract().Schema,
		AllowRepair: true,
	})
	if err != nil {
		t.Fatalf("ExtractStructuredJSON() error = %v", err)
	}
	if result == nil {
		t.Fatalf("ExtractStructuredJSON() returned nil result")
	}
	if srv.requests != 1 {
		t.Fatalf("requests = %d, want 1", srv.requests)
	}
}

func TestExtractStructuredJSON_LogsEveryFailedAttemptWithResponseAndContext(t *testing.T) {
	responses := []string{`{"topics":`, `{"topics":[{"lines":["1-2"]}]}`}
	srv := newStructuredOutputTestServer(t, responses)
	defer srv.Close()
	logger := &recordingLogger{}
	client := &OpenAIJSONClient{
		BaseURL: srv.URL(), APIKey: "secret-must-not-be-logged", ModelName: "default-model",
		Provider: ProviderOpenAICompatible, ProfileName: "primary-profile", HTTPClient: srv.Client(), logger: logger,
	}
	contract := testTopicContract()
	contract.MaxRetries = 1
	_, err := client.ExtractStructuredJSON(context.Background(), JSONExtractionInput{
		PromptName: "prompt-provisions-v8.md", PromptText: "prompt", InputText: "document",
		ModelName: "actual-model", RecordID: 224, RunID: 172, CallReason: "extract_provisions", CallLoc: "chunk",
		Metadata: map[string]any{"chunk_index": 4, "source_line_start": 10, "source_line_end": 20, "api_key": "metadata-secret"},
	}, contract)
	if err == nil {
		t.Fatal("expected exhausted validation retries")
	}
	var failures []logEntry
	for _, entry := range logger.Warnings() {
		if entry.message == "structured output attempt failed" {
			failures = append(failures, entry)
		}
	}
	if len(failures) != 2 {
		t.Fatalf("failed-attempt diagnostics = %d, want 2: %#v", len(failures), logger.Warnings())
	}
	for i, entry := range failures {
		fields := make(map[string]any)
		for j := 0; j+1 < len(entry.args); j += 2 {
			fields[entry.args[j].(string)] = entry.args[j+1]
		}
		for key, want := range map[string]any{
			"record_id": int64(224), "run_id": int64(172), "model_name": "actual-model",
			"provider": ProviderOpenAICompatible, "profile_name": "primary-profile",
			"prompt_name": "prompt-provisions-v8.md", "contract_name": contract.Name,
			"attempt": i + 1, "max_attempts": 2, "raw_response": responses[i],
			"chunk_index": 4, "source_line_start": 10, "source_line_end": 20,
		} {
			if fields[key] != want {
				t.Errorf("attempt %d field %s = %#v, want %#v", i+1, key, fields[key], want)
			}
		}
		if fields["error"] == nil || fields["schema_sha256"] == "" {
			t.Errorf("missing error/schema identity: %#v", fields)
		}
		if diagnostic := fmt.Sprint(entry.args); strings.Contains(diagnostic, "secret-must-not-be-logged") || strings.Contains(diagnostic, "metadata-secret") {
			t.Errorf("diagnostic exposes credentials: %s", diagnostic)
		}
	}
}

func TestExtractStructuredJSON_LogsProviderFailureWithContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"AllocationQuota.FreeTierOnly"}}`))
	}))
	defer srv.Close()
	logger := &recordingLogger{}
	client := &OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", ModelName: "fallback-model", HTTPClient: srv.Client(), logger: logger}
	_, err := client.ExtractStructuredJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt", RecordID: 224, RunID: 172,
	}, testTopicContract())
	if err == nil {
		t.Fatal("expected provider failure")
	}
	for _, entry := range logger.Warnings() {
		if entry.message == "structured output attempt failed" {
			if !strings.Contains(fmt.Sprint(entry.args), "AllocationQuota.FreeTierOnly") {
				t.Fatalf("provider diagnostic lacks quota error: %#v", entry)
			}
			return
		}
	}
	t.Fatal("no structured provider failure diagnostic")
}

type structuredOutputTestServer struct {
	server   *httptest.Server
	requests int
}

func newStructuredOutputTestServer(t *testing.T, responses []string) *structuredOutputTestServer {
	t.Helper()
	st := &structuredOutputTestServer{}
	idx := 0
	st.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if idx >= len(responses) {
			t.Fatalf("received unexpected request %d", idx+1)
		}
		st.requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, responses[idx])))
		idx++
	}))
	return st
}

func (s *structuredOutputTestServer) Close() {
	s.server.Close()
}

func (s *structuredOutputTestServer) URL() string {
	return s.server.URL
}

func (s *structuredOutputTestServer) Client() *http.Client {
	return s.server.Client()
}

func testTopicContract() StructuredOutputContract {
	return StructuredOutputContract{
		Name: "topic_payload",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"topics": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"topic_desc": {"type": "string"},
							"lines": {
								"type": "array",
								"items": {"type": "string"}
							}
						},
						"required": ["topic_desc", "lines"],
						"additionalProperties": false
					}
				}
			},
			"required": ["topics"],
			"additionalProperties": false
		}`),
	}
}
