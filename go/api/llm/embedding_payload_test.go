package llm

import (
	"encoding/json"
	"testing"
)

func TestEmbeddingPayloadUsesResponseIndex(t *testing.T) {
	var payload embeddingResponsePayload
	if err := json.Unmarshal([]byte(`{"data":[{"index":1,"embedding":[9,9]},{"index":0,"embedding":[1,1]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 2 || payload.Data[0].Embedding[0] != 1 || payload.Data[1].Embedding[0] != 9 {
		t.Fatalf("embeddings not in input order: %+v", payload.Data)
	}
	if payload.Usage == nil || payload.Usage.Total != 2 {
		t.Fatalf("usage lost: %+v", payload.Usage)
	}
}
