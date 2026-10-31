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
