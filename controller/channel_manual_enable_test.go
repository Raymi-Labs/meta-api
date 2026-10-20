package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func seedManualEnableMultiKeyChannel(t *testing.T, id int, tag string) *model.Channel {
	t.Helper()

	priority := int64(0)
	weight := uint(10)
	channelVendor := model.ChannelVendorMiMo
	channel := &model.Channel{
		Id:            id,
		Name:          "recover-channel",
		Key:           "key-1\nkey-2",
		Type:          constant.ChannelTypeOpenAI,
		Status:        common.ChannelStatusAutoDisabled,
		Group:         "default",
		Models:        "gpt-4o",
		ChannelVendor: &channelVendor,
		Tag:           common.GetPointer(tag),
		Priority:      &priority,
		Weight:        &weight,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:                   true,
			MultiKeySize:                 2,
			MultiKeyStatusList:           map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason:       map[int]string{0: "quota", 1: "quota"},
			MultiKeyDisabledTime:         map[int]int64{0: 1_800_000_000, 1: 1_800_000_001},
			MultiKeyPendingDisableUntil:  map[int]int64{0: 1_900_000_000},
			MultiKeyPendingDisableReason: map[int]string{0: "upstream unstable"},
		},
	}
	channel.SetPendingDisable(1_900_000_123, "wait for confirm")
	require.NoError(t, channel.Insert())
	return channel
}

func loadChannelForAssertion(t *testing.T, id int) *model.Channel {
	t.Helper()

	channel, err := model.GetChannelById(id, true)
	require.NoError(t, err)
	channel.PopulateRuntimeAvailability()
	return channel
}

func TestUpdateChannel_ManualEnableClearsTemporaryUnavailableState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedManualEnableMultiKeyChannel(t, 101, "manual-enable")

	body, err := common.Marshal(map[string]any{
		"id":     101,
		"status": common.ChannelStatusEnabled,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateChannel(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	channel := loadChannelForAssertion(t, 101)
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	require.Empty(t, channel.ChannelInfo.MultiKeyStatusList)
	require.Empty(t, channel.ChannelInfo.MultiKeyDisabledReason)
	require.Empty(t, channel.ChannelInfo.MultiKeyDisabledTime)
	require.Empty(t, channel.ChannelInfo.MultiKeyPendingDisableUntil)
	require.Empty(t, channel.ChannelInfo.MultiKeyPendingDisableReason)
	require.Zero(t, channel.PendingDisableUntil)
	require.Empty(t, channel.PendingDisableReason)
	require.True(t, channel.EffectiveAvailable)
	require.Equal(t, model.ChannelVendorMiMo, channel.GetChannelVendorSetting())
}

func TestUpdateChannel_UpdatesExplicitChannelVendor(t *testing.T) {
	for _, vendor := range []string{"protocol", "deepseek", "openai", "anthropic", "google", "qwen", "moonshot", "zhipu", "xai", "minimax", "mistral", "mimo", "inception", "auto"} {
		t.Run(vendor, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			setupChannelSearchControllerTestDB(t)
			seedManualEnableMultiKeyChannel(t, 104, "vendor-setting")

			body, err := common.Marshal(map[string]any{
				"id":             104,
				"channel_vendor": vendor,
			})
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			UpdateChannel(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)

			channel := loadChannelForAssertion(t, 104)
			require.Equal(t, vendor, channel.GetChannelVendorSetting())
		})
	}
}

func TestEnableTagChannels_ManualEnableClearsTemporaryUnavailableState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedManualEnableMultiKeyChannel(t, 102, "recover-tag")

	body, err := common.Marshal(map[string]any{
		"tag": "recover-tag",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/tag/enabled", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	EnableTagChannels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	channel := loadChannelForAssertion(t, 102)
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	require.Empty(t, channel.ChannelInfo.MultiKeyStatusList)
	require.Empty(t, channel.ChannelInfo.MultiKeyPendingDisableUntil)
	require.True(t, channel.EffectiveAvailable)
}

func TestManageMultiKeys_EnableKeyRestoresChannelAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	channel := seedManualEnableMultiKeyChannel(t, 103, "recover-key")
	channel.ClearPendingDisable()
	channel.ChannelInfo.MultiKeyPendingDisableUntil = map[int]int64{0: 1_900_000_000}
	channel.ChannelInfo.MultiKeyPendingDisableReason = map[int]string{0: "upstream unstable"}
	require.NoError(t, channel.SaveWithoutKey())

	body, err := common.Marshal(map[string]any{
		"channel_id": 103,
		"action":     "enable_key",
		"key_index":  0,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key/manage", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ManageMultiKeys(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	updated := loadChannelForAssertion(t, 103)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.NotContains(t, updated.ChannelInfo.MultiKeyStatusList, 0)
	require.NotContains(t, updated.ChannelInfo.MultiKeyPendingDisableUntil, 0)
	require.NotContains(t, updated.ChannelInfo.MultiKeyPendingDisableReason, 0)
	require.True(t, updated.EffectiveAvailable)
}

func TestManageMultiKeys_EnableAllKeysClearsTemporaryUnavailableState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	seedManualEnableMultiKeyChannel(t, 104, "recover-all-keys")

	body, err := common.Marshal(map[string]any{
		"channel_id": 104,
		"action":     "enable_all_keys",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key/manage", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ManageMultiKeys(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	channel := loadChannelForAssertion(t, 104)
	require.Equal(t, common.ChannelStatusEnabled, channel.Status)
	require.Empty(t, channel.ChannelInfo.MultiKeyStatusList)
	require.Empty(t, channel.ChannelInfo.MultiKeyPendingDisableUntil)
	require.Empty(t, channel.ChannelInfo.MultiKeyPendingDisableReason)
	require.Zero(t, channel.PendingDisableUntil)
	require.Empty(t, channel.PendingDisableReason)
	require.True(t, channel.EffectiveAvailable)
}
