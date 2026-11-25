package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiRawSchemaCleaningKeepsOrderAndRules(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"reasoning":{"type":"string","examples":["x"]},"answer":{"type":["integer","null"],"maximum":9007199254740993},"values":{"type":"array","items":[{"type":"string","additionalProperties":false},{"type":"integer"}]}},"required":["reasoning","answer"]}`)
	var legacy any
	require.NoError(t, common.Unmarshal(raw, &legacy))
	cleaned := cleanFunctionParameters(raw)
	encoded, err := common.Marshal(cleaned)
	require.NoError(t, err)
	legacyEncoded, err := common.Marshal(cleanFunctionParameters(legacy))
	require.NoError(t, err)
	// Compare structural behavior separately from exact large-number fidelity.
	require.JSONEq(t, string(legacyEncoded), string(encoded))
	properties := gjson.GetBytes(encoded, "properties")
	var keys []string
	properties.ForEach(func(key, _ gjson.Result) bool { keys = append(keys, key.String()); return true })
	require.Equal(t, []string{"reasoning", "answer", "values"}, keys)
	require.Equal(t, "9007199254740993", properties.Get("answer.maximum").Raw)
	require.Equal(t, "INTEGER", properties.Get("answer.type").String())
	require.True(t, properties.Get("answer.nullable").Bool())
	require.False(t, properties.Get("reasoning.examples").Exists())
	require.Equal(t, "STRING", properties.Get("values.items.type").String())
	require.Nil(t, cleanFunctionParameters(json.RawMessage(`{"type":"object","properties":{}}`)))
	require.JSONEq(t, `{"nullable":true}`, string(cleanRawFunctionSchema(json.RawMessage(`{"type":"null"}`), 0)))
}

func TestGeminiRawSchemaCleaningDepthBoundary(t *testing.T) {
	raw := `{"type":"string","title":"leaf"}`
	for i := 0; i < geminiFunctionSchemaMaxDepth+3; i++ {
		raw = `{"type":"object","properties":{"nested":` + raw + `},"additionalProperties":false}`
	}
	cleaned := cleanRawFunctionSchema(json.RawMessage(raw), 0)
	require.True(t, common.Valid(cleaned))
	path := strings.Repeat("properties.nested.", geminiFunctionSchemaMaxDepth)
	require.Equal(t, "OBJECT", gjson.GetBytes(cleaned, path+"type").String())
	require.False(t, gjson.GetBytes(cleaned, path+"properties").Exists())
	require.Equal(t, "[]", string(cleanRawFunctionSchema(json.RawMessage(`[{"type":"string"}]`), geminiFunctionSchemaMaxDepth)))
	response := cleanRawResponseSchema(json.RawMessage(raw), 0)
	require.False(t, gjson.GetBytes(response, strings.Repeat("properties.nested.", 4)+"additionalProperties").Exists())
	require.True(t, gjson.GetBytes(response, strings.Repeat("properties.nested.", 5)+"additionalProperties").Exists())
}

func TestGeminiConversionPreservesToolAndOutputSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"reasoning":{"type":"string","title":"Reason"},"answer":{"type":"integer","minimum":9007199254740993}},"additionalProperties":false}`
	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"answer"}],"tools":[{"type":"function","function":{"name":"answer","parameters":`+schema+`}}],"response_format":{"type":"json_schema","json_schema":{"name":"answer","schema":`+schema+`}}}`), &request))
	converted, err := CovertOpenAI2Gemini(nil, request, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: request.Model}})
	require.NoError(t, err)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	for _, path := range []string{"tools.0.functionDeclarations.0.parameters", "generationConfig.responseSchema"} {
		properties := gjson.GetBytes(encoded, path+".properties")
		var keys []string
		properties.ForEach(func(key, _ gjson.Result) bool { keys = append(keys, key.String()); return true })
		require.Equal(t, []string{"reasoning", "answer"}, keys)
		require.Equal(t, "9007199254740993", properties.Get("answer.minimum").Raw)
		require.False(t, gjson.GetBytes(encoded, path+".additionalProperties").Exists())
	}
	require.False(t, gjson.GetBytes(encoded, "generationConfig.responseSchema.properties.reasoning.title").Exists())
}

func TestNullableToolUnion(t *testing.T) {
	for _, tc := range []struct{ name, schema, want string }{
		{"CronCreate", `{"description":"delay","anyOf":[{"type":"integer","description":"inner","exclusiveMinimum":0,"maximum":525600},{"type":"null"}]}`, `{"type":"INTEGER","nullable":true,"description":"delay","maximum":525600}`},
		{"reversed_nested", `{"type":"array","items":{"oneOf":[{"type":"null"},{"type":"object","properties":{"model":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["model"]}]}}`, `{"type":"ARRAY","items":{"type":"OBJECT","nullable":true,"properties":{"model":{"type":"STRING","nullable":true}},"required":["model"]}}`},
		{"real_union", `{"anyOf":[{"type":"string"},{"type":"integer"}]}`, `{"anyOf":[{"type":"STRING"},{"type":"INTEGER"}]}`},
		{"outer_constraint", `{"type":"integer","anyOf":[{"type":"integer"},{"type":"null"}]}`, `{"type":"INTEGER","anyOf":[{"type":"INTEGER"},{"nullable":true}]}`},
		{"constrained_null", `{"anyOf":[{"type":"string"},{"type":"null","enum":[]}]}`, `{"anyOf":[{"type":"STRING"},{"enum":[],"nullable":true}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var legacy any
			require.NoError(t, common.Unmarshal([]byte(tc.schema), &legacy))
			for _, input := range []any{json.RawMessage(tc.schema), legacy} {
				cleaned, err := common.Marshal(cleanFunctionParameters(input))
				require.NoError(t, err)
				require.JSONEq(t, tc.want, string(cleaned))
			}
		})
	}
}
