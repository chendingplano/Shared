package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s
}

func TestOpenAICompleteHappyPath(t *testing.T) {
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
	})
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

	c, _ := NewClient(ProviderConfig{
		ID: ProviderOpenAICompatible, BaseURL: s.URL, APIKey: "sk-TESTVALUE1234",
	})
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
	})

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
	})
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
