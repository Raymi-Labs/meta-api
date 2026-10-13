package channel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["X-Upstream-Trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest:   false,
		OriginModelName: "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["X-Trace-Id"])

	_, hasAcceptEncoding := headers["Accept-Encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestAttachNewAPIRequestIDHeader(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	attachNewAPIRequestIDHeader(header, &relaycommon.RelayInfo{RequestId: " req-123 "})

	require.Equal(t, "req-123", header.Get(upstreamNewAPIRequestIDHeader))
}

func TestAttachNewAPIRequestIDHeaderSkipsBlank(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	attachNewAPIRequestIDHeader(header, &relaycommon.RelayInfo{RequestId: "   "})

	require.Empty(t, header.Get(upstreamNewAPIRequestIDHeader))
}

func TestAttachNewAPIRequestIDHeaderAllowsHeaderOverride(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	attachNewAPIRequestIDHeader(header, &relaycommon.RelayInfo{RequestId: "req-123"})
	req := &http.Request{Header: header}
	applyHeaderOverrideToRequest(req, map[string]string{
		upstreamNewAPIRequestIDHeader: "override-req",
	})

	require.Equal(t, "override-req", req.Header.Get(upstreamNewAPIRequestIDHeader))
}

func TestProcessHeaderOverride_StripsDeprecatedContextBetaForClaude46RuntimeOverride(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName:           "claude-sonnet-4-6",
		ChannelMeta:               &relaycommon.ChannelMeta{},
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"anthropic-beta": "context-1m-2025-08-07,computer-use-2025-01-24",
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "computer-use-2025-01-24", headers["anthropic-beta"])
}

func TestNewDoRequestErrorMapsResponseHeaderTimeoutToGatewayTimeout(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)

	apiErr := newDoRequestError(ctx, errors.New(`Post "http://example.test/v1/images/edits": net/http: timeout awaiting response headers`))
	require.Equal(t, http.StatusGatewayTimeout, apiErr.StatusCode)
	require.True(t, types.IsSkipRetryError(apiErr))
	require.Contains(t, apiErr.Error(), "upstream response header timeout")
}

func TestNewDoRequestErrorKeepsClientCancellationClassification(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestCtx)
	apiErr := newDoRequestError(ctx, context.Canceled)
	apiErr.SetMessage(apiErr.Error() + " (request id: req-canceled)")
	require.Equal(t, types.StatusClientClosedRequest, apiErr.StatusCode)
	require.Equal(t, types.FailureClientCanceled, types.ClassifyFailure(apiErr))
	require.True(t, types.IsSkipRetryError(apiErr))
}

func TestShouldUseImageHTTPClientForResponsesImageGenerationTool(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		Request: &dto.OpenAIResponsesRequest{
			Tools: []byte(`[{"type":"image_generation","size":"auto"}]`),
		},
	}

	require.True(t, shouldUseImageHTTPClient(info))
}

func TestShouldUseImageHTTPClientKeepsTextResponsesOnDefaultClient(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		Request: &dto.OpenAIResponsesRequest{
			Tools: []byte(`[{"type":"web_search_preview"}]`),
		},
	}

	require.False(t, shouldUseImageHTTPClient(info))
}
