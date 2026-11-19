package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
	"time"
)

func GetInviteTierStatus(user int) (*dto.InviteTierStatus, error) {
	s := common.GetInviteTierSettings()
	month := InviteBusinessTime(time.Now()).Format("2006-01")
	tier, e := getTierForCommission(DB, user, month)
	if e != nil {
		return nil, e
	}
	if tier < 0 || tier > 5 {
		return nil, ErrInviteInvalid
	}
	result := dto.InviteTierStatus{UserId: user, Enabled: s.Enabled, Tier: tier, FirstRate: s.FirstLevelRates[tier], MinimumWithdrawalCents: int64(s.MinWithdrawalAmounts[tier]), RequireIdentity: s.RequireIdentityForPayout, IdentityStatus: "missing"}
	if e := DB.Model(&InviteeQualification{}).Where("inviter_id = ? AND qualified_month = ? AND status = 1", user, month).Count(&result.QualifiedCount).Error; e != nil {
		return nil, e
	}
	if e := DB.Model(&User{}).Where("inviter_id = ?", user).Count(&result.DirectCount).Error; e != nil {
		return nil, e
	}
	if tier < 2 {
		result.NextTierRemaining = max(int64(0), int64(s.PromotionThresholds[tier])-result.QualifiedCount)
	}
	var p PromoterProfile
	e = DB.Where("user_id = ?", user).First(&p).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	result.DepartmentId = p.DepartmentId
	result.Frozen = p.WithdrawalFrozen || p.Cleared
	result.UpstreamCut = p.UpstreamCut
	var identity PromoterIdentity
	e = DB.Where("user_id = ?", user).First(&identity).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	if e == nil {
		result.IdentityStatus = identity.Status
	}
	var debt InviteCommissionDebt
	e = DB.Where("inviter_user_id = ?", user).First(&debt).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	result.DebtAmount = max(int64(0), debt.DebtAmount-debt.PaidAmount)
	return &result, nil
}
func GetInviteList(user int, admin bool, resource string, statement int, status string, page *common.PageInfo) error {
	var dest any
	var table any
	owner := "user_id"
	switch resource {
	case "statements":
		table = &InviteCommissionStatement{}
		dest = &[]InviteCommissionStatement{}
		owner = "inviter_user_id"
	case "ledgers":
		table = &InviteCommissionLedger{}
		dest = &[]InviteCommissionLedger{}
		owner = "inviter_user_id"
	case "applications":
		table = &PromoterApplication{}
		dest = &[]PromoterApplication{}
	case "early-payout":
		table = &InviteEarlyPayout{}
		dest = &[]InviteEarlyPayout{}
	case "identities":
		table = &PromoterIdentity{}
		dest = &[]PromoterIdentity{}
	case "rebind":
		table = &InviteRebind{}
		dest = &[]InviteRebind{}
	case "departments":
		table = &Department{}
		dest = &[]Department{}
	case "actions":
		table = &InviteAdminAction{}
		dest = &[]InviteAdminAction{}
	case "signals":
		table = &InviteRiskSignal{}
		dest = &[]InviteRiskSignal{}
	default:
		return ErrInviteInvalid
	}
	q := DB.Model(table)
	if !admin && resource != "departments" {
		if resource == "rebind" {
			q = q.Where("user_id = ? OR new_inviter_id = ?", user, user)
		} else {
			q = q.Where(owner+" = ?", user)
		}
	} else if admin && user > 0 {
		q = q.Where(owner+" = ?", user)
	}
	if resource == "ledgers" {
		q = q.Where("amount_version = 2")
		if statement > 0 {
			q = q.Where("statement_id = ?", statement)
		}
	}
	if status != "" && resource != "departments" && resource != "actions" && resource != "signals" {
		q = q.Where("status = ?", status)
	}
	var count int64
	if e := q.Count(&count).Error; e != nil {
		return e
	}
	if e := q.Order("id DESC").Limit(page.GetPageSize()).Offset(page.GetStartIdx()).Find(dest).Error; e != nil {
		return e
	}
	if !admin && resource == "ledgers" {
		for i := range *dest.(*[]InviteCommissionLedger) {
			l := &(*dest.(*[]InviteCommissionLedger))[i]
			l.TopupTradeNo = maskTradeNoForLog(l.TopupTradeNo)
			l.InviteeUserId = 0
			l.DirectInviteeUserId = 0
		}
	}
	page.SetTotal(int(count))
	page.SetItems(dest)
	return nil
}
