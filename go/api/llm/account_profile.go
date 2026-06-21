package llm

type Account struct {
	ID       string
	Name     string
	Provider ProviderID
	BaseURL  string
	APIKey   string
}

type AccountProfile struct {
	ID          string
	AccountID   string
	ProfileName string
	ModelName   string
}

type ResolvedRequestContext struct {
	AccountID      string
	ProfileID      string
	Provider       ProviderID
	ModelName      string
	PromptName     string
	ProviderConfig ProviderConfig
	Request        Request
}

func ResolveRequestContext(account Account, profile AccountProfile, req Request) ResolvedRequestContext {
	modelName := profile.ModelName
	if modelName == "" {
		modelName = req.Model
	}

	resolvedReq := req
	if modelName != "" {
		resolvedReq.Model = modelName
	}

	return ResolvedRequestContext{
		AccountID:  account.ID,
		ProfileID:  profile.ID,
		Provider:   account.Provider,
		ModelName:  modelName,
		PromptName: req.PromptName,
		ProviderConfig: ProviderConfig{
			ID:      account.Provider,
			BaseURL: account.BaseURL,
			APIKey:  account.APIKey,
		},
		Request: resolvedReq,
	}
}
