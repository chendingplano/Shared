package llm

type Account struct {
	ID       int64
	Name     string
	Provider ProviderID
	BaseURL  string
	APIKey   string
}

type AccountProfile struct {
	ID          int64
	AccountID   int64
	ProfileName string
	ModelName   string
}

type ResolvedRequestContext struct {
	AccountID      int64
	ProfileID      int64
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
