package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

type testUsageCaptureSink struct {
	mu      sync.Mutex
	records []UsageCaptureRecord
}

func (s *testUsageCaptureSink) Capture(_ context.Context, record UsageCaptureRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

func (s *testUsageCaptureSink) Records() []UsageCaptureRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]UsageCaptureRecord, len(s.records))
	copy(out, s.records)
	return out
}

func withDefaultUsageCaptureSink(sink UsageCaptureSink, fn func()) {
	original := DefaultUsageCaptureSink
	DefaultUsageCaptureSink = sink
	defer func() { DefaultUsageCaptureSink = original }()
	fn()
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s
}

func TestOpenAICompleteHappyPath(t *testing.T) {
	logger := loggerutil.CreateDefaultLogger("MID-20260708-04")
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-TESTVALUE1234" {
			t.Errorf("Authorization header: got %q", got)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id":"x","object":"chat.completion","model":"gpt-4o-mini",
		  "choices":[{"index":0,"message":{"role":"assistant","content":"hello world"},"finish_reason":"stop"}],
		  "usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
		}`))
	})

	c, err := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Complete(context.Background(), Request{
		Model: "gpt-4o-mini",
		Messages: []Message{
			{Role: RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hello world" {
		t.Errorf("content: got %q", resp.Content)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 5 {
		t.Errorf("usage: %+v", resp.Usage)
	}
}

func TestOpenAICompleteReturnsProviderErrorOn401(t *testing.T) {
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})

	logger := loggerutil.CreateDefaultLogger("MID-20260708-04")
	c, _ := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	}, logger)
	_, err := c.Complete(context.Background(), Request{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ProviderError, got %T %v", err, err)
	}
	if pe.HTTPStatus != 401 {
		t.Errorf("HTTPStatus: got %d", pe.HTTPStatus)
	}
	if !strings.Contains(pe.Body, "bad key") {
		t.Errorf("Body missing error text: %q", pe.Body)
	}
}

func TestOpenAICompleteCapturesUsageRecordOnSuccess(t *testing.T) {
	sink := &testUsageCaptureSink{}
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id":"req_abc","object":"chat.completion","model":"deepseek-v4-flash",
		  "choices":[{"index":0,"message":{"role":"assistant","content":"hello world"},"finish_reason":"stop"}],
		  "usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}
		}`))
	})

	c, _ := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	}, loggerutil.CreateDefaultLogger("MID-20260708-04"))
	_, err := c.Complete(context.Background(), Request{
		Model:      "deepseek-v4-flash",
		PromptName: "extract-products-v2",
		Messages:   []Message{{Role: RoleUser, Content: "hi"}},
		Capture: &RequestCapture{
			AccountID:     "acct_10",
			ProfileID:     "prof_20",
			InputBodyRef:  "archive/in.json.gz",
			OutputBodyRef: "archive/out.json.gz",
			Sink:          sink,
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	got := records[0]
	if got.AccountID != "acct_10" || got.ProfileID != "prof_20" {
		t.Fatalf("unexpected account/profile ids: %+v", got)
	}
	if got.PromptName != "extract-products-v2" || got.ModelName != "deepseek-v4-flash" {
		t.Fatalf("unexpected prompt/model: %+v", got)
	}
	if got.InputTokens != 8 || got.OutputTokens != 3 || got.TotalTokens != 11 {
		t.Fatalf("unexpected tokens: %+v", got)
	}
	if got.InputBodyRef != "archive/in.json.gz" || got.OutputBodyRef != "archive/out.json.gz" {
		t.Fatalf("unexpected refs: %+v", got)
	}
	if !strings.Contains(string(got.InputBody), `"model":"deepseek-v4-flash"`) {
		t.Fatalf("expected serialized request body, got %q", string(got.InputBody))
	}
	if !strings.Contains(string(got.OutputBody), `"content":"hello world"`) {
		t.Fatalf("expected raw response body, got %q", string(got.OutputBody))
	}
}

func TestOpenAICompleteCapturesUsageRecordOnProviderError(t *testing.T) {
	sink := &testUsageCaptureSink{}
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})

	c, _ := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	}, loggerutil.CreateDefaultLogger("MID-20260708-04"))
	_, err := c.Complete(context.Background(), Request{
		Model:      "deepseek-v4-flash",
		PromptName: "extract-products-v2",
		Messages:   []Message{{Role: RoleUser, Content: "hi"}},
		Capture: &RequestCapture{
			AccountID: "acct_10",
			ProfileID: "prof_20",
			Sink:      sink,
		},
	})
	if err == nil {
		t.Fatal("expected provider error")
	}

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	if records[0].ErrorMessage == "" {
		t.Fatalf("expected error message on captured record: %+v", records[0])
	}
	if !strings.Contains(string(records[0].OutputBody), "bad key") {
		t.Fatalf("expected raw provider error body, got %q", string(records[0].OutputBody))
	}
}

func TestOpenAICompleteUsesDefaultCaptureSinkWhenRequestCaptureMissing(t *testing.T) {
	sink := &testUsageCaptureSink{}
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id":"req_default","object":"chat.completion","model":"gpt-4o-mini",
		  "choices":[{"index":0,"message":{"role":"assistant","content":"hello world"},"finish_reason":"stop"}],
		  "usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
		}`))
	})

	c, _ := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	}, loggerutil.CreateDefaultLogger("MID-20260708-04"))

	withDefaultUsageCaptureSink(sink, func() {
		_, err := c.Complete(context.Background(), Request{
			Model:      "gpt-4o-mini",
			PromptName: "default-sink-test",
			Messages:   []Message{{Role: RoleUser, Content: "hi"}},
		})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}
	})

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	if records[0].PromptName != "default-sink-test" {
		t.Fatalf("PromptName = %q", records[0].PromptName)
	}
}

func TestOpenAIStreamHappyPath(t *testing.T) {
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`data: {"choices":[{"delta":{"content":"hel"}}]}`,
			`data: {"choices":[{"delta":{"content":"lo "}}]}`,
			`data: {"choices":[{"delta":{"content":"world"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}
		for _, f := range frames {
			_, _ = w.Write([]byte(f + "\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	})

	c, _ := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	}, loggerutil.CreateDefaultLogger("MID-20260708-04"))

	var gotDeltas []string
	doneSeen := false
	var finishReason string
	err := c.Stream(context.Background(), Request{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Stream:   true,
	}, func(ch StreamChunk) error {
		if ch.Delta != "" {
			gotDeltas = append(gotDeltas, ch.Delta)
		}
		if ch.Done {
			doneSeen = true
			finishReason = ch.FinishReason
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	want := []string{"hel", "lo ", "world"}
	if strings.Join(gotDeltas, "") != strings.Join(want, "") {
		t.Errorf("deltas: got %v want %v", gotDeltas, want)
	}
	if !doneSeen {
		t.Error("final Done chunk not delivered")
	}
	if finishReason != "stop" {
		t.Errorf("finish_reason: got %q", finishReason)
	}
}

func TestOpenAIStreamCapturesUsageRecordOnCompletion(t *testing.T) {
	sink := &testUsageCaptureSink{}
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`data: {"choices":[{"delta":{"content":"hel"}}]}`,
			`data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11}}`,
			`data: [DONE]`,
		}
		for _, f := range frames {
			_, _ = w.Write([]byte(f + "\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	})

	c, _ := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	}, loggerutil.CreateDefaultLogger("MID-20260708-04"))
	err := c.Stream(context.Background(), Request{
		Model:      "deepseek-v4-flash",
		PromptName: "extract-products-v2",
		Messages:   []Message{{Role: RoleUser, Content: "hi"}},
		Stream:     true,
		Capture: &RequestCapture{
			AccountID:     "acct_10",
			ProfileID:     "prof_20",
			InputBodyRef:  "archive/in.json.gz",
			OutputBodyRef: "archive/out.json.gz",
			Sink:          sink,
		},
	}, func(ch StreamChunk) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	got := records[0]
	if got.InputTokens != 9 || got.OutputTokens != 2 || got.TotalTokens != 11 {
		t.Fatalf("unexpected tokens: %+v", got)
	}
	if got.OutputBodyRef != "archive/out.json.gz" {
		t.Fatalf("unexpected output ref: %+v", got)
	}
	if !strings.Contains(string(got.OutputBody), "hello") {
		t.Fatalf("expected collected stream output body, got %q", string(got.OutputBody))
	}
}

func TestOpenAIStreamCapturesUsageRecordOnHandlerError(t *testing.T) {
	sink := &testUsageCaptureSink{}
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`data: {"choices":[{"delta":{"content":"hel"}}]}`,
			`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		}
		for _, f := range frames {
			_, _ = w.Write([]byte(f + "\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	})

	c, _ := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	}, loggerutil.CreateDefaultLogger("MID-20260708-04"))
	err := c.Stream(context.Background(), Request{
		Model:      "deepseek-v4-flash",
		PromptName: "extract-products-v2",
		Messages:   []Message{{Role: RoleUser, Content: "hi"}},
		Stream:     true,
		Capture: &RequestCapture{
			AccountID: "acct_10",
			ProfileID: "prof_20",
			Sink:      sink,
		},
	}, func(ch StreamChunk) error {
		if ch.Delta != "" {
			return errors.New("stop early")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected handler error")
	}

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("captured records = %d, want 1", len(records))
	}
	if records[0].ErrorMessage == "" {
		t.Fatalf("expected error message on captured record: %+v", records[0])
	}
}

func TestOpenAIStreamRespectsContextCancel(t *testing.T) {
	s := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"first"}}]}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	})

	c, _ := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	}, loggerutil.CreateDefaultLogger("MID-20260708-04"))
	ctx, cancel := context.WithCancel(context.Background())
	var firstSeen bool
	done := make(chan error, 1)
	go func() {
		done <- c.Stream(ctx, Request{
			Model:    "gpt-4o-mini",
			Messages: []Message{{Role: RoleUser, Content: "hi"}},
			Stream:   true,
		}, func(ch StreamChunk) error {
			if ch.Delta != "" {
				firstSeen = true
				cancel()
			}
			return nil
		})
	}()

	select {
	case err := <-done:
		if !firstSeen {
			t.Fatal("stream returned before any delta arrived")
		}
		if err == nil {
			t.Fatal("expected cancellation error, got nil")
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("stream did not return within 5s of cancel")
	}
}
