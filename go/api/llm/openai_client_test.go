package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestExtractJSON_PreservesAllFields(t *testing.T) {
	const llmJSON = `{
		"title":"Sample Standard",
		"doc_no":"T/ABC 100-2026",
		"publish_date":"2026-01-01",
		"implementation_date":"2026-06-01",
		"authors":["Org A"],
		"need_more_pages":false
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, llmJSON)))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL,
		APIKey:     "test-key",
		ModelName:  "gpt-test",
		HTTPClient: srv.Client(),
	}

	out, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	})
	if err != nil {
		t.Fatalf("ExtractJSON error: %v", err)
	}
	if got := out["title"]; got != "Sample Standard" {
		t.Fatalf("title=%v", got)
	}
	if got := out["implementation_date"]; got != "2026-06-01" {
		t.Fatalf("implementation_date=%v", got)
	}
}

func TestExtractJSON_StripsMarkdownJSONFence(t *testing.T) {
	const llmJSON = "```json\n{\n  \"title\": \"绿色建筑评价标准\",\n  \"doc_no\": \"GB/T 50378-2019\"\n}\n```"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, llmJSON)))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL,
		APIKey:     "test-key",
		ModelName:  "gemma4:26b",
		HTTPClient: srv.Client(),
	}

	out, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	})
	if err != nil {
		t.Fatalf("ExtractJSON error: %v", err)
	}
	if got := out["title"]; got != "绿色建筑评价标准" {
		t.Fatalf("title=%v", got)
	}
	if got := out["doc_no"]; got != "GB/T 50378-2019" {
		t.Fatalf("doc_no=%v", got)
	}
}

func TestExtractJSON_InvalidJSONIncludesRawResponse(t *testing.T) {
	const llmText = "not-json-response"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, llmText)))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL,
		APIKey:     "test-key",
		ModelName:  "gemma4:26b",
		HTTPClient: srv.Client(),
	}

	_, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	})
	if err == nil {
		t.Fatalf("expected ExtractJSON to fail on invalid JSON")
	}
	if !strings.Contains(err.Error(), `response="not-json-response"`) {
		t.Fatalf("expected raw response in error, got: %v", err)
	}
}

// TestExtractJSON_MalformedNestedArray reproduces the error where the LLM
// returns a top-level array whose outer object is never properly closed, but
// the first element of an inner "categories" array is a valid {category_path}
// object. scanForBestJSONObject should recover that object.
func TestExtractJSON_MalformedNestedArray(t *testing.T) {
	// Mirrors the exact malformed pattern from the production error log.
	const llmJSON = `[
{
"categories": [
{
"category_path": [
{"name": "A", "keywords": ["a1", "a2"], "confidence": 0.95},
{"name": "B", "keywords": ["b1"], "confidence": 0.94},
{"name": "C", "keywords": ["c1"], "confidence": 0.93}
],
"path_keywords": ["a1", "b1", "c1"],
"path_confidence": 0.93
},
{
"categories": [
{
"category_path": [
{"name": "A", "keywords": ["a1"], "confidence": 0.95},
{"name": "D", "keywords": ["d1"], "confidence": 0.94}
],
"path_keywords": ["a1", "d1"],
"path_confidence": 0.94
}
]
]
]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, llmJSON)))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL,
		APIKey:     "test-key",
		ModelName:  "gpt-test",
		HTTPClient: srv.Client(),
	}

	out, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	})
	if err != nil {
		t.Fatalf("ExtractJSON error: %v", err)
	}
	// The recovered object should have category_path with nested map elements.
	cp, ok := out["category_path"].([]any)
	if !ok || len(cp) == 0 {
		t.Fatalf("expected category_path array, got %v", out)
	}
	first, ok := cp[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first category_path element to be a map, got %T", cp[0])
	}
	if first["name"] != "A" {
		t.Fatalf("expected name=A, got %v", first["name"])
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestExtractJSON_IncludesThinkingWhenEnabled(t *testing.T) {
	const llmJSON = `{"status":"ok"}`

	client := &OpenAIJSONClient{
		BaseURL:      "https://api.deepseek.com",
		APIKey:       "test-key",
		ModelName:    "deepseek-v4-flash",
		ThinkingType: "enabled",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			thinking, ok := body["thinking"].(map[string]any)
			if !ok {
				t.Fatalf("expected thinking object in request body, got %T", body["thinking"])
			}
			if got := thinking["type"]; got != "enabled" {
				t.Fatalf("thinking.type=%v, want enabled", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, llmJSON))),
			}, nil
		})},
	}

	if _, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	}); err != nil {
		t.Fatalf("ExtractJSON error: %v", err)
	}
}

func TestExtractJSON_OmitsThinkingWhenDisabled(t *testing.T) {
	const llmJSON = `{"status":"ok"}`

	client := &OpenAIJSONClient{
		BaseURL:      "https://api.openai.com",
		APIKey:       "test-key",
		ModelName:    "gpt-5.4-mini",
		ThinkingType: "disabled",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if _, ok := body["thinking"]; ok {
				t.Fatalf("did not expect thinking field in request body when disabled")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, llmJSON))),
			}, nil
		})},
	}

	if _, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	}); err != nil {
		t.Fatalf("ExtractJSON error: %v", err)
	}
}

func TestExtractJSON_OmitsThinkingWhenUnset(t *testing.T) {
	const llmJSON = `{"status":"ok"}`

	client := &OpenAIJSONClient{
		BaseURL:   "https://api.deepseek.com",
		APIKey:    "test-key",
		ModelName: "deepseek-v4-flash",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if _, ok := body["thinking"]; ok {
				t.Fatalf("did not expect thinking field in request body")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, llmJSON))),
			}, nil
		})},
	}

	if _, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "text",
	}); err != nil {
		t.Fatalf("ExtractJSON error: %v", err)
	}
}

func TestBuildChatCompletionsEndpoint(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "default openai base", in: "", want: "https://api.openai.com/v1/chat/completions"},
		{name: "host without scheme", in: "127.0.0.1:11434", want: "http://127.0.0.1:11434/v1/chat/completions"},
		{name: "host with scheme", in: "http://127.0.0.1:11434", want: "http://127.0.0.1:11434/v1/chat/completions"},
		{name: "already v1", in: "http://127.0.0.1:11434/v1", want: "http://127.0.0.1:11434/v1/chat/completions"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := buildChatCompletionsEndpoint(tc.in)
			if got != tc.want {
				t.Fatalf("buildChatCompletionsEndpoint(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNewOpenAIJSONClientFromConfig(t *testing.T) {
	logger := loggerutil.CreateDefaultLogger("MID_26050803")
	client, err := NewOpenAIJSONClientFromConfig(OpenAIJSONClientConfig{
		ModelName:    "gpt-5.4-mini",
		APIKey:       "test-key",
		BaseURL:      "https://api.openai.com",
		TimeoutSec:   100,
		ThinkingType: "enabled",
	}, logger)
	if err != nil {
		t.Fatalf("NewOpenAIJSONClientFromConfig error: %v", err)
	}
	if client.ModelName != "gpt-5.4-mini" {
		t.Fatalf("ModelName=%q", client.ModelName)
	}
	if client.APIKey != "test-key" {
		t.Fatalf("APIKey=%q", client.APIKey)
	}
	if client.BaseURL != "https://api.openai.com" {
		t.Fatalf("BaseURL=%q", client.BaseURL)
	}
	if client.HTTPClient == nil || client.HTTPClient.Timeout != 100*time.Second {
		t.Fatalf("timeout=%v, want 100s", client.HTTPClient.Timeout)
	}
	if client.ThinkingType != "enabled" {
		t.Fatalf("ThinkingType=%q, want enabled", client.ThinkingType)
	}
}

func TestNewOpenAIJSONClientFromConfig_RequiresAllModelAttributes(t *testing.T) {
	logger := loggerutil.CreateDefaultLogger("MID_26050803")
	_, err := NewOpenAIJSONClientFromConfig(OpenAIJSONClientConfig{
		ModelName:  "gemma4:26b",
		APIKey:     "",
		BaseURL:    "",
		TimeoutSec: 0,
	}, logger)
	if err == nil {
		t.Fatalf("expected error when required model attributes are missing")
	}
	if !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
