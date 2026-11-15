package claude

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptorConvertOpenAIRequestAppliesIncrementalCache(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := &dto.GeneralOpenAIRequest{
		Model: "claude-opus-4-8",
		Messages: []dto.Message{
			{Role: "user", Content: "first question"},
			{Role: "assistant", Content: "first answer"},
			{Role: "user", Content: "latest question"},
		},
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{ClaudeIncrementalCacheEnabled: common.GetPointer(true)},
	}}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(ctx, info, request)
	require.NoError(t, err)
	claudeRequest, ok := converted.(*dto.ClaudeRequest)
	require.True(t, ok)
	require.Len(t, claudeRequest.Messages, 3)
	assert.Equal(t, "first answer", claudeRequest.Messages[1].Content)

	assert.Equal(t, "first question", claudeRequest.Messages[0].Content)

	latestContent, err := claudeRequest.Messages[2].ParseContent()
	require.NoError(t, err)
	require.Len(t, latestContent, 1)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(latestContent[0].CacheControl))
	assert.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyClaudeIncrementalCache))
}

func TestAdaptorConvertOpenAIRequestLeavesCacheDisabled(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "claude-opus-4-8",
		Messages: []dto.Message{
			{Role: "user", Content: "latest question"},
		},
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{ClaudeIncrementalCacheEnabled: common.GetPointer(false)}}}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)
	require.NoError(t, err)
	claudeRequest, ok := converted.(*dto.ClaudeRequest)
	require.True(t, ok)
	require.Len(t, claudeRequest.Messages, 1)
	assert.Equal(t, "latest question", claudeRequest.Messages[0].Content)
}

func TestAdaptorConvertClaudeRequestDoesNotApplyIncrementalCache(t *testing.T) {
	request := &dto.ClaudeRequest{
		Model: "claude-opus-4-8",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "native request"},
		},
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{ClaudeIncrementalCacheEnabled: common.GetPointer(true)},
	}}

	converted, err := (&Adaptor{}).ConvertClaudeRequest(nil, info, request)
	require.NoError(t, err)
	assert.Same(t, request, converted)
	assert.Equal(t, "native request", request.Messages[0].Content)
}

func TestApplyClaudeIncrementalCacheKeepsLatestFourBreakpoints(t *testing.T) {
	systemText := "system"
	request := &dto.ClaudeRequest{
		System: []dto.ClaudeMediaMessage{{
			Type:         "text",
			Text:         &systemText,
			CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
		}},
	}
	for index := 0; index < 4; index++ {
		text := "cached"
		request.Messages = append(request.Messages, dto.ClaudeMessage{
			Role: "user",
			Content: []dto.ClaudeMediaMessage{{
				Type:         "text",
				Text:         &text,
				CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
			}},
		})
	}
	request.Messages = append(request.Messages, dto.ClaudeMessage{Role: "user", Content: "latest"})

	require.True(t, applyClaudeIncrementalCache(request))
	assert.Len(t, collectClaudeCacheControls(request), maxClaudeCacheControlBlocks)

	system := request.System.([]dto.ClaudeMediaMessage)
	assert.Empty(t, system[0].CacheControl)
	firstMessage, err := request.Messages[0].ParseContent()
	require.NoError(t, err)
	assert.Empty(t, firstMessage[0].CacheControl)
	latestMessage, err := request.Messages[len(request.Messages)-1].ParseContent()
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(latestMessage[0].CacheControl))
}

func TestLegacyChannelDefaultsToIncrementalCache(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{Model: "claude-opus-5-5", Messages: []dto.Message{{Role: "user", Content: "history"}}}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)
	require.NoError(t, err)
	blocks, err := converted.(*dto.ClaudeRequest).Messages[0].ParseContent()
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"ephemeral"}`, string(blocks[0].CacheControl))
}
