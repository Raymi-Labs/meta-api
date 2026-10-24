package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const schemaOrderFixture = `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"integer","maximum":9007199254740993}},"required":["reasoning","answer"]}`

func TestSchemaJSONPreservesClientToolPayloads(t *testing.T) {
	var request GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"gpt-5","tools":[{"type":"function","function":{"name":"answer","parameters":`+schemaOrderFixture+`}}]}`), &request))
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, schemaOrderFixture, gjson.GetBytes(encoded, "tools.0.function.parameters").Raw)
	require.Contains(t, request.GetTokenCountMeta().CombineText, schemaOrderFixture)
	var claude ClaudeRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"claude-test","tools":[{"name":"answer","input_schema":`+schemaOrderFixture+`,"cache_control":{"type":"ephemeral"}},{"type":"web_search_20250305","name":"web_search"}]}`), &claude))
	functions, searches := ProcessTools(claude.GetTools())
	require.Len(t, functions, 1)
	require.Len(t, searches, 1)
	encoded, err = common.Marshal(functions[0])
	require.NoError(t, err)
	require.Equal(t, schemaOrderFixture, gjson.GetBytes(encoded, "input_schema").Raw)
	claude.AddTool(&Tool{Name: "extra", InputSchema: map[string]any{"type": "object"}})
	encoded, err = common.Marshal(claude)
	require.NoError(t, err)
	require.Equal(t, schemaOrderFixture, gjson.GetBytes(encoded, "tools.0.input_schema").Raw)
	require.Equal(t, "ephemeral", gjson.GetBytes(encoded, "tools.0.cache_control.type").String())
	require.Equal(t, "extra", gjson.GetBytes(encoded, "tools.2.name").String())
}

func TestSchemaJSONNullOptionalFieldsStayAbsent(t *testing.T) {
	var request ClaudeRequest
	require.NoError(t, common.Unmarshal([]byte(`{"tools":null}`), &request))
	require.Nil(t, request.Tools)
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(encoded, "tools").Exists())
	for _, source := range []string{`{"name":"f"}`, `{"name":"f","parameters":null}`} {
		var function FunctionRequest
		require.NoError(t, common.Unmarshal([]byte(source), &function))
		require.Nil(t, function.Parameters)
	}
}

func TestSchemaJSONKeepsClaudeInputSchemaValidation(t *testing.T) {
	for _, invalid := range []string{`[]`, `true`, `"text"`, `1`} {
		var tool Tool
		require.Error(t, common.Unmarshal([]byte(`{"name":"f","input_schema":`+invalid+`}`), &tool))
		var request ClaudeRequest
		require.NoError(t, common.Unmarshal([]byte(`{"tools":[{"name":"f","input_schema":`+invalid+`}]}`), &request))
		normal, _ := ProcessTools(request.GetTools())
		require.Empty(t, normal)
	}
	for _, schema := range []string{`null`, `{}`} {
		var tool Tool
		require.NoError(t, common.Unmarshal([]byte(`{"name":"f","input_schema":`+schema+`}`), &tool))
	}
}

func TestSchemaJSONPreservesGeminiToolsAndOutputSchema(t *testing.T) {
	for _, schemaField := range []string{"responseSchema", "response_schema"} {
		var request GeminiChatRequest
		require.NoError(t, common.Unmarshal([]byte(`{"tools":[{"functionDeclarations":[{"name":"answer","parameters":`+schemaOrderFixture+`}]}],"generationConfig":{"`+schemaField+`":`+schemaOrderFixture+`}}`), &request))
		request.SetTools(request.GetTools())
		encoded, err := common.Marshal(request)
		require.NoError(t, err)
		require.Equal(t, schemaOrderFixture, gjson.GetBytes(encoded, "tools.0.functionDeclarations.0.parameters").Raw)
		require.Equal(t, schemaOrderFixture, gjson.GetBytes(encoded, "generationConfig.responseSchema").Raw)
	}
}
