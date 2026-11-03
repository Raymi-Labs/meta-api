package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBoundedChatWalletLifecycle(t *testing.T) {
	for _, actual := range []int{-1, 500000, 5000000, 15000000} {
		t.Run(common.GetUUID(), func(t *testing.T) {
			truncate(t)
			seedUser(t, 987101, 260937323)
			seedToken(t, 987102, 987101, "bounded-wallet", 0)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 987102).Update("unlimited_quota", true).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			id := common.GetUUID()
			c.Set(common.RequestIdKey, id)
			info := &relaycommon.RelayInfo{UserId: 987101, TokenId: 987102, TokenKey: "bounded-wallet", TokenUnlimited: true, RequestId: id, RelayFormat: types.RelayFormatClaude, UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}, PriceData: types.PriceData{QuotaToPreConsume: 268371153, InitialChatQuota: 5000000, RequestedMaxTokens: 128000}}
			require.Nil(t, PreConsumeBilling(c, 268371153, info))
			require.Equal(t, 5000000, info.FinalPreConsumedQuota)
			var u model.User
			require.NoError(t, model.DB.First(&u, 987101).Error)
			require.Equal(t, 255937323, u.Quota)
			var token model.Token
			require.NoError(t, model.DB.First(&token, 987102).Error)
			require.Zero(t, token.UsedQuota)
			if actual < 0 {
				info.Billing.Refund(c)
				info.Billing.Refund(c)
				require.Eventually(t, func() bool { model.DB.First(&u, 987101); return u.Quota == 260937323 }, time.Second, 5*time.Millisecond)
			} else {
				require.NoError(t, info.Billing.Settle(actual))
				require.NoError(t, info.Billing.Settle(actual))
				info.Billing.Refund(c)
				require.NoError(t, model.DB.First(&u, 987101).Error)
				require.Equal(t, 260937323-actual, u.Quota)
			}
			require.NoError(t, model.DB.First(&token, 987102).Error)
			require.Zero(t, token.RemainQuota)
			require.Equal(t, max(actual, 0), token.UsedQuota)
			admin := map[string]interface{}{}
			AppendPreconsumeAdminInfo(c, admin)
			diagnostic := admin["preconsume"].(map[string]interface{})
			require.Equal(t, "bounded_chat_wallet", diagnostic["policy"])
			require.Equal(t, 128000, diagnostic["requested_max_tokens"])
		})
	}
}

func TestChatWalletReservationScope(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	base := relaycommon.RelayInfo{TokenUnlimited: true, RequestId: "scope", RelayFormat: types.RelayFormatClaude, PriceData: types.PriceData{InitialChatQuota: 1}}
	require.True(t, usesChatWalletReservation(c, &base))
	for _, change := range []func(*relaycommon.RelayInfo){
		func(i *relaycommon.RelayInfo) {
			i.Request = &dto.GeneralOpenAIRequest{Audio: []byte(`{"voice":"alloy"}`)}
		},
		func(i *relaycommon.RelayInfo) {
			i.Request = &dto.GeneralOpenAIRequest{Modalities: []byte(`["text","audio"]`)}
		},
		func(i *relaycommon.RelayInfo) {
			i.Request = &dto.GeminiChatRequest{GenerationConfig: dto.GeminiChatGenerationConfig{ResponseModalities: []string{"IMAGE"}}}
		},
		func(i *relaycommon.RelayInfo) {
			i.Request = &dto.OpenAIResponsesRequest{Tools: []byte(`[{"type":"image_generation"}]`)}
		},
		func(i *relaycommon.RelayInfo) {
			i.Request = &dto.GeneralOpenAIRequest{Messages: []dto.Message{{Content: []any{map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "synthetic", "format": "wav"}}}}}}
		},
		func(i *relaycommon.RelayInfo) {
			i.Request = &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{{FileData: &dto.GeminiFileData{MimeType: "video/mp4", FileUri: "https://example.invalid/video"}}}}}}
		},
		func(i *relaycommon.RelayInfo) { i.TokenUnlimited = false }, func(i *relaycommon.RelayInfo) { i.ForcePreConsume = true }, func(i *relaycommon.RelayInfo) { i.RequestId = "" }, func(i *relaycommon.RelayInfo) { i.PriceData.UsePrice = true }, func(i *relaycommon.RelayInfo) { i.PriceData.UseAudioDurationPrice = true }, func(i *relaycommon.RelayInfo) { i.RelayFormat = types.RelayFormatOpenAIRealtime }, func(i *relaycommon.RelayInfo) {
			i.RelayFormat = types.RelayFormatOpenAI
			i.RelayMode = relayconstant.RelayModeEmbeddings
		},
	} {
		next := base
		change(&next)
		require.False(t, usesChatWalletReservation(c, &next))
	}
	common.SetContextKey(c, constant.ContextKeyTokenPackageEnabled, true)
	require.False(t, usesChatWalletReservation(c, &base))
}

func TestFundingFallbackKeepsPositiveBalanceReason(t *testing.T) {
	for _, pref := range []string{"wallet_first", "subscription_first"} {
		t.Run(pref, func(t *testing.T) {
			truncate(t)
			seedUser(t, 987201, 5)
			seedToken(t, 987202, 987201, "fallback-positive", 100)
			require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
			require.NoError(t, model.DB.Where("id = ?", 987203).Delete(&model.SubscriptionPlan{}).Error)
			require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 987203, Title: "fixture", QuotaResetPeriod: "never"}).Error)
			seedSubscription(t, 987204, 987201, 100, 100)
			require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", 987204).Update("plan_id", 987203).Error)
			if pref == "subscription_first" {
				require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 987201).Update("quota", 0).Error)
				require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", 987204).Update("amount_used", 95).Error)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			info := &relaycommon.RelayInfo{UserId: 987201, TokenId: 987202, TokenKey: "fallback-positive", ForcePreConsume: true, RequestId: common.GetUUID(), UserSetting: dto.UserSetting{BillingPreference: pref}}
			err := PreConsumeBilling(c, 20, info)
			require.NotNil(t, err)
			var rejected *types.QuotaRejection
			require.ErrorAs(t, err, &rejected)
			require.Equal(t, types.QuotaReasonRequestTooLarge, rejected.RejectionReason())
			require.EqualValues(t, 5, rejected.Available)
			if pref == "wallet_first" {
				require.Equal(t, "wallet", rejected.Source)
			} else {
				require.Equal(t, "subscription", rejected.Source)
			}
			var token model.Token
			require.NoError(t, model.DB.First(&token, 987202).Error)
			require.Equal(t, 100, token.RemainQuota)
		})
	}
}
