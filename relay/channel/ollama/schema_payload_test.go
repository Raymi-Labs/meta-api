package ollama

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOllamaRequestPreservesToolAndResponseSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"integer","minimum":9007199254740993}}}`
	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"ollama-test","tools":[{"type":"function","function":{"name":"answer","parameters":`+schema+`}}],"response_format":{"type":"json_schema","json_schema":{"name":"answer","schema":`+schema+`}}}`), &request))
	converted, err := openAIChatToOllamaChat(nil, &request)
	require.NoError(t, err)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	require.Equal(t, schema, gjson.GetBytes(encoded, "format").Raw)
	require.Equal(t, schema, gjson.GetBytes(encoded, "tools.0.function.parameters").Raw)
	generated, err := openAIToGenerate(nil, &request)
	require.NoError(t, err)
	encoded, err = common.Marshal(generated)
	require.NoError(t, err)
	require.Equal(t, schema, gjson.GetBytes(encoded, "format").Raw)
}

func TestOllamaResponseFormatValidationAndOptionalFields(t *testing.T) {
	for _, raw := range []string{"", `null`, `{}`, `{"schema":null}`} {
		format, err := openAIResponseFormatToOllama(&dto.ResponseFormat{Type: "json_schema", JsonSchema: json.RawMessage(raw)})
		require.NoError(t, err)
		require.Nil(t, format)
	}
	format, err := openAIResponseFormatToOllama(&dto.ResponseFormat{Type: "json"})
	require.NoError(t, err)
	require.Equal(t, "json", format)
	for _, raw := range []string{`{`, `[]`, `1`} {
		request := &dto.GeneralOpenAIRequest{ResponseFormat: &dto.ResponseFormat{Type: "json_schema", JsonSchema: json.RawMessage(raw)}}
		_, err = openAIChatToOllamaChat(nil, request)
		require.Error(t, err)
		_, err = openAIToGenerate(nil, request)
		require.Error(t, err)
	}
}
