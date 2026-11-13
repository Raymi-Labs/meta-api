package service

import (
	"errors"
	"fmt"
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// BillingSourceEnterprise 企业成员预算。与个人钱包、订阅、令牌套餐互斥：
// 企业请求只动企业账本，个人钱包与令牌表的剩余额度都不参与。
const BillingSourceEnterprise = model.EnterpriseQuotaSource

// EnterpriseFunding 是企业成员预算这一资金来源。每个请求都有按 request_id 的持久预留，
// 调整用绝对目标值、终态幂等，所以重复的结算或退款不会二次生效，进程重启后也能对账。
type EnterpriseFunding struct {
	subject   model.EnterpriseSubject
	requestID string
	consumed  int // 当前在途占用，与持久预留一致
}

func (f *EnterpriseFunding) Source() string { return BillingSourceEnterprise }

func (f *EnterpriseFunding) resize(target int, state string, requireAvailable bool) error {
	r, err := model.AdjustEnterpriseReservation(f.subject, f.requestID, max(target, 0), state, requireAvailable)
	if err != nil {
		return err
	}
	f.consumed = int(r.Quota)
	return nil
}

func (f *EnterpriseFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	return f.resize(f.consumed+amount, model.EnterpriseReservationReserved, true)
}

// Settle 按真实费用终结。超出占用的部分直接计入成员预算，允许其为负。
func (f *EnterpriseFunding) Settle(delta int) error {
	return f.resize(f.consumed+delta, model.EnterpriseReservationSettled, false)
}

func (f *EnterpriseFunding) Refund() error {
	return refundWithRetry(func() error {
		return f.resize(0, model.EnterpriseReservationRefunded, false)
	})
}

func enterpriseDenied(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeAccessDenied, http.StatusForbidden,
		types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
}

// enterpriseQuotaError 给账本层的拒绝补上面向调用方的文案。账本只给出数字与原因，
// 这里决定怎么说，保证调用方能分清"额度用完""被其他请求占用""这次请求太大""令牌到上限"。
func enterpriseQuotaError(err error) *types.NewAPIError {
	var rejection *types.QuotaRejection
	if errors.As(err, &rejection) && rejection != nil {
		available, required := logger.FormatQuota(int(rejection.Available)), logger.FormatQuota(int(rejection.Required))
		switch {
		case errors.Is(err, model.ErrEnterpriseTokenCap):
			rejection.Message = fmt.Sprintf("企业令牌已达到用量上限，请调高该令牌的上限或改用其他令牌（剩余 %s，需要 %s）", available, required)
		case rejection.RejectionReason() == types.QuotaReasonInFlight:
			rejection.Message = fmt.Sprintf("企业额度不足以预留本次请求，其他请求仍占用额度；请等待其结束后重试（可用 %s，需要 %s）", available, required)
		case rejection.RejectionReason() == types.QuotaReasonRequestTooLarge:
			rejection.Message = fmt.Sprintf("本次请求预估费用超过企业可用额度，额度尚未用尽；请缩短上下文、降低输出上限或申请额度（可用 %s，需要 %s）", available, required)
		default:
			rejection.Message = fmt.Sprintf("企业额度不足, 剩余额度: %s", available)
		}
		return types.NewErrorWithStatusCode(rejection, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
			types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if model.IsEnterpriseUserError(err) {
		return enterpriseDenied(err)
	}
	return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
}

// enterpriseSettlementWired 只有结算链路已经接到企业账本的调用类型才允许建立企业预留。
// 异步任务、实时语音、网页体验区的后结算仍然只认个人钱包，放行会造成"预留在企业、扣费在个人"。
func enterpriseSettlementWired(info *relaycommon.RelayInfo) bool {
	return info.TaskRelayInfo == nil && !info.IsPlayground && info.RelayFormat != types.RelayFormatOpenAIRealtime
}

// newEnterpriseSession 为企业请求建立预留。任何不满足条件的情况都直接拒绝，绝不回落到个人钱包或订阅。
func newEnterpriseSession(c *gin.Context, info *relaycommon.RelayInfo, quota int) (*BillingSession, *types.NewAPIError) {
	if info.RequestId == "" || info.EnterpriseMemberId <= 0 || info.UserId <= 0 || !enterpriseSettlementWired(info) {
		return nil, enterpriseDenied(model.ErrEnterpriseUnsupported)
	}
	quota = max(quota, 0)
	sub := model.EnterpriseSubject{EnterpriseId: info.EnterpriseId, MemberId: info.EnterpriseMemberId, UserId: info.UserId, TokenId: info.TokenId,
		ModelName: info.OriginModelName}
	if c != nil {
		sub.TokenName = c.GetString("token_name")
	}

	// 同步聊天沿用个人钱包的有界首笔预留；其他类型按各自的完整预估预留。金额为 0 也建预留，
	// 这样免费请求同样受"额度为负后拒绝"约束，并且每个企业请求都有可对账的记录。
	bounded := usesChatWalletReservation(c, info) && info.PriceData.InitialChatQuota <= min(quota, common.GetTrustQuota())
	var receipt model.EnterpriseReservation
	var err error
	if bounded {
		receipt, err = model.ReserveEnterpriseChatQuota(sub, info.RequestId, quota, info.PriceData.InitialChatQuota, common.GetTrustQuota())
	} else {
		receipt, err = model.AdjustEnterpriseReservation(sub, info.RequestId, quota, model.EnterpriseReservationReserved, true)
	}
	if err != nil {
		return nil, enterpriseQuotaError(err)
	}

	info.UserQuota = int(min(receipt.AvailableBefore, math.MaxInt32))
	session := &BillingSession{
		relayInfo:        info,
		funding:          &EnterpriseFunding{subject: sub, requestID: info.RequestId, consumed: int(receipt.Quota)},
		preConsumedQuota: int(receipt.Quota),
		chatReservation:  bounded,
	}
	// 企业令牌是无限额令牌，令牌表只在结算时记一次真实用量，这里不预扣。
	session.syncRelayInfo()
	logger.LogInfo(c, fmt.Sprintf("企业 %d 成员 %d 预留 %s，原始预估 %s", info.EnterpriseId, info.EnterpriseMemberId,
		logger.FormatQuota(session.preConsumedQuota), logger.FormatQuota(quota)))
	return session, nil
}
