package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaudeConversionPreservesSchemaPayload(t *testing.T) {
	schema := `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"integer","minimum":9007199254740993}},"required":["reasoning","answer"]}`
	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"claude-test","max_tokens":1024,"messages":[{"role":"user","content":"answer"}],"tools":[{"type":"function","function":{"name":"answer","parameters":`+schema+`}}]}`), &request))
	converted, err := RequestOpenAI2ClaudeMessage(nil, request)
	require.NoError(t, err)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	require.Equal(t, schema, gjson.GetBytes(encoded, "tools.0.input_schema").Raw)
	// Normalization adds missing defaults without disturbing existing properties.
	raw := []byte(`{"properties":{"z":{"type":"integer","maximum":9007199254740993},"a":{"type":"string"}}}`)
	require.NoError(t, common.Unmarshal(append(append([]byte(`{"parameters":`), raw...), '}'), &request.Tools[0].Function))
	converted, err = RequestOpenAI2ClaudeMessage(nil, request)
	require.NoError(t, err)
	encoded, err = common.Marshal(converted)
	require.NoError(t, err)
	require.Equal(t, gjson.GetBytes(raw, "properties").Raw, gjson.GetBytes(encoded, "tools.0.input_schema.properties").Raw)
	require.Equal(t, "object", gjson.GetBytes(encoded, "tools.0.input_schema.type").String())
}
