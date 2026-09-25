package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const jevDefaultBaseURL = "https://jev-ai.pro/api"

// JevQuestion describes one Jev system-one question. Type is "noul", "choice",
// or "score". Criteria must be nil for "noul", a nonempty map[string]string
// for "choice", or a nonempty ordered []string for "score".
type JevQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"-"`
}

// JevQuestions is a set of Jev questions keyed by question ID. For example:
//
//	JevQuestions{"urgent": {
//	    Type: "noul", Instructions: "Does this message need urgent support?",
//	}}
//
// Set Request.JevQuestions when using ProviderJev. Configure the server key in
// JEV_AI_API_KEY; NewClient reads it when ProviderConfig.APIKey is empty.
type JevQuestions map[string]JevQuestion

func (q JevQuestion) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type         string `json:"type"`
		Instructions string `json:"instructions"`
		Criteria     any    `json:"criteria,omitempty"`
	}
	v := wire{Type: q.Type, Instructions: q.Instructions}
	switch q.Type {
	case "noul":
		if q.Criteria != nil {
			return nil, fmt.Errorf("jev: noul question cannot have criteria")
		}
	case "choice":
		criteria, ok := q.Criteria.(map[string]string)
		if !ok || len(criteria) == 0 {
			return nil, fmt.Errorf("jev: choice question requires nonempty option descriptions")
		}
		v.Criteria = criteria
	case "score":
		criteria, ok := q.Criteria.([]string)
		if !ok || len(criteria) == 0 {
			return nil, fmt.Errorf("jev: score question requires nonempty ordered labels")
		}
		v.Criteria = criteria
	default:
		return nil, fmt.Errorf("jev: unsupported question type %q", q.Type)
	}
	return json.Marshal(v)
}

type jevClient struct {
	cfg     ProviderConfig
	baseURL string
}

func newJevClient(cfg ProviderConfig) *jevClient {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = jevDefaultBaseURL
	}
	return &jevClient{cfg: cfg, baseURL: base}
}

func (c *jevClient) Complete(ctx context.Context, req Request) (*Response, error) {
	if len(req.JevQuestions) == 0 {
		return nil, fmt.Errorf("jev: at least one question is required")
	}
	state := ""
	found := false
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == RoleUser {
			state = req.Messages[i].Content
			found = true
			break
		}
	}
	if !found || strings.TrimSpace(state) == "" {
		return nil, fmt.Errorf("jev: nonempty user state is required")
	}
	lastUser := lastUserMessageIndex(req.Messages)
	if len(req.Messages[lastUser].Parts) != 0 {
		return nil, fmt.Errorf("jev: multipart user state is unsupported")
	}
	body, err := json.Marshal(struct {
		Model     string       `json:"model"`
		State     string       `json:"state"`
		Questions JevQuestions `json:"questions"`
	}{req.Model, state, req.JevQuestions})
	if err != nil {
		return nil, fmt.Errorf("jev: encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jev: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Provider: ProviderJevCompatible, Model: req.Model, Err: err}
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, jevProviderError(req.Model, resp.StatusCode, payload, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, jevProviderError(req.Model, resp.StatusCode, payload, nil)
	}
	var decoded struct {
		Answers json.RawMessage `json:"answers"`
		Usage   *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, jevProviderError(req.Model, resp.StatusCode, payload, err)
	}
	trimmed := bytes.TrimSpace(decoded.Answers)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return nil, jevProviderError(req.Model, resp.StatusCode, payload, fmt.Errorf("response answers must be a JSON object"))
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return nil, jevProviderError(req.Model, resp.StatusCode, payload, err)
	}
	out := &Response{Content: compact.String(), Raw: json.RawMessage(payload)}
	if decoded.Usage != nil {
		out.Usage = &Usage{InputTokens: decoded.Usage.InputTokens, OutputTokens: decoded.Usage.OutputTokens}
	}
	return out, nil
}

func jevProviderError(model string, status int, body []byte, err error) *ProviderError {
	if len(body) > 512 {
		body = body[:512]
	}
	return &ProviderError{Provider: ProviderJevCompatible, Model: model, HTTPStatus: status, Body: string(body), Err: err}
}

func lastUserMessageIndex(messages []Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			return i
		}
	}
	return -1
}

func (c *jevClient) Stream(ctx context.Context, req Request, on StreamHandler) error {
	return fmt.Errorf("jev: streaming is not supported")
}
