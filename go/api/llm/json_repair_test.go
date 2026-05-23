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
			got, ok := repairLLMJSON(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("repairLLMJSON() ok = %v, want %v (got %q)", ok, tc.wantOK, got)
			}
			if tc.wantOK && got != tc.want {
				t.Fatalf("repairLLMJSON() = %q, want %q", got, tc.want)
			}
		})
	}
}

