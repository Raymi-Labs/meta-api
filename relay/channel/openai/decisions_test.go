package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDecisionsResponseUsage(t *testing.T) {
	valid := `{"model":"typesafe/jev-1.14-20261014","answers":{"q":{"type":"noul","noul":0.99}},"usage":{"input_tokens":272,"output_tokens":22,"cost":0.000011424}}`
	for _, tc := range []struct {
		name, body string
		ok         bool
	}{
		{"valid", valid, true},
		{"missing usage", `{"model":"jev","answers":{"q":{}}}`, false},
		{"missing output", strings.Replace(valid, `"output_tokens":22,`, "", 1), false},
		{"negative usage", strings.Replace(valid, `272`, `-1`, 1), false},
		{"fractional usage", strings.Replace(valid, `272`, `2.5`, 1), false},
		{"empty answers", `{"model":"jev","answers":{},"usage":{"input_tokens":2,"output_tokens":0}}`, false},
		{"upstream error", `{"error":{"message":"failure"}}`, false},
		{"invalid JSON", `<html>error</html>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			u, err := decisionsResponse(c, &http.Response{Body: io.NopCloser(strings.NewReader(tc.body))}, &relaycommon.RelayInfo{})
			if tc.ok {
				require.Nil(t, err)
				require.Equal(t, tc.body, w.Body.String())
				require.Equal(t, 272, u.(*dto.Usage).PromptTokens)
				require.Equal(t, 22, u.(*dto.Usage).CompletionTokens)
			} else {
				require.NotNil(t, err)
				require.True(t, types.IsSkipRetryError(err))
				require.Empty(t, w.Body.String())
			}
		})
	}
}

func TestDecisionsOutboundBoundary(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, ChannelBaseUrl: "https://openrouter.ai/api/", UpstreamModelName: "typesafe/jev-1.14"}}
	u, err := decisionsRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://openrouter.ai/api/alpha/decisions", u)
	body := `{"model":"typesafe/jev-1.14","state":{"text":"Hello"},"questions":{"q":{"type":"noul","instructions":"Greeting?"}},"assets":{"pic":"opaque"},"provider":{"max_price":{"prompt":0}}}`
	r, err := validateDecisionsBody(info, strings.NewReader(body))
	require.NoError(t, err)
	raw, _ := io.ReadAll(r)
	require.Equal(t, body, string(raw))
	for _, invalid := range []string{`null`, strings.Replace(body, `"state":`, `"stream":true,"state":`, 1), strings.Replace(body, "typesafe/jev-1.14", "typesafe/jev-router", 1)} {
		_, err = validateDecisionsBody(info, strings.NewReader(invalid))
		require.Error(t, err)
	}
	info.ChannelType = constant.ChannelTypeOpenAI
	_, err = decisionsRequestURL(info)
	require.Error(t, err)
}
