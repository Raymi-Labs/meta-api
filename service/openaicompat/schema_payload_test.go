package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatToResponsesPreservesSchemaPayload(t *testing.T) {
	schema := `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"integer","minimum":9007199254740993}}}`
	for _, nested := range []bool{false, true} {
		format := `{"name":"result","strict":true,"schema":` + schema + `}`
		if nested {
			format = `{"json_schema":` + format + `}`
		}
		var request dto.GeneralOpenAIRequest
		require.NoError(t, common.Unmarshal([]byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"answer","parameters":`+schema+`}}],"response_format":{"type":"json_schema","json_schema":`+format+`}}`), &request))
		converted, err := ChatCompletionsRequestToResponsesRequest(&request)
		require.NoError(t, err)
		require.Equal(t, schema, gjson.GetBytes(converted.Tools, "0.parameters").Raw)
		require.Equal(t, schema, gjson.GetBytes(converted.Text, "format.schema").Raw)
		require.Equal(t, "json_schema", gjson.GetBytes(converted.Text, "format.type").String())
		require.True(t, gjson.GetBytes(converted.Text, "format.strict").Bool())
	}
}

func TestLegacyFunctionsKeepSchemaOrderAndValidation(t *testing.T) {
	schema := `{"type":"object","properties":{"z":{"type":"number","maximum":9007199254740993},"a":{"type":"string"}}}`
	functions, present, err := parseLegacyFunctions([]byte(`[{"name":"answer","parameters":` + schema + `}]`))
	require.NoError(t, err)
	require.True(t, present)
	encoded, err := common.Marshal(functions)
	require.NoError(t, err)
	require.Equal(t, schema, gjson.GetBytes(encoded, "0.parameters").Raw)
	for _, invalid := range []string{`[]`, `true`, `"text"`, `1`} {
		_, _, err = parseLegacyFunctions([]byte(`[{"name":"answer","parameters":` + invalid + `}]`))
		require.ErrorContains(t, err, "must be a JSON object")
	}
}
