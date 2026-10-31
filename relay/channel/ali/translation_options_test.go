package ali

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestTranslationOptionsSurviveAliConversion(t *testing.T) {
	for _, options := range []string{`,"translation_options":{"source_lang":"zh","target_lang":"en"}`, ""} {
		t.Run(options, func(t *testing.T) {
			var request dto.GeneralOpenAIRequest
			body := `{"model":"qwen3-livetranslate-flash","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"https://example.com/test.wav","format":"wav"}}]}],"stream":true,"modalities":["text"]` + options + `}`
			require.NoError(t, common.Unmarshal([]byte(body), &request))
			converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, &relaycommon.RelayInfo{}, &request)
			require.NoError(t, err)
			wire, err := common.Marshal(converted)
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, common.Unmarshal(wire, &result))
			if options == "" {
				require.NotContains(t, result, "translation_options")
			} else {
				require.Equal(t, map[string]any{"source_lang": "zh", "target_lang": "en"}, result["translation_options"])
			}
			require.Equal(t, true, result["stream"])
			require.Equal(t, []any{"text"}, result["modalities"])
		})
	}
}

func TestQwenMTTextOptionsAndSampling(t *testing.T) {
	for _, topP := range []float64{0, 1, 0.5} {
		var request dto.GeneralOpenAIRequest
		require.NoError(t, common.Unmarshal([]byte(`{"model":"qwen-mt-flash","messages":[{"role":"user","content":"A model description"}],"translation_options":{"source_lang":"auto","target_lang":"Traditional Chinese","terms":[{"source":"API","target":"API"}]}}`), &request))
		request.TopP = topP
		converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, &relaycommon.RelayInfo{}, &request)
		require.NoError(t, err)
		wire, err := common.Marshal(converted)
		require.NoError(t, err)
		var actual dto.GeneralOpenAIRequest
		require.NoError(t, common.Unmarshal(wire, &actual))
		require.JSONEq(t, string(request.TranslationOptions), string(actual.TranslationOptions))
		require.Equal(t, topP, actual.TopP)
		require.Len(t, actual.Messages, 1)
	}
	// Ordinary Qwen requests keep the existing sampling compatibility behavior.
	require.Equal(t, 0.001, requestOpenAI2Ali(dto.GeneralOpenAIRequest{Model: "qwen-plus"}).TopP)
}
