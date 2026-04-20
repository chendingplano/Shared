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
	"net/http"
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
	ID         ProviderID
	BaseURL    string            // empty → adapter default
	APIKey     string            // plaintext; never logged
	HTTPClient *http.Client      // optional injection for tests
	Extra      map[string]string // provider-specific knobs
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
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// Request describes one inference call.
type Request struct {
	Model       string
	Messages    []Message
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
