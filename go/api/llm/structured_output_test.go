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
