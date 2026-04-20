package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// openaiClient speaks the OpenAI Chat Completions REST API. It also powers
// the ProviderOpenAICompatible path when a user supplies a different BaseURL
// (e.g. Ollama, DeepSeek, Groq).
type openaiClient struct {
	cfg     ProviderConfig
	baseURL string // no trailing slash
}

func newOpenAIClient(cfg ProviderConfig, baseURL string) *openaiClient {
	return &openaiClient{cfg: cfg, baseURL: strings.TrimRight(baseURL, "/")}
}

type oaRequestBody struct {
	Model       string          `json:"model"`
	Messages    []oaMessage     `json:"messages"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []oaTool        `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
}

type oaMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []oaToolCall    `json:"tool_calls,omitempty"`
}

type oaTool struct {
	Type     string        `json:"type"`
	Function oaToolFuncDef `json:"function"`
}

type oaToolFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type oaToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function oaToolCallFn `json:"function"`
}

type oaToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaCompletion struct {
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role      string       `json:"role"`
			Content   string       `json:"content"`
			ToolCalls []oaToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage *oaUsage `json:"usage"`
}

type oaUsage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
	Total      int `json:"total_tokens"`
}

type oaStreamFrame struct {
	Choices []struct {
		Index        int     `json:"index"`
		FinishReason string  `json:"finish_reason"`
		Delta        oaDelta `json:"delta"`
	} `json:"choices"`
	Usage *oaUsage `json:"usage"`
}

type oaDelta struct {
	Role      string       `json:"role"`
	Content   string       `json:"content"`
	ToolCalls []oaToolCall `json:"tool_calls"`
}

func (c *openaiClient) buildBody(req Request, stream bool) ([]byte, error) {
	msgs := make([]oaMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		om := oaMessage{Role: string(m.Role), ToolCallID: m.ToolCallID}
		switch {
		case len(m.Parts) > 0:
			parts := make([]map[string]any, 0, len(m.Parts))
			for _, p := range m.Parts {
				switch p.Type {
				case "text", "":
					parts = append(parts, map[string]any{"type": "text", "text": p.Text})
				case "image_url":
					parts = append(parts, map[string]any{
						"type":      "image_url",
						"image_url": map[string]string{"url": p.ImageURL},
					})
				case "image_b64":
					url := "data:" + p.MIME + ";base64," + p.ImageB64
					parts = append(parts, map[string]any{
						"type":      "image_url",
						"image_url": map[string]string{"url": url},
					})
				}
			}
			b, err := json.Marshal(parts)
			if err != nil {
				return nil, err
			}
			om.Content = b
		default:
			b, err := json.Marshal(m.Content)
			if err != nil {
				return nil, err
			}
			om.Content = b
		}
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, oaToolCall{
				ID: tc.ID, Type: "function",
				Function: oaToolCallFn{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		msgs = append(msgs, om)
	}

	body := oaRequestBody{
		Model:       req.Model,
		Messages:    msgs,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      stream,
	}
	for _, t := range req.Tools {
		body.Tools = append(body.Tools, oaTool{
			Type: "function",
			Function: oaToolFuncDef{
				Name: t.Name, Description: t.Description, Parameters: t.Parameters,
			},
		})
	}
	if req.ToolChoice != "" {
		body.ToolChoice = json.RawMessage(`"` + req.ToolChoice + `"`)
	}
	return json.Marshal(body)
}

func (c *openaiClient) newHTTPRequest(ctx context.Context, payload []byte) (*http.Request, error) {
	url := c.baseURL + "/v1/chat/completions"
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	return r, nil
}

func (c *openaiClient) Complete(ctx context.Context, req Request) (*Response, error) {
	payload, err := c.buildBody(req, false)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newHTTPRequest(ctx, payload)
	if err != nil {
		return nil, err
	}
	resp, err := c.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Provider: ProviderOpenAI, Model: req.Model, Err: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Provider: ProviderOpenAI, Model: req.Model, HTTPStatus: resp.StatusCode, Err: err}
	}
	if resp.StatusCode >= 400 {
		return nil, &ProviderError{
			Provider: ProviderOpenAI, Model: req.Model,
			HTTPStatus: resp.StatusCode, Body: string(raw),
		}
	}

	var parsed oaCompletion
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, &ProviderError{
			Provider: ProviderOpenAI, Model: req.Model,
			HTTPStatus: resp.StatusCode, Body: string(raw), Err: err,
		}
	}
	out := &Response{Raw: raw}
	if len(parsed.Choices) > 0 {
		out.Content = parsed.Choices[0].Message.Content
		for _, tc := range parsed.Choices[0].Message.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
			})
		}
	}
	if parsed.Usage != nil {
		out.Usage = &Usage{
			InputTokens:  parsed.Usage.Prompt,
			OutputTokens: parsed.Usage.Completion,
			TotalTokens:  parsed.Usage.Total,
		}
	}
	return out, nil
}

func (c *openaiClient) Stream(ctx context.Context, req Request, on StreamHandler) error {
	payload, err := c.buildBody(req, true)
	if err != nil {
		return err
	}
	httpReq, err := c.newHTTPRequest(ctx, payload)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := c.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return &ProviderError{Provider: ProviderOpenAI, Model: req.Model, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return &ProviderError{
			Provider: ProviderOpenAI, Model: req.Model,
			HTTPStatus: resp.StatusCode, Body: string(body),
		}
	}

	finishReason := ""
	var lastUsage *Usage

	reader := bufio.NewReader(resp.Body)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line, rerr := reader.ReadString('\n')
		if rerr != nil {
			if errors.Is(rerr, io.EOF) && line == "" {
				break
			}
			if !errors.Is(rerr, io.EOF) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return &ProviderError{Provider: ProviderOpenAI, Model: req.Model, Err: rerr}
			}
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var frame oaStreamFrame
		if jerr := json.Unmarshal([]byte(data), &frame); jerr != nil {
			continue
		}
		if len(frame.Choices) > 0 {
			ch := frame.Choices[0]
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
			delta := ch.Delta
			if delta.Content != "" {
				if herr := on(StreamChunk{Delta: delta.Content}); herr != nil {
					return herr
				}
			}
			for _, tc := range delta.ToolCalls {
				toolCall := ToolCall{
					ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
				}
				if herr := on(StreamChunk{ToolCall: &toolCall}); herr != nil {
					return herr
				}
			}
		}
		if frame.Usage != nil {
			lastUsage = &Usage{
				InputTokens:  frame.Usage.Prompt,
				OutputTokens: frame.Usage.Completion,
				TotalTokens:  frame.Usage.Total,
			}
		}
	}

	return on(StreamChunk{Done: true, FinishReason: finishReason, Usage: lastUsage})
}

var _ Client = (*openaiClient)(nil)
