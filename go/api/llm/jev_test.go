package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestJevQuestionsWireCriteria(t *testing.T) {
	q := JevQuestions{
		"q1": {Type: "noul", Instructions: "Answer yes or no"},
		"q2": {Type: "choice", Instructions: "Select one", Criteria: map[string]string{"a": "Alpha"}},
		"q3": {Type: "score", Instructions: "Rate this", Criteria: []string{"Low", "High"}},
	}
	got, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"q1":{"type":"noul","instructions":"Answer yes or no"},"q2":{"type":"choice","instructions":"Select one","criteria":{"a":"Alpha"}},"q3":{"type":"score","instructions":"Rate this","criteria":["Low","High"]}}`
	if string(got) != want {
		t.Fatalf("wire = %s, want %s", got, want)
	}
}

func TestJevQuestionValidation(t *testing.T) {
	cases := []JevQuestion{
		{Type: "other"}, {Type: "noul", Criteria: map[string]string{"x": "X"}},
		{Type: "choice"}, {Type: "choice", Criteria: []string{"x"}},
		{Type: "score"}, {Type: "score", Criteria: map[string]string{"x": "X"}},
	}
	for _, q := range cases {
		if _, err := json.Marshal(q); err == nil {
			t.Errorf("Marshal(%+v) succeeded", q)
		}
	}
}

func TestNewJevClientAPIKeyResolution(t *testing.T) {
	old, had := os.LookupEnv("JEV_AI_API_KEY")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("JEV_AI_API_KEY", old)
		} else {
			_ = os.Unsetenv("JEV_AI_API_KEY")
		}
	})
	_ = os.Setenv("JEV_AI_API_KEY", "   ")
	if _, err := NewClient(ProviderConfig{ID: ProviderJev}, nil); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("whitespace key error = %v", err)
	}
	_ = os.Unsetenv("JEV_AI_API_KEY")
	if _, err := NewClient(ProviderConfig{ID: ProviderJev}, nil); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("unset key error = %v", err)
	}
	_ = os.Setenv("JEV_AI_API_KEY", "  env-secret  ")
	c, err := NewClient(ProviderConfig{ID: ProviderJev}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.(*jevClient).cfg.APIKey != "env-secret" {
		t.Fatalf("resolved key = %q", c.(*jevClient).cfg.APIKey)
	}
	c, err = NewClient(ProviderConfig{ID: ProviderJev, APIKey: "explicit"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.(*jevClient).cfg.APIKey != "explicit" {
		t.Fatalf("explicit key lost: %q", c.(*jevClient).cfg.APIKey)
	}
}

func TestJevCompleteRequestAndResponse(t *testing.T) {
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		var body struct {
			Model     string       `json:"model"`
			State     string       `json:"state"`
			Questions JevQuestions `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "model-x" || body.State != "last state" || len(body.Questions) != 1 {
			t.Errorf("body = %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"answers":{"a":1},"usage":{"input_tokens":7,"output_tokens":3}}`)
	}))
	defer srv.Close()
	c, err := NewClient(ProviderConfig{ID: ProviderJev, APIKey: "secret", BaseURL: srv.URL + "///", HTTPClient: srv.Client()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Complete(context.Background(), Request{Model: "model-x", Messages: []Message{{Role: RoleUser, Content: "earlier"}, {Role: RoleUser, Content: "last state"}}, JevQuestions: JevQuestions{"id": {Type: "noul", Instructions: "Do it"}}})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/systemone" || auth != "Bearer secret" {
		t.Fatalf("path=%q auth=%q", path, auth)
	}
	if resp.Content != `{"a":1}` || resp.Usage == nil || resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 3 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestJevCompleteValidationAndErrors(t *testing.T) {
	c, _ := NewClient(ProviderConfig{ID: ProviderJev, APIKey: "key", BaseURL: "http://example.invalid"}, nil)
	for _, req := range []Request{{Messages: []Message{{Role: RoleUser, Content: "x"}}}, {JevQuestions: JevQuestions{"id": {Type: "noul", Instructions: "Do it"}}}, {JevQuestions: JevQuestions{"id": {Type: "noul", Instructions: "Do it"}}, Messages: []Message{{Role: RoleUser, Content: "  "}}}, {JevQuestions: JevQuestions{"id": {Type: "noul", Instructions: "Do it"}}, Messages: []Message{{Role: RoleUser, Content: "x", Parts: []ContentPart{{Type: "text"}}}}}} {
		if _, err := c.Complete(context.Background(), req); err == nil {
			t.Errorf("Complete(%+v) succeeded", req)
		}
	}
}

func TestJevErrorBodyTruncatedAndDoesNotExposeRequest(t *testing.T) {
	secret := "sensitive-question-content"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, strings.Repeat("x", 900))
	}))
	defer srv.Close()
	c, _ := NewClient(ProviderConfig{ID: ProviderJev, APIKey: "key", BaseURL: srv.URL, HTTPClient: srv.Client()}, nil)
	_, err := c.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: secret}}, JevQuestions: JevQuestions{"id": {Type: "noul", Instructions: "Do it"}}})
	var pe *ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v", err)
	}
	if len(pe.Body) > 512 {
		t.Fatalf("body length %d", len(pe.Body))
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("request leaked in error: %v", err)
	}
}

type jevRoundTripper func(*http.Request) (*http.Response, error)

func (f jevRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type jevErrorReader struct{}

func (jevErrorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (jevErrorReader) Close() error             { return nil }

func TestJevProviderErrorsWrapTransportAndReadFailures(t *testing.T) {
	for name, client := range map[string]*http.Client{
		"transport": {Transport: jevRoundTripper(func(*http.Request) (*http.Response, error) { return nil, errors.New("dial failed") })},
		"read": {Transport: jevRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 502, Body: jevErrorReader{}}, nil
		})},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := NewClient(ProviderConfig{ID: ProviderJev, APIKey: "hidden-key", BaseURL: "http://example.test", HTTPClient: client}, nil)
			_, err := c.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: RoleUser, Content: "private state"}}, JevQuestions: JevQuestions{"id": {Type: "noul"}}})
			var pe *ProviderError
			if !errors.As(err, &pe) {
				t.Fatalf("expected ProviderError, got %v", err)
			}
			if strings.Contains(err.Error(), "private state") || strings.Contains(err.Error(), "hidden-key") {
				t.Fatalf("request leaked: %v", err)
			}
			if name == "read" && pe.HTTPStatus != 502 {
				t.Fatalf("status = %d", pe.HTTPStatus)
			}
		})
	}
}

func TestJevMalformedResponseIsProviderError(t *testing.T) {
	for _, body := range []string{"not-json", `{"answers":[]}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); io.WriteString(w, body) }))
		c, _ := NewClient(ProviderConfig{ID: ProviderJev, APIKey: "key", BaseURL: srv.URL, HTTPClient: srv.Client()}, nil)
		_, err := c.Complete(context.Background(), Request{Model: "m", Messages: []Message{{Role: RoleUser, Content: "private"}}, JevQuestions: JevQuestions{"id": {Type: "noul"}}})
		srv.Close()
		var pe *ProviderError
		if !errors.As(err, &pe) {
			t.Fatalf("body %q: expected ProviderError, got %v", body, err)
		}
		if pe.HTTPStatus != 200 || pe.Body != body {
			t.Fatalf("ProviderError = %+v", pe)
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatalf("request leaked: %v", err)
		}
	}
}

func TestJevStreamUnsupported(t *testing.T) {
	c, _ := NewClient(ProviderConfig{ID: ProviderJev, APIKey: "key"}, nil)
	err := c.Stream(context.Background(), Request{}, nil)
	if err == nil || !strings.Contains(err.Error(), "streaming is not supported") {
		t.Fatalf("Stream error = %v", err)
	}
}
