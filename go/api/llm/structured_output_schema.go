package llm

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type compiledSchema struct {
	name   string
	schema *jsonschema.Schema
}

var structuredSchemaCache sync.Map

func compileStructuredSchema(contract StructuredOutputContract) (*compiledSchema, error) {
	if err := contract.Validate(); err != nil {
		return nil, err
	}

	cacheKey := strings.TrimSpace(contract.Name) + "\x00" + string(contract.Schema)
	if cached, ok := structuredSchemaCache.Load(cacheKey); ok {
		if compiled, ok := cached.(*compiledSchema); ok {
			return compiled, nil
		}
	}

	compiler := jsonschema.NewCompiler()
	resourceURL := "mem://" + strings.TrimSpace(contract.Name) + ".schema.json"
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(contract.Schema))
	if err != nil {
		return nil, errors.Join(ErrStructuredOutputInvalidContract, fmt.Errorf("decode schema for %s: %w", contract.Name, err))
	}
	if err := compiler.AddResource(resourceURL, schemaDoc); err != nil {
		return nil, errors.Join(ErrStructuredOutputInvalidContract, fmt.Errorf("add schema resource for %s: %w", contract.Name, err))
	}

	schema, err := compiler.Compile(resourceURL)
	if err != nil {
		return nil, errors.Join(ErrStructuredOutputInvalidContract, fmt.Errorf("compile schema for %s: %w", contract.Name, err))
	}

	compiled := &compiledSchema{
		name:   strings.TrimSpace(contract.Name),
		schema: schema,
	}
	structuredSchemaCache.Store(cacheKey, compiled)
	return compiled, nil
}

func validateStructuredJSON(contract StructuredOutputContract, payload map[string]any) error {
	compiled, err := compileStructuredSchema(contract)
	if err != nil {
		return err
	}
	if err := compiled.schema.Validate(payload); err != nil {
		return &StructuredOutputError{
			Kind: ErrStructuredOutputSchemaValidation,
			Err:  err,
		}
	}
	return nil
}
