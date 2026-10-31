package service

import (
	"errors"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"testing"
)

type ttsReservation struct {
	quota int
	err   error
}

func (b *ttsReservation) Settle(int) error         { return nil }
func (b *ttsReservation) Refund(*gin.Context)      {}
func (b *ttsReservation) NeedsRefund() bool        { return true }
func (b *ttsReservation) GetPreConsumedQuota() int { return b.quota }
func (b *ttsReservation) Reserve(n int) error      { b.quota = n; return b.err }

func TestAliTTSReservationUsesCumulativeUsageAndPropagatesExhaustion(t *testing.T) {
	b := &ttsReservation{}
	info := &relaycommon.RelayInfo{Billing: b, OriginModelName: "tts-fixture"}
	info.PriceData.ModelRatio = 2
	info.PriceData.GroupRatioInfo.GroupRatio = 1
	info.PriceData.QuotaToPreConsume = 10
	u := &dto.Usage{PromptTokens: 7, TotalTokens: 7}
	u.PromptTokensDetails.TextTokens = 7
	require.NoError(t, ReserveAliTTSUsage(info, u))
	require.Equal(t, 24, b.quota)
	b.err = errors.New("quota exhausted")
	require.ErrorContains(t, ReserveAliTTSUsage(info, u), "exhausted")
	info.PriceData.UsePrice = true
	require.ErrorContains(t, ReserveAliTTSUsage(info, u), "per-unit")
	session := &BillingSession{relayInfo: &relaycommon.RelayInfo{ForcePreConsume: true}}
	require.False(t, session.shouldTrust(nil))
}
