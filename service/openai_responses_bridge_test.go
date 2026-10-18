package service

import (
	"encoding/base64"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

func setResponsesBridgeTestOptions(t *testing.T, updates map[string]string) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	originalValues := make(map[string]string, len(updates))
	originalExists := make(map[string]bool, len(updates))
	for key, value := range updates {
		originalValues[key], originalExists[key] = common.OptionMap[key]
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		for key := range updates {
			if originalExists[key] {
				common.OptionMap[key] = originalValues[key]
			} else {
				delete(common.OptionMap, key)
			}
		}
		common.OptionMapRWMutex.Unlock()
	})
}

func TestClaudeImageSourceToMessageImageURL_UsesSignedBridgeURL(t *testing.T) {
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesMediaTransportModeOption: "bridge",
		responsesMediaBridgeEnabledOption: "true",
	})

	originalServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = ""
	t.Cleanup(func() {
		system_setting.ServerAddress = originalServerAddress
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "https://example.com/v1/messages", nil)
	ctx.Request.Host = "example.com"

	imageURL, err := ClaudeImageSourceToMessageImageURL(ctx, &dto.ClaudeMessageSource{
		Type:      "base64",
		MediaType: "image/png",
		Data:      "aGVsbG8=",
	})
	require.NoError(t, err)
	require.NotNil(t, imageURL)
	require.Contains(t, imageURL.Url, "/v1/bridge/media/")
	require.NotContains(t, imageURL.Url, "data:")

	parsedURL, err := url.Parse(imageURL.Url)
	require.NoError(t, err)
	id := strings.TrimPrefix(parsedURL.Path, "/v1/bridge/media/")
	require.NotEmpty(t, id)
	t.Cleanup(func() {
		_ = openAIResponsesMediaStore.delete(id)
	})

	getRecorder := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRecorder)
	getCtx.Request = httptest.NewRequest("GET", imageURL.Url, nil)
	getCtx.Params = gin.Params{{Key: "id", Value: id}}

	ServeOpenAIResponsesMedia(getCtx)

	require.Equal(t, 200, getRecorder.Code)
	require.Equal(t, "image/png", getRecorder.Header().Get("Content-Type"))
	require.Equal(t, "hello", getRecorder.Body.String())
}

func TestClaudeImageSourceToMessageImageURL_FallsBackToDataURLWhenBridgeDisabled(t *testing.T) {
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesMediaTransportModeOption: "bridge",
		responsesMediaBridgeEnabledOption: "false",
	})

	imageURL, err := ClaudeImageSourceToMessageImageURL(nil, &dto.ClaudeMessageSource{
		Type:      "base64",
		MediaType: "image/png",
		Data:      "aGVsbG8=",
	})
	require.NoError(t, err)
	require.NotNil(t, imageURL)
	require.Equal(t, "data:image/png;base64,aGVsbG8=", imageURL.Url)
}

func TestClaudeImageSourceToMessageImageURL_PreservesRemoteURL(t *testing.T) {
	imageURL, err := ClaudeImageSourceToMessageImageURL(nil, &dto.ClaudeMessageSource{
		Type: "url",
		Url:  "https://cdn.example.com/image.png",
	})
	require.NoError(t, err)
	require.NotNil(t, imageURL)
	require.Equal(t, "https://cdn.example.com/image.png", imageURL.Url)
}

func TestClaudeImageSourceToMessageImageURL_AutoUsesDataForSmallPayload(t *testing.T) {
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesMediaTransportModeOption: "auto",
		responsesMediaBridgeEnabledOption: "true",
	})

	originalSingleLimit := responsesMediaAutoSingleInlineLimit
	originalTotalLimit := responsesMediaAutoTotalInlineLimit
	responsesMediaAutoSingleInlineLimit = 16
	responsesMediaAutoTotalInlineLimit = 32
	t.Cleanup(func() {
		responsesMediaAutoSingleInlineLimit = originalSingleLimit
		responsesMediaAutoTotalInlineLimit = originalTotalLimit
	})

	imageURL, err := ClaudeImageSourceToMessageImageURL(nil, &dto.ClaudeMessageSource{
		Type:      "base64",
		MediaType: "image/png",
		Data:      "aGVsbG8=",
	})
	require.NoError(t, err)
	require.NotNil(t, imageURL)
	require.Equal(t, "data:image/png;base64,aGVsbG8=", imageURL.Url)
}

func TestClaudeImageSourceToMessageImageURL_AutoUsesBridgeWhenThresholdExceeded(t *testing.T) {
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesMediaTransportModeOption: "auto",
		responsesMediaBridgeEnabledOption: "true",
	})

	originalSingleLimit := responsesMediaAutoSingleInlineLimit
	originalTotalLimit := responsesMediaAutoTotalInlineLimit
	responsesMediaAutoSingleInlineLimit = 8
	responsesMediaAutoTotalInlineLimit = 12
	t.Cleanup(func() {
		responsesMediaAutoSingleInlineLimit = originalSingleLimit
		responsesMediaAutoTotalInlineLimit = originalTotalLimit
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "https://example.com/v1/messages", nil)
	ctx.Request.Host = "example.com"

	rawBytes := strings.Repeat("a", 7)
	base64Data := base64.StdEncoding.EncodeToString([]byte(rawBytes))

	imageURL, err := ClaudeImageSourceToMessageImageURL(ctx, &dto.ClaudeMessageSource{
		Type:      "base64",
		MediaType: "image/png",
		Data:      base64Data,
	})
	require.NoError(t, err)
	require.NotNil(t, imageURL)
	require.Contains(t, imageURL.Url, "/v1/bridge/media/")
}

func TestClaudeImageSourceToMessageImageURL_ChannelOverrideWins(t *testing.T) {
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesMediaTransportModeOption: "bridge",
		responsesMediaBridgeEnabledOption: "true",
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "https://example.com/v1/messages", nil)
	ctx.Request.Host = "example.com"
	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		ClaudeImageTransportMode: dto.ClaudeImageTransportModeData,
	})

	imageURL, err := ClaudeImageSourceToMessageImageURL(ctx, &dto.ClaudeMessageSource{
		Type:      "base64",
		MediaType: "image/png",
		Data:      "aGVsbG8=",
	})
	require.NoError(t, err)
	require.NotNil(t, imageURL)
	require.Equal(t, "data:image/png;base64,aGVsbG8=", imageURL.Url)
}

func TestResponsesSessionBridge_FindsLongestPrefix(t *testing.T) {
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesSessionBridgeEnabledOption:  "true",
		responsesSessionBridgeUseRedisOption: "false",
	})
	openAIResponsesSessionStore.mu.Lock()
	openAIResponsesSessionStore.entries = make(map[string]map[string]*responsesSessionEntry)
	openAIResponsesSessionStore.once = sync.Once{}
	openAIResponsesSessionStore.mu.Unlock()

	info := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         2,
		OriginModelName: "gpt-5",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 3,
		},
	}

	initialReq := &dto.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []dto.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hello"},
		},
	}
	assistant := dto.Message{Role: "assistant", Content: "hi"}

	err := StoreResponsesSessionBridge(info, initialReq, assistant, "resp_1")
	require.NoError(t, err)

	nextReq := &dto.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []dto.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "what's next?"},
		},
	}

	match, err := ApplyResponsesSessionBridge(info, nextReq)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.Equal(t, "resp_1", match.ResponseID)
	require.Equal(t, 3, match.PrefixLength)
	require.Len(t, match.Trimmed.Messages, 1)
	require.Equal(t, "what's next?", match.Trimmed.Messages[0].StringContent())
}

func TestResponsesSessionBridge_DisabledSkipsMatch(t *testing.T) {
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesSessionBridgeEnabledOption: "false",
	})

	info := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         2,
		OriginModelName: "gpt-5",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 3,
		},
	}
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []dto.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hello"},
		},
	}

	err := StoreResponsesSessionBridge(info, req, dto.Message{Role: "assistant", Content: "hi"}, "resp_1")
	require.NoError(t, err)

	match, err := ApplyResponsesSessionBridge(info, &dto.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []dto.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "again"},
		},
	})
	require.NoError(t, err)
	require.Nil(t, match)
}

func TestResponsesSessionBridge_DeepSeekKeepsFullHistory(t *testing.T) {
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesSessionBridgeEnabledOption:  "true",
		responsesSessionBridgeUseRedisOption: "false",
	})
	info := &relaycommon.RelayInfo{UserId: 997901, TokenId: 997902, OriginModelName: "deepseek-flash",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 997903, ChannelType: constant.ChannelTypeDeepSeek, ApiType: constant.APITypeDeepSeek, ChannelBaseUrl: "https://api.deepseek.com", UpstreamModelName: "deepseek-flash"}}
	initial := &dto.GeneralOpenAIRequest{Model: "deepseek-flash", Messages: []dto.Message{{Role: "user", Content: "Hi"}}}
	require.NoError(t, StoreResponsesSessionBridge(info, initial, dto.Message{Role: "assistant", Content: "OK"}, "resp_deepseek"))
	next := &dto.GeneralOpenAIRequest{Model: "deepseek-flash", Messages: []dto.Message{{Role: "user", Content: "Hi"}, {Role: "assistant", Content: "OK"}, {Role: "user", Content: "Continue"}}}
	match, err := ApplyResponsesSessionBridge(info, next)
	require.NoError(t, err)
	require.Nil(t, match)
	require.Len(t, next.Messages, 3)
	require.True(t, hasResponsesPreviousResponseIDUnsupported(info))
}

func TestResponsesSessionBridge_PreviousResponseIDUnsupportedSkipsMatch(t *testing.T) {
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesSessionBridgeEnabledOption:  "true",
		responsesSessionBridgeUseRedisOption: "false",
	})
	openAIResponsesSessionStore.mu.Lock()
	openAIResponsesSessionStore.entries = make(map[string]map[string]*responsesSessionEntry)
	openAIResponsesSessionStore.once = sync.Once{}
	openAIResponsesSessionStore.mu.Unlock()

	info := &relaycommon.RelayInfo{
		UserId:          101,
		TokenId:         102,
		OriginModelName: "gpt-5",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         103,
			ApiType:           constant.APITypeOpenAI,
			ChannelBaseUrl:    "https://example.com",
			UpstreamModelName: "gpt-5",
		},
	}

	initialReq := &dto.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []dto.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hello"},
		},
	}

	err := StoreResponsesSessionBridge(info, initialReq, dto.Message{Role: "assistant", Content: "hi"}, "resp_1")
	require.NoError(t, err)

	MarkResponsesPreviousResponseIDUnsupported(info)

	match, err := ApplyResponsesSessionBridge(info, &dto.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []dto.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "again"},
		},
	})
	require.NoError(t, err)
	require.Nil(t, match)
}

func TestResponsesMediaBridgeCleanupOrphanedFiles_RemovesResiduals(t *testing.T) {
	tempDir := t.TempDir()
	setResponsesBridgeTestOptions(t, map[string]string{
		responsesMediaBridgePathOption: tempDir,
	})

	bridgeDir := filepath.Join(tempDir, "new-api-responses-media-bridge")
	require.NoError(t, os.MkdirAll(bridgeDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bridgeDir, "orphan.bin"), []byte("hello"), 0600))

	stats, err := openAIResponsesMediaStore.cleanupOrphanedFiles(true)
	require.NoError(t, err)
	require.Equal(t, 1, stats.DeletedFiles)

	entries, err := os.ReadDir(bridgeDir)
	require.NoError(t, err)
	require.Len(t, entries, 0)
}
