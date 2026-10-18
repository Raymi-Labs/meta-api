package deepseek

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

// Validate the final payload, including pass-through bodies and parameter
// overrides. DeepSeek silently ignores state references, which would otherwise
// turn a continuation into a new conversation with missing history.
func prepareResponsesRequestBody(body io.Reader) (io.Reader, error) {
	var payload map[string]json.RawMessage
	if err := common.DecodeJson(body, &payload); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	for _, field := range []string{"previous_response_id", "conversation"} {
		value := bytes.TrimSpace(payload[field])
		if len(value) > 0 && !bytes.Equal(value, []byte("null")) && !bytes.Equal(value, []byte(`""`)) {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("DeepSeek Responses does not support %s; send the full conversation history, including reasoning and tool results, in input", field),
				types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry(),
			)
		}
	}
	// Usage is delivered in terminal Responses events; DeepSeek does not support
	// stream_options. Keep opaque input/tool items intact during this cleanup.
	delete(payload, "stream_options")
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
