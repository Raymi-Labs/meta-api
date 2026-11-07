package gemini

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEmbeddingUsageMetadataAcrossHandlers(t *testing.T) {
	for _, mode := range []string{"compatible", "single", "batch"} {
		for _, tc := range []struct {
			name, metadata string
			tokens         int
			estimated      bool
		}{
			{"reported", `,"usageMetadata":{"promptTokenCount":37,"promptTokenDetails":[{"modality":"TEXT","tokenCount":30},{"modality":"AUDIO","tokenCount":7}]}`, 37, false},
			{"absent", "", 91, true},
			{"null", `,"usageMetadata":null`, 91, true},
			{"zero", `,"usageMetadata":{"promptTokenCount":0}`, 91, true},
			{"negative", `,"usageMetadata":{"promptTokenCount":-1}`, 91, true},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/embedding", nil)
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-embedding-001"}, IsGeminiBatchEmbedding: mode == "batch"}
				info.SetEstimatePromptTokens(91)
				vector := `"embeddings":[{"values":[0.1,0.2]}]`
				if mode == "single" {
					vector = `"embedding":{"values":[0.1,0.2]}`
				}
				body := fmt.Sprintf("{%s%s}", vector, tc.metadata)
				resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
				var usage *dto.Usage
				if mode == "compatible" {
					u, e := GeminiEmbeddingHandler(c, info, resp)
					require.Nil(t, e)
					usage = u
				} else {
					u, e := NativeGeminiEmbeddingHandler(c, resp, info)
					require.Nil(t, e)
					usage = u
				}
				require.Equal(t, tc.tokens, usage.PromptTokens)
				require.Equal(t, tc.tokens, usage.TotalTokens)
				require.Zero(t, usage.CompletionTokens)
				require.Equal(t, tc.estimated, usage.InputTokensEstimated)
				if !tc.estimated {
					require.Equal(t, 30, usage.PromptTokensDetails.TextTokens)
					require.Equal(t, 7, usage.PromptTokensDetails.AudioTokens)
				}
				if mode == "compatible" {
					var out dto.OpenAIEmbeddingResponse
					require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
					require.Equal(t, tc.tokens, out.Usage.PromptTokens)
					require.Equal(t, []float64{0.1, 0.2}, out.Data[0].Embedding)
				} else {
					require.JSONEq(t, body, w.Body.String())
				}
			})
		}
	}
}
