package llm

import "testing"

func TestRepairLLMJSON(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
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
			name:   "preserves LaTeX prime from Runshen alert 21589",
			input:  `{"v":2,"l":"zh","p":[{"n":"制冷设备制冷量选择","t":"m","x":"制冷(热)量 $(Q_{0}^{\prime})$","s":[495],"lt":"p","q":0.9}],"valid":"\n\u4e2d\\\""}`,
			want:   `{"v":2,"l":"zh","p":[{"n":"制冷设备制冷量选择","t":"m","x":"制冷(热)量 $(Q_{0}^{\\prime})$","s":[495],"lt":"p","q":0.9}],"valid":"\n\u4e2d\\\""}`,
			wantOK: true,
		},
		{
			name:   "does not turn incomplete Unicode escapes into text",
			input:  `{"x":"\u12z4"}`,
			wantOK: false,
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

// Products span arrays may hold table row and header refs (N#rK, N#hK). The
// shapes below are the prod responses logged on 2026-11-16 (ChenWeb#17),
// records 377, 383 and 384, model deepseek-v4-flash.
func TestRepairLLMJSONRebuildsUnbalancedSpanArraysWithRowRefs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			// Without the rebuild, the inner-quote repair turned this into
			// the valid JSON string "153,\"158#r1", which then failed schema
			// validation.
			name:  "missing close quote before a row ref (record 384)",
			input: `{"v":2,"p":[{"n":"热泵","s":[151,"153,"158#r1","158#r2"],"o":"m"}]}`,
			want:  `{"v":2,"p":[{"n":"热泵","s":[151,153,"158#r1","158#r2"],"o":"m"}]}`,
		},
		{
			name:  "missing open quote on the first row ref (record 377)",
			input: `{"v":2,"p":[{"n":"盘管式机房空调","s":[330#r1","330#r2","330#r6],"o":"m"}]}`,
			want:  `{"v":2,"p":[{"n":"盘管式机房空调","s":["330#r1","330#r2","330#r6"],"o":"m"}]}`,
		},
		{
			name:  "header ref",
			input: `{"s":[12#h1","13]}`,
			want:  `{"s":["12#h1",13]}`,
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

// When the stray quotes happen to pair up, the inner-quote repair alone yields
// valid JSON with a corrupted span ("3474,\"3475"). The span rebuild must win
// over it (record 383, ChenWeb#17).
func TestRepairLLMJSONPrefersSpanRebuildOverInnerQuoteRepair(t *testing.T) {
	input := `{"v":2,"p":[{"n":"管道","s":[3468,"3469","3470","3474,"3475"],"o":"c"}]}`
	want := `{"v":2,"p":[{"n":"管道","s":[3468,3469,3470,3474,3475],"o":"c"}]}`
	got, ok := RepairLLMJSON(input)
	if !ok {
		t.Fatalf("RepairLLMJSON returned ok=false")
	}
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
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

func TestRepairLLMJSONQuotesBareTableRowCitation(t *testing.T) {
	input := `{"v":2,"p":[{"n":"product","s":[248#r5]},{"n":"other","s":[249,250#h1]}]}`
	want := `{"v":2,"p":[{"n":"product","s":["248#r5"]},{"n":"other","s":[249,"250#h1"]}]}`
	if got, ok := RepairLLMJSON(input); !ok || got != want {
		t.Fatalf("RepairLLMJSON = %q, %t, want %q, true", got, ok, want)
	}
}
