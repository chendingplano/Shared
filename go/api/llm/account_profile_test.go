package llm

import "testing"

func TestResolveRequestContextBuildsProviderConfigFromAccountProfile(t *testing.T) {
	account := Account{
		ID:       7,
		Name:     "DeepSeek Prod",
		Provider: ProviderOpenAICompatible,
		BaseURL:  "https://api.deepseek.com",
		APIKey:   "sk-test-value",
	}
	profile := AccountProfile{
		ID:          11,
		AccountID:   7,
		ProfileName: "deepseek-v4-flash",
		ModelName:   "deepseek-v4-flash",
	}
	req := Request{
		Model:      "ignored-by-profile",
		PromptName: "extract-products-v2",
		Messages:   []Message{{Role: RoleUser, Content: "hi"}},
	}

	resolved := ResolveRequestContext(account, profile, req)

	if resolved.AccountID != 7 {
		t.Fatalf("AccountID = %d, want 7", resolved.AccountID)
	}
	if resolved.ProfileID != 11 {
		t.Fatalf("ProfileID = %d, want 11", resolved.ProfileID)
	}
	if resolved.Provider != ProviderOpenAICompatible {
		t.Fatalf("Provider = %q", resolved.Provider)
	}
	if resolved.ModelName != "deepseek-v4-flash" {
		t.Fatalf("ModelName = %q", resolved.ModelName)
	}
	if resolved.PromptName != "extract-products-v2" {
		t.Fatalf("PromptName = %q", resolved.PromptName)
	}
	if resolved.ProviderConfig.ID != ProviderOpenAICompatible {
		t.Fatalf("ProviderConfig.ID = %q", resolved.ProviderConfig.ID)
	}
	if resolved.ProviderConfig.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("ProviderConfig.BaseURL = %q", resolved.ProviderConfig.BaseURL)
	}
	if resolved.ProviderConfig.APIKey != "sk-test-value" {
		t.Fatalf("ProviderConfig.APIKey = %q", resolved.ProviderConfig.APIKey)
	}
}

func TestResolveRequestContextFallsBackToRequestModelWhenProfileModelEmpty(t *testing.T) {
	account := Account{
		ID:       9,
		Name:     "OpenAI Backup",
		Provider: ProviderOpenAI,
		APIKey:   "sk-live",
	}
	profile := AccountProfile{
		ID:        19,
		AccountID: 9,
	}
	req := Request{
		Model:      "gpt-5-mini",
		PromptName: "classify-topic",
	}

	resolved := ResolveRequestContext(account, profile, req)

	if resolved.ModelName != "gpt-5-mini" {
		t.Fatalf("ModelName = %q, want gpt-5-mini", resolved.ModelName)
	}
	if resolved.ProviderConfig.ID != ProviderOpenAI {
		t.Fatalf("ProviderConfig.ID = %q", resolved.ProviderConfig.ID)
	}
}
