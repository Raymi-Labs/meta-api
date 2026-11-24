package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AutoTopupDisabled = "disabled"
	AutoTopupSetup    = "setup"
	AutoTopupArmed    = "armed"
	AutoTopupWaiting  = "waiting"
	AutoTopupFailed   = "failed"
)

type WalletAutoTopup struct {
	UserID       int     `json:"-" gorm:"primaryKey;autoIncrement:false"`
	State        string  `json:"state" gorm:"type:varchar(16);index;not null"`
	Threshold    int64   `json:"threshold"`
	Quantity     int64   `json:"quantity"`
	GrantMoney   float64 `json:"grant_money"`
	UnitMinor    int64   `json:"unit_minor"`
	Currency     string  `json:"currency" gorm:"type:varchar(8)"`
	PriceID      string  `json:"-" gorm:"type:varchar(191)"`
	SetupRef     string  `json:"-" gorm:"type:varchar(64);index"`
	Customer     string  `json:"-" gorm:"type:varchar(191)"`
	Method       string  `json:"-" gorm:"type:varchar(191)"`
	AuthorizedAt int64   `json:"authorized_at"`
	Trade        string  `json:"-" gorm:"type:varchar(64);index"`
	Intent       string  `json:"-" gorm:"type:varchar(191)"`
}

func GetWalletAutoTopup(userID int) (WalletAutoTopup, error) {
	var rule WalletAutoTopup
	err := DB.First(&rule, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return WalletAutoTopup{UserID: userID, State: AutoTopupDisabled}, nil
	}
	return rule, err
}

func BeginWalletAutoTopupSetup(rule *WalletAutoTopup) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		// Serialize even the first configuration, when no settings row exists yet.
		if _, err := lockPulseFundingUserTx(tx, rule.UserID); err != nil {
			return err
		}
		var existing WalletAutoTopup
		res := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", rule.UserID).Limit(1).Find(&existing)
		if res.Error != nil {
			return res.Error
		}
		if existing.Trade != "" {
			return errors.New("请等待当前自动充值订单完成")
		}
		rule.State = AutoTopupSetup
		return tx.Save(rule).Error
	})
}

func ActivateWalletAutoTopup(ref, customer, method string) error {
	if ref == "" || customer == "" || method == "" {
		return errors.New("自动充值授权缺少支付方式")
	}
	return DB.Model(&WalletAutoTopup{}).Where("setup_ref = ? AND customer = ? AND state = ?", ref, customer, AutoTopupSetup).
		Updates(map[string]any{"state": AutoTopupArmed, "method": method, "authorized_at": common.GetTimestamp()}).Error
}

func DisableWalletAutoTopup(userID int) error {
	// Keep the immutable payment coordinates of an already started order.
	return DB.Model(&WalletAutoTopup{}).Where("user_id = ?", userID).
		Updates(map[string]any{"state": AutoTopupDisabled, "setup_ref": ""}).Error
}

func WalletAutoTopupCandidates(after, limit int) ([]int, error) {
	var ids []int
	err := DB.Model(&WalletAutoTopup{}).Select("wallet_auto_topups.user_id").
		Joins("LEFT JOIN users ON users.id = wallet_auto_topups.user_id").
		Where("wallet_auto_topups.user_id > ?", after).
		Where("trade <> '' OR (users.deleted_at IS NULL AND users.status = ? AND ((state = ? AND users.quota < threshold) OR (state = ? AND users.quota >= threshold)))", common.UserStatusEnabled, AutoTopupArmed, AutoTopupWaiting).
		Order("wallet_auto_topups.user_id").Limit(limit).Pluck("wallet_auto_topups.user_id", &ids).Error
	return ids, err
}

// Confirm settled usage before charging. Synchronous requests temporarily
// subtract their reservation from users.quota even when they later refund it.
func settledAutoTopupBalance(tx *gorm.DB, user *User) (int64, error) {
	var held int64
	err := tx.Model(&PulseWalletReservation{}).Where("user_id = ? AND status = ?", user.Id, "reserved").
		Select("COALESCE(SUM(quota),0)").Scan(&held).Error
	return int64(user.Quota) + held, err
}

func ClaimWalletAutoTopup(userID int, priceID string, minimum int64) (WalletAutoTopup, error) {
	var rule WalletAutoTopup
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Same lock order as setup. Disabled or deleted users cannot start new payments.
		user, err := lockPulseFundingUserTx(tx, userID)
		if err != nil {
			return err
		}
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&rule, "user_id = ?", userID).Error; err != nil {
			return err
		}
		if rule.Trade != "" {
			return nil
		}
		if err := tx.Unscoped().Select("status", commonGroupCol).First(user, userID).Error; err != nil {
			return err
		}
		if user.DeletedAt.Valid || user.Status != common.UserStatusEnabled || (rule.State != AutoTopupArmed && rule.State != AutoTopupWaiting) {
			return nil
		}
		balance, err := settledAutoTopupBalance(tx, user)
		if err != nil {
			return err
		}
		if balance >= rule.Threshold {
			if rule.State == AutoTopupWaiting {
				return tx.Model(&rule).Update("state", AutoTopupArmed).Error
			}
			return nil
		}
		if rule.State != AutoTopupArmed || priceID == "" {
			return nil
		}
		groupRatio := common.GetTopupGroupRatio(user.Group)
		if groupRatio == 0 {
			groupRatio = 1
		}
		if rule.PriceID != priceID || rule.Quantity < minimum || rule.GrantMoney != float64(rule.Quantity)*groupRatio {
			return tx.Model(&rule).Update("state", AutoTopupFailed).Error
		}
		rule.Trade = "auto_" + common.GetUUID()
		order := TopUp{UserId: userID, TradeNo: rule.Trade, Amount: rule.Quantity, Money: rule.GrantMoney,
			PaidCurrency: rule.Currency, PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
			PaidMoney: rule.PaidMoney(), CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
		if err = tx.Create(&order).Error; err != nil {
			return err
		}
		return tx.Model(&rule).Update("trade", rule.Trade).Error
	})
	return rule, err
}

func (rule WalletAutoTopup) PaidMoney() float64 {
	return StripeMoneyFromMinor(rule.Quantity*rule.UnitMinor, rule.Currency)
}

func StripeMoneyFromMinor(amount int64, currency string) float64 {
	if amount <= 0 || NormalizePaymentCurrency(currency) == "" {
		return 0
	}
	switch strings.ToUpper(currency) {
	case "BIF", "CLP", "DJF", "GNF", "JPY", "KMF", "KRW", "MGA", "PYG", "RWF", "VND", "VUV", "XAF", "XOF", "XPF":
		return float64(amount)
	case "BHD", "JOD", "KWD", "OMR", "TND":
		return float64(amount) / 1000
	default:
		return float64(amount) / 100
	}
}

func FailWalletAutoTopup(rule WalletAutoTopup) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		// Match the observed intent as well as the trade: a delayed creation error
		// cannot retire a different worker's already-bound intent.
		res := tx.Model(&WalletAutoTopup{}).Where("user_id = ? AND trade = ? AND intent = ?", rule.UserID, rule.Trade, rule.Intent).
			Updates(map[string]any{"trade": "", "intent": "", "state": gorm.Expr("CASE WHEN state = ? THEN state ELSE ? END", AutoTopupDisabled, AutoTopupFailed)})
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		return tx.Model(&TopUp{}).Where("trade_no = ? AND status = ?", rule.Trade, common.TopUpStatusPending).Update("status", common.TopUpStatusFailed).Error
	})
}

func finishWalletAutoTopupTx(tx *gorm.DB, order *TopUp) error {
	if !strings.HasPrefix(order.TradeNo, "auto_") {
		return nil
	}
	var user User
	if err := tx.First(&user, order.UserId).Error; err != nil {
		return err
	}
	balance, err := settledAutoTopupBalance(tx, &user)
	if err != nil {
		return err
	}
	return tx.Model(&WalletAutoTopup{}).Where("user_id = ? AND trade = ?", order.UserId, order.TradeNo).
		Updates(map[string]any{"trade": "", "intent": "", "state": gorm.Expr("CASE WHEN state = ? THEN state WHEN threshold <= ? THEN ? ELSE ? END", AutoTopupDisabled, balance, AutoTopupArmed, AutoTopupWaiting)}).Error
}
