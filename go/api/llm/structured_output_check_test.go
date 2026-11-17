package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A response can pass the schema yet fail a caller's check against its input
// (a cited row the input does not have). That failure must get the same
// retries as a schema failure, with the check's message in the retry input,
// instead of surfacing at once and sending the caller to a fallback model.
func TestExtractStructuredJSON_RetriesAfterCheckParsedFailure(t *testing.T) {
	responses := []string{
		`{"topics":[{"topic_desc":"alarm","lines":["264#r10"]}]}`,
		`{"topics":[{"topic_desc":"alarm","lines":["264#r9"]}]}`,
	}
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, responses[len(bodies)-1])
	}))
	defer srv.Close()

	contract := testTopicContract()
	contract.MaxRetries = 2
	contract.CheckParsed = func(parsed map[string]any) error {
		line := parsed["topics"].([]any)[0].(map[string]any)["lines"].([]any)[0]
		if line == "264#r10" {
			return errors.New(`span "264#r10" cites a row that line 264 does not have`)
		}
		return nil
	}
	client := &OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", ModelName: "gpt-test", HTTPClient: srv.Client()}
	result, err := client.ExtractStructuredJSON(context.Background(), JSONExtractionInput{PromptText: "prompt", InputText: "text"}, contract)
	if err != nil {
		t.Fatalf("ExtractStructuredJSON() error = %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	if !strings.Contains(bodies[1], `cites a row that line 264 does not have`) {
		t.Fatalf("retry input does not carry the check failure: %s", bodies[1])
	}
	if got := result.Parsed["topics"].([]any)[0].(map[string]any)["lines"].([]any)[0]; got != "264#r9" {
		t.Fatalf("result line = %v, want the retried response", got)
	}
}

func TestExtractStructuredJSON_CheckParsedFailureExhaustsWithRaw(t *testing.T) {
	bad := `{"topics":[{"topic_desc":"alarm","lines":["264#r10"]}]}`
	srv := newStructuredOutputTestServer(t, []string{bad, bad})
	defer srv.Close()

	contract := testTopicContract()
	contract.MaxRetries = 1
	contract.CheckParsed = func(map[string]any) error { return errors.New("row not in input") }
	client := &OpenAIJSONClient{BaseURL: srv.URL(), APIKey: "test-key", ModelName: "gpt-test", HTTPClient: srv.Client()}
	_, err := client.ExtractStructuredJSON(context.Background(), JSONExtractionInput{PromptText: "prompt", InputText: "text"}, contract)
	var structured *StructuredOutputError
	if !errors.As(err, &structured) || !errors.Is(err, ErrStructuredOutputRetriesExhausted) || !errors.Is(err, ErrStructuredOutputCheckFailed) {
		t.Fatalf("error = %v, want retries exhausted wrapping a check failure", err)
	}
	if structured.Raw != bad {
		t.Fatalf("Raw = %q, want the last response so callers can log it", structured.Raw)
	}
	if srv.requests != 2 {
		t.Fatalf("requests = %d, want 2", srv.requests)
	}
}
