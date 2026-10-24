package gemini

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/types"
)

// Validate every item before converting: ParseInput intentionally serves other
// providers and would silently discard unsupported items from a mixed batch.
func plainTextEmbeddingInputs(input any) ([]string, error) {
	invalidInput := func() ([]string, error) {
		return nil, types.NewErrorWithStatusCode(
			errors.New("gemini embedding input must be a non-empty string or a non-empty array of non-empty strings"),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	var inputs []string
	switch value := input.(type) {
	case string:
		inputs = []string{value}
	case []string:
		inputs = value
	case []any:
		inputs = make([]string, len(value))
		for i, item := range value {
			text, ok := item.(string)
			if !ok {
				return invalidInput()
			}
			inputs[i] = text
		}
	default:
		return invalidInput()
	}
	if len(inputs) == 0 {
		return invalidInput()
	}
	for _, text := range inputs {
		if text == "" {
			return invalidInput()
		}
	}
	return inputs, nil
}
