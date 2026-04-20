package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Sentinel errors.
var (
	ErrUnsupportedProvider   = errors.New("llm: unsupported provider")
	ErrMissingAPIKey         = errors.New("llm: APIKey is required")
	ErrMissingBaseURL        = errors.New("llm: BaseURL is required for openai_compatible")
	ErrAdapterNotImplemented = errors.New("llm: adapter not yet implemented")
)

// ProviderError wraps a non-2xx response from a provider with enough context
// for logs and debugging. The API key is never included.
type ProviderError struct {
	Provider   ProviderID
	Model      string
	HTTPStatus int
	Body       string
	Err        error
}

func (e *ProviderError) Error() string {
	base := fmt.Sprintf("llm: provider=%s model=%s http=%d",
		e.Provider, e.Model, e.HTTPStatus)
	if e.Err != nil {
		base += ": " + e.Err.Error()
	}
	if e.Body != "" {
		base += " body=" + truncate(e.Body, 512)
	}
	return base
}

func (e *ProviderError) Unwrap() error { return e.Err }

// NewClient dispatches to the correct adapter based on cfg.ID.
//
// For Slice 1 of the Prompt Optimizer feature, OpenAI and OpenAI-compatible
// return fully-working clients. Anthropic and Gemini return a client whose
// Complete / Stream methods return ErrAdapterNotImplemented — this keeps the
// factory exhaustive so later slices are purely additive.
func NewClient(cfg ProviderConfig) (Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%w (provider=%s)", ErrMissingAPIKey, cfg.ID)
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = defaultHTTPClient()
	}
	switch cfg.ID {
	case ProviderOpenAI:
		return newOpenAIClient(cfg, "https://api.openai.com"), nil
	case ProviderOpenAICompatible:
		if cfg.BaseURL == "" {
			return nil, ErrMissingBaseURL
		}
		return newOpenAIClient(cfg, cfg.BaseURL), nil
	case ProviderAnthropic:
		return &notImplementedClient{provider: ProviderAnthropic}, nil
	case ProviderGemini:
		return &notImplementedClient{provider: ProviderGemini}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, cfg.ID)
	}
}

// notImplementedClient is the placeholder for adapters reserved for later
// slices. It satisfies Client so callers don't need provider-specific
// branching at the call site.
type notImplementedClient struct {
	provider ProviderID
}

func (c *notImplementedClient) Complete(ctx context.Context, req Request) (*Response, error) {
	return nil, fmt.Errorf("%w: %s", ErrAdapterNotImplemented, c.provider)
}

func (c *notImplementedClient) Stream(ctx context.Context, req Request, on StreamHandler) error {
	return fmt.Errorf("%w: %s", ErrAdapterNotImplemented, c.provider)
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}

// redactAPIKey returns a log-safe preview of an API key. Long keys are
// revealed as first-3 + "***" + last-4; anything shorter collapses to "***".
// Never log the plaintext key.
func redactAPIKey(k string) string {
	if len(k) < 8 {
		return "***"
	}
	return k[:3] + "***" + k[len(k)-4:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
