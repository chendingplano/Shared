package llm

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestStructuredOutputContract_Validate(t *testing.T) {
	tests := []struct {
		name     string
		contract StructuredOutputContract
		wantErr  error
	}{
		{
			name: "missing contract name",
			contract: StructuredOutputContract{
				Schema: json.RawMessage(`{"type":"object"}`),
			},
			wantErr: ErrStructuredOutputInvalidContract,
		},
		{
			name: "missing schema",
			contract: StructuredOutputContract{
				Name: "topics",
			},
			wantErr: ErrStructuredOutputInvalidContract,
		},
		{
			name: "valid contract",
			contract: StructuredOutputContract{
				Name:   "topics",
				Schema: json.RawMessage(`{"type":"object"}`),
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.contract.Validate()
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() error = %v, want errors.Is(_, %v)", err, tc.wantErr)
			}
		})
	}
}

func TestStructuredOutputErrors_ClassifyFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "parse failure",
			err: &StructuredOutputError{
				Kind: ErrStructuredOutputParse,
				Err:  errors.New("bad json"),
			},
			want: ErrStructuredOutputParse,
		},
		{
			name: "schema validation failure",
			err: &StructuredOutputError{
				Kind: ErrStructuredOutputSchemaValidation,
				Err:  errors.New("missing field"),
			},
			want: ErrStructuredOutputSchemaValidation,
		},
		{
			name: "provider failure",
			err: &StructuredOutputError{
				Kind: ErrStructuredOutputProvider,
				Err:  errors.New("request failed"),
			},
			want: ErrStructuredOutputProvider,
		},
		{
			name: "retries exhausted",
			err: &StructuredOutputError{
				Kind: ErrStructuredOutputRetriesExhausted,
				Err:  errors.New("attempts exceeded"),
			},
			want: ErrStructuredOutputRetriesExhausted,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, tc.want) {
				t.Fatalf("errors.Is(%v, %v) = false", tc.err, tc.want)
			}
		})
	}
}

