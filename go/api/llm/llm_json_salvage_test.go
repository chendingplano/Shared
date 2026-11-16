package llm

import "testing"

// record307Envelope is the compact merged-products response logged for run
// 701 / record 307: the span arrays have unbalanced quotes, which
// RepairLLMJSON must rebuild into a valid envelope.
const record307Envelope = `{"v":2,"p":[{"n":"波纹管","t":"cmp","r":"sc","d":"文件在应力符号说明中将波纹管作为计算对象。","s":[492,"493,"494,"495,"496","497,"498,"499,"502,"503,"504,505],"o":"d","q":0.9,"cr":"正文明确提及波纹管及其材料"},{"n":"加强环","t":"cmp","r":"sc","d":"文件说明波纹管加强环周向薄膜应力的符号。","s":[493],"o":"d","q":0.85,"cr":"正文明确提及"},{"n":"紧固件","t":"cmp","r":"sc","d":"文件说明波纹管紧固件薄膜应力的符号。","s":[494],"o":"d","q":0.85,"cr":"正文明确提及"}]}`

// A repaired envelope must come back as the envelope, not as one product.
func TestParseLLMJSONMap_RecordEnvelopeRepairedWhole(t *testing.T) {
	m, err := parseLLMJSONMap(record307Envelope)
	if err != nil {
		t.Fatalf("parseLLMJSONMap error: %v", err)
	}
	if _, ok := m["v"]; !ok {
		t.Fatalf("expected the envelope with key v, got keys %v", m)
	}
	if p, ok := m["p"].([]any); !ok || len(p) != 3 {
		t.Fatalf("expected 3 products under p, got %v", m["p"])
	}
}

// An envelope that cannot be repaired (truncated, so the outer object never
// closes) must fail to parse. It must not be silently replaced by its first
// product, which then fails schema validation with a misleading message.
func TestParseLLMJSONMap_UnrepairableEnvelopeIsNotSalvagedAsProduct(t *testing.T) {
	truncated := record307Envelope[:len(record307Envelope)-2] // drop the final "]}"
	m, err := parseLLMJSONMap(truncated)
	if err == nil {
		t.Fatalf("expected a parse error for a truncated envelope, got map with keys %v", m)
	}
}
