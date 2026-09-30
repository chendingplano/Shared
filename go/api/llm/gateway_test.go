package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPiGatewayDelegatesRunAndCapturesUser(t *testing.T) {
	var captured UsageCaptureRecord
	var payload map[string]json.RawMessage
	sink := captureSinkFunc(func(_ context.Context, record UsageCaptureRecord) (string, error) {
		captured = record
		return "event-gateway", nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs" || r.Header.Get("Authorization") != "Bearer private-secret-private-secret" {
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"type\":\"done\"}\n"))
	}))
	defer server.Close()

	body, err := NewPiGatewayClient(server.URL, "private-secret-private-secret", server.Client()).Start(context.Background(), PiGatewayRun{RunID: "run-1", ConversationID: "conversation-1", UserID: "user-1", Message: "hello", Capability: "capability", Profile: map[string]string{"slug": "help"}, History: []map[string]string{{"role": "user", "content": "earlier"}}, Capture: &RequestCapture{UserID: "user-1", Sink: sink}})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	_, _ = io.ReadAll(body)
	if captured.UserID != "user-1" || captured.Provider != ProviderOpenAICompatible {
		t.Fatalf("unexpected capture: %+v", captured)
	}
	for _, key := range []string{"runId", "conversationId", "userId", "message", "capability", "profile", "history"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("missing wire key %q: %s", key, payload)
		}
	}
	if _, ok := payload["Capture"]; ok {
		t.Fatalf("capture leaked into gateway body: %s", payload)
	}
	if string(payload["capability"]) != `"capability"` {
		t.Fatalf("capability missing from outbound body: %s", payload)
	}
	if strings.Contains(string(captured.InputBody), "capability") || strings.Contains(string(captured.InputBody), "Capability") {
		t.Fatalf("capability leaked into captured body: %s", captured.InputBody)
	}
	if captured.Metadata["capture_semantics"] != "start_request_audit" {
		t.Fatalf("capture metadata = %#v", captured.Metadata)
	}
}

func TestPiGatewaySendsKnowledgeOnlyWhenSet(t *testing.T) {
	for _, tc := range []struct {
		name      string
		knowledge any
		want      bool
	}{
		{"absent", nil, false},
		{"present", map[string]any{"defaultStoreId": "7"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var payload map[string]json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = w.Write([]byte("{\"type\":\"done\"}\n"))
			}))
			defer server.Close()
			body, err := NewPiGatewayClient(server.URL, "private-secret-private-secret", server.Client()).Start(context.Background(), PiGatewayRun{RunID: "run-1", ConversationID: "conversation-1", UserID: "user-1", Message: "hello", Capability: "capability", Profile: map[string]string{"slug": "help"}, History: []any{}, Knowledge: tc.knowledge})
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.ReadAll(body)
			_ = body.Close()
			if _, ok := payload["knowledge"]; ok != tc.want {
				t.Fatalf("knowledge present=%v, want %v: %s", ok, tc.want, payload["knowledge"])
			}
		})
	}
}

func TestPiGatewaySendsHistorySummaryOnlyWhenSet(t *testing.T) {
	var payloads []map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&payload)
		payloads = append(payloads, payload)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{}\n"))
	}))
	defer server.Close()
	client := NewPiGatewayClient(server.URL, "private-secret-private-secret", server.Client())
	for _, summary := range []string{"", "earlier turns"} {
		body, err := client.Start(context.Background(), PiGatewayRun{RunID: "r", ConversationID: "c", UserID: "u", Message: "m", Capability: "cap", HistorySummary: summary})
		if err != nil {
			t.Fatal(err)
		}
		body.Close()
	}
	if _, ok := payloads[0]["historySummary"]; ok {
		t.Fatalf("empty summary was sent: %s", payloads[0])
	}
	if string(payloads[1]["historySummary"]) != `"earlier turns"` {
		t.Fatalf("summary missing: %s", payloads[1])
	}
}

func TestPiGatewayModelInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer private-secret-private-secret" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Path == "/v1/models/anthropic/claude-sonnet-4-5" {
			_, _ = w.Write([]byte(`{"provider":"anthropic","model":"claude-sonnet-4-5","contextWindow":200000,"maxTokens":64000}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := NewPiGatewayClient(server.URL, "private-secret-private-secret", server.Client())
	info, err := client.ModelInfo(context.Background(), "anthropic", "claude-sonnet-4-5")
	if err != nil || info.ContextWindow != 200000 || info.MaxTokens != 64000 {
		t.Fatalf("info = %+v, err = %v", info, err)
	}
	if _, err := client.ModelInfo(context.Background(), "anthropic", "nope"); err != ErrPiGatewayModelUnknown {
		t.Fatalf("unknown model err = %v", err)
	}
}

func TestPiGatewaySummarize(t *testing.T) {
	var captured UsageCaptureRecord
	sink := captureSinkFunc(func(_ context.Context, record UsageCaptureRecord) (string, error) {
		captured = record
		return "event-summary", nil
	})
	var got PiGatewaySummaryRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/summaries" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"summary":"  folded  "}`))
	}))
	defer server.Close()
	client := NewPiGatewayClient(server.URL, "private-secret-private-secret", server.Client())
	summary, err := client.Summarize(context.Background(), PiGatewaySummaryRequest{UserID: "u", Provider: "anthropic", Model: "claude-sonnet-4-5", SystemPrompt: "sum", PreviousSummary: "old", Messages: []PiGatewayMessage{{Role: "user", Content: "q"}, {Role: "assistant", Content: "a"}}, MaxTokens: 500, Capture: &RequestCapture{UserID: "u", Sink: sink}})
	if err != nil || summary != "folded" {
		t.Fatalf("summary = %q, err = %v", summary, err)
	}
	if got.PreviousSummary != "old" || len(got.Messages) != 2 || got.MaxTokens != 500 || got.SystemPrompt != "sum" {
		t.Fatalf("request = %+v", got)
	}
	if captured.PromptName != "pi_gateway_summary" || captured.UserID != "u" {
		t.Fatalf("capture = %+v", captured)
	}
}
