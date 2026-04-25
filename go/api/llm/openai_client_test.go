package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestNewOpenAIJSONClientFromProcessorEnv_UsesSharedFallback(t *testing.T) {
	t.Setenv("EXTRACT_DOCMETA_LLM_NAME", "")
	t.Setenv("EXTRACT_DOCMETA_LLM_API_KEY", "")
	t.Setenv("EXTRACT_DOCMETA_LLM_BASE_URL", "")
	t.Setenv("EXTRACT_DOCMETA_LLM_TIMEOUT_SEC", "")
	t.Setenv("SHARED_LLM_NAME", "gpt-5.4-mini")
	t.Setenv("SHARED_LLM_API_KEY", "shared-key")
	t.Setenv("SHARED_LLM_BASE_URL", "https://api.openai.com")
	t.Setenv("SHARED_LLM_TIMEOUT_SEC", "")

	client, err := NewOpenAIJSONClientFromProcessorEnv("EXTRACT_DOCMETA")
	if err != nil {
		t.Fatalf("NewOpenAIJSONClientFromProcessorEnv error: %v", err)
	}
	if client.ModelName != "gpt-5.4-mini" {
		t.Fatalf("ModelName=%q, want shared fallback", client.ModelName)
	}
	if client.APIKey != "shared-key" {
		t.Fatalf("APIKey=%q, want shared fallback", client.APIKey)
	}
	if client.BaseURL != "https://api.openai.com" {
		t.Fatalf("BaseURL=%q, want shared fallback", client.BaseURL)
	}
	if client.HTTPClient == nil || client.HTTPClient.Timeout != 100*time.Second {
		t.Fatalf("timeout=%v, want 100s", client.HTTPClient.Timeout)
	}
}

func TestNewOpenAIJSONClientFromProcessorEnv_RequiresSpecificFieldsWhenSpecificNameSet(t *testing.T) {
	t.Setenv("EXTRACT_METRICS_LLM_NAME", "gemma4:26b")
	t.Setenv("EXTRACT_METRICS_LLM_API_KEY", "")
	t.Setenv("EXTRACT_METRICS_LLM_BASE_URL", "")
	t.Setenv("EXTRACT_METRICS_LLM_TIMEOUT_SEC", "")
	t.Setenv("SHARED_LLM_API_KEY", "shared-key")
	t.Setenv("SHARED_LLM_BASE_URL", "http://shared-llm")
	t.Setenv("SHARED_LLM_TIMEOUT_SEC", "120")

	_, err := NewOpenAIJSONClientFromProcessorEnv("EXTRACT_METRICS")
	if err == nil {
		t.Fatalf("expected error when specific processor name is set but specific key/base/timeout are missing")
	}
	if !strings.Contains(err.Error(), "EXTRACT_METRICS_LLM_API_KEY is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
