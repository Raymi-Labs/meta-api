package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/types"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldWriteRelayErrorResponseSkipsErrorAlreadySentInResponsesStream(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.True(t, shouldWriteRelayErrorResponse(c))

	common.SetContextKey(c, constant.ContextKeyResponsesStreamErrorWritten, true)
	require.False(t, shouldWriteRelayErrorResponse(c))
}

func TestResponsesFinalErrorUsesFailureEvent(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Header("Content-Type", "text/event-stream")
	c.Writer.WriteHeaderNow()
	writeRelayStreamError(c, types.RelayFormatOpenAIResponses, types.NewErrorWithStatusCode(errors.New("unsupported video input"), types.ErrorCode("unsupported_video_input"), 400))
	require.Equal(t, 200, recorder.Code)
	require.Contains(t, recorder.Body.String(), "event: error\n")
	require.Contains(t, recorder.Body.String(), `"code":"unsupported_video_input"`)
	require.Contains(t, recorder.Body.String(), `"type":"error"`)
	require.NotContains(t, recorder.Body.String(), "[DONE]")
	require.NotContains(t, recorder.Body.String(), "response.completed")
}
