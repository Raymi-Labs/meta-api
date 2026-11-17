package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrEnterpriseOperationConflict = errors.New("该操作编号已用于其他内容")
	ErrEnterpriseOperationInvalid  = errors.New("操作编号无效，请刷新页面后重试")
	ErrEnterpriseOperationPending  = errors.New("操作结果待确认，请使用原操作重试")
	ErrEnterpriseBalanceChanged    = errors.New("余额已变化，请刷新后重新回收")
	ErrEnterpriseAmountOverflow    = errors.New("额度累计值超出范围")
	enterpriseOperationKeyPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
)

// EnterpriseOperation is committed with the wallet and audit. Never expire the
// deduplication key: deleting it would allow a historical request to execute again.
type EnterpriseOperation struct {
	Id             int64  `json:"id" gorm:"primaryKey"`
	OperationID    string `json:"operation_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	EnterpriseID   int    `json:"enterprise_id" gorm:"not null;index"`
	ActorID        int    `json:"actor_id" gorm:"not null;index"`
	ActorMemberID  int    `json:"-" gorm:"not null;default:0"`
	Action         string `json:"action" gorm:"type:varchar(64);not null"`
	TargetID       int64  `json:"target_id" gorm:"not null;default:0"`
	RequestKeyHash string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	PayloadHash    string `json:"-" gorm:"type:char(64);not null"`
	Result         string `json:"-" gorm:"type:text;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"not null"`
}

// Totals are strings on the wire: cumulative int64 amounts can exceed JS's safe
// integer range. They describe this transaction, not the latest balance.
type EnterpriseOperationResult struct {
	OperationID          string            `json:"operation_id"`
	Replayed             bool              `json:"replayed"`
	Action               string            `json:"action"`
	EnterpriseID         int               `json:"enterprise_id"`
	MemberID             int               `json:"member_id,omitempty"`
	Amount               int64             `json:"amount,string"`
	Delta                int64             `json:"delta,string"`
	UnallocatedAfter     int64             `json:"unallocated_after,string"`
	MemberAvailableAfter int64             `json:"member_available_after,string"`
	LedgerID             int64             `json:"ledger_id,omitempty"`
	Ledger               *EnterpriseLedger `json:"-"`
	ReservationID        int64             `json:"reservation_id,omitempty"`
	ReservationState     string            `json:"reservation_state,omitempty"`
	ReservationQuota     int64             `json:"reservation_quota,string"`
}

type enterpriseOperationPayload struct {
	EnterpriseID int    `json:"enterprise_id"`
	TargetID     int64  `json:"target_id"`
	Action       string `json:"action"`
	Platform     bool   `json:"platform"`
	Command      any    `json:"command"`
}

func enterpriseOperationKeyHash(actorID int, key string) (string, error) {
	if actorID <= 0 || !enterpriseOperationKeyPattern.MatchString(key) {
		return "", ErrEnterpriseOperationInvalid
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("enterprise:%d:%s", actorID, key)))
	return hex.EncodeToString(hash[:]), nil
}

// Read current primary-DB authority even when replaying a successful operation.
// Enterprise is already locked, so membership changes serialize here. Account
// status is checked at admission; no User row lock is added to reverse the
// existing account-lifecycle lock order.
func authorizeEnterpriseOperationTx(tx *gorm.DB, actor EnterpriseActor, enterpriseID int) (int, error) {
	var user User
	if err := tx.Select("id", "role", "status").First(&user, actor.UserId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrEnterpriseForbidden
		}
		return 0, err
	}
	if user.Status != common.UserStatusEnabled {
		return 0, ErrEnterpriseForbidden
	}
	if actor.Platform {
		if user.Role < common.RoleRootUser {
			return 0, ErrEnterpriseForbidden
		}
		return 0, nil
	}
	member, err := enterpriseActorMemberTx(tx, enterpriseID, actor)
	if err != nil {
		return 0, err
	}
	if !EnterpriseRoleAtLeast(member.Role, EnterpriseRoleAdmin) {
		return 0, ErrEnterpriseForbidden
	}
	return member.Id, nil
}

func enterpriseReceiptResult(op *EnterpriseOperation, actorMemberID int) (*EnterpriseOperationResult, error) {
	if op.ActorMemberID != actorMemberID {
		return nil, ErrEnterpriseForbidden
	}
	if op.Result == "" {
		return nil, ErrEnterpriseOperationPending
	}
	var result EnterpriseOperationResult
	if err := common.Unmarshal([]byte(op.Result), &result); err != nil {
		return nil, err
	}
	result.Replayed = true
	return &result, nil
}

// GetEnterpriseOperation only returns the current actor's receipt in this scope.
// An absent receipt is not proof that a concurrent POST has not committed yet.
func GetEnterpriseOperation(actor EnterpriseActor, enterpriseID int, key string) (*EnterpriseOperationResult, error) {
	hash, err := enterpriseOperationKeyHash(actor.UserId, key)
	if err != nil {
		return nil, err
	}
	var result *EnterpriseOperationResult
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockEnterpriseTx(tx, enterpriseID); err != nil {
			return err
		}
		memberID, err := authorizeEnterpriseOperationTx(tx, actor, enterpriseID)
		if err != nil {
			return err
		}
		var op EnterpriseOperation
		err = tx.Where("request_key_hash = ? AND enterprise_id = ?", hash, enterpriseID).First(&op).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result, err = enterpriseReceiptResult(&op, memberID)
		return err
	})
	return result, err
}

// Internal legacy entry points have no external retry contract and retain their
// signatures. HTTP mutations require a key. All external writes use this path.
func runEnterpriseOperation(actor EnterpriseActor, enterpriseID int, targetID int64, action, key string, command any,
	apply func(tx *gorm.DB, ent *Enterprise) (*EnterpriseOperationResult, error),
) (*EnterpriseOperationResult, error) {
	var result *EnterpriseOperationResult
	if key == "" {
		err := enterpriseTx(enterpriseID, func(tx *gorm.DB, ent *Enterprise) error {
			var err error
			result, err = apply(tx, ent)
			return err
		})
		return result, err
	}
	keyHash, err := enterpriseOperationKeyHash(actor.UserId, key)
	if err != nil {
		return nil, err
	}
	payload, err := common.Marshal(enterpriseOperationPayload{enterpriseID, targetID, action, actor.Platform, command})
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(hash[:])
	duplicateReceipt := false
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// No snapshot read before the enterprise lock. This avoids an old MySQL
		// RR snapshot after waiting for the first operation to commit.
		ent, err := lockEnterpriseTx(tx, enterpriseID)
		if err != nil {
			return err
		}
		memberID, err := authorizeEnterpriseOperationTx(tx, actor, enterpriseID)
		if err != nil {
			return err
		}
		var op EnterpriseOperation
		err = tx.Where("request_key_hash = ?", keyHash).First(&op).Error
		if err == nil {
			if op.PayloadHash != payloadHash {
				return ErrEnterpriseOperationConflict
			}
			result, err = enterpriseReceiptResult(&op, memberID)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if ent.Status == EnterpriseStatusArchived {
			return ErrEnterpriseArchived
		}
		// Reserve the unique key before any money mutation, in the same transaction.
		op = EnterpriseOperation{OperationID: common.GetUUID(), EnterpriseID: enterpriseID, ActorID: actor.UserId,
			ActorMemberID: memberID, Action: action, TargetID: targetID, RequestKeyHash: keyHash, PayloadHash: payloadHash, CreatedAt: time.Now().Unix()}
		if err := tx.Create(&op).Error; err != nil {
			duplicateReceipt = isEnterpriseDuplicateKeyErr(err)
			return err
		}
		result, err = apply(tx, ent)
		if err != nil {
			return err
		}
		if result == nil {
			return errors.New("enterprise operation returned no result")
		}
		result.OperationID, result.Replayed = op.OperationID, false
		if result.Ledger != nil {
			result.LedgerID = result.Ledger.Id
		}
		data, err := common.Marshal(result)
		if err != nil {
			return err
		}
		return tx.Model(&EnterpriseOperation{}).Where("id = ?", op.Id).Update("result", string(data)).Error
	})
	if err == nil {
		return result, nil
	}
	if !duplicateReceipt {
		return nil, err
	}
	// PostgreSQL aborts a transaction after a unique violation. Re-read only
	// after rollback, with current authorization and in a fresh transaction.
	replayed, readErr := GetEnterpriseOperation(actor, enterpriseID, key)
	if readErr != nil {
		return nil, readErr
	}
	if replayed == nil {
		return nil, ErrEnterpriseOperationConflict
	}
	var original EnterpriseOperation
	if readErr = DB.Where("request_key_hash = ?", keyHash).First(&original).Error; readErr != nil {
		return nil, readErr
	}
	if original.PayloadHash != payloadHash {
		return nil, ErrEnterpriseOperationConflict
	}
	return replayed, nil
}

func enterpriseCheckedAdd(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, ErrEnterpriseAmountOverflow
	}
	return a + b, nil
}
