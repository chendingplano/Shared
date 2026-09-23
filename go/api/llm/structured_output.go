package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
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

	startedAt := time.Now().UTC()
	model := strings.TrimSpace(in.ModelName)
	if model == "" {
		model = strings.TrimSpace(c.ModelName)
	}

	originalInput := in.InputText
	var lastErr error
	var lastRaw string

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		in.InputText = buildStructuredRetryInput(originalInput, contract.Name, attempt, lastErr)
		content, err := c.extractTextWithFormat(ctx, in, true)
		if err != nil {
			// extractTextWithFormat already captured this attempt's usage event
			// (including raw_response on error) at the transport layer.
			if errors.Is(err, ErrLLMResponseTruncated) {
				// Deterministic given the same input and max_tokens -- retrying
				// without raising max_tokens would just truncate identically
				// again, burning the retry budget for nothing. Alarm and stop.
				c.ensureLogger().Error("(MID_26092202) ALARM: llm response truncated before completion; raise max_output_tokens for this model",
					"model_name", model,
					"call_reason", in.CallReason,
					"call_loc", in.CallLoc,
					"record_id", in.RecordID,
					"run_id", in.RunID,
					"max_output_tokens", c.MaxOutputTokens,
					"raw_response", content)
				return nil, &StructuredOutputError{
					Kind: ErrStructuredOutputTruncated,
					Err:  err,
					Raw:  content,
				}
			}
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
			exhaustedErr := &StructuredOutputError{
				Kind: ErrStructuredOutputRetriesExhausted,
				Err:  lastErr,
				Raw:  lastRaw,
			}
			c.captureUsage(ctx, in, model, startedAt, nil, nil, "", lastRaw, 0, 0, 0, 0, exhaustedErr)
			return nil, exhaustedErr
		}

		if err := validateStructuredJSON(contract, parsed); err != nil {
			lastErr = err
			if attempt < maxAttempts {
				continue
			}
			exhaustedErr := &StructuredOutputError{
				Kind: ErrStructuredOutputRetriesExhausted,
				Err:  lastErr,
				Raw:  lastRaw,
			}
			c.captureUsage(ctx, in, model, startedAt, nil, nil, "", lastRaw, 0, 0, 0, 0, exhaustedErr)
			return nil, exhaustedErr
		}

		return &StructuredOutputResult{
			Parsed: parsed,
			Raw:    content,
		}, nil
	}

	exhaustedErr := &StructuredOutputError{
		Kind: ErrStructuredOutputRetriesExhausted,
		Err:  lastErr,
		Raw:  lastRaw,
	}
	c.captureUsage(ctx, in, model, startedAt, nil, nil, "", lastRaw, 0, 0, 0, 0, exhaustedErr)
	return nil, exhaustedErr
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
