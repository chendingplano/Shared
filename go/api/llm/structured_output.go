package llm

import (
	"context"
	"fmt"
	"strings"
)

// ExtractStructuredJSON returns validated JSON for the supplied machine-readable
// contract, retrying parse or schema failures up to contract.MaxRetries times.
func (c *OpenAIJSONClient) ExtractStructuredJSON(
	ctx context.Context,
	in JSONExtractionInput,
	contract StructuredOutputContract,
) (*StructuredOutputResult, error) {
	if err := contract.Validate(); err != nil {
		return nil, err
	}

	maxAttempts := contract.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	originalInput := in.InputText
	var lastErr error
	var lastRaw string

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		in.InputText = buildStructuredRetryInput(originalInput, contract.Name, attempt, lastErr)
		content, err := c.extractTextWithFormat(ctx, in, true)
		if err != nil {
			return nil, &StructuredOutputError{
				Kind: ErrStructuredOutputProvider,
				Err:  err,
			}
		}
		lastRaw = content

		parsed, err := parseLLMJSONMap(content)
		if err != nil {
			lastErr = &StructuredOutputError{
				Kind: ErrStructuredOutputParse,
				Err:  err,
				Raw:  content,
			}
			if attempt < maxAttempts {
				continue
			}
			return nil, &StructuredOutputError{
				Kind: ErrStructuredOutputRetriesExhausted,
				Err:  lastErr,
				Raw:  lastRaw,
			}
		}

		if err := validateStructuredJSON(contract, parsed); err != nil {
			lastErr = err
			if attempt < maxAttempts {
				continue
			}
			return nil, &StructuredOutputError{
				Kind: ErrStructuredOutputRetriesExhausted,
				Err:  lastErr,
				Raw:  lastRaw,
			}
		}

		return &StructuredOutputResult{
			Parsed: parsed,
			Raw:    content,
		}, nil
	}

	return nil, &StructuredOutputError{
		Kind: ErrStructuredOutputRetriesExhausted,
		Err:  lastErr,
		Raw:  lastRaw,
	}
}

func buildStructuredRetryInput(originalInput, contractName string, attempt int, lastErr error) string {
	if attempt <= 1 || lastErr == nil {
		return originalInput
	}

	var b strings.Builder
	b.WriteString(originalInput)
	if strings.TrimSpace(originalInput) != "" {
		b.WriteString("\n\n")
	}
	b.WriteString("Retry instructions:\n")
	b.WriteString("Previous response for contract ")
	b.WriteString(contractName)
	b.WriteString(" was invalid.\n")
	b.WriteString("Return JSON only with the same intended data.\n")
	b.WriteString("Validation issue: ")
	b.WriteString(summarizeStructuredRetryError(lastErr))
	return b.String()
}

func summarizeStructuredRetryError(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(err))
}
