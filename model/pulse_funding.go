package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const PulseFundingContextKey = "newapi_verified_wallet_funding"

// PulseFundingLedger is an append-only audit of proven paid allowance. Unknown
// historical balances and gifts never create an allowance. Amounts are quota
// integers, not display currency or transferable wallet credit.
type PulseFundingLedger struct {
	ID         string `gorm:"primaryKey;type:varchar(64)"`
	UserID     int    `gorm:"not null;index"`
	Kind       string `gorm:"type:varchar(32);not null"`
	SourceRef  string `gorm:"type:varchar(255);not null;index"`
	QuotaDelta int64  `gorm:"not null"`
	PaidDelta  int64  `gorm:"not null"`
	PaidAfter  int64  `gorm:"not null"`
	CreatedAt  int64  `gorm:"not null;index"`
}

// PulseWalletReservation follows one synchronous wallet request from reserve to
// settle/refund. Its final snapshot is immutable; proof_ref deduplicates logs.
type PulseWalletReservation struct {
	ID              string `gorm:"primaryKey;type:varchar(64)"`
	UserID          int    `gorm:"not null;index"`
	RequestID       string `gorm:"type:varchar(64);not null;uniqueIndex"`
	Quota           int64  `gorm:"not null"`
	PaidQuota       int64  `gorm:"not null"`
	FundingEpoch    uint64 `gorm:"not null"`
	Status          string `gorm:"type:varchar(16);not null"`
	CreatedAt       int64  `gorm:"not null"`
	UpdatedAt       int64  `gorm:"not null"`
	AvailableBefore int    `gorm:"-" json:"-"`
}

type PulseFundingSnapshot struct {
	Version      int    `json:"version"`
	Status       string `json:"status"`
	PaidQuota    int64  `json:"paid_quota"`
	NonPaidQuota int64  `json:"non_paid_quota"`
	UnknownQuota int64  `json:"unknown_quota"`
	ProofRef     string `json:"proof_ref,omitempty"`
	Reason       string `json:"reason,omitempty"`
	UserID       int    `json:"-"`
	RequestID    string `json:"-"`
}

func unknownPulseFunding(quota int) PulseFundingSnapshot {
	return PulseFundingSnapshot{Version: 1, Status: "unknown", UnknownQuota: int64(max(quota, 0)), Reason: "funding_source_not_attributed"}
}

// The log writer owns this reserved key. Callers cannot smuggle a paid claim
// through arbitrary Other metadata, including upstream responses.
func withPulseFunding(other map[string]interface{}, snapshot PulseFundingSnapshot) map[string]interface{} {
	result := make(map[string]interface{}, len(other)+1)
	for key, value := range other {
		result[key] = value
	}
	result["pulse_funding"] = snapshot
	return result
}

func lockPulseFundingUserTx(tx *gorm.DB, userID int) (*User, error) {
	if common.UsingSQLite {
		// SQLite has no SELECT FOR UPDATE. Acquire its writer reservation before
		// the first read, otherwise parallel deferred transactions can all read
		// the old balance and fail upgrading their snapshots with SQLITE_BUSY.
		if err := tx.Unscoped().Model(&User{}).Where("id = ?", userID).
			UpdateColumn("pulse_paid_quota", gorm.Expr("pulse_paid_quota")).Error; err != nil {
			return nil, err
		}
	}
	query := tx.Unscoped().Select("id", "quota", "pulse_paid_quota", "pulse_funding_epoch", "deleted_at").Where("id = ?", userID)
	if !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var user User
	if err := query.First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func appendPulseFundingLedgerTx(tx *gorm.DB, user *User, kind, source string, quotaDelta, paidDelta int64) error {
	return tx.Create(&PulseFundingLedger{ID: common.GetUUID(), UserID: user.Id, Kind: kind, SourceRef: source,
		QuotaDelta: quotaDelta, PaidDelta: paidDelta, PaidAfter: user.PulsePaidQuota, CreatedAt: common.GetTimestamp()}).Error
}

// InvalidatePulsePaidFundingTx conservatively burns attribution when a wallet
// mutation has no matched reservation. Epoch changes also prevent an in-flight
// refund from resurrecting allowance after an administrative change/clawback.
// Call before the balance mutation, within the same transaction.
func InvalidatePulsePaidFundingTx(tx *gorm.DB, userID int, reason string) error {
	user, err := lockPulseFundingUserTx(tx, userID)
	if err != nil {
		return err
	}
	if err := cashInvalidateWalletTx(tx, user); err != nil {
		return err
	}
	if user.PulseFundingEpoch == 0 && user.PulsePaidQuota == 0 {
		return nil
	}
	oldPaid := user.PulsePaidQuota
	user.PulsePaidQuota = 0
	if user.PulseFundingEpoch >= math.MaxInt64 {
		return errors.New("paid funding epoch overflow")
	}
	user.PulseFundingEpoch++
	updated := tx.Unscoped().Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
		"pulse_paid_quota": 0, "pulse_funding_epoch": user.PulseFundingEpoch,
	})
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return appendPulseFundingLedgerTx(tx, user, "invalidate", reason, 0, -oldPaid)
}

// creditOnlinePaidFundingTx is called only by successful verified provider
// callbacks, after their integer wallet grant, in the same transaction. Manual
// completion, redemption, gifts, and historical successful orders are excluded.
func creditOnlinePaidFundingTx(tx *gorm.DB, topUp *TopUp, quota int, source string) error {
	switch source {
	case "stripe", "creem", "epay", "epay_return":
	default:
		return nil
	}
	if topUp == nil || topUp.Status != common.TopUpStatusSuccess || topUp.Money <= 0 || quota <= 0 || strings.TrimSpace(topUp.TradeNo) == "" {
		return nil
	}
	user, err := lockPulseFundingUserTx(tx, topUp.UserId)
	if err != nil {
		return err
	}
	oldPaid := user.PulsePaidQuota
	// Repaying a pre-existing negative balance is not new spendable allowance.
	available := int64(max(user.Quota, 0))
	paidGrant := min(int64(quota), max(available-oldPaid, 0))
	user.PulsePaidQuota += paidGrant
	if user.PulseFundingEpoch == 0 {
		user.PulseFundingEpoch = 1
	}
	if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"pulse_paid_quota": user.PulsePaidQuota, "pulse_funding_epoch": user.PulseFundingEpoch,
	}).Error; err != nil {
		return err
	}
	return appendPulseFundingLedgerTx(tx, user, "online_payment", topUp.TradeNo, int64(quota), paidGrant)
}

// AdjustPulseWalletReservation moves a reservation to an absolute total. A
// retried transition therefore cannot charge/refund twice. Wallet and paid
// allowance mutations always commit atomically; Redis is only a cache.
func AdjustPulseWalletReservation(userID int, requestID string, target int, state string, requireAvailable bool) (PulseWalletReservation, error) {
	return adjustPulseWalletReservation(userID, requestID, target, state, requireAvailable, nil)
}

type chatWalletAdmission struct{ initial, trust int }

// ReserveChatWalletQuota decides the hold using the same locked balance that
// pays it. Cached balances cannot grant multiple requests the same funds.
func ReserveChatWalletQuota(userID int, requestID string, full, initial, trust int) (PulseWalletReservation, error) {
	if initial <= 0 || initial > full || trust <= 0 || initial > trust {
		return PulseWalletReservation{}, errors.New("invalid chat wallet admission")
	}
	return adjustPulseWalletReservation(userID, requestID, full, "reserved", true, &chatWalletAdmission{initial, trust})
}

func adjustPulseWalletReservation(userID int, requestID string, target int, state string, requireAvailable bool, chat *chatWalletAdmission) (PulseWalletReservation, error) {
	var result PulseWalletReservation
	if userID <= 0 || requestID == "" || len(requestID) > 64 || target < 0 || (state != "reserved" && state != "settled" && state != "refunded") || (state == "refunded" && target != 0) {
		return result, errors.New("invalid wallet funding reservation")
	}
	var walletDelta int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		user, err := lockPulseFundingUserTx(tx, userID)
		if err != nil {
			return err
		}
		if user.DeletedAt.Valid {
			return gorm.ErrRecordNotFound
		}
		availableBefore := user.Quota
		err = tx.Where("request_id = ?", requestID).First(&result).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = PulseWalletReservation{ID: common.GetUUID(), UserID: userID, RequestID: requestID, FundingEpoch: user.PulseFundingEpoch, Status: "reserved", CreatedAt: common.GetTimestamp()}
			if err := tx.Create(&result).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if result.UserID != userID {
			return errors.New("wallet funding subject conflict")
		}
		if result.Status != "reserved" {
			if result.Status == state && result.Quota == int64(target) {
				return nil
			}
			return errors.New("wallet funding already finalized")
		}
		result.AvailableBefore = availableBefore
		if chat != nil {
			if result.Quota > 0 {
				// A repeated admission keeps the original hold, even if another
				// request has since lowered the available wallet balance.
				target = int(result.Quota)
			} else if user.Quota > chat.trust {
				target = chat.initial
			}
		}
		delta := int64(target) - result.Quota
		if delta > 0 && requireAvailable && int64(user.Quota) < delta {
			if chat != nil {
				var held int64
				if err := tx.Model(&PulseWalletReservation{}).Where("user_id = ? AND status = ?", userID, "reserved").Select("COALESCE(SUM(quota), 0)").Scan(&held).Error; err != nil {
					return err
				}
				rejection := &types.QuotaRejection{Source: "wallet", Available: int64(user.Quota), Required: delta, Known: true}
				if held > 0 {
					rejection.Reason = types.QuotaReasonInFlight
				}
				return rejection
			}
			return ErrInsufficientImageQuota
		}
		if delta > 0 && int64(user.Quota) < math.MinInt64+delta {
			return errors.New("wallet quota overflow")
		}
		if delta < 0 && int64(user.Quota) > math.MaxInt64+delta {
			return errors.New("wallet quota overflow")
		}
		oldPaid := user.PulsePaidQuota
		if result.FundingEpoch != user.PulseFundingEpoch {
			// Attribution was invalidated during the upstream request. Do not
			// recover old allowance or certify this mixed request as paid.
			result.PaidQuota = 0
		}
		if delta > 0 {
			paid := min(delta, max(user.PulsePaidQuota, 0))
			user.PulsePaidQuota -= paid
			if result.FundingEpoch == user.PulseFundingEpoch {
				result.PaidQuota += paid
			}
		} else if delta < 0 {
			// Refund the unknown portion first, preserving paid-first final consumption.
			unknownReserved := result.Quota - result.PaidQuota
			paidRefund := min(max(-delta-unknownReserved, 0), result.PaidQuota)
			result.PaidQuota -= paidRefund
			user.PulsePaidQuota += paidRefund
		}
		if err := cashAdjustReservationTx(tx, user, &result, target, state); err != nil {
			return err
		}
		user.Quota -= int(delta)
		if user.PulsePaidQuota > int64(max(user.Quota, 0)) {
			user.PulsePaidQuota = int64(max(user.Quota, 0))
		}
		// A no-op finalization may match zero changed rows on MySQL; its user
		// was already locked and verified above. Actual money/allowance changes
		// must affect exactly one row before a paid proof can be committed.
		if delta != 0 || user.PulsePaidQuota != oldPaid {
			updated := tx.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
				"quota": user.Quota, "pulse_paid_quota": user.PulsePaidQuota,
			})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		result.Quota = int64(target)
		result.Status = state
		result.UpdatedAt = common.GetTimestamp()
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		if err := appendPulseFundingLedgerTx(tx, user, state, result.ID, -delta, user.PulsePaidQuota-oldPaid); err != nil {
			return err
		}
		walletDelta = -delta
		return nil
	})
	if err == nil && walletDelta != 0 {
		if cacheErr := invalidateUserCache(userID); cacheErr != nil {
			common.SysLog("failed to invalidate paid funding cache: " + cacheErr.Error())
		}
	}
	return result, err
}

func (r PulseWalletReservation) FundingSnapshot() PulseFundingSnapshot {
	if r.Status != "settled" || r.Quota <= 0 || r.PaidQuota <= 0 || r.PaidQuota > r.Quota {
		return unknownPulseFunding(int(r.Quota))
	}
	return PulseFundingSnapshot{Version: 1, Status: "verified", PaidQuota: r.PaidQuota, UnknownQuota: r.Quota - r.PaidQuota,
		ProofRef: "wallet:" + r.ID, UserID: r.UserID, RequestID: r.RequestID}
}

func adjustUnattributedWalletQuota(userID, delta int, reason string) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := InvalidatePulsePaidFundingTx(tx, userID, reason); err != nil {
			return err
		}
		res := tx.Model(&User{}).Where("id = ?", userID).Update("quota", gorm.Expr("quota + ?", delta))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err == nil {
		if cacheErr := invalidateUserCache(userID); cacheErr != nil {
			common.SysLog(fmt.Sprintf("failed to invalidate unattributed wallet cache: %v", cacheErr))
		}
	}
	return err
}
