package gemini

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiEmbeddingAcceptsPlainTextWithoutDroppingOrRewriting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input any
		want  []string
	}{
		{"string", "  文本\n", []string{"  文本\n"}},
		{"decoded_array", []any{"first", "second", "first"}, []string{"first", "second", "first"}},
		{"typed_array", []string{"one", "two"}, []string{"one", "two"}},
		{"whitespace_is_text", " ", []string{" "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-embedding-001"}}
			result, err := (&Adaptor{}).ConvertEmbeddingRequest(nil, info, dto.EmbeddingRequest{Input: tc.input, Dimensions: 128})
			require.NoError(t, err)
			require.True(t, info.IsGeminiBatchEmbedding)
			body, err := common.Marshal(result)
			require.NoError(t, err)
			var batch dto.GeminiBatchEmbeddingRequest
			require.NoError(t, common.Unmarshal(body, &batch))
			require.Len(t, batch.Requests, len(tc.want))
			for i, text := range tc.want {
				require.Equal(t, "models/gemini-embedding-001", batch.Requests[i].Model)
				require.Equal(t, 128, batch.Requests[i].OutputDimensionality)
				require.Len(t, batch.Requests[i].Content.Parts, 1)
				require.Equal(t, text, batch.Requests[i].Content.Parts[0].Text)
			}
		})
	}
}

func TestGeminiEmbeddingRejectsEntireInvalidInput(t *testing.T) {
	for _, raw := range []string{
		`null`, `""`, `[]`, `[""]`, `["valid", ""]`,
		`123`, `true`, `{}`, `{"type":"text","text":"text"}`,
		`[1,2]`, `["valid",2]`, `["valid",false]`, `["valid",null]`,
		`[[1,2]]`, `["valid",["nested"]]`,
		`[{"type":"text","text":"text"}]`,
		`["valid",{"type":"text","text":"must not disappear"}]`,
		`[{"type":"image_url","image_url":{"url":"https://example.invalid/image"}},"valid"]`,
	} {
		t.Run(raw, func(t *testing.T) {
			var request dto.EmbeddingRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"input":`+raw+`}`, &request))
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-embedding-001"}}
			converted, err := (&Adaptor{}).ConvertEmbeddingRequest(nil, info, request)
			require.Error(t, err)
			require.Nil(t, converted, "never return a partial batch")
			require.False(t, info.IsGeminiBatchEmbedding)
			// EmbeddingHelper wraps conversion errors; the typed 400 must survive.
			apiErr := types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
			require.True(t, types.IsSkipRetryError(apiErr))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
			require.False(t, service.ShouldRetryChannelError(c, apiErr, 3))
		})
	}
	for _, input := range []any{[]string(nil), []string{}, []string{"valid", ""}, []int{1, 2}} {
		_, err := plainTextEmbeddingInputs(input)
		require.Error(t, err)
	}
}
