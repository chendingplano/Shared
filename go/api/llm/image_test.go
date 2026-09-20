package llm

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type captureSinkFunc func(context.Context, UsageCaptureRecord) (string, error)

func (f captureSinkFunc) Capture(ctx context.Context, record UsageCaptureRecord) (string, error) {
	return f(ctx, record)
}

func TestDashScopeResponseLimitsAndExplicitOversizeErrors(t *testing.T) {
	if dashScopeSubmitResponseMaxBytes != 10<<20 {
		t.Fatalf("submit limit = %d, want %d", dashScopeSubmitResponseMaxBytes, 10<<20)
	}
	if dashScopePollResponseMaxBytes != 40<<20 {
		t.Fatalf("poll limit = %d, want %d", dashScopePollResponseMaxBytes, 40<<20)
	}
	for _, test := range []struct {
		name  string
		limit int64
		label string
	}{
		{"submit", dashScopeSubmitResponseMaxBytes, "DashScope submit response too large"},
		{"poll", dashScopePollResponseMaxBytes, "DashScope poll response too large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readImageResponse(strings.NewReader(strings.Repeat("x", int(test.limit))), test.limit, test.label); err != nil {
				t.Fatalf("exact limit rejected: %v", err)
			}
			if _, err := readImageResponse(strings.NewReader(strings.Repeat("x", int(test.limit+1))), test.limit, test.label); err == nil || err.Error() != test.label {
				t.Fatalf("oversize error = %v, want %q", err, test.label)
			}
		})
	}
}

func TestImageClientDashScopeUses150SecondDeadline(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		deadline, ok := request.Context().Deadline()
		if !ok || time.Until(deadline) < 149*time.Second || time.Until(deadline) > 151*time.Second {
			t.Errorf("unexpected deadline: %v", deadline)
		}
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("no quota")), Header: make(http.Header)}, nil
	})}
	_, _ = NewImageClient("https://dashscope.example", "secret", client).Generate(context.Background(), ImageRequest{Style: ImageStyleDashScope, Model: "wan", Prompt: "draw"})
}

func TestImageClientOpenAICompatibleCapturesSuccess(t *testing.T) {
	var gotPath, gotUser string
	sink := captureSinkFunc(func(_ context.Context, record UsageCaptureRecord) (string, error) {
		gotUser = record.UserID
		return "event-1", nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("png")) + `"}]}`))
	}))
	defer server.Close()

	response, err := NewImageClient(server.URL, "secret", server.Client()).Generate(context.Background(), ImageRequest{Style: ImageStyleOpenAICompatible, Model: "image-1", Prompt: "draw", Capture: &RequestCapture{UserID: "user-1", Sink: sink}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/images/generations" || gotUser != "user-1" || string(response.Data) != "png" || response.Usage.EventID != "event-1" {
		t.Fatalf("unexpected response: path=%q user=%q response=%+v", gotPath, gotUser, response)
	}
}

func TestImageClientDashScopeCapturesFailure(t *testing.T) {
	var captured UsageCaptureRecord
	sink := captureSinkFunc(func(_ context.Context, record UsageCaptureRecord) (string, error) {
		captured = record
		return "event-2", nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no quota", http.StatusTooManyRequests) }))
	defer server.Close()

	_, err := NewImageClient(server.URL, "secret", server.Client()).Generate(context.Background(), ImageRequest{Style: ImageStyleDashScope, Model: "wan", Prompt: "draw", Capture: &RequestCapture{UserID: "user-2", Sink: sink}})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if captured.UserID != "user-2" || captured.Provider != ProviderOpenAICompatible || captured.ErrorMessage == "" {
		t.Fatalf("unexpected capture: %+v", captured)
	}
}
