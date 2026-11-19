package model

import (
	"errors"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	AffWithdrawalStatusPending  = "pending"
	AffWithdrawalStatusApproved = "approved"
	AffWithdrawalStatusRejected = "rejected"

	AffWithdrawalMinimumQuota       = 250
	AffWithdrawalMinimumAmountCents = int64(5000)
)

var (
	ErrAffWithdrawalInvalidQuota      = errors.New("提现额度不合法")
	ErrAffWithdrawalInvalidPayment    = errors.New("提现换算配置不合法")
	ErrAffWithdrawalQuotaTooLow       = errors.New("提现额度不能低于 250 闪电")
	ErrAffWithdrawalAmountTooLow      = errors.New("提现金额不能低于 50 元")
	ErrAffWithdrawalInsufficientQuota = errors.New("待使用收益不足")
	ErrAffWithdrawalAlreadyReviewed   = errors.New("提现申请已审核")
	ErrAffWithdrawalNotFound          = errors.New("提现申请不存在")
)

type AffWithdrawal struct {
	Id                   int     `json:"id"`
	UserId               int     `json:"user_id" gorm:"index;not null"`
	Quota                int     `json:"quota" gorm:"type:int;not null;default:0"`
	AmountCents          int64   `json:"amount_cents" gorm:"not null;default:0"`
	QuotaPerUnitSnapshot float64 `json:"quota_per_unit_snapshot" gorm:"type:decimal(20,6);not null;default:0"`
	PriceSnapshot        float64 `json:"price_snapshot" gorm:"type:decimal(20,6);not null;default:0"`
	AlipayAccount        string  `json:"alipay_account" gorm:"type:varchar(128);not null;default:''"`
	AlipayName           string  `json:"alipay_name" gorm:"type:varchar(64);not null;default:''"`
	Status               string  `json:"status" gorm:"type:varchar(16);index;not null;default:'pending'"`
	ReviewerUserId       int     `json:"reviewer_user_id" gorm:"index;not null;default:0"`
	AdminRemark          string  `json:"admin_remark" gorm:"type:varchar(255);not null;default:''"`
	CreatedAt            int64   `json:"created_at" gorm:"index"`
	ReviewedAt           int64   `json:"reviewed_at" gorm:"index;not null;default:0"`

	Username    string `json:"username,omitempty" gorm:"column:username;->"`
	DisplayName string `json:"display_name,omitempty" gorm:"column:display_name;->"`
}

type AffWithdrawalSearchParams struct {
	Status   string
	Username string
}

func CalculateAffWithdrawalAmountCents(quota int, quotaPerUnit float64, price float64) (int64, error) {
	if quota <= 0 {
		return 0, ErrAffWithdrawalInvalidQuota
	}
	if quotaPerUnit <= 0 || price <= 0 ||
		math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) ||
		math.IsNaN(price) || math.IsInf(price, 0) {
		return 0, ErrAffWithdrawalInvalidPayment
	}
	roundedCents := decimal.NewFromInt(int64(quota)).
		Div(decimal.NewFromFloat(quotaPerUnit)).
		Mul(decimal.NewFromFloat(price)).
		Mul(decimal.NewFromInt(100)).
		Round(0)
	if !roundedCents.IsPositive() {
		return 0, ErrAffWithdrawalAmountTooLow
	}
	cents := roundedCents.BigInt()
	if !cents.IsInt64() {
		return 0, ErrAffWithdrawalInvalidPayment
	}
	return cents.Int64(), nil
}

func validateAffWithdrawalQuota(quota int) error {
	if quota <= 0 {
		return ErrAffWithdrawalInvalidQuota
	}
	if quota < AffWithdrawalMinimumQuota {
		return ErrAffWithdrawalQuotaTooLow
	}
	return nil
}

func validateAffWithdrawalMinimum(quota int, amountCents int64) error {
	if err := validateAffWithdrawalQuota(quota); err != nil {
		return err
	}
	if amountCents < AffWithdrawalMinimumAmountCents {
		return ErrAffWithdrawalAmountTooLow
	}
	return nil
}

func CreateAffWithdrawal(userID int, quota int, alipayAccount string, alipayName string) (*AffWithdrawal, error) {
	account := strings.TrimSpace(alipayAccount)
	name := strings.TrimSpace(alipayName)
	if userID <= 0 {
		return nil, errors.New("用户不存在")
	}
	if quota <= 0 {
		return nil, ErrAffWithdrawalInvalidQuota
	}
	if !common.GetInviteTierSettings().Enabled {
		if err := validateAffWithdrawalQuota(quota); err != nil {
			return nil, err
		}
	}
	if account == "" {
		return nil, errors.New("支付宝账号不能为空")
	}
	if len([]rune(account)) > 128 {
		return nil, errors.New("支付宝账号不能超过 128 个字符")
	}
	if name == "" {
		return nil, errors.New("支付宝姓名不能为空")
	}
	if len([]rune(name)) > 64 {
		return nil, errors.New("支付宝姓名不能超过 64 个字符")
	}

	quotaPerUnitSnapshot := common.QuotaPerUnit
	priceSnapshot := operation_setting.Price
	amountCents, err := CalculateAffWithdrawalAmountCents(quota, quotaPerUnitSnapshot, priceSnapshot)
	if err != nil {
		return nil, err
	}
	if !common.GetInviteTierSettings().Enabled {
		if err := validateAffWithdrawalMinimum(quota, amountCents); err != nil {
			return nil, err
		}
	}

	withdrawal := &AffWithdrawal{
		UserId:               userID,
		Quota:                quota,
		AmountCents:          amountCents,
		QuotaPerUnitSnapshot: quotaPerUnitSnapshot,
		PriceSnapshot:        priceSnapshot,
		AlipayAccount:        account,
		AlipayName:           name,
		Status:               AffWithdrawalStatusPending,
		CreatedAt:            common.GetTimestamp(),
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if common.InviteTierLifecycleActive() {
			if _, e := lockInviteUser(tx, userID); e != nil {
				return e
			}
			if e := checkInvitePayoutEligibility(tx, userID); e != nil {
				return e
			}
			minimum, e := InviteWithdrawalMinimum(tx, userID)
			if e != nil {
				return e
			}
			if amountCents < minimum {
				return ErrInviteMinimum
			}
		}
		result := tx.Model(&User{}).
			Where("id = ? AND aff_quota >= ?", userID, quota).
			Update("aff_quota", gorm.Expr("aff_quota - ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrAffWithdrawalInsufficientQuota
		}
		return tx.Create(withdrawal).Error
	})
	if err != nil {
		return nil, err
	}
	return withdrawal, nil
}

func ApproveAffWithdrawal(id int, reviewerUserID int, adminRemark string) (*AffWithdrawal, error) {
	return reviewAffWithdrawal(id, reviewerUserID, AffWithdrawalStatusApproved, adminRemark)
}

func RejectAffWithdrawal(id int, reviewerUserID int, adminRemark string) (*AffWithdrawal, error) {
	return reviewAffWithdrawal(id, reviewerUserID, AffWithdrawalStatusRejected, adminRemark)
}

func reviewAffWithdrawal(id int, reviewerUserID int, targetStatus string, adminRemark string) (*AffWithdrawal, error) {
	if id <= 0 {
		return nil, ErrAffWithdrawalNotFound
	}
	if reviewerUserID <= 0 {
		return nil, errors.New("审核人不存在")
	}
	remark := strings.TrimSpace(adminRemark)
	if len([]rune(remark)) > 255 {
		return nil, errors.New("审核备注不能超过 255 个字符")
	}
	if targetStatus != AffWithdrawalStatusApproved && targetStatus != AffWithdrawalStatusRejected {
		return nil, errors.New("审核动作不合法")
	}

	var withdrawal AffWithdrawal
	now := common.GetTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&withdrawal, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAffWithdrawalNotFound
			}
			return err
		}
		if _, e := lockInviteUser(tx, withdrawal.UserId); e != nil {
			return e
		}
		if e := inviteLock(tx).First(&withdrawal, id).Error; e != nil {
			return e
		}
		if withdrawal.Status != AffWithdrawalStatusPending {
			return ErrAffWithdrawalAlreadyReviewed
		}
		if targetStatus == AffWithdrawalStatusApproved {
			minimum, e := InviteWithdrawalMinimum(tx, withdrawal.UserId)
			if e != nil {
				return e
			}
			if common.InviteTierLifecycleActive() {
				if e := checkInvitePayoutEligibility(tx, withdrawal.UserId); e != nil {
					return e
				}
				if withdrawal.AmountCents < minimum {
					return ErrInviteMinimum
				}
			} else if err := validateAffWithdrawalMinimum(withdrawal.Quota, withdrawal.AmountCents); err != nil {
				return err
			}
		}

		result := tx.Model(&AffWithdrawal{}).
			Where("id = ? AND status = ?", id, AffWithdrawalStatusPending).
			Updates(map[string]interface{}{
				"status":           targetStatus,
				"reviewer_user_id": reviewerUserID,
				"admin_remark":     remark,
				"reviewed_at":      now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrAffWithdrawalAlreadyReviewed
		}
		if targetStatus == AffWithdrawalStatusRejected {
			if err := tx.Model(&User{}).Where("id = ?", withdrawal.UserId).
				Update("aff_quota", gorm.Expr("aff_quota + ?", withdrawal.Quota)).Error; err != nil {
				return err
			}
		}
		withdrawal.Status = targetStatus
		withdrawal.ReviewerUserId = reviewerUserID
		withdrawal.AdminRemark = remark
		withdrawal.ReviewedAt = now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &withdrawal, nil
}

func affWithdrawalListWithUser(tx *gorm.DB) *gorm.DB {
	return tx.Table("aff_withdrawals").
		Select("aff_withdrawals.*, users.username AS username, users.display_name AS display_name").
		Joins("LEFT JOIN users ON users.id = aff_withdrawals.user_id")
}

func applyAffWithdrawalSearch(query *gorm.DB, params AffWithdrawalSearchParams, includeUsername bool) *gorm.DB {
	if params.Status != "" {
		query = query.Where("aff_withdrawals.status = ?", params.Status)
	}
	if includeUsername && params.Username != "" {
		query = applyPaymentRecordUsernameFilter(query, params.Username, "users.id", "users.username")
	}
	return query
}

func GetUserAffWithdrawalsByParams(userID int, params AffWithdrawalSearchParams, pageInfo *common.PageInfo) ([]*AffWithdrawal, int64, error) {
	if userID <= 0 {
		return nil, 0, errors.New("用户不存在")
	}
	if pageInfo == nil {
		pageInfo = &common.PageInfo{Page: 1, PageSize: common.ItemsPerPage}
	}

	var total int64
	countQuery := applyAffWithdrawalSearch(DB.Model(&AffWithdrawal{}).Where("user_id = ?", userID), params, false)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var withdrawals []*AffWithdrawal
	dataQuery := applyAffWithdrawalSearch(DB.Model(&AffWithdrawal{}).Where("user_id = ?", userID), params, false)
	if err := dataQuery.Order("id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&withdrawals).Error; err != nil {
		return nil, 0, err
	}
	return withdrawals, total, nil
}

func GetAllAffWithdrawalsByParams(params AffWithdrawalSearchParams, pageInfo *common.PageInfo) ([]*AffWithdrawal, int64, error) {
	if pageInfo == nil {
		pageInfo = &common.PageInfo{Page: 1, PageSize: common.ItemsPerPage}
	}

	countQuery := applyAffWithdrawalSearch(affWithdrawalListWithUser(DB), params, true)
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	dataQuery := applyAffWithdrawalSearch(affWithdrawalListWithUser(DB), params, true)
	var withdrawals []*AffWithdrawal
	if err := dataQuery.Order("aff_withdrawals.id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&withdrawals).Error; err != nil {
		return nil, 0, err
	}
	return withdrawals, total, nil
}
