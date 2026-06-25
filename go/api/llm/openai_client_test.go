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

func TestBuildMessagesCanPlaceDocumentBeforeTaskForPromptCache(t *testing.T) {
	messages := buildMessages("Review grammar only.", `{"doc_context":"Spec","lines":[]}`, true)

	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[0]["role"] != "system" {
		t.Fatalf("system role = %q", messages[0]["role"])
	}
	if strings.Contains(messages[0]["content"], "Review grammar only.") {
		t.Fatalf("reviewer task should not be in common system message: %q", messages[0]["content"])
	}
	user := messages[1]["content"]
	docPos := strings.Index(user, `{"doc_context":"Spec","lines":[]}`)
	taskPos := strings.Index(user, "Review grammar only.")
	if docPos < 0 || taskPos < 0 {
		t.Fatalf("user message missing document or task: %q", user)
	}
	if docPos > taskPos {
		t.Fatalf("document should precede reviewer task for prefix cache: %q", user)
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
		ModelName:    "deepseek-v4-flash",
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
		ModelName:    "deepseek-v4-flash",
		APIKey:       "test-key",
		BaseURL:      "https://api.openai.com",
		TimeoutSec:   100,
		ThinkingType: "enabled",
	}, logger)
	if err != nil {
		t.Fatalf("NewOpenAIJSONClientFromConfig error: %v", err)
	}
	if client.ModelName != "deepseek-v4-flash" {
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

func TestExtractJSON_CapturesUsageRecordWithLookupHints(t *testing.T) {
	sink := &testUsageCaptureSink{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"req_json_1",
			"choices":[{"message":{"content":"{\"ok\":true}"}}],
			"usage":{"prompt_tokens":7,"completion_tokens":5,"total_tokens":12}
		}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:     srv.URL,
		APIKey:      "test-key",
		ModelName:   "deepseek-chat",
		ProfileName: "deepseek-prod",
		Provider:    ProviderOpenAICompatible,
		HTTPClient:  srv.Client(),
	}

	withDefaultUsageCaptureSink(sink, func() {
		_, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
			PromptName: "extract-products-v2",
			PromptText: "prompt",
			InputText:  "text",
		})
		if err != nil {
			t.Fatalf("ExtractJSON error: %v", err)
		}
	})

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	got := records[0]
	if got.PromptName != "extract-products-v2" {
		t.Fatalf("PromptName=%q", got.PromptName)
	}
	if got.ProfileName != "deepseek-prod" {
		t.Fatalf("ProfileName=%q", got.ProfileName)
	}
	if got.BaseURL != srv.URL {
		t.Fatalf("BaseURL=%q", got.BaseURL)
	}
	if got.APIKey != "test-key" {
		t.Fatalf("APIKey=%q", got.APIKey)
	}
	if got.InputTokens != 7 || got.OutputTokens != 5 || got.TotalTokens != 12 {
		t.Fatalf("unexpected tokens: %+v", got)
	}
	if !strings.Contains(string(got.InputBody), `"model":"deepseek-chat"`) {
		t.Fatalf("expected serialized request body, got %q", string(got.InputBody))
	}
	if !strings.Contains(string(got.OutputBody), `"prompt_tokens":7`) {
		t.Fatalf("expected raw response body, got %q", string(got.OutputBody))
	}
}

func TestExtractJSON_CapturesDeepSeekPromptCacheTokens(t *testing.T) {
	sink := &testUsageCaptureSink{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"req_cache_1",
			"choices":[{"message":{"content":"{\"ok\":true}"}}],
			"usage":{
				"prompt_tokens":1200,
				"completion_tokens":50,
				"total_tokens":1250,
				"prompt_cache_hit_tokens":1000,
				"prompt_cache_miss_tokens":200
			}
		}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL,
		APIKey:     "test-key",
		ModelName:  "deepseek-v4-flash",
		Provider:   ProviderID("deepseek"),
		HTTPClient: srv.Client(),
	}

	withDefaultUsageCaptureSink(sink, func() {
		_, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
			PromptName: "review-correctness",
			PromptText: "prompt",
			InputText:  "text",
		})
		if err != nil {
			t.Fatalf("ExtractJSON error: %v", err)
		}
	})

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	got := records[0]
	if got.InputTokens != 1200 || got.OutputTokens != 50 || got.TotalTokens != 1250 {
		t.Fatalf("unexpected tokens: %+v", got)
	}
	if got.PromptCacheHitTokens != 1000 || got.PromptCacheMissTokens != 200 {
		t.Fatalf("unexpected prompt cache tokens: %+v", got)
	}
}

func TestExtractJSON_ExposesLastUsageWithDeepSeekPromptCacheTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-cache",
			"choices":[{"message":{"content":"{\"ok\":true}"}}],
			"usage":{
				"prompt_tokens":1200,
				"completion_tokens":12,
				"total_tokens":1212,
				"prompt_cache_hit_tokens":900,
				"prompt_cache_miss_tokens":300
			}
		}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:    srv.URL,
		APIKey:     "test-key",
		ModelName:  "deepseek-v4-flash",
		HTTPClient: srv.Client(),
	}

	if _, err := client.ExtractJSON(context.Background(), JSONExtractionInput{
		PromptText: "prompt",
		InputText:  "input",
	}); err != nil {
		t.Fatalf("ExtractJSON returned error: %v", err)
	}

	usage := client.LastJSONUsage()
	if usage == nil {
		t.Fatal("LastJSONUsage() = nil, want usage")
	}
	if usage.PromptCacheHitTokens != 900 || usage.PromptCacheMissTokens != 300 {
		t.Fatalf("cache usage hit/miss=%d/%d, want 900/300", usage.PromptCacheHitTokens, usage.PromptCacheMissTokens)
	}
}

func TestExtractJSON_DefaultsLoggerWhenClientBuiltWithoutConstructor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
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
	if got := out["ok"]; got != true {
		t.Fatalf("ok=%v, want true", got)
	}
	if client.logger == nil {
		t.Fatalf("logger was not initialized")
	}
}

func TestEmbed_DefaultsHTTPClientWhenNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		ModelName: "text-embedding-3-small",
	}

	vec, err := client.Embed(context.Background(), EmbedInput{
		InputText: "test input",
	})
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if client.HTTPClient == nil {
		t.Fatalf("HTTPClient was not initialized")
	}
	if len(vec) != 3 {
		t.Fatalf("embedding length=%d, want 3", len(vec))
	}
}

func TestEmbed_CapturesUsageRecordWithLookupHints(t *testing.T) {
	sink := &testUsageCaptureSink{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data":[{"embedding":[0.1,0.2,0.3]}],
			"usage":{"prompt_tokens":6,"total_tokens":6}
		}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:     srv.URL,
		APIKey:      "test-key",
		ModelName:   "text-embedding-3-small",
		ProfileName: "embedding-prod",
		Provider:    ProviderOpenAICompatible,
		HTTPClient:  srv.Client(),
	}

	withDefaultUsageCaptureSink(sink, func() {
		_, err := client.Embed(context.Background(), EmbedInput{
			InputText: "test input",
		})
		if err != nil {
			t.Fatalf("Embed error: %v", err)
		}
	})

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	got := records[0]
	if got.ProfileName != "embedding-prod" {
		t.Fatalf("ProfileName=%q", got.ProfileName)
	}
	if got.BaseURL != srv.URL || got.APIKey != "test-key" {
		t.Fatalf("unexpected lookup hints: %+v", got)
	}
	if got.InputTokens != 6 || got.OutputTokens != 0 || got.TotalTokens != 6 {
		t.Fatalf("unexpected tokens: %+v", got)
	}
	if !strings.Contains(string(got.InputBody), `"input":"test input"`) {
		t.Fatalf("expected serialized request body, got %q", string(got.InputBody))
	}
	if !strings.Contains(string(got.OutputBody), `"prompt_tokens":6`) {
		t.Fatalf("expected raw response body, got %q", string(got.OutputBody))
	}
}

func TestEmbed_CapturesProviderErrorBody(t *testing.T) {
	sink := &testUsageCaptureSink{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad embedding key"}}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:     srv.URL,
		APIKey:      "test-key",
		ModelName:   "text-embedding-3-small",
		ProfileName: "embedding-prod",
		Provider:    ProviderOpenAICompatible,
		HTTPClient:  srv.Client(),
	}

	withDefaultUsageCaptureSink(sink, func() {
		_, err := client.Embed(context.Background(), EmbedInput{
			InputText: "test input",
		})
		if err == nil {
			t.Fatalf("expected Embed error")
		}
	})

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	if records[0].ErrorMessage == "" {
		t.Fatalf("expected error message on captured record: %+v", records[0])
	}
	if !strings.Contains(string(records[0].OutputBody), "bad embedding key") {
		t.Fatalf("expected raw provider error body, got %q", string(records[0].OutputBody))
	}
}

func TestEmbedBatch_SendsInputArrayAndReturnsVectors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		input, ok := body["input"].([]any)
		if !ok {
			t.Fatalf("input type=%T, want []any", body["input"])
		}
		if len(input) != 2 {
			t.Fatalf("input len=%d, want 2", len(input))
		}
		if input[0] != "first input" || input[1] != "second input" {
			t.Fatalf("input=%v, want [first input second input]", input)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]},{"embedding":[0.3,0.4]}]}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		ModelName: "text-embedding-3-small",
	}

	vecs, err := client.EmbedBatch(context.Background(), EmbedBatchInput{
		InputTexts: []string{"first input", "second input"},
	})
	if err != nil {
		t.Fatalf("EmbedBatch error: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("embedding count=%d, want 2", len(vecs))
	}
	if len(vecs[0]) != 2 || len(vecs[1]) != 2 {
		t.Fatalf("unexpected embedding lengths: %+v", vecs)
	}
}

func TestEmbedBatch_IncludesConfiguredDimensions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := int(body["dimensions"].(float64)); got != 1536 {
			t.Fatalf("dimensions=%d, want 1536", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:             srv.URL,
		APIKey:              "test-key",
		ModelName:           "text-embedding-v4",
		EmbeddingDimensions: 1536,
	}

	if _, err := client.EmbedBatch(context.Background(), EmbedBatchInput{
		InputTexts: []string{"first input"},
	}); err != nil {
		t.Fatalf("EmbedBatch error: %v", err)
	}
}

func TestEmbed_RespectsRequestsPerMinuteLimit(t *testing.T) {
	resetEmbeddingRateLimiterForTest()
	t.Cleanup(resetEmbeddingRateLimiterForTest)
	t.Setenv("EMBEDDING_MAX_REQUESTS_PER_MINUTE", "600")
	t.Setenv("EMBEDDING_MAX_TOKENS_PER_MINUTE", "600000")

	var firstRequestAt time.Time
	var secondRequestAt time.Time
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			firstRequestAt = time.Now()
		case 2:
			secondRequestAt = time.Now()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		ModelName: "text-embedding-3-small",
	}

	if _, err := client.Embed(context.Background(), EmbedInput{InputText: "first input"}); err != nil {
		t.Fatalf("first Embed error: %v", err)
	}
	if _, err := client.Embed(context.Background(), EmbedInput{InputText: "second input"}); err != nil {
		t.Fatalf("second Embed error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("request count=%d, want 2", requestCount)
	}
	if gap := secondRequestAt.Sub(firstRequestAt); gap < 8*time.Millisecond {
		t.Fatalf("request gap=%s, want at least 8ms", gap)
	}
}

func TestEmbedBatch_RespectsTokenPerMinuteLimitForCombinedInputs(t *testing.T) {
	resetEmbeddingRateLimiterForTest()
	t.Cleanup(resetEmbeddingRateLimiterForTest)
	t.Setenv("EMBEDDING_MAX_REQUESTS_PER_MINUTE", "600000")
	t.Setenv("EMBEDDING_MAX_TOKENS_PER_MINUTE", "600")

	var firstRequestAt time.Time
	var secondRequestAt time.Time
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			firstRequestAt = time.Now()
		case 2:
			secondRequestAt = time.Now()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]},{"embedding":[0.3,0.4]}]}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		ModelName: "text-embedding-3-small",
	}
	inputs := []string{"abcdefghij", "klmnopqrst"}
	if _, err := client.EmbedBatch(context.Background(), EmbedBatchInput{InputTexts: inputs}); err != nil {
		t.Fatalf("first EmbedBatch error: %v", err)
	}
	if _, err := client.EmbedBatch(context.Background(), EmbedBatchInput{InputTexts: inputs}); err != nil {
		t.Fatalf("second EmbedBatch error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("request count=%d, want 2", requestCount)
	}
	if gap := secondRequestAt.Sub(firstRequestAt); gap < 90*time.Millisecond {
		t.Fatalf("request gap=%s, want at least 90ms", gap)
	}
}

func TestExtractJSON_AllowsBurstWithinRequestsPerMinuteBudget(t *testing.T) {
	resetLLMRequestRateLimiterForTest()
	t.Cleanup(resetLLMRequestRateLimiterForTest)
	t.Setenv("DOC_PROCESS_LLM_MAX_REQUESTS_PER_MINUTE", "3")
	t.Setenv("DOC_PROCESS_LLM_MAX_TOKENS_PER_MINUTE", "600000")
	t.Setenv("DOC_PROCESS_LLM_TOKEN_RESERVE_PER_CALL", "1")

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		ModelName: "deepseek-v4-flash",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	for i := 0; i < 3; i++ {
		if _, err := client.ExtractJSON(ctx, JSONExtractionInput{PromptText: "prompt", InputText: "burst input"}); err != nil {
			t.Fatalf("ExtractJSON request %d error: %v", i+1, err)
		}
	}
	if requestCount != 3 {
		t.Fatalf("request count=%d, want 3", requestCount)
	}
}

func TestExtractJSON_AllowsBurstWithinTokensPerMinuteBudget(t *testing.T) {
	resetLLMRequestRateLimiterForTest()
	t.Cleanup(resetLLMRequestRateLimiterForTest)
	t.Setenv("DOC_PROCESS_LLM_MAX_REQUESTS_PER_MINUTE", "600000")
	t.Setenv("DOC_PROCESS_LLM_MAX_TOKENS_PER_MINUTE", "42")
	t.Setenv("DOC_PROCESS_LLM_TOKEN_RESERVE_PER_CALL", "1")

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		ModelName: "deepseek-v4-flash",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	prompt := strings.Repeat("abcdefghij", 3)
	input := strings.Repeat("klmnopqrst", 3)
	if _, err := client.ExtractJSON(ctx, JSONExtractionInput{PromptText: prompt, InputText: input}); err != nil {
		t.Fatalf("first ExtractJSON error: %v", err)
	}
	if _, err := client.ExtractJSON(ctx, JSONExtractionInput{PromptText: prompt, InputText: input}); err != nil {
		t.Fatalf("second ExtractJSON error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("request count=%d, want 2", requestCount)
	}
}

func TestExtractJSON_PerModelRequestsPerMinuteOverrideAllowsConfiguredBurst(t *testing.T) {
	resetLLMRequestRateLimiterForTest()
	t.Cleanup(resetLLMRequestRateLimiterForTest)
	t.Setenv("DOC_PROCESS_LLM_MAX_REQUESTS_PER_MINUTE", "1")
	t.Setenv("DOC_PROCESS_LLM_MAX_TOKENS_PER_MINUTE", "600000")
	t.Setenv("DOC_PROCESS_LLM_MAX_REQUESTS_PER_MINUTE_OVERRIDES", "gpt-5.4-mini=2")
	t.Setenv("DOC_PROCESS_LLM_TOKEN_RESERVE_PER_CALL", "1")

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		ModelName: "gpt-5.4-mini",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if _, err := client.ExtractJSON(ctx, JSONExtractionInput{PromptText: "prompt", InputText: "first input"}); err != nil {
		t.Fatalf("first ExtractJSON error: %v", err)
	}
	if _, err := client.ExtractJSON(ctx, JSONExtractionInput{PromptText: "prompt", InputText: "second input"}); err != nil {
		t.Fatalf("second ExtractJSON error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("request count=%d, want 2", requestCount)
	}
}

func TestExtractJSON_PerModelTokensPerMinuteOverrideAllowsConfiguredBurst(t *testing.T) {
	resetLLMRequestRateLimiterForTest()
	t.Cleanup(resetLLMRequestRateLimiterForTest)
	t.Setenv("DOC_PROCESS_LLM_MAX_REQUESTS_PER_MINUTE", "600000")
	t.Setenv("DOC_PROCESS_LLM_MAX_TOKENS_PER_MINUTE", "41")
	t.Setenv("DOC_PROCESS_LLM_MAX_TOKENS_PER_MINUTE_OVERRIDES", "gpt-5.4-mini=42")
	t.Setenv("DOC_PROCESS_LLM_TOKEN_RESERVE_PER_CALL", "1")

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
	}))
	defer srv.Close()

	client := &OpenAIJSONClient{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		ModelName: "gpt-5.4-mini",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	prompt := strings.Repeat("abcdefghij", 3)
	input := strings.Repeat("klmnopqrst", 3)
	if _, err := client.ExtractJSON(ctx, JSONExtractionInput{PromptText: prompt, InputText: input}); err != nil {
		t.Fatalf("first ExtractJSON error: %v", err)
	}
	if _, err := client.ExtractJSON(ctx, JSONExtractionInput{PromptText: prompt, InputText: input}); err != nil {
		t.Fatalf("second ExtractJSON error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("request count=%d, want 2", requestCount)
	}
}
