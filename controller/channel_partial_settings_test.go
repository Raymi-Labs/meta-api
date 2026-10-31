package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func applySettingsPatch(t *testing.T, body map[string]any) *model.Channel {
	t.Helper()
	raw, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(raw))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	return loadChannelForAssertion(t, body["id"].(int))
}

func TestChannelPartialUpdatesPreserveSettings(t *testing.T) {
	for _, activeQuota := range []bool{false, true} {
		t.Run(map[bool]string{false: "inactive-quota", true: "active-quota"}[activeQuota], func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			setupChannelSearchControllerTestDB(t)
			channel := seedManualEnableMultiKeyChannel(t, 205, "preserve-settings")
			settings := dto.ChannelSettings{NonStreamResponseHeaderTimeoutSec: 600, Proxy: "socks5://proxy.invalid:1080", MaxConcurrency: 7, RpmLimit: 23, PassThroughBodyEnabled: true, CPAUserIdentityEnabled: true, CPAInstanceID: "fixture"}
			if activeQuota {
				settings.QuotaPolicy = dto.QuotaPolicy{Enabled: true, Period: "week", CountLimit: 100, AnchorTime: 1800000000}
			}
			channel.SetSetting(settings)
			original := *channel.Setting
			// Unknown fields must survive a patch that does not touch the JSON column.
			original = original[:len(original)-1] + `,"future_extension":{"keep":true}}`
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("setting", original).Error)
			for _, patch := range []map[string]any{
				{"id": 205, "status": common.ChannelStatusEnabled},
				{"id": 205, "status": common.ChannelStatusManuallyDisabled},
				{"id": 205, "priority": 9},
				{"id": 205, "weight": 0},
				{"id": 205, "status": common.ChannelStatusEnabled, "setting": nil},
			} {
				got := applySettingsPatch(t, patch)
				require.NotNil(t, got.Setting)
				require.Equal(t, original, *got.Setting)
				if status, ok := patch["status"].(int); ok {
					require.Equal(t, status, got.Status)
				}
				if priority, ok := patch["priority"].(int); ok {
					require.Equal(t, int64(priority), *got.Priority)
				}
				if weight, ok := patch["weight"].(int); ok {
					require.Equal(t, uint(weight), *got.Weight)
				}
			}
		})
	}
}

func TestChannelExplicitSettingsStillNormalizeQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupChannelSearchControllerTestDB(t)
	channel := seedManualEnableMultiKeyChannel(t, 206, "explicit-settings")
	original := dto.ChannelSettings{NonStreamResponseHeaderTimeoutSec: 600, Proxy: "socks5://proxy.invalid:1080", QuotaPolicy: dto.QuotaPolicy{Enabled: true, Period: "week", CountLimit: 100, AnchorTime: 1800000000}}
	channel.SetSetting(original)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 206).Update("setting", *channel.Setting).Error)
	updated := original
	updated.NonStreamResponseHeaderTimeoutSec = 180
	updated.Proxy = ""
	updated.QuotaPolicy.AnchorTime = 0
	raw, err := common.Marshal(updated)
	require.NoError(t, err)
	got := applySettingsPatch(t, map[string]any{"id": 206, "setting": string(raw)})
	require.Equal(t, 180, got.GetSetting().NonStreamResponseHeaderTimeoutSec)
	require.Empty(t, got.GetSetting().Proxy)
	require.Equal(t, int64(1800000000), got.GetSetting().QuotaPolicy.AnchorTime)
	updated.QuotaPolicy.CountLimit = 200
	raw, err = common.Marshal(updated)
	require.NoError(t, err)
	before := common.GetTimestamp()
	got = applySettingsPatch(t, map[string]any{"id": 206, "setting": string(raw)})
	require.GreaterOrEqual(t, got.GetSetting().QuotaPolicy.AnchorTime, before)
	require.LessOrEqual(t, got.GetSetting().QuotaPolicy.AnchorTime, common.GetTimestamp())
	for _, reset := range []string{"{}", ""} {
		got = applySettingsPatch(t, map[string]any{"id": 206, "setting": reset})
		require.Zero(t, got.GetSetting().NonStreamResponseHeaderTimeoutSec)
		require.False(t, got.GetSetting().QuotaPolicy.Enabled)
		require.Zero(t, got.GetSetting().QuotaPolicy.AnchorTime)
	}
}
