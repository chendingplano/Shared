package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type JSONExtractionInput struct {
	PromptText string
	ModelName  string
	InputText  string
}

type OpenAIJSONClient struct {
	BaseURL    string
	APIKey     string
	ModelName  string
	HTTPClient *http.Client
	logger     ApiTypes.JimoLogger
}

func NewOpenAIJSONClientFromProcessorEnv(processor string) (*OpenAIJSONClient, error) {
	processor = strings.ToUpper(strings.TrimSpace(processor))
	if processor == "" {
		return nil, errors.New("processor is required")
	}

	modelKey := processor + "_LLM_NAME"
	apiKeyKey := processor + "_LLM_API_KEY"
	baseURLKey := processor + "_LLM_BASE_URL"
	timeoutKey := processor + "_LLM_TIMEOUT_SEC"

	specificModel := strings.TrimSpace(os.Getenv(modelKey))
	useSharedFallback := specificModel == ""

	model, err := resolveScopedString(modelKey, "SHARED_LLM_NAME", useSharedFallback)
	if err != nil {
		return nil, err
	}
	apiKey, err := resolveScopedString(apiKeyKey, "SHARED_LLM_API_KEY", useSharedFallback)
	if err != nil {
		return nil, err
	}
	baseURL, err := resolveScopedString(baseURLKey, "SHARED_LLM_BASE_URL", useSharedFallback)
	if err != nil {
		return nil, err
	}
	timeoutSec, err := resolveScopedTimeout(timeoutKey, "SHARED_LLM_TIMEOUT_SEC", useSharedFallback, 100)
	if err != nil {
		return nil, err
	}

	logger := loggerutil.CreateDefaultLogger("MID_26041820")
	return &OpenAIJSONClient{
		BaseURL:   baseURL,
		APIKey:    apiKey,
		ModelName: model,
		logger:    logger,
		HTTPClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
	}, nil
}

func resolveScopedString(specificKey, sharedKey string, allowSharedFallback bool) (string, error) {
	specific := strings.TrimSpace(os.Getenv(specificKey))
	if specific != "" {
		return specific, nil
	}
	if !allowSharedFallback {
		return "", fmt.Errorf("%s is required when processor-specific LLM name is set", specificKey)
	}
	shared := strings.TrimSpace(os.Getenv(sharedKey))
	if shared == "" {
		return "", fmt.Errorf("%s is required", sharedKey)
	}
	return shared, nil
}

func resolveScopedTimeout(specificKey, sharedKey string, allowSharedFallback bool, sharedDefault int) (int, error) {
	specificRaw := strings.TrimSpace(os.Getenv(specificKey))
	if specificRaw != "" {
		n, err := parsePositiveInt(specificRaw)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %w", specificKey, err)
		}
		return n, nil
	}
	if !allowSharedFallback {
		return 0, fmt.Errorf("%s is required when processor-specific LLM name is set", specificKey)
	}

	sharedRaw := strings.TrimSpace(os.Getenv(sharedKey))
	if sharedRaw == "" {
		return sharedDefault, nil
	}
	n, err := parsePositiveInt(sharedRaw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", sharedKey, err)
	}
	return n, nil
}

func (c *OpenAIJSONClient) ExtractJSON(ctx context.Context, in JSONExtractionInput) (map[string]any, error) {
	content, err := c.extractTextWithFormat(ctx, in, true)
	if err != nil {
		return nil, err
	}
	if c.logger != nil {
		c.logger.Info("llm raw response", "content", content)
	}

	parsed, err := parseLLMJSONMap(content)
	if err != nil {
		return nil, fmt.Errorf("llm response is not valid json: %w", err)
	}
	return parsed, nil
}

func (c *OpenAIJSONClient) ExtractText(ctx context.Context, in JSONExtractionInput) (string, error) {
	return c.extractTextWithFormat(ctx, in, false)
}

func (c *OpenAIJSONClient) extractTextWithFormat(ctx context.Context, in JSONExtractionInput, jsonResponse bool) (string, error) {
	model := strings.TrimSpace(in.ModelName)
	if model == "" {
		model = strings.TrimSpace(c.ModelName)
	}
	if model == "" {
		return "", errors.New("model name is empty")
	}

	prompt := strings.TrimSpace(in.PromptText)
	if prompt == "" {
		return "", errors.New("prompt text is empty")
	}

	body := map[string]any{
		"model":       model,
		"messages":    buildMessages(prompt, in.InputText),
		"temperature": 0,
	}
	if jsonResponse {
		body["response_format"] = map[string]string{"type": "json_object"}
	}

	bs, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	endpoint := buildChatCompletionsEndpoint(c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bs))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	content, err := parseOpenAIContent(respBody)
	if err != nil {
		return "", err
	}
	return content, nil
}

func buildChatCompletionsEndpoint(baseURL string) string {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "https://api.openai.com"
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func buildMessages(prompt string, documentText string) []map[string]string {
	const documentPlaceholder = "{{DOCUMENT_TEXT}}"
	if strings.Contains(prompt, documentPlaceholder) {
		systemPrompt := strings.ReplaceAll(prompt, documentPlaceholder, documentText)
		return []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Return JSON only."},
		}
	}
	return []map[string]string{
		{"role": "system", "content": prompt},
		{"role": "user", "content": documentText},
	}
}

func parseOpenAIContent(respBody []byte) (string, error) {
	var payload struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(payload.Choices) == 0 {
		return "", errors.New("llm response has no choices")
	}

	switch content := payload.Choices[0].Message.Content.(type) {
	case string:
		return strings.TrimSpace(content), nil
	case []any:
		var b strings.Builder
		for _, item := range content {
			part, _ := item.(map[string]any)
			if strings.EqualFold(asString(part["type"]), "text") {
				b.WriteString(asString(part["text"]))
			}
		}
		text := strings.TrimSpace(b.String())
		if text == "" {
			return "", errors.New("llm response content is empty")
		}
		return text, nil
	default:
		return "", errors.New("unsupported llm content shape")
	}
}

func parseLLMJSONMap(content string) (map[string]any, error) {
	tryDecode := func(s string) (map[string]any, error) {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	}

	raw := strings.TrimSpace(content)
	if raw == "" {
		return nil, errors.New("empty llm content")
	}
	if parsed, err := tryDecode(raw); err == nil {
		return parsed, nil
	}

	cleaned := cleanMarkdownJSONFence(raw)
	if cleaned != raw {
		if parsed, err := tryDecode(cleaned); err == nil {
			return parsed, nil
		}
		raw = cleaned
	}

	if extracted, ok := extractJSONObject(raw); ok {
		if parsed, err := tryDecode(extracted); err == nil {
			return parsed, nil
		}
	}

	return nil, fmt.Errorf("unable to parse llm json content")
}

func cleanMarkdownJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	firstNL := strings.IndexByte(s, '\n')
	if firstNL >= 0 {
		s = s[firstNL+1:]
	}
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

func extractJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return "", false
	}
	return strings.TrimSpace(s[start : end+1]), true
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func parsePositiveInt(raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, fmt.Errorf("must be >= 1")
	}
	return n, nil
}
