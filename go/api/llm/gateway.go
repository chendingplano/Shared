package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

type PiGatewayRun struct {
	RunID, ConversationID, UserID, Message, Capability string
	Profile                                            any
	History                                            any
	// Knowledge is optional run-scoped knowledge context. Leave it nil when the
	// run has none; it is then omitted from the gateway request.
	Knowledge any
	// HistorySummary is an optional rolling summary of earlier turns. The
	// gateway appends it to the system prompt; empty means none.
	HistorySummary string
	Capture        *RequestCapture
}

type piGatewayWireRun struct {
	RunID          string `json:"runId"`
	ConversationID string `json:"conversationId"`
	UserID         string `json:"userId"`
	Message        string `json:"message"`
	Capability     string `json:"capability"`
	Profile        any    `json:"profile"`
	History        any    `json:"history"`
	Knowledge      any    `json:"knowledge,omitempty"`
	HistorySummary string `json:"historySummary,omitempty"`
}

// piGatewayCaptureInput deliberately excludes Capability, which is a bearer
// token for the gateway and must never be archived by usage capture.
type piGatewayCaptureInput struct {
	RunID          string `json:"runId"`
	ConversationID string `json:"conversationId"`
	UserID         string `json:"userId"`
	Message        string `json:"message"`
	Profile        any    `json:"profile"`
	History        any    `json:"history"`
	Knowledge      any    `json:"knowledge,omitempty"`
	HistorySummary string `json:"historySummary,omitempty"`
}

// piGatewayMaxRequestBytes matches the gateway's request body limit.
const piGatewayMaxRequestBytes = 512 * 1024

// ErrPiGatewayModelUnknown means the gateway's model registry does not know
// the requested provider/model pair.
var ErrPiGatewayModelUnknown = errors.New("Pi gateway does not know this model")

// PiGatewayModelInfo is what the gateway reports about a configured model.
type PiGatewayModelInfo struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ContextWindow int    `json:"contextWindow"`
	MaxTokens     int    `json:"maxTokens"`
}

// PiGatewayMessage is one plain-text conversation message.
type PiGatewayMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PiGatewaySummaryRequest asks the gateway to fold earlier turns into a
// rolling summary with the given model, without tools.
type PiGatewaySummaryRequest struct {
	UserID          string             `json:"-"`
	Provider        string             `json:"provider"`
	Model           string             `json:"model"`
	SystemPrompt    string             `json:"systemPrompt"`
	PreviousSummary string             `json:"previousSummary,omitempty"`
	Messages        []PiGatewayMessage `json:"messages"`
	MaxTokens       int                `json:"maxTokens"`
	Capture         *RequestCapture    `json:"-"`
}

type PiGatewayClient struct {
	baseURL *url.URL
	secret  string
	client  *http.Client
}

func NewPiGatewayClient(rawURL, secret string, client *http.Client) *PiGatewayClient {
	if rawURL == "" {
		rawURL = "http://127.0.0.1:4317"
	}
	parsed, _ := url.Parse(rawURL)
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute}
	}
	return &PiGatewayClient{parsed, secret, client}
}

func (g *PiGatewayClient) BaseURL() string {
	if g == nil || g.baseURL == nil {
		return ""
	}
	return g.baseURL.String()
}
func (g *PiGatewayClient) available() bool {
	return g != nil && g.baseURL != nil && (g.baseURL.Scheme == "http" || g.baseURL.Scheme == "https") && g.baseURL.Host != "" && len(g.secret) >= 16 && g.client != nil
}
func (g *PiGatewayClient) endpoint(path string) string {
	u := *g.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = ""
	return u.String()
}
func (g *PiGatewayClient) Start(ctx context.Context, run PiGatewayRun) (body io.ReadCloser, err error) {
	started := time.Now()
	raw, marshalErr := json.Marshal(piGatewayWireRun{RunID: run.RunID, ConversationID: run.ConversationID, UserID: run.UserID, Message: run.Message, Capability: run.Capability, Profile: run.Profile, History: run.History, Knowledge: run.Knowledge, HistorySummary: run.HistorySummary})
	captureInput, _ := json.Marshal(piGatewayCaptureInput{RunID: run.RunID, ConversationID: run.ConversationID, UserID: run.UserID, Message: run.Message, Profile: run.Profile, History: run.History, Knowledge: run.Knowledge, HistorySummary: run.HistorySummary})
	defer func() {
		captureUsageRecord(ctx, Request{UserID: run.UserID, Capture: run.Capture}, UsageCaptureInput{UserID: run.UserID, Provider: ProviderOpenAICompatible, BaseURL: gatewayBaseURL(g), ModelName: "pi_gateway", PromptName: "pi_gateway_run", CallReason: "pi_gateway_run", CallLoc: "LLM_GATE_001", RequestStartedAt: started, RequestFinishedAt: time.Now(), InputBody: captureInput, ErrorMessage: errorString(err), Metadata: map[string]any{"capture_semantics": "start_request_audit"}}, loggerutil.CreateDefaultLogger("20260920-202"))
	}()
	if !g.available() {
		return nil, errors.New("Pi gateway unavailable")
	}
	if marshalErr != nil {
		return nil, marshalErr
	}
	if len(raw) > piGatewayMaxRequestBytes {
		return nil, errors.New("Pi run request too large")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint("/v1/runs"), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("Pi gateway refused run (%d)", response.StatusCode)
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/x-ndjson") {
		response.Body.Close()
		return nil, errors.New("Pi gateway returned unexpected stream type")
	}
	return response.Body, nil
}
func (g *PiGatewayClient) postControl(ctx context.Context, path string, payload any) error {
	if !g.available() {
		return errors.New("Pi gateway unavailable")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint(path), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Pi control refused (%d)", resp.StatusCode)
	}
	return nil
}
func (g *PiGatewayClient) Cancel(ctx context.Context, runID string) error {
	return g.postControl(ctx, "/v1/runs/"+url.PathEscape(runID)+"/cancel", map[string]any{})
}
func (g *PiGatewayClient) Decide(ctx context.Context, runID, requestID string, allowed bool) error {
	return g.postControl(ctx, "/v1/runs/"+url.PathEscape(runID)+"/permissions/"+url.PathEscape(requestID), map[string]bool{"allowed": allowed})
}

// ModelInfo asks the gateway for a model's limits. It returns
// ErrPiGatewayModelUnknown when the gateway does not know the pair.
func (g *PiGatewayClient) ModelInfo(ctx context.Context, provider, model string) (PiGatewayModelInfo, error) {
	var info PiGatewayModelInfo
	if !g.available() {
		return info, errors.New("Pi gateway unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.endpoint("/v1/models/"+url.PathEscape(provider)+"/"+url.PathEscape(model)), nil)
	if err != nil {
		return info, err
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	resp, err := g.client.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return info, ErrPiGatewayModelUnknown
	}
	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("Pi gateway refused model info (%d)", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16*1024)).Decode(&info); err != nil {
		return info, err
	}
	if info.ContextWindow <= 0 {
		return info, errors.New("Pi gateway returned no context window")
	}
	return info, nil
}

// Summarize asks the gateway to fold earlier turns into a rolling summary
// using the conversation's own model, and returns the summary text.
func (g *PiGatewayClient) Summarize(ctx context.Context, in PiGatewaySummaryRequest) (summary string, err error) {
	started := time.Now()
	raw, marshalErr := json.Marshal(in)
	defer func() {
		captureUsageRecord(ctx, Request{UserID: in.UserID, Capture: in.Capture}, UsageCaptureInput{UserID: in.UserID, Provider: ProviderOpenAICompatible, BaseURL: gatewayBaseURL(g), ModelName: in.Provider + "/" + in.Model, PromptName: "pi_gateway_summary", CallReason: "pi_gateway_summary", CallLoc: "LLM_GATE_002", RequestStartedAt: started, RequestFinishedAt: time.Now(), InputBody: raw, OutputBody: []byte(summary), ErrorMessage: errorString(err)}, loggerutil.CreateDefaultLogger("20261004-417"))
	}()
	if !g.available() {
		return "", errors.New("Pi gateway unavailable")
	}
	if marshalErr != nil {
		return "", marshalErr
	}
	if len(raw) > piGatewayMaxRequestBytes {
		return "", errors.New("Pi summary request too large")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint("/v1/summaries"), bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Pi gateway refused summary (%d)", resp.StatusCode)
	}
	var out struct {
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256*1024)).Decode(&out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Summary) == "" {
		return "", errors.New("Pi gateway returned an empty summary")
	}
	return strings.TrimSpace(out.Summary), nil
}

func gatewayBaseURL(g *PiGatewayClient) string {
	if g == nil || g.baseURL == nil {
		return ""
	}
	return g.baseURL.String()
}
