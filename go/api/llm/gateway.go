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
	Capture                                            *RequestCapture
}

type piGatewayWireRun struct {
	RunID          string `json:"runId"`
	ConversationID string `json:"conversationId"`
	UserID         string `json:"userId"`
	Message        string `json:"message"`
	Capability     string `json:"capability"`
	Profile        any    `json:"profile"`
	History        any    `json:"history"`
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
	raw, marshalErr := json.Marshal(piGatewayWireRun{RunID: run.RunID, ConversationID: run.ConversationID, UserID: run.UserID, Message: run.Message, Capability: run.Capability, Profile: run.Profile, History: run.History})
	captureInput, _ := json.Marshal(piGatewayCaptureInput{RunID: run.RunID, ConversationID: run.ConversationID, UserID: run.UserID, Message: run.Message, Profile: run.Profile, History: run.History})
	defer func() {
		captureUsageRecord(ctx, Request{UserID: run.UserID, Capture: run.Capture}, UsageCaptureInput{UserID: run.UserID, Provider: ProviderOpenAICompatible, BaseURL: gatewayBaseURL(g), ModelName: "pi_gateway", PromptName: "pi_gateway_run", CallReason: "pi_gateway_run", CallLoc: "LLM_GATE_001", RequestStartedAt: started, RequestFinishedAt: time.Now(), InputBody: captureInput, ErrorMessage: errorString(err), Metadata: map[string]any{"capture_semantics": "start_request_audit"}}, loggerutil.CreateDefaultLogger("20260920-202"))
	}()
	if !g.available() {
		return nil, errors.New("Pi gateway unavailable")
	}
	if marshalErr != nil {
		return nil, marshalErr
	}
	if len(raw) > 128*1024 {
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
func gatewayBaseURL(g *PiGatewayClient) string {
	if g == nil || g.baseURL == nil {
		return ""
	}
	return g.baseURL.String()
}
