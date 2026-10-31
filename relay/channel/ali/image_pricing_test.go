package ali

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageModelsUseMappedSyncProtocolAndReferences(t *testing.T) {
	for _, name := range []string{"qwen-image-3.0-pro", "wan2.7-image-pro", "z-image-turbo"} {
		info := &relaycommon.RelayInfo{OriginModelName: "my-alias", RelayMode: relayconstant.RelayModeImagesGenerations, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: name, ChannelBaseUrl: "https://example.com"}}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
		a := &Adaptor{}
		a.Init(info)
		req := dto.ImageRequest{Model: name, Prompt: "draw", Size: "1024x1024"}
		converted, err := a.ConvertImageRequest(c, info, req)
		require.NoError(t, err)
		require.True(t, a.IsSyncImageModel)
		require.Equal(t, "1024*1024", converted.(*AliImageRequest).Parameters.Size)
		url, err := a.GetRequestURL(info)
		require.NoError(t, err)
		require.Equal(t, "https://example.com/api/v1/services/aigc/multimodal-generation/generation", url)
		headers := http.Header{}
		require.NoError(t, a.SetupRequestHeader(c, &headers, info))
		require.Empty(t, headers.Get("X-DashScope-Async"))
		if name == "z-image-turbo" {
			continue
		}
		info.RelayMode = relayconstant.RelayModeImagesEdits
		require.NoError(t, common.Unmarshal([]byte(`{"model":"`+name+`","prompt":"edit","image":["https://example.com/a","https://example.com/b"],"size":"1024x1024"}`), &req))
		converted, err = a.ConvertImageRequest(c, info, req)
		require.NoError(t, err)
		require.Len(t, converted.(*AliImageRequest).Input.(AliImageInput).Messages[0].Content, 3)
		require.True(t, a.IsSyncImageModel)
		url, err = a.GetRequestURL(info)
		require.NoError(t, err)
		require.Contains(t, url, "multimodal-generation/generation")
		require.NoError(t, a.SetupRequestHeader(c, &headers, info))
		require.Empty(t, headers.Get("X-DashScope-Async"))
	}
}
func TestAliImageResponseRequiresPayloadAndReportsActualCount(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{"output":{"choices":[{"message":{"content":[{"image":"https://example.com/a.png"}]}}]},"usage":{"image_count":3}}`, true},
		{`{"output":{"choices":[{"message":{"content":[{"text":"no image"}]}}]}}`, false},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		info := &relaycommon.RelayInfo{}
		resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}
		apiErr, _ := aliImageHandler(&Adaptor{IsSyncImageModel: true}, c, resp, info)
		if tc.ok {
			require.Nil(t, apiErr)
			require.Equal(t, 1, *info.ImageResponseCount)
		} else {
			require.NotNil(t, apiErr)
		}
	}
}
