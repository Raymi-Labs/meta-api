package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const enterpriseTestPersonalQuota = 123456

// enterpriseBilling 是一家已入账的企业和它的一名成员，成员持有一个企业令牌。
type enterpriseBilling struct {
	t      *testing.T
	ent    *model.Enterprise
	owner  *model.User
	user   *model.User
	member *model.EnterpriseMember
	token  *model.Token
	mails  []enterpriseTestMail
}

type enterpriseTestMail struct{ receiver, subject, content string }

func enterpriseTestUser(t *testing.T) *model.User {
	t.Helper()
	id := common.GetUUID()
	user := &model.User{Username: "ent-" + id[:12], AffCode: id[:12], Email: "ent-" + id[:12] + "@example.com",
		Quota: enterpriseTestPersonalQuota, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

// newEnterpriseBilling 给成员分配 allocated 额度；tokenLimit 为 0 表示令牌不另设上限。
func newEnterpriseBilling(t *testing.T, allocated, tokenLimit int64) *enterpriseBilling {
	t.Helper()
	require.NoError(t, model.MigrateEnterpriseTables(model.DB))
	oldEnabled, oldLog := common.EnterpriseEnabled, constant.ErrorLogEnabled
	common.EnterpriseEnabled, constant.ErrorLogEnabled = true, false
	f := &enterpriseBilling{t: t}
	// 提醒邮件同步执行并截获：既不真的发信，也不会有后台协程在测试结束后继续读库。
	oldSend, oldAsync := enterpriseSendEmail, enterpriseAsync
	enterpriseAsync = func(run func()) { run() }
	enterpriseSendEmail = func(subject, receiver, content string) error {
		f.mails = append(f.mails, enterpriseTestMail{receiver, subject, content})
		return nil
	}
	t.Cleanup(func() {
		enterpriseSendEmail, enterpriseAsync = oldSend, oldAsync
		common.EnterpriseEnabled, constant.ErrorLogEnabled = oldEnabled, oldLog
		for _, table := range []string{"enterprises", "enterprise_members", "enterprise_invitations", "enterprise_audit_events",
			"enterprise_ledgers", "enterprise_reservations", "enterprise_token_budgets", "enterprise_quota_requests",
			"users", "tokens", "logs", "channels"} {
			require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
		}
	})

	f.owner, f.user = enterpriseTestUser(t), enterpriseTestUser(t)
	var err error
	f.ent, err = model.CreateEnterprise(1, "测试企业"+common.GetUUID()[:6], "default", "", f.owner.Id)
	require.NoError(t, err)
	_, err = model.CreditEnterprise(1, f.ent.Id, 100_000_000, "BANK-"+common.GetUUID()[:8], "")
	require.NoError(t, err)
	invite, _, err := model.CreateEnterpriseInvitation(model.EnterpriseActor{UserId: f.owner.Id}, f.ent.Id, f.user.Email, model.EnterpriseRoleMember, allocated)
	require.NoError(t, err)
	joined, err := model.AcceptEnterpriseInvitation(invite, f.user.Id)
	require.NoError(t, err)
	require.False(t, joined.QuotaShortfall)
	f.member = joined.Member

	f.token = &model.Token{UserId: f.user.Id, Key: common.GetUUID(), Name: "企业令牌", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, Group: "default", EnterpriseId: f.ent.Id, MembershipId: f.member.Id}
	require.NoError(t, model.CreateEnterpriseToken(f.token, tokenLimit))
	seedChannel(t, 1)
	return f
}

// request 构造一次企业请求。initial > 0 表示同步聊天（走有界首笔预留），否则按向量请求的完整预估预留。
func (f *enterpriseBilling) request(estimate, initial int) (*gin.Context, *relaycommon.RelayInfo) {
	f.t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	id := common.GetUUID()
	c.Set(common.RequestIdKey, id)
	info := &relaycommon.RelayInfo{UserId: f.user.Id, TokenId: f.token.Id, TokenKey: f.token.Key, TokenUnlimited: true,
		RequestId: id, EnterpriseId: f.ent.Id, EnterpriseMemberId: f.member.Id, OriginModelName: "fixture-model",
		RelayFormat: types.RelayFormatEmbedding, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1},
		PriceData: types.PriceData{QuotaToPreConsume: estimate, InitialChatQuota: initial}}
	info.PriceData.GroupRatioInfo.GroupRatio = 1
	if initial > 0 {
		info.RelayFormat = types.RelayFormatClaude
	}
	return c, info
}

// accept 发起一次必须被受理的请求。
func (f *enterpriseBilling) accept(estimate, initial int) (*gin.Context, *relaycommon.RelayInfo) {
	f.t.Helper()
	c, info := f.request(estimate, initial)
	apiErr := PreConsumeBilling(c, estimate, info)
	require.Nil(f.t, apiErr, "%v", apiErr)
	require.Equal(f.t, BillingSourceEnterprise, info.BillingSource)
	return c, info
}

// reject 发起一次必须被拒绝的请求，并确认拒绝不留任何预留记录。
func (f *enterpriseBilling) reject(estimate, initial int) (*gin.Context, *types.NewAPIError) {
	f.t.Helper()
	c, info := f.request(estimate, initial)
	before := f.balance()
	apiErr := PreConsumeBilling(c, estimate, info)
	require.NotNil(f.t, apiErr)
	require.Nil(f.t, info.Billing)
	require.Equal(f.t, http.StatusForbidden, apiErr.StatusCode)
	require.Nil(f.t, f.reservation(info.RequestId), "被拒绝的请求不应留下预留")
	require.Equal(f.t, before, f.balance())
	return c, apiErr
}

func (f *enterpriseBilling) memberRow() model.EnterpriseMember {
	f.t.Helper()
	var row model.EnterpriseMember
	require.NoError(f.t, model.DB.First(&row, f.member.Id).Error)
	return row
}

func (f *enterpriseBilling) reservation(requestID string) *model.EnterpriseReservation {
	f.t.Helper()
	var rows []model.EnterpriseReservation
	require.NoError(f.t, model.DB.Where("request_id = ?", requestID).Find(&rows).Error)
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

func (f *enterpriseBilling) tokenRow() model.Token {
	f.t.Helper()
	var row model.Token
	require.NoError(f.t, model.DB.First(&row, f.token.Id).Error)
	return row
}

func (f *enterpriseBilling) userRow() model.User {
	f.t.Helper()
	var row model.User
	require.NoError(f.t, model.DB.First(&row, f.user.Id).Error)
	return row
}

// balance 返回企业账本并确认守恒式成立。
func (f *enterpriseBilling) balance() model.EnterpriseBalance {
	f.t.Helper()
	bal, err := model.GetEnterpriseBalance(f.ent.Id)
	require.NoError(f.t, err)
	require.True(f.t, bal.Balanced, "守恒式不成立: %+v", bal)
	return *bal
}

// ledger 断言成员预算三项。
func (f *enterpriseBilling) ledger(available, reserved, consumed int64) {
	f.t.Helper()
	row := f.memberRow()
	require.Equal(f.t, [3]int64{available, reserved, consumed}, [3]int64{row.Available, row.Reserved, row.Consumed})
	f.balance()
}

// personalUntouched 企业请求无论成败都不能动个人钱包、个人已用额度、令牌剩余额度，也不能留下个人钱包预留。
func (f *enterpriseBilling) personalUntouched() {
	f.t.Helper()
	user := f.userRow()
	require.Equal(f.t, enterpriseTestPersonalQuota, user.Quota, "个人钱包零变动")
	require.Zero(f.t, user.UsedQuota, "企业消费不计入个人已用额度")
	token := f.tokenRow()
	require.Zero(f.t, token.RemainQuota)
	require.True(f.t, token.UnlimitedQuota)
	var holds int64
	require.NoError(f.t, model.DB.Model(&model.PulseWalletReservation{}).Where("user_id = ?", f.user.Id).Count(&holds).Error)
	require.Zero(f.t, holds, "企业请求不应建立个人钱包预留")
}

func (f *enterpriseBilling) refunded(c *gin.Context, info *relaycommon.RelayInfo) {
	f.t.Helper()
	require.True(f.t, info.Billing.NeedsRefund())
	info.Billing.Refund(c)
	info.Billing.Refund(c)
	require.Eventually(f.t, func() bool {
		r := f.reservation(info.RequestId)
		return r != nil && r.Status == model.EnterpriseReservationRefunded
	}, time.Second, 5*time.Millisecond)
	require.Zero(f.t, f.reservation(info.RequestId).Quota)
}

func enterpriseDiagnostic(t *testing.T, c *gin.Context) map[string]interface{} {
	t.Helper()
	admin := map[string]interface{}{}
	AppendPreconsumeAdminInfo(c, admin)
	return admin["preconsume"].(map[string]interface{})
}

func enterpriseRejection(t *testing.T, apiErr *types.NewAPIError) *types.QuotaRejection {
	t.Helper()
	require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	var rejection *types.QuotaRejection
	require.True(t, errors.As(apiErr, &rejection))
	return rejection
}

func TestEnterpriseChatReservationLifecycle(t *testing.T) {
	f := newEnterpriseBilling(t, 20_000_000, 0)

	// 可用额度高于信任线：同步聊天只占首笔，不按完整预估占用。
	c, info := f.accept(8_000_000, 1_000_000)
	require.Equal(t, 1_000_000, info.FinalPreConsumedQuota)
	require.Equal(t, 20_000_000, info.UserQuota)
	f.ledger(19_000_000, 1_000_000, 0)
	diagnostic := enterpriseDiagnostic(t, c)
	require.Equal(t, "bounded_chat_wallet", diagnostic["policy"])
	require.Equal(t, BillingSourceEnterprise, diagnostic["source"])
	require.Equal(t, 20_000_000, diagnostic["enterprise_available_before"])
	require.NotContains(t, diagnostic, "wallet_available_before")
	require.Zero(t, f.tokenRow().UsedQuota, "令牌用量只在结算时记")

	// 同一请求重复准入沿用原占用，不会二次预留。
	retry := *info
	retry.Billing = nil
	require.Nil(t, PreConsumeBilling(c, 8_000_000, &retry))
	require.Equal(t, 1_000_000, retry.FinalPreConsumedQuota)
	f.ledger(19_000_000, 1_000_000, 0)

	// 结算按真实用量多退少补；重复结算与之后的退款都不再生效。
	require.NoError(t, SettleBilling(c, info, 3_000_000))
	require.NoError(t, info.Billing.Settle(3_000_000))
	require.False(t, info.Billing.NeedsRefund())
	info.Billing.Refund(c)
	f.ledger(17_000_000, 0, 3_000_000)
	stored := f.reservation(info.RequestId)
	require.Equal(t, model.EnterpriseReservationSettled, stored.Status)
	require.EqualValues(t, 3_000_000, stored.Quota)
	require.Equal(t, f.token.Id, stored.TokenId)
	require.Equal(t, 3_000_000, f.tokenRow().UsedQuota)
	// 终态之后同一请求不能再换一个金额结算。
	_, err := model.AdjustEnterpriseReservation(model.EnterpriseSubject{EnterpriseId: f.ent.Id, MemberId: f.member.Id, UserId: f.user.Id, TokenId: f.token.Id},
		info.RequestId, 4_000_000, model.EnterpriseReservationSettled, false)
	require.ErrorIs(t, err, model.ErrEnterpriseFinalized)
	f.ledger(17_000_000, 0, 3_000_000)

	// 成功结算清掉成员与令牌两个作用域的额度失败信号。
	settled, _ := c.Get("quota_notice_settled")
	scopes := map[string]bool{}
	for _, item := range settled.([]model.QuotaNoticeResource) {
		scopes[item.Scope] = true
	}
	require.True(t, scopes["enterprise:"+strconv.Itoa(f.member.Id)], "%v", scopes)
	require.True(t, scopes["enterprise_token:"+strconv.Itoa(f.token.Id)], "%v", scopes)
	require.False(t, scopes["wallet:0"], "%v", scopes)

	// 日志带付款方与企业归属；个人计费偏好与企业请求无关。
	info.UserSetting.BillingPreference = "wallet_first"
	other := map[string]interface{}{}
	appendBillingInfo(info, other)
	require.Equal(t, map[string]interface{}{"billing_source": BillingSourceEnterprise, "enterprise_id": f.ent.Id, "enterprise_member_id": f.member.Id}, other)

	// 用量统计：个人只计请求次数，渠道照常计用量。
	RecordRelayUsageStats(info, 3_000_000)
	require.Equal(t, 1, f.userRow().RequestCount)
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, 1).Error)
	require.EqualValues(t, 3_000_000, channel.UsedQuota)
	f.personalUntouched()
}

func TestEnterpriseLowBalanceUsesFullEstimate(t *testing.T) {
	f := newEnterpriseBilling(t, 4_000_000, 0)

	// 可用额度不高于信任线：聊天也按完整预估占用。
	c, info := f.accept(3_000_000, 1_000_000)
	require.Equal(t, 3_000_000, info.FinalPreConsumedQuota)
	require.Equal(t, "full_estimate_low_balance", enterpriseDiagnostic(t, c)["policy"])
	f.ledger(1_000_000, 3_000_000, 0)

	// 其他请求占着额度时，新请求被拒并说明原因。
	_, apiErr := f.reject(2_000_000, 1_000_000)
	rejection := enterpriseRejection(t, apiErr)
	require.Equal(t, types.QuotaReasonInFlight, rejection.RejectionReason())
	require.Contains(t, apiErr.Error(), "其他请求仍占用额度")

	// 失败退款释放全部占用，重复退款不重复生效。
	f.refunded(c, info)
	f.ledger(4_000_000, 0, 0)
	require.Error(t, info.Billing.Settle(1), "已退款的请求不能再结算")
	f.ledger(4_000_000, 0, 0)

	// 额度没用完，但这一次请求太大。
	c, apiErr = f.reject(8_000_000, 1_000_000)
	rejection = enterpriseRejection(t, apiErr)
	require.Equal(t, BillingSourceEnterprise, rejection.Source)
	require.Equal(t, f.member.Id, rejection.ResourceID)
	require.EqualValues(t, 4_000_000, rejection.Available)
	require.EqualValues(t, 8_000_000, rejection.Required)
	require.Equal(t, types.QuotaReasonRequestTooLarge, rejection.RejectionReason())
	require.Contains(t, apiErr.Error(), "本次请求预估费用超过企业可用额度")
	diagnostic := enterpriseDiagnostic(t, c)
	require.Equal(t, false, diagnostic["accepted"])
	require.Equal(t, BillingSourceEnterprise, diagnostic["source"])
	require.Equal(t, "full_estimate_low_balance", diagnostic["policy"])
	f.personalUntouched()
	require.Zero(t, f.tokenRow().UsedQuota)
}

func TestEnterpriseOverdraftBlocksNewRequests(t *testing.T) {
	f := newEnterpriseBilling(t, 2_000_000, 0)

	// 非聊天请求按完整预估占用；真实费用超出时成员预算可以为负。
	c, info := f.accept(1_000_000, 0)
	require.Equal(t, 1_000_000, info.FinalPreConsumedQuota)
	require.Equal(t, "full_estimate", enterpriseDiagnostic(t, c)["policy"])
	require.NoError(t, SettleBilling(c, info, 5_000_000))
	f.ledger(-3_000_000, 0, 5_000_000)
	require.Equal(t, 5_000_000, f.tokenRow().UsedQuota)

	// 为负之后拒绝一切新请求，包括预估为 0 的免费请求。
	for _, estimate := range []int{1_000_000, 0} {
		_, apiErr := f.reject(estimate, 0)
		rejection := enterpriseRejection(t, apiErr)
		require.Equal(t, types.QuotaReasonExhausted, rejection.RejectionReason())
		require.EqualValues(t, -3_000_000, rejection.Available)
		require.Contains(t, apiErr.Error(), "企业额度不足")
	}

	// 补额先抵欠额：补 4M 后只剩 1M 可用。
	require.NoError(t, model.AllocateEnterpriseQuota(model.EnterpriseActor{UserId: f.owner.Id}, f.ent.Id, f.member.Id, 4_000_000, "补额"))
	f.ledger(1_000_000, 0, 5_000_000)
	c, info = f.accept(600_000, 0)
	require.NoError(t, SettleBilling(c, info, 600_000))
	f.ledger(400_000, 0, 5_600_000)
	f.personalUntouched()
}

func TestEnterpriseFreeRequestStillLeavesRecord(t *testing.T) {
	f := newEnterpriseBilling(t, 1_000_000, 0)

	// 预估为 0 也建预留：成功时按真实用量结算，失败时显式释放，不会挂成待对账。
	c, info := f.accept(0, 0)
	require.NotNil(t, f.reservation(info.RequestId))
	require.NoError(t, SettleBilling(c, info, 0))
	require.Equal(t, model.EnterpriseReservationSettled, f.reservation(info.RequestId).Status)
	_, exists := c.Get("quota_notice_settled")
	require.False(t, exists)

	c, info = f.accept(0, 0)
	f.refunded(c, info)
	f.ledger(1_000_000, 0, 0)
	require.Zero(t, f.tokenRow().UsedQuota)
	f.personalUntouched()
}

func TestEnterpriseTokenCap(t *testing.T) {
	f := newEnterpriseBilling(t, 20_000_000, 1_500_000)

	c, info := f.accept(1_000_000, 0)
	f.ledger(19_000_000, 1_000_000, 0)

	// 成员额度充足，但令牌自己的上限不够。
	_, apiErr := f.reject(1_000_000, 0)
	rejection := enterpriseRejection(t, apiErr)
	require.ErrorIs(t, apiErr, model.ErrEnterpriseTokenCap)
	require.Equal(t, model.EnterpriseTokenQuotaSource, rejection.Source)
	require.Equal(t, f.token.Id, rejection.ResourceID)
	require.EqualValues(t, 500_000, rejection.Available)
	require.Contains(t, apiErr.Error(), "企业令牌已达到用量上限")

	// 结算不受令牌上限阻拦（费用已经发生），但会累计到令牌已用量。
	require.NoError(t, SettleBilling(c, info, 1_800_000))
	budgets, err := model.GetEnterpriseTokenBudgets([]int{f.token.Id})
	require.NoError(t, err)
	require.EqualValues(t, 1_800_000, budgets[f.token.Id].UsedQuota)
	f.ledger(18_200_000, 0, 1_800_000)
	_, apiErr = f.reject(1, 0)
	require.Equal(t, types.QuotaReasonExhausted, enterpriseRejection(t, apiErr).RejectionReason())

	// 调高上限后恢复。
	require.NoError(t, model.SetEnterpriseTokenLimit(model.EnterpriseSubject{EnterpriseId: f.ent.Id, MemberId: f.member.Id, UserId: f.user.Id, TokenId: f.token.Id}, 5_000_000))
	c, info = f.accept(1_000_000, 0)
	f.refunded(c, info)
	budgets, err = model.GetEnterpriseTokenBudgets([]int{f.token.Id})
	require.NoError(t, err)
	require.EqualValues(t, 1_800_000, budgets[f.token.Id].UsedQuota, "退款同时释放令牌占用")
	f.personalUntouched()
}

func TestEnterpriseImageReserveGrowsHold(t *testing.T) {
	f := newEnterpriseBilling(t, 6_000_000, 0)
	c, info := f.request(1_000_000, 0)
	info.RelayFormat, info.ImageRequestCount, info.ForcePreConsume = types.RelayFormatOpenAIImage, 1, true
	require.Nil(t, PreConsumeBilling(c, 1_000_000, info))

	// 出图数量确定后把占用补到目标值；目标不高于当前占用时不动。
	require.NoError(t, info.Billing.Reserve(3_000_000))
	require.NoError(t, info.Billing.Reserve(2_000_000))
	require.Equal(t, 3_000_000, info.FinalPreConsumedQuota)
	f.ledger(3_000_000, 3_000_000, 0)

	// 补不够时拒绝并保持原占用。
	err := info.Billing.Reserve(9_000_000)
	var apiErr *types.NewAPIError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	require.Equal(t, 3_000_000, info.FinalPreConsumedQuota)
	f.ledger(3_000_000, 3_000_000, 0)

	require.NoError(t, SettleBilling(c, info, 2_000_000))
	f.ledger(4_000_000, 0, 2_000_000)
	require.Equal(t, 2_000_000, f.tokenRow().UsedQuota)
	f.personalUntouched()
}

func TestEnterpriseDisabledMemberInFlightRequestStillSettles(t *testing.T) {
	f := newEnterpriseBilling(t, 20_000_000, 0)
	c, info := f.accept(1_000_000, 0)
	unallocated := f.balance().Unallocated

	// 停用：剩余额度并回未分配，在途占用留在成员行等待结算。
	require.NoError(t, model.DisableEnterpriseMember(model.EnterpriseActor{UserId: f.owner.Id}, f.ent.Id, f.member.Id, "离职"))
	f.ledger(0, 1_000_000, 0)
	require.Equal(t, unallocated+19_000_000, f.balance().Unallocated)
	require.Equal(t, common.TokenStatusDisabled, f.tokenRow().Status)

	// 停用后不再受理新请求。
	_, apiErr := f.reject(1, 0)
	require.Equal(t, types.ErrorCodeAccessDenied, apiErr.GetErrorCode())
	require.ErrorIs(t, apiErr, model.ErrEnterpriseMemberInactive)

	// 已受理的请求照常按真实费用结算；多退的部分由回收任务并回未分配。
	require.NoError(t, SettleBilling(c, info, 400_000))
	f.ledger(600_000, 0, 400_000)
	oldNotify := notifyEnterpriseReview
	notifyEnterpriseReview = func(int64) { t.Fatal("没有超时预留，不应告警") }
	t.Cleanup(func() { notifyEnterpriseReview = oldNotify })
	review, swept := runEnterpriseMaintenanceOnce(time.Now())
	require.Zero(t, review)
	require.Equal(t, 1, swept)
	f.ledger(0, 0, 400_000)
	require.Equal(t, unallocated+19_600_000, f.balance().Unallocated)
	f.personalUntouched()
}

func TestEnterpriseViolationFee(t *testing.T) {
	settings := model_setting.GetGrokSettings()
	oldEnabled, oldAmount := settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount
	settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount = true, 0.05
	t.Cleanup(func() { settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount = oldEnabled, oldAmount })
	fee := int64(0.05 * common.QuotaPerUnit)
	f := newEnterpriseBilling(t, 1_000_000, 0)
	violation := NormalizeViolationFeeError(types.NewOpenAIError(errors.New("Failed check: SAFETY_CHECK_TYPE_CSAM"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest))

	// 请求失败先退预留，再从触发成员的企业预算扣违规费；同一请求重复进入只扣一次。
	c, info := f.accept(200_000, 0)
	f.refunded(c, info)
	logs := countLogs(t)
	require.True(t, ChargeViolationFeeIfNeeded(c, info, violation))
	require.True(t, ChargeViolationFeeIfNeeded(c, info, violation))
	f.ledger(1_000_000-fee, 0, fee)
	require.Equal(t, logs+1, countLogs(t))
	require.EqualValues(t, fee, f.tokenRow().UsedQuota)
	require.Equal(t, 1, f.userRow().RequestCount)
	entry := getLastLog(t)
	require.EqualValues(t, fee, entry.Quota)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(entry.Other, &other))
	require.Equal(t, BillingSourceEnterprise, other["billing_source"])
	require.EqualValues(t, f.ent.Id, other["enterprise_id"])
	require.EqualValues(t, f.member.Id, other["enterprise_member_id"])
	require.Equal(t, true, other["violation_fee"])

	// 违规费可以把成员预算扣成负数，之后新请求被拒。（把成员改成只剩 1 可用，其余记为已消费，守恒式不变。）
	require.NoError(t, model.DB.Model(&model.EnterpriseMember{}).Where("id = ?", f.member.Id).
		Updates(map[string]any{"available": 1, "consumed": 999_999}).Error)
	c, info = f.accept(0, 0)
	f.refunded(c, info)
	require.True(t, ChargeViolationFeeIfNeeded(c, info, violation))
	require.EqualValues(t, 1-fee, f.memberRow().Available)
	f.balance()
	f.reject(0, 0)

	// 没有请求号就没有幂等键，宁可不扣也不重复扣。
	_, info = f.request(0, 0)
	info.RequestId = ""
	require.False(t, ChargeViolationFeeIfNeeded(c, info, violation))
	require.EqualValues(t, 1-fee, f.memberRow().Available)
	f.personalUntouched()
}

func TestEnterpriseUnsupportedEntrancesFailClosed(t *testing.T) {
	f := newEnterpriseBilling(t, 1_000_000, 0)
	for name, change := range map[string]func(*relaycommon.RelayInfo){
		"缺请求号": func(i *relaycommon.RelayInfo) { i.RequestId = "" },
		"缺成员":  func(i *relaycommon.RelayInfo) { i.EnterpriseMemberId = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			c, info := f.request(1000, 0)
			id := info.RequestId
			change(info)
			apiErr := PreConsumeBilling(c, 1000, info)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
			require.Equal(t, types.ErrorCodeAccessDenied, apiErr.GetErrorCode())
			require.ErrorIs(t, apiErr, model.ErrEnterpriseUnsupported)
			require.Nil(t, info.Billing)
			require.Nil(t, f.reservation(id))
		})
	}

	// 没有企业会话时，旧的个人钱包结算路径对企业请求直接报错。
	c, info := f.request(1000, 0)
	require.ErrorIs(t, PostConsumeQuota(info, 1000, 0, false), model.ErrEnterpriseUnsupported)
	require.ErrorIs(t, SettleBilling(c, info, 1000), model.ErrEnterpriseUnsupported)
	f.ledger(1_000_000, 0, 0)
	require.Zero(t, f.tokenRow().UsedQuota)
	f.personalUntouched()
}

// 成员额度耗尽后客户端若持续重试，令牌与个人令牌一样连续三次后自动停用；请求过大或被占用不计入。
func TestEnterpriseExhaustionDisablesTokenAfterRepeatedFailures(t *testing.T) {
	f := newEnterpriseBilling(t, 1_000_000, 0)
	observe := func(estimate int) {
		c, apiErr := f.reject(estimate, 0)
		token := f.tokenRow()
		MarkTokenQuotaIdentity(c, &token)
		MarkRequestFailure(c, apiErr)
		ObserveTokenQuotaOutcome(c)
	}
	for range 4 {
		observe(2_000_000)
	}
	require.Zero(t, f.tokenRow().QuotaFailureCount, "请求过大不是额度耗尽")

	c, info := f.accept(1_000_000, 0)
	require.NoError(t, SettleBilling(c, info, 1_000_000))
	f.ledger(0, 0, 1_000_000)
	for i := 1; i <= 3; i++ {
		require.Equal(t, common.TokenStatusEnabled, f.tokenRow().Status)
		observe(1000)
		require.Equal(t, i, f.tokenRow().QuotaFailureCount)
	}
	require.Equal(t, common.TokenStatusDisabled, f.tokenRow().Status)
	f.personalUntouched()
}

func TestEnterpriseMaintenanceMovesStaleReservationsToReview(t *testing.T) {
	f := newEnterpriseBilling(t, 5_000_000, 0)
	var notified []int64
	oldNotify := notifyEnterpriseReview
	notifyEnterpriseReview = func(count int64) { notified = append(notified, count) }
	t.Cleanup(func() { notifyEnterpriseReview = oldNotify })
	// 配置过小时按下限 10 分钟处理，避免把正常的长请求误判为超时。
	t.Setenv("ENTERPRISE_RESERVATION_STALE_SECONDS", "5")
	require.EqualValues(t, 600, enterpriseReservationStaleSeconds())

	c, info := f.accept(1_000_000, 0)
	_, fresh := f.accept(500_000, 0)
	require.NoError(t, model.DB.Model(&model.EnterpriseReservation{}).Where("request_id = ?", info.RequestId).
		Update("updated_at", time.Now().Unix()-601).Error)

	// 功能开关关闭时什么都不做。
	common.EnterpriseEnabled = false
	review, swept := runEnterpriseMaintenanceOnce(time.Now())
	require.Zero(t, review+int64(swept))
	common.EnterpriseEnabled = true

	// 超时未终结的预留转待对账并告警一次；额度继续占用，不自动退款。未超时的不受影响。
	review, _ = runEnterpriseMaintenanceOnce(time.Now())
	require.EqualValues(t, 1, review)
	review, _ = runEnterpriseMaintenanceOnce(time.Now())
	require.Zero(t, review)
	require.Equal(t, []int64{1}, notified)
	require.Equal(t, model.EnterpriseReservationReview, f.reservation(info.RequestId).Status)
	require.Equal(t, model.EnterpriseReservationReserved, f.reservation(fresh.RequestId).Status)
	f.ledger(3_500_000, 1_500_000, 0)
	items, total, err := model.ListEnterpriseReviewReservations(f.ent.Id, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, info.RequestId, items[0].RequestId)

	// 迟到的结算仍然能把待对账的预留终结。
	require.NoError(t, SettleBilling(c, info, 300_000))
	require.Equal(t, model.EnterpriseReservationSettled, f.reservation(info.RequestId).Status)
	f.ledger(4_200_000, 500_000, 300_000)
	_, total, err = model.ListEnterpriseReviewReservations(f.ent.Id, 0, 10)
	require.NoError(t, err)
	require.Zero(t, total)
	f.personalUntouched()
}

func TestEnterpriseMaintenanceLoopStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ticks, runs, done := make(chan time.Time), make(chan struct{}, 4), make(chan struct{})
	go func() {
		runEnterpriseMaintenanceLoop(ctx, ticks, func() { runs <- struct{}{} })
		close(done)
	}()
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("回收任务循环没有按预期推进")
		}
	}
	wait(runs) // 启动即跑一次
	ticks <- time.Now()
	wait(runs)
	cancel()
	wait(done)
	require.Empty(t, runs)
}
