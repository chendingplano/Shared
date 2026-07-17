package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const (
	anthropicDefaultBaseURL = "https://api.anthropic.com"
	anthropicDefaultVersion = "2023-06-01"
)

type anthropicClient struct {
	cfg     ProviderConfig
	baseURL string
	logger  ApiTypes.JimoLogger
}

func newAnthropicClient(cfg ProviderConfig, logger ApiTypes.JimoLogger) *anthropicClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = anthropicDefaultBaseURL
	}
	return &anthropicClient{cfg: cfg, baseURL: baseURL, logger: logger}
}

func (c *anthropicClient) apiVersion() string {
	if v, ok := c.cfg.Extra["anthropic-version"]; ok && v != "" {
		return v
	}
	return anthropicDefaultVersion
}

// Anthropic request/response types

type anRequestBody struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	System      string        `json:"system,omitempty"`
	Messages    []anMessage   `json:"messages"`
	Tools       []anTool      `json:"tools,omitempty"`
	ToolChoice  *anToolChoice `json:"tool_choice,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
}

type anMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []anContentBlock
}

type anContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type anResponse struct {
	ID      string           `json:"id"`
	Type    string           `json:"type"`
	Role    string           `json:"role"`
	Content []anContentBlock `json:"content"`
	Usage   *anUsage         `json:"usage"`
}

type anUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// SSE event types for streaming

type anStreamEvent struct {
	Type         string          `json:"type"`
	Message      *anResponse     `json:"message,omitempty"`
	Index        int             `json:"index"`
	Delta        *anStreamDelta  `json:"delta,omitempty"`
	Usage        *anUsage        `json:"usage,omitempty"`
	ContentBlock *anContentBlock `json:"content_block,omitempty"`
}

type anStreamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

func (c *anthropicClient) buildBody(req Request, stream bool) ([]byte, error) {
	var systemText string
	var msgs []anMessage

	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			if systemText != "" {
				systemText += "\n\n"
			}
			systemText += m.Content
			continue
		}
		am := anMessage{Role: string(m.Role)}
		switch {
		case len(m.ToolCalls) > 0:
			var blocks []anContentBlock
			if m.Content != "" {
				blocks = append(blocks, anContentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				inputRaw := json.RawMessage(tc.Arguments)
				if !json.Valid(inputRaw) {
					inputRaw = json.RawMessage("{}")
				}
				blocks = append(blocks, anContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: inputRaw,
				})
			}
			am.Content = blocks
		case m.Role == RoleTool:
			am.Role = "user"
			am.Content = []anContentBlock{{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			}}
		case len(m.Parts) > 0:
			var blocks []anContentBlock
			for _, p := range m.Parts {
				switch p.Type {
				case "text", "":
					blocks = append(blocks, anContentBlock{Type: "text", Text: p.Text})
				case "image_url":
					// Anthropic doesn't support image_url directly; skip unsupported type
				case "image_b64":
					// base64 image blocks require source/media_type; not mapped here
				}
			}
			am.Content = blocks
		default:
			am.Content = m.Content
		}
		msgs = append(msgs, am)
	}

	maxTokens := 8096
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	body := anRequestBody{
		Model:       req.Model,
		MaxTokens:   maxTokens,
		System:      systemText,
		Messages:    msgs,
		Stream:      stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	for _, t := range req.Tools {
		schema := t.Parameters
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		body.Tools = append(body.Tools, anTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}

	if req.ToolChoice != "" {
		switch req.ToolChoice {
		case "auto":
			body.ToolChoice = &anToolChoice{Type: "auto"}
		case "none":
			body.ToolChoice = &anToolChoice{Type: "none"}
		case "required", "any":
			body.ToolChoice = &anToolChoice{Type: "any"}
		default:
			body.ToolChoice = &anToolChoice{Type: "tool", Name: req.ToolChoice}
		}
	}

	return json.Marshal(body)
}

func (c *anthropicClient) newHTTPRequest(ctx context.Context, payload []byte, stream bool) (*http.Request, error) {
	url := c.baseURL + "/v1/messages"
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("x-api-key", c.cfg.APIKey)
	r.Header.Set("anthropic-version", c.apiVersion())
	if stream {
		r.Header.Set("Accept", "text/event-stream")
	} else {
		r.Header.Set("Accept", "application/json")
	}
	return r, nil
}

func anUsageToShared(u *anUsage) *Usage {
	if u == nil {
		return nil
	}
	return &Usage{
		InputTokens:           u.InputTokens,
		OutputTokens:          u.OutputTokens,
		TotalTokens:           u.InputTokens + u.OutputTokens,
		PromptCacheHitTokens:  u.CacheReadInputTokens,
		PromptCacheMissTokens: u.CacheCreationInputTokens,
	}
}

func (c *anthropicClient) Complete(ctx context.Context, req Request) (*Response, error) {
	startedAt := time.Now().UTC()
	payload, err := c.buildBody(req, false)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newHTTPRequest(ctx, payload, false)
	if err != nil {
		return nil, err
	}
	resp, err := c.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		captureUsageRecord(ctx, req, UsageCaptureInput{
			AccountID:         captureAccountID(req),
			ProfileID:         captureProfileID(req),
			Provider:          c.cfg.ID,
			BaseURL:           c.baseURL,
			APIKey:            c.cfg.APIKey,
			ProfileName:       c.cfg.ProfileName,
			ModelName:         req.Model,
			PromptName:        req.PromptName,
			RecordID:          req.RecordID,
			CallReason:        req.CallReason,
			CallLoc:           req.CallLoc,
			RunID:             req.RunID,
			RequestStartedAt:  startedAt,
			RequestFinishedAt: time.Now().UTC(),
			ErrorMessage:      err.Error(),
			InputBody:         payload,
		}, c.logger)
		return nil, &ProviderError{Provider: ProviderAnthropic, Model: req.Model, Err: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		captureUsageRecord(ctx, req, UsageCaptureInput{
			AccountID:         captureAccountID(req),
			ProfileID:         captureProfileID(req),
			Provider:          c.cfg.ID,
			BaseURL:           c.baseURL,
			APIKey:            c.cfg.APIKey,
			ProfileName:       c.cfg.ProfileName,
			ModelName:         req.Model,
			PromptName:        req.PromptName,
			RecordID:          req.RecordID,
			CallReason:        req.CallReason,
			CallLoc:           req.CallLoc,
			RunID:             req.RunID,
			RequestStartedAt:  startedAt,
			RequestFinishedAt: time.Now().UTC(),
			ErrorMessage:      err.Error(),
			InputBody:         payload,
		}, c.logger)
		return nil, &ProviderError{Provider: ProviderAnthropic, Model: req.Model, HTTPStatus: resp.StatusCode, Err: err}
	}
	if resp.StatusCode >= 400 {
		captureUsageRecord(ctx, req, UsageCaptureInput{
			AccountID:         captureAccountID(req),
			ProfileID:         captureProfileID(req),
			Provider:          c.cfg.ID,
			BaseURL:           c.baseURL,
			APIKey:            c.cfg.APIKey,
			ProfileName:       c.cfg.ProfileName,
			ModelName:         req.Model,
			PromptName:        req.PromptName,
			RecordID:          req.RecordID,
			CallReason:        req.CallReason,
			CallLoc:           req.CallLoc,
			RunID:             req.RunID,
			RequestStartedAt:  startedAt,
			RequestFinishedAt: time.Now().UTC(),
			ErrorMessage:      string(raw),
			InputBody:         payload,
			OutputBody:        raw,
		}, c.logger)
		return nil, &ProviderError{
			Provider: ProviderAnthropic, Model: req.Model,
			HTTPStatus: resp.StatusCode, Body: string(raw),
		}
	}

	var parsed anResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		captureUsageRecord(ctx, req, UsageCaptureInput{
			AccountID:         captureAccountID(req),
			ProfileID:         captureProfileID(req),
			Provider:          c.cfg.ID,
			BaseURL:           c.baseURL,
			APIKey:            c.cfg.APIKey,
			ProfileName:       c.cfg.ProfileName,
			ModelName:         req.Model,
			PromptName:        req.PromptName,
			RecordID:          req.RecordID,
			CallReason:        req.CallReason,
			CallLoc:           req.CallLoc,
			RunID:             req.RunID,
			RequestStartedAt:  startedAt,
			RequestFinishedAt: time.Now().UTC(),
			ErrorMessage:      err.Error(),
			InputBody:         payload,
			OutputBody:        raw,
		}, c.logger)
		return nil, &ProviderError{
			Provider: ProviderAnthropic, Model: req.Model,
			HTTPStatus: resp.StatusCode, Body: string(raw), Err: err,
		}
	}

	out := &Response{Raw: raw, Usage: anUsageToShared(parsed.Usage)}
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			out.Content += block.Text
		case "tool_use":
			args := string(block.Input)
			if args == "" {
				args = "{}"
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	usage := out.Usage
	eventID := captureUsageRecord(ctx, req, UsageCaptureInput{
		AccountID:             captureAccountID(req),
		ProfileID:             captureProfileID(req),
		Provider:              c.cfg.ID,
		ModelName:             req.Model,
		PromptName:            req.PromptName,
		RequestStartedAt:      startedAt,
		RequestFinishedAt:     time.Now().UTC(),
		InputTokens:           usageInputTokens(usage),
		OutputTokens:          usageOutputTokens(usage),
		PromptCacheHitTokens:  usagePromptCacheHitTokens(usage),
		PromptCacheMissTokens: usagePromptCacheMissTokens(usage),
		ProviderRequestID:     parsed.ID,
		InputBody:             payload,
		OutputBody:            raw,
	}, c.logger)
	if eventID != "" {
		if out.Usage == nil {
			out.Usage = &Usage{}
		}
		out.Usage.EventID = eventID
	}
	return out, nil
}

func (c *anthropicClient) Stream(ctx context.Context, req Request, on StreamHandler) error {
	startedAt := time.Now().UTC()
	payload, err := c.buildBody(req, true)
	if err != nil {
		return err
	}
	httpReq, err := c.newHTTPRequest(ctx, payload, true)
	if err != nil {
		return err
	}
	resp, err := c.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		captureUsageRecord(ctx, req, UsageCaptureInput{
			AccountID:         captureAccountID(req),
			ProfileID:         captureProfileID(req),
			Provider:          c.cfg.ID,
			BaseURL:           c.baseURL,
			APIKey:            c.cfg.APIKey,
			ProfileName:       c.cfg.ProfileName,
			ModelName:         req.Model,
			PromptName:        req.PromptName,
			RecordID:          req.RecordID,
			CallReason:        req.CallReason,
			CallLoc:           req.CallLoc,
			RunID:             req.RunID,
			RequestStartedAt:  startedAt,
			RequestFinishedAt: time.Now().UTC(),
			ErrorMessage:      err.Error(),
			InputBody:         payload,
		}, c.logger)
		return &ProviderError{Provider: ProviderAnthropic, Model: req.Model, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		captureUsageRecord(ctx, req, UsageCaptureInput{
			AccountID:         captureAccountID(req),
			ProfileID:         captureProfileID(req),
			Provider:          c.cfg.ID,
			BaseURL:           c.baseURL,
			APIKey:            c.cfg.APIKey,
			ProfileName:       c.cfg.ProfileName,
			ModelName:         req.Model,
			PromptName:        req.PromptName,
			RecordID:          req.RecordID,
			CallReason:        req.CallReason,
			CallLoc:           req.CallLoc,
			RunID:             req.RunID,
			RequestStartedAt:  startedAt,
			RequestFinishedAt: time.Now().UTC(),
			ErrorMessage:      string(body),
			InputBody:         payload,
			OutputBody:        body,
		}, c.logger)
		return &ProviderError{
			Provider: ProviderAnthropic, Model: req.Model,
			HTTPStatus: resp.StatusCode, Body: string(body),
		}
	}

	// Per-block tool call accumulator (partial_json arrives as deltas).
	type toolAccum struct {
		id   string
		name string
		args strings.Builder
	}
	toolBlocks := map[int]*toolAccum{}

	var inputUsage *anUsage
	var outputTokens int
	stopReason := ""
	var output strings.Builder
	messageID := ""

	reader := bufio.NewReader(resp.Body)
	for {
		if ctx.Err() != nil {
			captureUsageRecord(ctx, req, UsageCaptureInput{
				AccountID:             captureAccountID(req),
				ProfileID:             captureProfileID(req),
				Provider:              c.cfg.ID,
				ModelName:             req.Model,
				PromptName:            req.PromptName,
				RequestStartedAt:      startedAt,
				RequestFinishedAt:     time.Now().UTC(),
				InputTokens:           anInputTokens(inputUsage),
				OutputTokens:          outputTokens,
				PromptCacheHitTokens:  anCacheReadTokens(inputUsage),
				PromptCacheMissTokens: anCacheCreationTokens(inputUsage),
				ErrorMessage:          ctx.Err().Error(),
				InputBody:             payload,
				OutputBody:            []byte(output.String()),
				ProviderRequestID:     messageID,
			}, c.logger)
			return ctx.Err()
		}

		line, rerr := reader.ReadString('\n')
		if rerr != nil {
			if rerr == io.EOF && line == "" {
				break
			}
			if rerr != io.EOF {
				captureUsageRecord(ctx, req, UsageCaptureInput{
					AccountID:             captureAccountID(req),
					ProfileID:             captureProfileID(req),
					Provider:              c.cfg.ID,
					ModelName:             req.Model,
					PromptName:            req.PromptName,
					RequestStartedAt:      startedAt,
					RequestFinishedAt:     time.Now().UTC(),
					InputTokens:           anInputTokens(inputUsage),
					OutputTokens:          outputTokens,
					PromptCacheHitTokens:  anCacheReadTokens(inputUsage),
					PromptCacheMissTokens: anCacheCreationTokens(inputUsage),
					ErrorMessage:          rerr.Error(),
					InputBody:             payload,
					OutputBody:            []byte(output.String()),
					ProviderRequestID:     messageID,
				}, c.logger)
				return &ProviderError{Provider: ProviderAnthropic, Model: req.Model, Err: rerr}
			}
		}

		line = strings.TrimRight(line, "\r\n")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}

		var ev anStreamEvent
		if jerr := json.Unmarshal([]byte(data), &ev); jerr != nil {
			continue
		}

		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				messageID = ev.Message.ID
				if ev.Message.Usage != nil {
					inputUsage = ev.Message.Usage
				}
			}
		case "content_block_start":
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				toolBlocks[ev.Index] = &toolAccum{
					id:   ev.ContentBlock.ID,
					name: ev.ContentBlock.Name,
				}
			}
		case "content_block_delta":
			if ev.Delta == nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					output.WriteString(ev.Delta.Text)
					if herr := on(StreamChunk{Delta: ev.Delta.Text}); herr != nil {
						captureUsageRecord(ctx, req, UsageCaptureInput{
							AccountID:             captureAccountID(req),
							ProfileID:             captureProfileID(req),
							Provider:              c.cfg.ID,
							ModelName:             req.Model,
							PromptName:            req.PromptName,
							RequestStartedAt:      startedAt,
							RequestFinishedAt:     time.Now().UTC(),
							InputTokens:           anInputTokens(inputUsage),
							OutputTokens:          outputTokens,
							PromptCacheHitTokens:  anCacheReadTokens(inputUsage),
							PromptCacheMissTokens: anCacheCreationTokens(inputUsage),
							ErrorMessage:          herr.Error(),
							InputBody:             payload,
							OutputBody:            []byte(output.String()),
							ProviderRequestID:     messageID,
						}, c.logger)
						return herr
					}
				}
			case "input_json_delta":
				if tb, ok := toolBlocks[ev.Index]; ok {
					tb.args.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "content_block_stop":
			if tb, ok := toolBlocks[ev.Index]; ok {
				args := tb.args.String()
				if args == "" {
					args = "{}"
				}
				toolCall := ToolCall{ID: tb.id, Name: tb.name, Arguments: args}
				delete(toolBlocks, ev.Index)
				if herr := on(StreamChunk{ToolCall: &toolCall}); herr != nil {
					captureUsageRecord(ctx, req, UsageCaptureInput{
						AccountID:             captureAccountID(req),
						ProfileID:             captureProfileID(req),
						Provider:              c.cfg.ID,
						ModelName:             req.Model,
						PromptName:            req.PromptName,
						RequestStartedAt:      startedAt,
						RequestFinishedAt:     time.Now().UTC(),
						InputTokens:           anInputTokens(inputUsage),
						OutputTokens:          outputTokens,
						PromptCacheHitTokens:  anCacheReadTokens(inputUsage),
						PromptCacheMissTokens: anCacheCreationTokens(inputUsage),
						ErrorMessage:          herr.Error(),
						InputBody:             payload,
						OutputBody:            []byte(output.String()),
						ProviderRequestID:     messageID,
					}, c.logger)
					return herr
				}
			}
		case "message_delta":
			if ev.Delta != nil {
				stopReason = ev.Delta.StopReason
			}
			if ev.Usage != nil {
				outputTokens = ev.Usage.OutputTokens
			}
		}
	}

	finalUsage := &Usage{
		InputTokens:           anInputTokens(inputUsage),
		OutputTokens:          outputTokens,
		TotalTokens:           anInputTokens(inputUsage) + outputTokens,
		PromptCacheHitTokens:  anCacheReadTokens(inputUsage),
		PromptCacheMissTokens: anCacheCreationTokens(inputUsage),
	}

	if herr := on(StreamChunk{Done: true, FinishReason: stopReason, Usage: finalUsage}); herr != nil {
		captureUsageRecord(ctx, req, UsageCaptureInput{
			AccountID:             captureAccountID(req),
			ProfileID:             captureProfileID(req),
			Provider:              c.cfg.ID,
			BaseURL:               c.baseURL,
			APIKey:                c.cfg.APIKey,
			ProfileName:           c.cfg.ProfileName,
			ModelName:             req.Model,
			PromptName:            req.PromptName,
			RecordID:              req.RecordID,
			CallReason:            req.CallReason,
			CallLoc:               req.CallLoc,
			RunID:                 req.RunID,
			RequestStartedAt:      startedAt,
			RequestFinishedAt:     time.Now().UTC(),
			InputTokens:           finalUsage.InputTokens,
			OutputTokens:          finalUsage.OutputTokens,
			PromptCacheHitTokens:  finalUsage.PromptCacheHitTokens,
			PromptCacheMissTokens: finalUsage.PromptCacheMissTokens,
			ErrorMessage:          herr.Error(),
			InputBody:             payload,
			OutputBody:            []byte(output.String()),
			ProviderRequestID:     messageID,
		}, c.logger)
		return herr
	}

	captureUsageRecord(ctx, req, UsageCaptureInput{
		AccountID:             captureAccountID(req),
		ProfileID:             captureProfileID(req),
		Provider:              c.cfg.ID,
		ModelName:             req.Model,
		PromptName:            req.PromptName,
		RequestStartedAt:      startedAt,
		RequestFinishedAt:     time.Now().UTC(),
		InputTokens:           finalUsage.InputTokens,
		OutputTokens:          finalUsage.OutputTokens,
		PromptCacheHitTokens:  finalUsage.PromptCacheHitTokens,
		PromptCacheMissTokens: finalUsage.PromptCacheMissTokens,
		InputBody:             payload,
		OutputBody:            []byte(output.String()),
		ProviderRequestID:     messageID,
	}, c.logger)
	return nil
}

var _ Client = (*anthropicClient)(nil)

func anInputTokens(u *anUsage) int {
	if u == nil {
		return 0
	}
	return u.InputTokens
}

func anCacheReadTokens(u *anUsage) int {
	if u == nil {
		return 0
	}
	return u.CacheReadInputTokens
}

func anCacheCreationTokens(u *anUsage) int {
	if u == nil {
		return 0
	}
	return u.CacheCreationInputTokens
}
