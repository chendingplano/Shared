package llm

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeLogprobServer answers each chat completion with top_logprobs chosen by
// pick(prompt) and records every request body it receives.
func fakeLogprobServer(t *testing.T, pick func(prompt string) map[string]float64) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "bad path "+r.URL.Path, http.StatusNotFound)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		msg := body["messages"].([]any)[0].(map[string]any)
		prompt, _ := msg["content"].(string)
		if prompt == "" {
			for _, p := range msg["content"].([]any) {
				if text, ok := p.(map[string]any)["text"].(string); ok {
					prompt += text
				}
			}
		}
		var top []map[string]any
		for tok, p := range pick(prompt) {
			top = append(top, map[string]any{"token": tok, "logprob": math.Log(p)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cmpl-1",
			"choices": []any{map[string]any{
				"logprobs": map[string]any{"content": []any{map[string]any{"top_logprobs": top}}},
			}},
			"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 1, "total_tokens": 101},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

func TestJevEmulatedCompleteAllQuestionTypes(t *testing.T) {
	srv, bodies := fakeLogprobServer(t, func(prompt string) map[string]float64 {
		switch {
		case strings.Contains(prompt, "urgency"): // [A] yes, [B] no
			return map[string]float64{"A": 0.9, " A": 0.05, "B": 0.05}
		case strings.Contains(prompt, "Which team"): // sorted: billing, sales, technical
			return map[string]float64{"C": 0.85, "A": 0.15, "B": 1e-9, "Hello": 1e-9}
		default: // levels: Calm, Frustrated, Very angry
			return map[string]float64{"A": 1e-9, "B": 0.95, "C": 0.05}
		}
	})
	client, err := NewClient(ProviderConfig{ID: ProviderJevEmulated, BaseURL: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Complete(context.Background(), Request{
		Model:    "local",
		Messages: []Message{{Role: RoleUser, Content: "Help! My Stripe integration has failed for 3 days."}},
		JevQuestions: JevQuestions{
			"is_urgent": {Type: "noul", Instructions: "Does this convey urgency?"},
			"department": {Type: "choice", Instructions: "Which team should handle this?", Criteria: map[string]string{
				"billing": "Payments", "technical": "Bugs and integrations", "sales": "Pricing",
			}},
			"frustration": {Type: "score", Instructions: "How frustrated is the customer?", Criteria: []string{"Calm", "Frustrated", "Very angry"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	answers, err := ParseJevAnswers(resp.Content)
	if err != nil {
		t.Fatal(err)
	}
	near := func(name string, got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	near("noul", *answers["is_urgent"].Noul, 0.95)
	dept := answers["department"]
	if dept.Choice != "technical" {
		t.Errorf("choice = %q, want technical", dept.Choice)
	}
	near("technical prob", dept.Probabilities["technical"], 0.85)
	near("choice confidence", *dept.Confidence, (0.85-1.0/3)/(1-1.0/3))
	fr := answers["frustration"]
	near("score", *fr.Score, 1.05)
	near("score confidence", *fr.Confidence, 1-0.05/(2.0/3))
	if fr.Legend["2"] != "Very angry" {
		t.Errorf("legend = %v", fr.Legend)
	}
	if resp.Usage.InputTokens != 300 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if len(*bodies) != 3 {
		t.Fatalf("calls = %d, want 3", len(*bodies))
	}
	b := (*bodies)[0]
	if b["max_tokens"] != float64(1) || b["logprobs"] != true || b["top_logprobs"] != float64(20) || b["temperature"] != float64(0) {
		t.Errorf("request body = %v", b)
	}
}

func TestJevEmulatedStateLeadsEveryPrompt(t *testing.T) {
	head1, _ := jevEmulatedPrompt("S", "Q1", []jevOption{{label: "yes"}, {label: "no"}})
	head2, tail2 := jevEmulatedPrompt("S", "Q2", []jevOption{{label: "x", description: "ex"}, {label: "y"}})
	if head1 != head2 || !strings.HasPrefix(head1, "State:\nS") {
		t.Fatalf("heads differ: %q vs %q", head1, head2)
	}
	if !strings.Contains(tail2, "[A] x: ex\n[B] y\n") {
		t.Fatalf("tail = %q", tail2)
	}
}

func TestJevEmulatedImagesAreAttached(t *testing.T) {
	srv, bodies := fakeLogprobServer(t, func(string) map[string]float64 {
		return map[string]float64{"A": 0.99, "B": 0.01}
	})
	client, _ := NewClient(ProviderConfig{ID: ProviderJevEmulated, BaseURL: srv.URL + "/v1"}, nil)
	_, err := client.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Parts: []ContentPart{
			{Type: "text", Text: "Inspect this webcam frame."},
			{Type: "image_b64", ImageB64: "AAAA", MIME: "image/png"},
		}}},
		JevQuestions: JevQuestions{"person": {Type: "noul", Instructions: "Is a person visible?"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := (*bodies)[0]["messages"].([]any)[0].(map[string]any)["content"].([]any)
	img := parts[1].(map[string]any)["image_url"].(map[string]any)["url"]
	if len(parts) != 3 || img != "data:image/png;base64,AAAA" {
		t.Fatalf("content parts = %v", parts)
	}
}

func TestJevEmulatedNormalize(t *testing.T) {
	// "[A" and "a" both count for A; the junk token sets the cutoff.
	got, err := jevEmulatedNormalize(map[string]float64{"[A": math.Log(0.3), "a": math.Log(0.3), "B": math.Log(0.4), "zz": -30}, 2)
	if err != nil || math.Abs(got[0]-0.6) > 1e-9 || math.Abs(got[1]-0.4) > 1e-9 {
		t.Fatalf("got %v, %v", got, err)
	}
	// C is missing but could hold up to e^-30 relative mass: negligible.
	if _, err := jevEmulatedNormalize(map[string]float64{"A": 0, "B": -1, "x": -30}, 3); err != nil {
		t.Fatalf("negligible missing option rejected: %v", err)
	}
	// C is missing and the cutoff is high, so it could matter.
	if _, err := jevEmulatedNormalize(map[string]float64{"A": 0, "B": -1, "x": -2}, 3); err == nil {
		t.Fatal("expected error for non-negligible missing option")
	}
	if _, err := jevEmulatedNormalize(map[string]float64{"Sure": 0}, 2); err == nil {
		t.Fatal("expected error when no option letter is returned")
	}
}

func TestJevEmulatedErrors(t *testing.T) {
	if _, err := NewClient(ProviderConfig{ID: ProviderJevEmulated}, nil); !errors.Is(err, ErrMissingBaseURL) {
		t.Fatalf("missing BaseURL error = %v", err)
	}
	if _, err := NewClient(ProviderConfig{ID: ProviderJevEmulated, BaseURL: "http://x", Extra: map[string]string{"top_logprobs": "0"}}, nil); err == nil {
		t.Fatal("expected error for top_logprobs=0")
	}
	if _, err := NewClient(ProviderConfig{ID: ProviderJevEmulated, BaseURL: "http://x", Extra: map[string]string{"thinking": "off"}}, nil); err == nil {
		t.Fatal("expected error for thinking=off")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A"}}]}`))
	}))
	t.Cleanup(srv.Close)
	client, _ := NewClient(ProviderConfig{ID: ProviderJevEmulated, BaseURL: srv.URL}, nil)
	base := Request{Messages: []Message{{Role: RoleUser, Content: "state"}}}
	for name, q := range map[string]JevQuestion{
		"no logprobs":     {Type: "noul", Instructions: "Q?"},
		"one option":      {Type: "choice", Instructions: "Q?", Criteria: map[string]string{"a": ""}},
		"noul criteria":   {Type: "noul", Instructions: "Q?", Criteria: map[string]string{"true": "t"}},
		"no instructions": {Type: "score", Criteria: []string{"lo", "hi"}},
	} {
		req := base
		req.JevQuestions = JevQuestions{"q": q}
		if _, err := client.Complete(context.Background(), req); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestJevEmulatedThinking(t *testing.T) {
	srv, bodies := fakeLogprobServer(t, func(string) map[string]float64 { return map[string]float64{"A": 0.6, "B": 0.4} })
	q := JevQuestions{"q": {Type: "noul", Instructions: "Q?"}}
	for _, tc := range []struct {
		extra map[string]string
		want  any
	}{
		{nil, nil},
		{map[string]string{"thinking": " Disabled "}, map[string]any{"type": "disabled"}},
	} {
		client, err := NewClient(ProviderConfig{ID: ProviderJevEmulated, BaseURL: srv.URL, Extra: tc.extra}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "s"}}, JevQuestions: q}); err != nil {
			t.Fatal(err)
		}
		got := (*bodies)[len(*bodies)-1]["thinking"]
		if (tc.want == nil) != (got == nil) || (tc.want != nil && got.(map[string]any)["type"] != "disabled") {
			t.Errorf("extra %v: thinking = %v, want %v", tc.extra, got, tc.want)
		}
	}
}
