package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 企业工作空间接口。成员侧与企业后台接口由 middleware.EnterpriseMemberAuth 写入企业范围，
// 平台侧由 middleware.EnterprisePlatformScope 写入；处理函数只从上下文取企业 ID，不直接信任路径参数。

func enterpriseApiError(c *gin.Context, err error) {
	if model.IsEnterpriseUserError(err) {
		common.ApiError(c, err)
		return
	}
	if errors.Is(err, model.ErrEnterpriseFinalized) {
		common.ApiErrorMsg(c, "该记录已处理")
		return
	}
	logger.LogError(c.Request.Context(), "enterprise api failed: "+err.Error())
	common.ApiErrorMsg(c, "操作失败，请稍后重试")
}

func enterpriseScope(c *gin.Context) (int, model.EnterpriseActor, bool) {
	enterpriseID := common.GetContextKeyInt(c, constant.ContextKeyEnterpriseId)
	if enterpriseID <= 0 {
		common.ApiError(c, model.ErrEnterpriseNotFound)
		return 0, model.EnterpriseActor{}, false
	}
	return enterpriseID, model.EnterpriseActor{
		UserId:   c.GetInt("id"),
		Platform: common.GetContextKeyBool(c, constant.ContextKeyEnterprisePlatform),
	}, true
}

func enterprisePathInt(c *gin.Context, name string) (int, bool) {
	value, err := strconv.Atoi(c.Param(name))
	if err != nil || value <= 0 {
		common.ApiError(c, model.ErrEnterpriseNotFound)
		return 0, false
	}
	return value, true
}

func enterpriseBody(c *gin.Context, v any) bool {
	if err := common.DecodeJson(c.Request.Body, v); err != nil {
		common.ApiError(c, model.ErrEnterpriseInvalidInput)
		return false
	}
	return true
}

func enterprisePage(c *gin.Context, pageInfo *common.PageInfo, items any, total int64, err error) {
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// ---- 成员本人 ----

// GetEnterpriseSelf 返回登录账号的企业身份与额度；未加入企业时 data 为 null。
func GetEnterpriseSelf(c *gin.Context) {
	userID := c.GetInt("id")
	member, err := model.GetActiveEnterpriseMemberByUser(userID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	if member == nil {
		common.ApiSuccess(c, nil)
		return
	}
	ent, err := model.GetEnterpriseByID(member.EnterpriseId)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	manager := model.EnterpriseRoleAtLeast(member.Role, model.EnterpriseRoleAdmin)
	data := gin.H{
		"enterprise": gin.H{"id": ent.Id, "name": ent.Name, "status": ent.Status, "timezone": ent.Timezone},
		"member": gin.H{
			"id": member.Id, "role": member.Role, "available": member.Available, "reserved": member.Reserved,
			"consumed": member.Consumed, "joined_at": member.JoinedAt,
		},
		"models":  service.EnterpriseMemberModels(ent, member),
		"manager": manager,
	}
	pending, _, err := model.ListEnterpriseQuotaRequests(ent.Id, userID, model.EnterpriseRequestPending, 0, 1)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	if len(pending) > 0 {
		data["pending_request"] = pending[0]
	}
	if manager {
		count, err := model.CountEnterprisePendingRequests(ent.Id)
		if err != nil {
			enterpriseApiError(c, err)
			return
		}
		data["pending_requests"] = count
		data["unallocated"] = ent.Unallocated
	}
	common.ApiSuccess(c, data)
}

func enterpriseSelfMember(c *gin.Context) (*model.EnterpriseMember, bool) {
	member, err := model.GetActiveEnterpriseMemberByUser(c.GetInt("id"))
	if err != nil {
		enterpriseApiError(c, err)
		return nil, false
	}
	if member == nil {
		common.ApiError(c, model.ErrEnterpriseNotFound)
		return nil, false
	}
	return member, true
}

func ListEnterpriseSelfRequests(c *gin.Context) {
	member, ok := enterpriseSelfMember(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseQuotaRequests(member.EnterpriseId, member.UserId, c.Query("status"),
		pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

func CreateEnterpriseSelfRequest(c *gin.Context) {
	member, ok := enterpriseSelfMember(c)
	if !ok {
		return
	}
	var req struct {
		Amount int64  `json:"amount"`
		Reason string `json:"reason"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	created, err := model.CreateEnterpriseQuotaRequest(member.UserId, member.EnterpriseId, req.Amount, req.Reason)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	if ent, err := model.GetEnterpriseByID(member.EnterpriseId); err == nil {
		service.NotifyEnterpriseRequestCreated(ent, created, member.Role)
	}
	common.ApiSuccess(c, created)
}

func CancelEnterpriseSelfRequest(c *gin.Context) {
	member, ok := enterpriseSelfMember(c)
	if !ok {
		return
	}
	requestID, ok := enterprisePathInt(c, "request_id")
	if !ok {
		return
	}
	if err := model.CancelEnterpriseQuotaRequest(member.UserId, member.EnterpriseId, requestID); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func LeaveEnterpriseSelf(c *gin.Context) {
	member, ok := enterpriseSelfMember(c)
	if !ok {
		return
	}
	if err := model.LeaveEnterprise(member.UserId, member.EnterpriseId); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ---- 邀请 ----

// PreviewEnterpriseInvitation 邀请接受页的公开预览，只返回企业名称、被邀请邮箱与角色。
func PreviewEnterpriseInvitation(c *gin.Context) {
	preview, err := model.PreviewEnterpriseInvitation(strings.TrimSpace(c.Query("token")))
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, preview)
}

func AcceptEnterpriseInvitation(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	result, err := model.AcceptEnterpriseInvitation(strings.TrimSpace(req.Token), c.GetInt("id"))
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	if result.QuotaShortfall {
		service.NotifyEnterpriseInviteShortfall(result.Enterprise, result.Invitation)
	}
	common.ApiSuccess(c, gin.H{
		"enterprise_id":   result.Enterprise.Id,
		"enterprise_name": result.Enterprise.Name,
		"role":            result.Member.Role,
		"available":       result.Member.Available,
		"quota_shortfall": result.QuotaShortfall,
	})
}

func ListEnterpriseInvitations(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseInvitations(enterpriseID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

func CreateEnterpriseInvitation(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	var req struct {
		Email        string `json:"email"`
		Role         string `json:"role"`
		InitialQuota int64  `json:"initial_quota"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	token, inv, err := model.CreateEnterpriseInvitation(actor, enterpriseID, req.Email, req.Role, req.InitialQuota)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	ent, err := model.GetEnterpriseByID(enterpriseID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	// 链接只在创建时返回一次；邮件发不出去时邀请人可以复制链接转交。
	link, sent := service.SendEnterpriseInvitation(ent, inv, token)
	common.ApiSuccess(c, gin.H{"invitation": inv, "link": link, "mail_sent": sent})
}

func RevokeEnterpriseInvitation(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	invitationID, ok := enterprisePathInt(c, "invitation_id")
	if !ok {
		return
	}
	if err := model.RevokeEnterpriseInvitation(actor, enterpriseID, invitationID); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ---- 企业后台 ----

func GetEnterpriseOverview(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	ent, err := model.GetEnterpriseByID(enterpriseID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	model.FillEnterpriseOwners(ent)
	balance, err := model.GetEnterpriseBalance(enterpriseID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	summary, err := model.GetEnterpriseSummary(enterpriseID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"enterprise": ent, "balance": balance, "summary": summary})
}

func ListEnterpriseMembers(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseMembers(enterpriseID, c.Query("status"), c.Query("keyword"),
		pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

func GetEnterpriseMember(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	memberID, ok := enterprisePathInt(c, "member_id")
	if !ok {
		return
	}
	member, err := model.GetEnterpriseMemberDetail(enterpriseID, memberID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, member)
}

type enterpriseAmountRequest struct {
	Amount int64  `json:"amount"`
	Reason string `json:"reason"`
}

func enterpriseMemberAmount(c *gin.Context, apply func(actor model.EnterpriseActor, enterpriseID, memberID int, amount int64, reason string) error) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	memberID, ok := enterprisePathInt(c, "member_id")
	if !ok {
		return
	}
	var req enterpriseAmountRequest
	if !enterpriseBody(c, &req) {
		return
	}
	if err := apply(actor, enterpriseID, memberID, req.Amount, req.Reason); err != nil {
		enterpriseApiError(c, err)
		return
	}
	member, err := model.GetEnterpriseMemberDetail(enterpriseID, memberID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, member)
}

func AllocateEnterpriseQuota(c *gin.Context) {
	enterpriseMemberAmount(c, model.AllocateEnterpriseQuota)
}

func ReclaimEnterpriseQuota(c *gin.Context) {
	enterpriseMemberAmount(c, model.ReclaimEnterpriseQuota)
}

func SetEnterpriseMemberModels(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	memberID, ok := enterprisePathInt(c, "member_id")
	if !ok {
		return
	}
	var req struct {
		ModelMode string   `json:"model_mode"`
		Models    []string `json:"models"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	if err := model.SetEnterpriseMemberModels(actor, enterpriseID, memberID, req.ModelMode, req.Models); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func SetEnterpriseMemberRole(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	memberID, ok := enterprisePathInt(c, "member_id")
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	if err := model.SetEnterpriseMemberRole(actor, enterpriseID, memberID, req.Role); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func TransferEnterpriseOwnership(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	memberID, ok := enterprisePathInt(c, "member_id")
	if !ok {
		return
	}
	if err := model.TransferEnterpriseOwnership(actor, enterpriseID, memberID); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func DisableEnterpriseMember(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	memberID, ok := enterprisePathInt(c, "member_id")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	if err := model.DisableEnterpriseMember(actor, enterpriseID, memberID, req.Reason); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ListEnterpriseQuotaRequests(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseQuotaRequests(enterpriseID, 0, c.Query("status"),
		pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

func reviewEnterpriseQuotaRequest(c *gin.Context, approve bool) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	requestID, ok := enterprisePathInt(c, "request_id")
	if !ok {
		return
	}
	var req struct {
		Amount int64  `json:"amount"`
		Note   string `json:"note"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	reviewed, err := model.ReviewEnterpriseQuotaRequest(actor, enterpriseID, requestID, approve, req.Amount, req.Note)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	if ent, err := model.GetEnterpriseByID(enterpriseID); err == nil {
		service.NotifyEnterpriseRequestReviewed(ent, reviewed)
	}
	common.ApiSuccess(c, reviewed)
}

func ApproveEnterpriseQuotaRequest(c *gin.Context) {
	reviewEnterpriseQuotaRequest(c, true)
}

func RejectEnterpriseQuotaRequest(c *gin.Context) {
	reviewEnterpriseQuotaRequest(c, false)
}

func ListEnterpriseLedger(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	memberID, _ := strconv.Atoi(c.Query("member_id"))
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseLedger(enterpriseID, memberID, c.Query("kind"),
		pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

func ListEnterpriseAudit(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseAuditEvents(enterpriseID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

// GetEnterpriseModelOptions 返回平台对企业开放的模型，供"指定模型"挑选。
func GetEnterpriseModelOptions(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	ent, err := model.GetEnterpriseByID(enterpriseID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, service.EnterprisePlatformModels(ent.BillingGroup))
}

func UpdateEnterpriseSettings(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	var req struct {
		Name                string   `json:"name"`
		Timezone            string   `json:"timezone"`
		ModelMode           string   `json:"model_mode"`
		Models              []string `json:"models"`
		LowBalanceThreshold int64    `json:"low_balance_threshold"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	if err := model.UpdateEnterpriseSettings(actor, enterpriseID, model.EnterpriseSettingsInput{
		Name: req.Name, Timezone: req.Timezone, ModelMode: req.ModelMode, Models: req.Models,
		LowBalanceThreshold: req.LowBalanceThreshold,
	}); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ---- 平台后台 ----

func AdminListEnterprises(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterprises(c.Query("keyword"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

func AdminCreateEnterprise(c *gin.Context) {
	var req struct {
		Name         string `json:"name"`
		Owner        string `json:"owner"`
		BillingGroup string `json:"billing_group"`
		Timezone     string `json:"timezone"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	ownerID, err := model.ResolveEnterpriseOwnerCandidate(req.Owner)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	ent, err := model.CreateEnterprise(c.GetInt("id"), req.Name, req.BillingGroup, req.Timezone, ownerID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	model.FillEnterpriseOwners(ent)
	common.ApiSuccess(c, ent)
}

func AdminUpdateEnterprise(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	var req struct {
		Name         string `json:"name"`
		BillingGroup string `json:"billing_group"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	if err := model.UpdateEnterpriseByPlatform(actor.UserId, enterpriseID, req.Name, req.BillingGroup); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminSetEnterpriseStatus(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	if err := model.SetEnterpriseStatus(actor.UserId, enterpriseID, req.Status, req.Reason); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminCreditEnterprise(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	var req struct {
		Amount  int64  `json:"amount"`
		Voucher string `json:"voucher"`
		Reason  string `json:"reason"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	entry, err := model.CreditEnterprise(actor.UserId, enterpriseID, req.Amount, req.Voucher, req.Reason)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, entry)
}

func AdminAdjustEnterprise(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	var req struct {
		Delta  int64  `json:"delta"`
		Reason string `json:"reason"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	entry, err := model.AdjustEnterprise(actor.UserId, enterpriseID, req.Delta, req.Reason)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, entry)
}

func AdminListEnterpriseReservations(c *gin.Context) {
	enterpriseID, _ := strconv.Atoi(c.Query("enterprise_id"))
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseReviewReservations(enterpriseID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

func AdminResolveEnterpriseReservation(c *gin.Context) {
	reservationID, err := strconv.ParseInt(c.Param("reservation_id"), 10, 64)
	if err != nil || reservationID <= 0 {
		common.ApiError(c, model.ErrEnterpriseNotFound)
		return
	}
	var req struct {
		Settle bool   `json:"settle"`
		Amount int64  `json:"amount"`
		Reason string `json:"reason"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	if err := model.ResolveEnterpriseReservation(c.GetInt("id"), reservationID, req.Settle, req.Amount, req.Reason); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
