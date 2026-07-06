// Package llm provides a provider-agnostic client for calling large language
// model APIs (OpenAI, Anthropic, Gemini, and OpenAI-compatible endpoints such
// as Ollama or DeepSeek). Each adapter speaks the provider's REST API
// directly using net/http + encoding/json; no heavy vendor SDKs are pulled
// into the shared library.
//
// A typical caller looks like:
//
//	client, err := llm.NewClient(llm.ProviderConfig{
//	    ID:     llm.ProviderOpenAI,
//	    APIKey: key,
//	})
//	if err != nil { ... }
//	err = client.Stream(ctx, req, func(c llm.StreamChunk) error {
//	    fmt.Print(c.Delta)
//	    return nil
//	})
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// ProviderID enumerates supported providers.
type ProviderID string

const (
	ProviderOpenAI           ProviderID = "openai"
	ProviderAnthropic        ProviderID = "anthropic"
	ProviderGemini           ProviderID = "gemini"
	ProviderOpenAICompatible ProviderID = "openai_compatible"
)

// ProviderConfig is the input to NewClient.
type ProviderConfig struct {
	ID          ProviderID
	BaseURL     string            // empty → adapter default
	APIKey      string            // plaintext; never logged
	ProfileName string            // logical name for usage capture resolution
	HTTPClient  *http.Client      // optional injection for tests
	Extra       map[string]string // provider-specific knobs
}

// Role identifies the author of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentPart represents one segment of a multimodal message.
type ContentPart struct {
	Type     string // "text" | "image_url" | "image_b64"
	Text     string
	ImageURL string
	ImageB64 string
	MIME     string
}

// ToolCall is the adapter-neutral view of an assistant tool invocation.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON string
}

// ToolDef describes a tool that the model may call.
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema fragment
}

// Message is one entry in a chat conversation.
type Message struct {
	Role       Role
	Content    string        // preferred for plain text
	Parts      []ContentPart // optional multimodal parts
	ToolCalls  []ToolCall    // assistant-emitted
	ToolCallID string        // tool-role reply correlation
}

// Usage carries token-count accounting when a provider supplies it.
type Usage struct {
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
}

// Request describes one inference call.
type Request struct {
	Model       string
	PromptName  string
	RecordID    int64
	CallReason  string
	CallLoc     string
	Metadata    map[string]any
	Messages    []Message
	Capture     *RequestCapture
	Temperature *float64
	MaxTokens   *int
	TopP        *float64
	Stream      bool
	Tools       []ToolDef
	ToolChoice  string
}

// Response is the result of a non-streaming call.
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Raw       json.RawMessage // raw provider body for debugging
	Usage     *Usage
}

// StreamChunk is one incremental event emitted during a streaming call.
// At end of stream the adapter emits a final chunk with Done == true.
type StreamChunk struct {
	Delta        string
	ToolCall     *ToolCall
	Done         bool
	FinishReason string
	Usage        *Usage
}

// StreamHandler is the callback shape for Client.Stream. Returning an error
// cancels the stream and propagates the error to the caller.
type StreamHandler func(StreamChunk) error

// Client is the adapter-neutral interface used by callers.
type Client interface {
	Complete(ctx context.Context, req Request) (*Response, error)
	Stream(ctx context.Context, req Request, on StreamHandler) error
}

// StructuredOutputContract declares the machine-readable output shape expected
// from a JSON-producing LLM call.
type StructuredOutputContract struct {
	Name              string
	Schema            json.RawMessage
	AllowRepair       bool
	MaxRetries        int
	DisallowExtraKeys bool
}

// Validate ensures the contract has the minimum fields required to enforce a
// structured JSON response.
func (c StructuredOutputContract) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.Join(ErrStructuredOutputInvalidContract, errors.New("structured output contract name is required"))
	}
	if len(c.Schema) == 0 {
		return errors.Join(ErrStructuredOutputInvalidContract, errors.New("structured output contract schema is required"))
	}
	return nil
}

// StructuredOutputResult is the parsed result plus raw model content used to
// produce it.
type StructuredOutputResult struct {
	Parsed map[string]any
	Raw    string
}

func legacyJSONObjectContract() StructuredOutputContract {
	return StructuredOutputContract{
		Name:        "legacy_json_object",
		Schema:      json.RawMessage(`{"type":"object"}`),
		AllowRepair: true,
		MaxRetries:  1,
	}
}
