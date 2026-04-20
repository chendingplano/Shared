package llm

import (
	"errors"
	"strings"
	"testing"
)

func TestNewClientUnsupportedProvider(t *testing.T) {
	_, err := NewClient(ProviderConfig{ID: "not-a-real-provider", APIKey: "k"})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("want ErrUnsupportedProvider, got %v", err)
	}
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	_, err := NewClient(ProviderConfig{ID: ProviderOpenAI, APIKey: ""})
	if err == nil {
		t.Fatal("expected error when APIKey is empty")
	}
}

func TestNewClientAcceptsKnownProviders(t *testing.T) {
	cases := []ProviderID{
		ProviderOpenAI,
		ProviderOpenAICompatible,
		ProviderAnthropic,
		ProviderGemini,
	}
	for _, p := range cases {
		cfg := ProviderConfig{ID: p, APIKey: "sk-TESTVALUE1234"}
		if p == ProviderOpenAICompatible {
			cfg.BaseURL = "http://localhost:11434"
		}
		c, err := NewClient(cfg)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", p, err)
		}
		if c == nil {
			t.Fatalf("%s: got nil client without error", p)
		}
	}
}

func TestRedactAPIKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "***"},
		{"short", "***"},
		{"sk-abcdef", "sk-***cdef"},
		{"sk-1234567890", "sk-***7890"},
	}
	for _, c := range cases {
		got := redactAPIKey(c.in)
		if got != c.want {
			t.Errorf("redactAPIKey(%q): want %q got %q", c.in, c.want, got)
		}
	}
}

func TestProviderErrorMessage(t *testing.T) {
	e := &ProviderError{
		Provider:   ProviderOpenAI,
		Model:      "gpt-4o-mini",
		HTTPStatus: 401,
		Body:       `{"error":"bad key"}`,
	}
	msg := e.Error()
	for _, s := range []string{"openai", "gpt-4o-mini", "401"} {
		if !strings.Contains(msg, s) {
			t.Errorf("ProviderError.Error() missing %q: %s", s, msg)
		}
	}
}
