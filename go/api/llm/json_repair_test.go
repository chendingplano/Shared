package llm

import "testing"

func TestRepairLLMJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantOK   bool
	}{
		{
			name:   "strips markdown fence",
			input:  "```json\n{\"title\":\"sample\"}\n```",
			want:   "{\"title\":\"sample\"}",
			wantOK: true,
		},
		{
			name:   "extracts leading prose object",
			input:  "Here is the JSON you requested:\n{\"title\":\"sample\"}",
			want:   "{\"title\":\"sample\"}",
			wantOK: true,
		},
		{
			name:   "repairs embedded quotes in string field",
			input:  "{\"topics\":[{\"topic_desc_en\":\"Design code \\\"GB 50019\\\" and \"Energy Efficiency\" GB 50189\",\"lines\":[\"1-2\"]}]}",
			want:   "{\"topics\":[{\"topic_desc_en\":\"Design code \\\"GB 50019\\\" and \\\"Energy Efficiency\\\" GB 50189\",\"lines\":[\"1-2\"]}]}",
			wantOK: true,
		},
		{
			name:   "extracts object from truncated wrapper",
			input:  "[{\"title\":\"sample\"}",
			want:   "{\"title\":\"sample\"}",
			wantOK: true,
		},
		{
			name:   "unrepairable payload stays failed",
			input:  "{\"title\": ",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, ok := RepairLLMJSON(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("RepairLLMJSON() ok = %v, want %v (got %q)", ok, tc.wantOK, got)
			}
			if tc.wantOK && got != tc.want {
				t.Fatalf("RepairLLMJSON() = %q, want %q", got, tc.want)
			}
		})
	}
}


func TestRepairLLMJSONRebuildsUnbalancedSpanArrays(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "unbalanced quotes from record 307",
			input: `{"v":2,"p":[{"n":"a","s":[492,"493,"494,"495,"496","497,"498,"499,"502,"503,"504,505],"o":"d"}]}`,
			want:  `{"v":2,"p":[{"n":"a","s":[492,493,494,495,496,497,498,499,502,503,504,505],"o":"d"}]}`,
		},
		{
			name:  "unbalanced quotes with a range",
			input: `{"s":[10,"11,"20-25]}`,
			want:  `{"s":[10,11,"20-25"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RepairLLMJSON(tt.input)
			if !ok {
				t.Fatalf("RepairLLMJSON(%s) returned ok=false", tt.input)
			}
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRepairLLMJSONLeavesValidAndTextArraysAlone(t *testing.T) {
	if got, ok := rebuildBrokenNumericArrays(`{"s":["1","2"],"x":[1,2]}`); ok {
		t.Fatalf("valid arrays rewritten: %s", got)
	}
	in := `{"s":[1,"abc,"2]}`
	if got, ok := rebuildBrokenNumericArrays(in); ok {
		t.Fatalf("array with text token rewritten: %s", got)
	}
}

func TestRepairLLMJSONQuotesBareSpanRanges(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single bare range in array",
			input: `{"v":2,"p":[{"s":[1598-1617],"q":0.85}]}`,
			want:  `{"v":2,"p":[{"s":["1598-1617"],"q":0.85}]}`,
		},
		{
			name:  "mixed line and bare range",
			input: `{"s":[143,1598-1617]}`,
			want:  `{"s":[143,"1598-1617"]}`,
		},
		{
			name:  "adjacent ranges",
			input: `{"s":[1-2,3-4]}`,
			want:  `{"s":["1-2","3-4"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RepairLLMJSON(tt.input)
			if !ok {
				t.Fatalf("RepairLLMJSON(%s) returned ok=false", tt.input)
			}
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
