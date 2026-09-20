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
