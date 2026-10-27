package llm

import (
	"encoding/json"
	"sort"
)

// UnmarshalJSON restores input order when a provider returns batch embeddings
// out of order. Missing indexes retain the provider's order for compatibility.
func (p *embeddingResponsePayload) UnmarshalJSON(data []byte) error {
	var raw struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Usage *struct {
			Prompt int `json:"prompt_tokens"`
			Total  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	sort.SliceStable(raw.Data, func(i, j int) bool { return raw.Data[i].Index < raw.Data[j].Index })
	p.Data = make([]struct {
		Embedding []float64 `json:"embedding"`
	}, len(raw.Data))
	for i, item := range raw.Data {
		p.Data[i].Embedding = item.Embedding
	}
	p.Usage = raw.Usage
	return nil
}
