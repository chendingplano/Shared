package llm

import (
	"errors"
	"fmt"
)

var (
	ErrStructuredOutputInvalidContract  = errors.New("structured output invalid contract")
	ErrStructuredOutputProvider         = errors.New("structured output provider failure")
	ErrStructuredOutputParse            = errors.New("structured output parse failure")
	ErrStructuredOutputSchemaValidation = errors.New("structured output schema validation failure")
	ErrStructuredOutputRetriesExhausted = errors.New("structured output retries exhausted")
	// ErrStructuredOutputTruncated marks a response cut off by the provider
	// before it finished (finish_reason=length). Distinct from
	// ErrStructuredOutputParse/SchemaValidation because those imply the model
	// produced a complete but wrong/malformed response; this means the
	// content is real but incomplete, so JSON-repair heuristics must not run
	// on it, and retrying with the same max_tokens will fail identically.
	ErrStructuredOutputTruncated = errors.New("structured output truncated by provider before completion")
)

// ErrLLMResponseTruncated is the transport-level signal (from
// extractTextWithFormat) that the provider stopped generating due to its
// output-token limit. ExtractStructuredJSON maps this to
// ErrStructuredOutputTruncated.
var ErrLLMResponseTruncated = errors.New("llm response truncated (finish_reason=length)")

// StructuredOutputError classifies failures encountered while producing or
// validating structured LLM JSON responses.
type StructuredOutputError struct {
	Kind error
	Err  error
	Raw  string
}

func (e *StructuredOutputError) Error() string {
	if e == nil {
		return "<nil>"
	}
	switch {
	case e.Kind == nil && e.Err == nil:
		return "structured output error"
	case e.Kind == nil:
		return e.Err.Error()
	case e.Err == nil:
		return e.Kind.Error()
	default:
		return fmt.Sprintf("%s: %v", e.Kind, e.Err)
	}
}

func (e *StructuredOutputError) Unwrap() []error {
	if e == nil {
		return nil
	}
	out := make([]error, 0, 2)
	if e.Kind != nil {
		out = append(out, e.Kind)
	}
	if e.Err != nil {
		out = append(out, e.Err)
	}
	return out
}
