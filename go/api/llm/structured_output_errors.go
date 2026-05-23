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
)

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
