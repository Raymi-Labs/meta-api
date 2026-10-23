package service_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type claudeUsageSettlement struct{ quota, calls int }

func (s *claudeUsageSettlement) Reserve(int) error        { return nil }
func (s *claudeUsageSettlement) GetPreConsumedQuota() int { return 500 }
func (s *claudeUsageSettlement) NeedsRefund() bool        { return false }
func (s *claudeUsageSettlement) Refund(*gin.Context)      {}
func (s *claudeUsageSettlement) Settle(quota int) error {
	s.quota, s.calls = quota, s.calls+1
	return nil
}

func TestMessagesCrossProtocolUsageSettlementAndConsumeLog(t *testing.T) {
	require.NoError(t, model.DB.Create(&model.User{Id: 9102, Username: "cross_protocol_cache_fixture", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	t.Cleanup(func() {
		model.DB.Where("user_id = ?", 9102).Delete(&model.Log{})
		model.DB.Unscoped().Delete(&model.User{}, 9102)
	})
	for _, tiered := range []bool{false, true} {
		for _, tc := range []struct {
			name     string
			format   types.RelayFormat
			channel  int
			semantic string
			prompt   int
			isClaude bool
		}{
			{"chat", types.RelayFormatOpenAI, constant.ChannelTypeOpenAI, "", 1000, false},
			{"responses", types.RelayFormatOpenAIResponses, constant.ChannelTypeOpenAI, "", 1000, false},
			{"openrouter chat", types.RelayFormatOpenAI, constant.ChannelTypeOpenRouter, "", 1000, false},
			{"openrouter messages", types.RelayFormatClaude, constant.ChannelTypeOpenRouter, "", 1000, false},
			{"native claude", types.RelayFormatClaude, constant.ChannelTypeAnthropic, "", 300, true},
			{"explicit anthropic usage", types.RelayFormatOpenAI, constant.ChannelTypeOpenAI, "anthropic", 300, true},
		} {
			t.Run(fmt.Sprintf("%s/tiered=%t", tc.name, tiered), func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				requestID := "cross-protocol-cache/" + t.Name()
				ctx.Set(common.RequestIdKey, requestID)
				settlement := &claudeUsageSettlement{}
				info := &relaycommon.RelayInfo{UserId: 9102, UserQuota: 1000000, OriginModelName: "cross-protocol-fixture", StartTime: time.Now(),
					RelayFormat: types.RelayFormatClaude, FinalRequestRelayFormat: tc.format,
					RequestConversionChain: []types.RelayFormat{types.RelayFormatClaude, tc.format},
					ChannelMeta:            &relaycommon.ChannelMeta{ChannelType: tc.channel}, Billing: settlement,
					PriceData: types.PriceData{ModelRatio: 1, CompletionRatio: 5, CacheRatio: 0.1, CacheCreationRatio: 1.25,
						CacheCreation5mRatio: 1.25, CacheCreation1hRatio: 2, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}}
				if tiered {
					const expr = `tier("default", p*2+c*10+cr*0.2+cc*2.5+cc1h*4)`
					info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expr,
						ExprHash: billingexpr.ExprHashString(expr), GroupRatio: 1, QuotaPerUnit: 500000}
				}
				usage := &dto.Usage{PromptTokens: tc.prompt, CompletionTokens: 10, UsageSemantic: tc.semantic,
					PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 600, CachedCreationTokens: 100}}
				service.PostClaudeConsumeQuota(ctx, info, usage)
				require.Equal(t, 1, settlement.calls)
				require.Equal(t, 535, settlement.quota, "300 uncached + 60 cache read + 125 creation + 50 output")
				require.Equal(t, tc.prompt, usage.PromptTokens, "settlement must not mutate upstream usage")
				require.Equal(t, &relaycommon.GroupHealthCacheUsage{ReadTokens: 600, InputTokens: 1000}, info.GroupHealthCacheUsage)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Equal(t, 535, logs[0].Quota)
				require.Equal(t, tc.prompt, logs[0].PromptTokens)
				require.Equal(t, tc.isClaude, gjson.Get(logs[0].Other, "claude").Bool())
				wantSemantic, wantPromptSemantic := "openai", "inclusive"
				if tc.isClaude {
					wantSemantic, wantPromptSemantic = "anthropic", "non_cache"
				}
				require.Equal(t, wantSemantic, gjson.Get(logs[0].Other, "usage_semantic").String())
				require.Equal(t, wantPromptSemantic, gjson.Get(logs[0].Other, "prompt_tokens_semantic").String())
				if tiered {
					require.EqualValues(t, 300, gjson.Get(logs[0].Other, "tiered_params.p").Int())
					require.EqualValues(t, 1000, gjson.Get(logs[0].Other, "tiered_params.len").Int())
				}
				stat, err := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", "", "", 0, "", requestID, false)
				require.NoError(t, err)
				require.InDelta(t, 0.6, stat.CacheGlobalRate, 1e-12)
				require.Equal(t, 535, stat.Quota)
			})
		}
	}
}

func TestClaudeUsageSettlementAndConsumeLog(t *testing.T) {
	require.NoError(t, model.DB.Create(&model.User{Id: 9101, Username: "claude_usage_fixture", Quota: 1000000, Status: common.UserStatusEnabled}).Error)
	t.Cleanup(func() {
		model.DB.Where("user_id = ?", 9101).Delete(&model.Log{})
		model.DB.Unscoped().Delete(&model.User{}, 9101)
	})
	for _, tc := range []struct {
		name   string
		input  int
		output int
		cache  bool
		quota  int
	}{
		{"complete final usage", 100, 20, true, 239},
		{"explicit zero with cached input", 0, 0, true, 39},
		{"successful zero usage", 0, 0, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			ctx.Set(common.RequestIdKey, tc.name)
			settlement := &claudeUsageSettlement{}
			info := &relaycommon.RelayInfo{UserId: 9101, UserQuota: 1000000, OriginModelName: "claude-fixture", StartTime: time.Now(),
				RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{}, Billing: settlement,
				PriceData: types.PriceData{ModelRatio: 1, CompletionRatio: 5, CacheRatio: 0.1, CacheCreationRatio: 1.25,
					CacheCreation5mRatio: 1.25, CacheCreation1hRatio: 2, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}}
			usageFields := map[string]any{"input_tokens": tc.input, "output_tokens": tc.output}
			if tc.cache {
				usageFields["cache_read_input_tokens"] = 30
				usageFields["cache_creation_input_tokens"] = 20
				usageFields["cache_creation"] = map[string]int{"ephemeral_5m_input_tokens": 5, "ephemeral_1h_input_tokens": 15}
			}
			terminal, err := common.Marshal(map[string]any{"type": "message_stop", "usage": usageFields})
			require.NoError(t, err)
			info.IsStream, info.DisablePing = true, true
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader("data: " + string(terminal) + "\n\n"))}
			u, apiErr := claude.ClaudeStreamHandler(ctx, resp, info)
			require.Nil(t, apiErr)
			require.True(t, info.StreamStatus.IsSuccessful())
			service.PostClaudeConsumeQuota(ctx, info, u)
			require.Equal(t, 1, settlement.calls)
			require.Equal(t, tc.quota, settlement.quota)
			var logs []*model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ?", tc.name).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.Equal(t, model.LogTypeConsume, logs[0].Type)
			require.Equal(t, tc.quota, logs[0].Quota)
			require.Equal(t, tc.input, logs[0].PromptTokens)
			require.Equal(t, tc.output, logs[0].CompletionTokens)
			require.NotContains(t, logs[0].Content, "上游出错")
			if tc.cache {
				require.EqualValues(t, 30, gjson.Get(logs[0].Other, "cache_tokens").Int())
				require.EqualValues(t, 20, gjson.Get(logs[0].Other, "cache_creation_tokens").Int())
			}
		})
	}
}
