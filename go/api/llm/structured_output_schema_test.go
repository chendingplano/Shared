package llm

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCompileStructuredSchema(t *testing.T) {
	contract := StructuredOutputContract{
		Name: "simple_object",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"title": {"type": "string"}
			},
			"required": ["title"],
			"additionalProperties": false
		}`),
	}

	schema, err := compileStructuredSchema(contract)
	if err != nil {
		t.Fatalf("compileStructuredSchema() error = %v", err)
	}
	if schema == nil {
		t.Fatalf("compileStructuredSchema() returned nil schema")
	}
}

func TestValidateStructuredJSON(t *testing.T) {
	contract := StructuredOutputContract{
		Name: "topic_payload",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"topics": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"topic_desc": {"type": "string"},
							"lines": {
								"type": "array",
								"items": {"type": "string"}
							}
						},
						"required": ["topic_desc", "lines"],
						"additionalProperties": false
					}
				}
			},
			"required": ["topics"],
			"additionalProperties": false
		}`),
	}

	tests := []struct {
		name    string
		payload map[string]any
		wantErr error
	}{
		{
			name: "valid nested payload",
			payload: map[string]any{
				"topics": []any{
					map[string]any{
						"topic_desc": "alarm requirements",
						"lines":      []any{"10-12"},
					},
				},
			},
		},
		{
			name: "missing required field",
			payload: map[string]any{
				"other": []any{},
			},
			wantErr: ErrStructuredOutputSchemaValidation,
		},
		{
			name: "wrong field type",
			payload: map[string]any{
				"topics": "not-an-array",
			},
			wantErr: ErrStructuredOutputSchemaValidation,
		},
		{
			name: "unexpected extra field",
			payload: map[string]any{
				"topics": []any{
					map[string]any{
						"topic_desc": "alarm requirements",
						"lines":      []any{"10-12"},
						"extra":      true,
					},
				},
			},
			wantErr: ErrStructuredOutputSchemaValidation,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateStructuredJSON(contract, tc.payload)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("validateStructuredJSON() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("validateStructuredJSON() error = %v, want errors.Is(_, %v)", err, tc.wantErr)
			}
		})
	}
}

