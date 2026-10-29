package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testRequestGate holds every call until release is closed, unless err is set.
type testRequestGate struct {
	err     error
	release chan struct{}
	// closedChecks, when > 0, makes Open report closed for that many checks.
	closedChecks atomic.Int32

	mu    sync.Mutex
	calls []RequestGateCall
}

func (g *testRequestGate) Wait(ctx context.Context, call RequestGateCall) error {
	g.mu.Lock()
	g.calls = append(g.calls, call)
	g.mu.Unlock()
	if g.err != nil {
		return g.err
	}
	if g.release == nil {
		return nil
	}
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (g *testRequestGate) Open(context.Context) bool {
	return g.closedChecks.Add(-1) < 0
}

func (g *testRequestGate) Calls() []RequestGateCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]RequestGateCall(nil), g.calls...)
}

func countingServer(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

const gateTestChatBody = `{"choices":[{"message":{"content":"ok"}}]}`

func TestRequestGateHoldsChatUntilReleased(t *testing.T) {
	srv, hits := countingServer(t, gateTestChatBody)
	client := &OpenAIJSONClient{BaseURL: srv.URL, APIKey: "k", ModelName: "gpt-test", HTTPClient: srv.Client()}
	gate := &testRequestGate{release: make(chan struct{})}
	ctx := WithRequestGate(context.Background(), gate)

	done := make(chan error, 1)
	go func() {
		_, err := client.ExtractText(ctx, JSONExtractionInput{
			PromptName: "p", PromptText: "prompt", InputText: "text",
			UserID: "u1", RecordID: 7, RunID: 9, CallReason: "reason", CallLoc: "LOC_1",
		})
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatal("request sent while the gate was closed")
	}
	close(gate.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("requests = %d, want 1", hits.Load())
	}
	calls := gate.Calls()
	if len(calls) != 1 {
		t.Fatalf("gate calls = %d, want 1", len(calls))
	}
	u := calls[0].Usage
	if calls[0].Kind != RequestGateCallChat || u.ModelName != "gpt-test" || u.PromptName != "p" || u.UserID != "u1" ||
		u.RecordID != 7 || u.RunID != 9 || u.CallReason != "reason" || u.CallLoc != "LOC_1" || u.BaseURL != srv.URL {
		t.Fatalf("unexpected gate call: %+v", calls[0])
	}
}

func TestRequestGateCancellationSendsNothing(t *testing.T) {
	srv, hits := countingServer(t, `{"data":[{"embedding":[1]}]}`)
	client := &OpenAIJSONClient{BaseURL: srv.URL, APIKey: "k", ModelName: "emb", HTTPClient: srv.Client()}
	stopped := errors.New("stopped")
	ctx := WithRequestGate(context.Background(), &testRequestGate{err: stopped})

	if _, err := client.Embed(ctx, EmbedInput{InputText: "a"}); !errors.Is(err, stopped) {
		t.Fatalf("Embed err = %v", err)
	}
	if _, err := client.EmbedBatch(ctx, EmbedBatchInput{InputTexts: []string{"a", "b"}}); !errors.Is(err, stopped) {
		t.Fatalf("EmbedBatch err = %v", err)
	}
	if _, err := client.ExtractStructuredJSON(ctx, JSONExtractionInput{PromptText: "p", InputText: "t"}, StructuredOutputContract{Name: "c", Schema: []byte(`{"type":"object"}`)}); !errors.Is(err, stopped) {
		t.Fatalf("ExtractStructuredJSON err = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("requests = %d, want 0", hits.Load())
	}
}

func TestRequestGateRechecksAfterPermit(t *testing.T) {
	srv, hits := countingServer(t, `{"data":[{"embedding":[1]}]}`)
	client := &OpenAIJSONClient{BaseURL: srv.URL, APIKey: "k", ModelName: "emb", HTTPClient: srv.Client()}
	gate := &testRequestGate{}
	gate.closedChecks.Store(1) // the span closes while the call waits for its permit
	ctx := WithRequestGate(context.Background(), gate)

	if _, err := client.Embed(ctx, EmbedInput{InputText: "a", CallLoc: "LOC_E"}); err != nil {
		t.Fatal(err)
	}
	calls := gate.Calls()
	if len(calls) != 2 || calls[0].Kind != RequestGateCallEmbedding || calls[0].Usage.CallLoc != "LOC_E" {
		t.Fatalf("gate calls = %+v, want two embedding waits", calls)
	}
	if hits.Load() != 1 {
		t.Fatalf("requests = %d, want 1", hits.Load())
	}
}

func TestRequestGateHoldsChatClients(t *testing.T) {
	srv, hits := countingServer(t, `{"choices":[{"message":{"content":"ok"}}],"content":[{"type":"text","text":"ok"}]}`)
	stopped := errors.New("stopped")
	for _, id := range []ProviderID{ProviderOpenAI, ProviderAnthropic} {
		client, err := NewClient(ProviderConfig{ID: id, APIKey: "k", BaseURL: srv.URL}, nil)
		if err != nil {
			t.Fatal(err)
		}
		gate := &testRequestGate{err: stopped}
		ctx := WithRequestGate(context.Background(), gate)
		req := Request{Model: "m", PromptName: "p", RecordID: 3, CallLoc: "LOC_C", Messages: []Message{{Role: RoleUser, Content: "hi"}}}
		if _, err := client.Complete(ctx, req); !errors.Is(err, stopped) {
			t.Fatalf("%s Complete err = %v", id, err)
		}
		if err := client.Stream(ctx, req, func(StreamChunk) error { return nil }); !errors.Is(err, stopped) {
			t.Fatalf("%s Stream err = %v", id, err)
		}
		if calls := gate.Calls(); len(calls) != 2 || calls[0].Usage.RecordID != 3 || calls[0].Usage.CallLoc != "LOC_C" {
			t.Fatalf("%s gate calls = %+v", id, calls)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("requests = %d, want 0", hits.Load())
	}
}

func TestNoRequestGateLeavesClientUnchanged(t *testing.T) {
	srv, hits := countingServer(t, gateTestChatBody)
	client := &OpenAIJSONClient{BaseURL: srv.URL, APIKey: "k", ModelName: "gpt-test", HTTPClient: srv.Client()}
	if _, err := client.ExtractText(context.Background(), JSONExtractionInput{PromptText: "p", InputText: "t"}); err != nil {
		t.Fatal(err)
	}
	if WithRequestGate(context.Background(), nil) != context.Background() {
		t.Fatal("nil gate changed the context")
	}
	if hits.Load() != 1 {
		t.Fatalf("requests = %d, want 1", hits.Load())
	}
}
