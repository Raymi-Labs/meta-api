package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSystemTranslationThroughTokenAuthRoutingAndBilling(t *testing.T) {
	descriptionTestDB(t)
	db := model.DB
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.UserSubscription{}, &model.Log{}, &model.ContentSafetyViolation{}, &model.PulseWalletReservation{}, &model.PulseFundingLedger{}, &model.ModelPermission{}))
	oldLogDB, oldRedis, oldMemory, oldBatch, oldConsume, oldSQLite, oldCount := model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite, constant.CountToken
	model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite, constant.CountToken = db, false, false, false, true, true, false
	t.Cleanup(func() {
		model.LOG_DB, common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.UsingSQLite, constant.CountToken = oldLogDB, oldRedis, oldMemory, oldBatch, oldConsume, oldSQLite, oldCount
	})
	require.NoError(t, i18n.Init())
	service.InitTokenEncoders()
	service.InitHttpClient()
	oldRatio := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"qwen-mt-flash":1}`))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 100).Updates(map[string]any{"quota": 1000000, "group": "default"}).Error)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", 100).Updates(map[string]any{"unlimited_quota": false, "remain_quota": 1000000, "group": "default"}).Error)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/compatible-mode/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer fixture-aliyun-key", r.Header.Get("Authorization"))
		var request dto.GeneralOpenAIRequest
		require.NoError(t, common.DecodeJson(r.Body, &request))
		require.JSONEq(t, `{"source_lang":"auto","target_lang":"Chinese"}`, string(request.TranslationOptions))
		require.Equal(t, model.SystemTranslationModel, request.Model)
		require.Len(t, request.Messages, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-system-translation","object":"chat.completion","model":"qwen-mt-flash","choices":[{"index":0,"message":{"role":"assistant","content":"模型介绍"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	t.Cleanup(upstream.Close)
	base := upstream.URL
	require.NoError(t, db.Create(&model.Channel{Id: 3000, Name: "translation", Group: "default", Models: model.SystemTranslationModel, Type: constant.ChannelTypeAli, Status: common.ChannelStatusEnabled, Key: "fixture-aliyun-key", BaseURL: &base, AutoBan: common.GetPointer(0)}).Error)
	require.NoError(t, db.Create(&model.Ability{ChannelId: 3000, Group: "default", Model: model.SystemTranslationModel, Enabled: true}).Error)
	r := gin.New()
	r.POST("/v1/chat/completions", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) {
		c.Set(common.RequestIdKey, "system-translation-billing-test")
		Relay(c, types.RelayFormatOpenAI)
	})
	local := httptest.NewServer(r)
	t.Cleanup(local.Close)
	t.Setenv("PORT", strings.TrimPrefix(local.URL, "http://127.0.0.1:"))
	text, err := service.TranslateSystemText(context.Background(), "Model description", "zh-CN")
	require.NoError(t, err)
	require.Equal(t, "模型介绍", text)
	require.Equal(t, 1, calls)
	var log model.Log
	require.Eventually(t, func() bool {
		return db.Where("request_id = ?", "system-translation-billing-test").First(&log).Error == nil
	}, 3*time.Second, 20*time.Millisecond)
	require.Equal(t, model.LogTypeConsume, log.Type)
	require.Equal(t, 100, log.TokenId)
	require.Equal(t, 3000, log.ChannelId)
	require.Equal(t, model.SystemTranslationModel, log.ModelName)
	require.Positive(t, log.Quota)
	require.Eventually(t, func() bool {
		var user model.User
		var token model.Token
		return db.First(&user, 100).Error == nil && db.First(&token, 100).Error == nil && user.Quota == 1000000-log.Quota && token.RemainQuota == 1000000-log.Quota
	}, 3*time.Second, 20*time.Millisecond)
}
