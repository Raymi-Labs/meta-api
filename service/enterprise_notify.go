package service

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

// 企业通知只走现有邮件通道。通知失败只记日志，不撤销已经完成的业务操作。

const (
	EnterpriseInvitePath    = "/enterprise/invite"
	EnterpriseApprovalsPath = "/enterprise/console/approvals"
	EnterpriseMembersPath   = "/enterprise/console/members"
	EnterpriseOverviewPath  = "/enterprise/console"
	EnterpriseMyQuotaPath   = "/console/enterprise"
)

func enterpriseLink(path string) string {
	return strings.TrimRight(system_setting.ServerAddress, "/") + path
}

// EnterpriseInvitationLink 是邀请接受页的地址，令牌只出现在这条链接里。
func EnterpriseInvitationLink(token string) string {
	return enterpriseLink(EnterpriseInvitePath) + "?token=" + url.QueryEscape(token)
}

func enterpriseMailBody(lines []string, link, action string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("<p>" + line + "</p>")
	}
	if link != "" {
		b.WriteString(fmt.Sprintf("<p><a href='%s'>%s</a></p><p>如果链接无法点击，请复制到浏览器中打开：<br> %s </p>", link, action, link))
	}
	return b.String()
}

// 测试里替换，避免真的发邮件、并让异步提醒变成同步执行。
var (
	enterpriseSendEmail = common.SendEmail
	enterpriseAsync     = func(run func()) { gopool.Go(run) }
)

func sendEnterpriseMail(receiver, subject, content string) bool {
	if receiver == "" {
		return false
	}
	if err := enterpriseSendEmail(subject, receiver, content); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("enterprise mail failed: subject=%q err=%v", subject, err))
		return false
	}
	return true
}

func enterpriseMailAsync(contacts []model.EnterpriseContact, subject, content string) {
	if len(contacts) == 0 {
		return
	}
	gopool.Go(func() {
		for _, contact := range contacts {
			sendEnterpriseMail(contact.Email, subject, content)
		}
	})
}

func enterpriseQuotaText(quota int64) string {
	return logger.FormatQuota(int(quota))
}

// SendEnterpriseInvitation 同步发送邀请邮件并返回是否送出；送不出时邀请人仍可复制链接转交。
func SendEnterpriseInvitation(ent *model.Enterprise, inv *model.EnterpriseInvitation, token string) (link string, sent bool) {
	link = EnterpriseInvitationLink(token)
	lines := []string{fmt.Sprintf("%s 邀请你加入企业「%s」。", html.EscapeString(common.SystemName), html.EscapeString(ent.Name))}
	if inv.InitialQuota > 0 {
		lines = append(lines, "加入后将获得初始额度 "+enterpriseQuotaText(inv.InitialQuota)+"。")
	}
	lines = append(lines, "邀请 7 天内有效，仅可使用一次；请使用本邮箱对应的账号登录后接受。")
	subject := fmt.Sprintf("%s：加入企业「%s」的邀请", common.SystemName, ent.Name)
	return link, sendEnterpriseMail(inv.Email, subject, enterpriseMailBody(lines, link, "接受邀请"))
}

// NotifyEnterpriseInviteShortfall 邀请的初始额度因企业未分配余额不足没有发放时，提示邀请人。
func NotifyEnterpriseInviteShortfall(ent *model.Enterprise, inv *model.EnterpriseInvitation) {
	contact, ok := model.GetEnterpriseContact(inv.InviterId)
	if !ok {
		return
	}
	lines := []string{fmt.Sprintf("%s 已加入企业「%s」，但企业未分配余额不足，邀请时填写的初始额度 %s 没有发放，该成员当前额度为 0。",
		html.EscapeString(inv.Email), html.EscapeString(ent.Name), enterpriseQuotaText(inv.InitialQuota)),
		"企业入账后可在成员页为其分配额度。"}
	enterpriseMailAsync([]model.EnterpriseContact{contact}, fmt.Sprintf("%s：成员初始额度未发放", common.SystemName),
		enterpriseMailBody(lines, enterpriseLink(EnterpriseMembersPath), "打开成员页"))
}

// NotifyEnterpriseRequestCreated 把新的额度申请通知给能处理它的人：
// 普通成员的申请通知所有者与管理员，管理员的申请只通知所有者。申请人本人不通知。
func NotifyEnterpriseRequestCreated(ent *model.Enterprise, req *model.EnterpriseQuotaRequest, requesterRole string) {
	contacts, err := model.ListEnterpriseManagerContacts(ent.Id, requesterRole != model.EnterpriseRoleMember)
	if err != nil {
		logger.LogWarn(context.Background(), "enterprise request notify failed: "+err.Error())
		return
	}
	receivers := contacts[:0]
	for _, contact := range contacts {
		if contact.UserId != req.UserId {
			receivers = append(receivers, contact)
		}
	}
	name := "成员"
	if requester, ok := model.GetEnterpriseContact(req.UserId); ok {
		name = requester.Name
	}
	lines := []string{fmt.Sprintf("%s 申请追加额度 %s。", html.EscapeString(name), enterpriseQuotaText(req.Amount))}
	if req.Reason != "" {
		lines = append(lines, "申请理由："+html.EscapeString(req.Reason))
	}
	enterpriseMailAsync(receivers, fmt.Sprintf("%s：「%s」有新的额度申请", common.SystemName, ent.Name),
		enterpriseMailBody(lines, enterpriseLink(EnterpriseApprovalsPath), "去审批"))
}

// NotifyEnterpriseRequestReviewed 把审批结果通知申请人。
func NotifyEnterpriseRequestReviewed(ent *model.Enterprise, req *model.EnterpriseQuotaRequest) {
	contact, ok := model.GetEnterpriseContact(req.UserId)
	if !ok {
		return
	}
	var lines []string
	subject := fmt.Sprintf("%s：额度申请未通过", common.SystemName)
	if req.Status == model.EnterpriseRequestApproved {
		subject = fmt.Sprintf("%s：额度申请已通过", common.SystemName)
		lines = append(lines, fmt.Sprintf("你在企业「%s」申请的额度已通过，到账 %s（申请 %s）。",
			html.EscapeString(ent.Name), enterpriseQuotaText(req.ApprovedAmount), enterpriseQuotaText(req.Amount)))
	} else {
		lines = append(lines, fmt.Sprintf("你在企业「%s」申请的额度 %s 未通过。", html.EscapeString(ent.Name), enterpriseQuotaText(req.Amount)))
	}
	if req.ReviewNote != "" {
		lines = append(lines, "审批备注："+html.EscapeString(req.ReviewNote))
	}
	enterpriseMailAsync([]model.EnterpriseContact{contact}, subject,
		enterpriseMailBody(lines, enterpriseLink(EnterpriseMyQuotaPath), "查看我的额度"))
}

// enterpriseMemberLowThreshold 成员低额提醒线沿用本人在个人设置里填的额度预警值，没填用平台默认值。
func enterpriseMemberLowThreshold(info *relaycommon.RelayInfo) int64 {
	if info.UserSetting.QuotaWarningThreshold > 0 {
		return int64(info.UserSetting.QuotaWarningThreshold)
	}
	return int64(common.QuotaRemindThreshold)
}

// checkEnterpriseMemberLowBalance 在企业请求结算后调用。先看内存里的结算后余额，
// 绝大多数请求到此为止，不产生额外的数据库写入。
func checkEnterpriseMemberLowBalance(info *relaycommon.RelayInfo) {
	session, ok := info.Billing.(*BillingSession)
	if !ok || session == nil {
		return
	}
	funding, ok := session.funding.(*EnterpriseFunding)
	if !ok {
		return
	}
	remindEnterpriseMemberIfLow(info, funding.availableAfter)
}

// remindEnterpriseMemberIfLow 按一次账本变更后的成员可用额度决定要不要提醒。
func remindEnterpriseMemberIfLow(info *relaycommon.RelayInfo, availableAfter int64) {
	threshold := enterpriseMemberLowThreshold(info)
	if availableAfter >= threshold {
		return
	}
	enterpriseID, memberID, userID := info.EnterpriseId, info.EnterpriseMemberId, info.UserId
	enterpriseAsync(func() { notifyEnterpriseMemberLowBalance(enterpriseID, memberID, userID, threshold) })
}

// notifyEnterpriseMemberLowBalance 给成员本人发一次低额提醒。成员获得新额度后才会再次提醒。
func notifyEnterpriseMemberLowBalance(enterpriseID, memberID, userID int, threshold int64) bool {
	claimed, err := model.ClaimEnterpriseMemberLowBalance(memberID, threshold)
	if err != nil {
		logger.LogWarn(context.Background(), "enterprise member low balance notify failed: "+err.Error())
		return false
	}
	if !claimed {
		return false
	}
	contact, ok := model.GetEnterpriseContact(userID)
	if !ok {
		return false
	}
	ent, err := model.GetEnterpriseByID(enterpriseID)
	if err != nil {
		return false
	}
	member, err := model.GetEnterpriseMember(enterpriseID, memberID)
	if err != nil {
		return false
	}
	line := fmt.Sprintf("你在企业「%s」的额度即将用尽，当前可用 %s。", html.EscapeString(ent.Name), enterpriseQuotaText(member.Available))
	if member.Available <= 0 {
		line = fmt.Sprintf("你在企业「%s」的额度已用完，新的请求会被拒绝。", html.EscapeString(ent.Name))
	}
	return sendEnterpriseMail(contact.Email, fmt.Sprintf("%s：企业额度不足", common.SystemName),
		enterpriseMailBody([]string{line}, enterpriseLink(EnterpriseMyQuotaPath), "申请额度"))
}

// notifyEnterpriseLowBalances 由后台任务调用：企业未分配余额低于企业自己设的提醒线时，
// 提醒所有者与管理员一次。返回本轮提醒的企业数。
func notifyEnterpriseLowBalances(limit int) int {
	ents, err := model.ClaimEnterpriseLowBalances(limit)
	if err != nil {
		common.SysError("enterprise low balance sweep failed: " + err.Error())
	}
	for _, ent := range ents {
		contacts, err := model.ListEnterpriseManagerContacts(ent.Id, false)
		if err != nil {
			common.SysError("enterprise low balance contacts failed: " + err.Error())
			continue
		}
		line := fmt.Sprintf("企业「%s」未分配余额为 %s，已低于提醒线 %s。余额用完后将无法再给成员分配额度。",
			html.EscapeString(ent.Name), enterpriseQuotaText(ent.Unallocated), enterpriseQuotaText(ent.LowBalanceThreshold))
		subject := fmt.Sprintf("%s：「%s」企业余额不足", common.SystemName, ent.Name)
		content := enterpriseMailBody([]string{line}, enterpriseLink(EnterpriseOverviewPath), "查看企业总览")
		for _, contact := range contacts {
			sendEnterpriseMail(contact.Email, subject, content)
		}
	}
	return len(ents)
}
