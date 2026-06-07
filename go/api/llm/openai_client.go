package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	BaseURL      string
	APIKey       string
	ModelName    string
	ThinkingType string
	HTTPClient   *http.Client
	logger       ApiTypes.JimoLogger
}

type OpenAIJSONClientConfig struct {
	ModelName    string
	APIKey       string
	BaseURL      string
	TimeoutSec   int
	ThinkingType string
}

func (c *OpenAIJSONClient) httpClient() *http.Client {
	if c.HTTPClient == nil {
		c.HTTPClient = defaultHTTPClient()
	}
	return c.HTTPClient
}

func NewOpenAIJSONClientFromConfig(cfg OpenAIJSONClientConfig, logger ApiTypes.JimoLogger) (*OpenAIJSONClient, error) {
	model := strings.TrimSpace(cfg.ModelName)
	if model == "" {
		return nil, errors.New("(MID_26050155) model name is required")
	}

	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("(MID_26050170) api key is required")
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, errors.New("(MID_26050171) base url is required")
	}

	timeoutSec := cfg.TimeoutSec
	if timeoutSec <= 0 {
		return nil, errors.New("(MID_26050172) timeout_sec must be a positive integer")
	}

	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26052101")
	}

	return &OpenAIJSONClient{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		ModelName:    model,
		ThinkingType: normalizeThinkingType(cfg.ThinkingType),
		logger:       logger,
		HTTPClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
	}, nil
}

// ExtractJSON is the legacy compatibility API for callers that still expect a
// top-level JSON object without supplying an explicit schema contract. New code
// should prefer ExtractStructuredJSON so shape validation lives in code rather
// than prompts.
func (c *OpenAIJSONClient) ExtractJSON(ctx context.Context, in JSONExtractionInput) (map[string]any, error) {
	result, err := c.ExtractStructuredJSON(ctx, in, legacyJSONObjectContract())
	if err != nil {
		raw := ""
		var structuredErr *StructuredOutputError
		if errors.As(err, &structuredErr) {
			raw = structuredErr.Raw
		}
		return nil, fmt.Errorf("(MID_26050140) llm response is not valid json: %w; response=%q", err, strings.TrimSpace(raw))
	}
	return result.Parsed, nil
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
		return "", errors.New("(MID_26050164) model name is empty")
	}

	prompt := strings.TrimSpace(in.PromptText)
	if prompt == "" {
		return "", errors.New("(MID_26050156) prompt text is empty")
	}

	body := map[string]any{
		"model":       model,
		"messages":    buildMessages(prompt, in.InputText),
		"temperature": 0,
	}
	if thinkingType := normalizeThinkingType(c.ThinkingType); thinkingType == "enabled" {
		body["thinking"] = map[string]string{"type": thinkingType}
	}
	if jsonResponse {
		body["response_format"] = map[string]string{"type": "json_object"}
	}

	bs, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("(MID_26050175) failed resolveScopedString, error:%w", err)
	}

	endpoint := buildChatCompletionsEndpoint(c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bs))
	if err != nil {
		return "", fmt.Errorf("(MID_26050176) failed resolveScopedString, error:%w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.httpClient()
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("(MID_26050154) openai request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("(MID_26053001) caller_context_cancelled: %w", readErr)
		}
		var netErr net.Error
		if errors.As(readErr, &netErr) && netErr.Timeout() {
			return "", fmt.Errorf("(MID_26053002) http_client_timeout (%v): %w", httpClient.Timeout, readErr)
		}
		return "", fmt.Errorf("(MID_26053003) network_error: %w", readErr)
	}

	if c.logger == nil {
		c.logger = loggerutil.CreateDefaultLogger("MID_26052102")
	}

	/*
		c.logger.Info("llm raw http response",
			"model_name", model,
			"status_code", resp.StatusCode,
			"response_body", strings.TrimSpace(string(respBody)))
	*/

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("(MID_26050141) openai request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	content, err := parseOpenAIContent(respBody)
	if err != nil {
		return "", fmt.Errorf("(MID_26050177) failed resolveScopedString, error:%w", err)
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

func normalizeThinkingType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "enabled", "disabled":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
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
		return "", fmt.Errorf("(MID_26050142) decode llm response: %w, json:%s", err, payload)
	}
	if len(payload.Choices) == 0 {
		return "", errors.New("(MID_26050157) llm response has no choices")
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
			return "", errors.New("(MID_26050158) llm response content is empty")
		}
		return text, nil
	default:
		return "", errors.New("(MID_26050159) unsupported llm content shape")
	}
}

func parseLLMJSONMap(content string) (map[string]any, error) {
	tryDecode := func(s string) (map[string]any, error) {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			return nil, fmt.Errorf("(MID_26050178) failed resolveScopedString, error:%w", err)
		}
		return parsed, nil
	}

	raw := strings.TrimSpace(content)
	if raw == "" {
		return nil, errors.New("(MID_26050160) empty llm content")
	}
	if parsed, err := tryDecode(raw); err == nil {
		return parsed, nil
	}

	if repaired, ok := repairLLMJSON(raw); ok {
		if parsed, err := tryDecode(repaired); err == nil {
			return parsed, nil
		}
		raw = repaired
	}

	// LLM sometimes wraps the object in an array — extract the first element.
	if firstObj, ok := extractFirstJSONObjectFromArray(raw); ok {
		if parsed, err := tryDecode(firstObj); err == nil {
			return parsed, nil
		}
	}

	if extracted, ok := extractJSONObject(raw); ok {
		if parsed, err := tryDecode(extracted); err == nil {
			return parsed, nil
		}
	}

	// Last resort: the top-level JSON is malformed (e.g. an array whose outer
	// object is never closed). Scan all complete {..} blocks and return the
	// first one that contains nested map values — those are richer than leaf
	// nodes and more likely to carry the data the caller needs.
	if m, ok := scanForBestJSONObject(raw); ok {
		return m, nil
	}

	return nil, fmt.Errorf("(MID_26050144) unable to parse llm json content")
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

// extractFirstJSONObjectFromArray extracts the first JSON object from a
// JSON array like [{"k":"v"},{...}]. Returns the object text without the
// enclosing array brackets.
func extractFirstJSONObjectFromArray(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return "", false
	}
	start := strings.IndexByte(s, '{')
	if start < 1 {
		return "", false
	}
	obj, _, ok := extractBalancedObject(s, start)
	return obj, ok
}

// extractBalancedObject returns the {..} block starting at position start in s,
// tracking balanced braces and string literals. Returns the extracted text,
// the end index (inclusive), and whether the block was properly terminated.
func extractBalancedObject(s string, start int) (string, int, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[start : i+1]), i, true
			}
		}
	}
	return "", -1, false
}

// scanForBestJSONObject scans s for all complete {..} blocks. It returns the
// first one that contains a value which is itself a map or an array whose
// elements include at least one map — these "rich" objects are more useful
// than leaf nodes (e.g. {"name":"x","confidence":0.9}). Falls back to the
// first valid parseable map if no rich object is found.
func scanForBestJSONObject(s string) (map[string]any, bool) {
	var firstValid map[string]any
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		obj, end, ok := extractBalancedObject(s, i)
		if !ok {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(obj), &m); err != nil || len(m) == 0 {
			i = end
			continue
		}
		if firstValid == nil {
			firstValid = m
		}
		if hasNestedCollections(m) {
			return m, true
		}
		i = end
	}
	if firstValid != nil {
		return firstValid, true
	}
	return nil, false
}

// hasNestedCollections reports whether m has at least one value that is a
// map[string]any or a []any containing at least one map[string]any element.
func hasNestedCollections(m map[string]any) bool {
	for _, v := range m {
		switch val := v.(type) {
		case map[string]any:
			return true
		case []any:
			for _, elem := range val {
				if _, ok := elem.(map[string]any); ok {
					return true
				}
			}
		}
	}
	return false
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

/*
func parsePositiveInt(raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("(MID_26050179) failed resolveScopedString, error:%w", err)
	}
	if n < 1 {
		return 0, fmt.Errorf("(MID_26050145) must be >= 1")
	}
	return n, nil
}
*/

// EmbedInput holds parameters for a single embedding call.
type EmbedInput struct {
	ModelName string
	InputText string
}

// EmbedBatchInput holds parameters for a batched embedding call.
type EmbedBatchInput struct {
	ModelName  string
	InputTexts []string
}

// Embed calls the OpenAI embeddings API and returns the embedding vector.
func (c *OpenAIJSONClient) Embed(ctx context.Context, in EmbedInput) ([]float64, error) {
	model := strings.TrimSpace(in.ModelName)
	if model == "" {
		model = strings.TrimSpace(c.ModelName)
	}
	if model == "" {
		return nil, errors.New("(MID_26050161) embedding model name is empty")
	}
	if strings.TrimSpace(in.InputText) == "" {
		return nil, errors.New("(MID_26050162) embedding input text is empty")
	}

	body := map[string]any{
		"model": model,
		"input": in.InputText,
	}
	vecs, err := c.embedRequest(ctx, body, in.ModelName)
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, errors.New("(MID_26050163) embedding response has no data")
	}
	return vecs[0], nil
}

// EmbedBatch calls the OpenAI embeddings API for multiple inputs in one request.
func (c *OpenAIJSONClient) EmbedBatch(ctx context.Context, in EmbedBatchInput) ([][]float64, error) {
	model := strings.TrimSpace(in.ModelName)
	if model == "" {
		model = strings.TrimSpace(c.ModelName)
	}
	if model == "" {
		return nil, errors.New("(MID_26060601) embedding model name is empty")
	}
	if len(in.InputTexts) == 0 {
		return nil, errors.New("(MID_26060602) embedding input texts are empty")
	}

	inputs := make([]string, 0, len(in.InputTexts))
	for _, text := range in.InputTexts {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, errors.New("(MID_26060603) embedding batch input text is empty")
		}
		inputs = append(inputs, text)
	}

	body := map[string]any{
		"model": model,
		"input": inputs,
	}
	return c.embedRequest(ctx, body, in.ModelName)
}

func (c *OpenAIJSONClient) embedRequest(ctx context.Context, body map[string]any, modelName string) ([][]float64, error) {
	bs, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26050180) failed resolveScopedString, error:%w", err)
	}

	endpoint := buildEmbeddingsEndpoint(c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bs))
	if err != nil {
		return nil, fmt.Errorf("(MID_26050181) failed resolveScopedString, error:%w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("(MID_26050146) embedding request failed: %w, model-name:%s", err, modelName)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("(MID_26052902) failed reading embedding response body: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("(MID_26050147) embedding request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var payload struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("(MID_26050148) decode embedding response: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, errors.New("(MID_26050163) embedding response has no data")
	}
	out := make([][]float64, 0, len(payload.Data))
	for _, item := range payload.Data {
		out = append(out, item.Embedding)
	}
	return out, nil
}

func buildEmbeddingsEndpoint(baseURL string) string {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "https://api.openai.com"
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/embeddings"
	}
	return base + "/v1/embeddings"
}
