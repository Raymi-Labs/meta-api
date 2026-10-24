package helper_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestEmbeddingMissingInputReturnsBadRequest(t *testing.T) {
	for _, body := range []string{`null`, `{"model":"text-embedding-3-small"}`, `{"model":"text-embedding-3-small","input":null}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		defer common.CleanupBodyStorage(c)
		_, err := helper.GetAndValidateEmbeddingRequest(c, relayconstant.RelayModeEmbeddings)
		require.Error(t, err)
		apiErr := types.NewError(err, types.ErrorCodeInvalidRequest)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		require.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
		require.True(t, types.IsSkipRetryError(apiErr))
	}
}

func TestEmbeddingValidationPreservesOpenAITokenIDs(t *testing.T) {
	for _, input := range []string{`[1,2,3]`, `[[1,2],[3,4]]`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"text-embedding-3-small","input":`+input+`}`))
		c.Request.Header.Set("Content-Type", "application/json")
		defer common.CleanupBodyStorage(c)
		request, err := helper.GetAndValidateEmbeddingRequest(c, relayconstant.RelayModeEmbeddings)
		require.NoError(t, err)
		converted, err := (&openai.Adaptor{}).ConvertEmbeddingRequest(c, &relaycommon.RelayInfo{}, *request)
		require.NoError(t, err)
		body, err := common.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, input, gjson.GetBytes(body, "input").Raw)
	}
}
