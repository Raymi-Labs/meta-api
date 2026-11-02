package model

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// BenefitSourcePulseReward is the only source type accepted by the Pulse
	// internal receiver. The source_ref is the immutable Pulse reward grant ID.
	BenefitSourcePulseReward = "pulse_reward"

	PulseBenefitStatusApplied        = "applied"
	PulseBenefitStatusAlreadyApplied = "already_applied"
	PulseBenefitStatusRolledBack     = "rolled_back"
	PulseBenefitStatusNotFound       = "not_found"
)

var (
	ErrPulseBenefitConflict = errors.New("pulse benefit source_ref payload conflict")
	ErrPulseBenefitNotFound = errors.New("pulse benefit not found")
)

// PulseBenefitGrantRequest is the narrow internal contract between Pulse and
// new-api. It intentionally contains no transferable quota: activity rewards
// must not be converted into user-created wallet redemption value.
type PulseBenefitGrantRequest struct {
	GrantID           string `json:"grant_id"`
	UserID            int    `json:"user_id"`
	Amount            int    `json:"amount"`
	TransferableQuota bool   `json:"transferable_quota"`
	SourceRef         string `json:"source_ref"`
	RewardType        string `json:"reward_type"`
	PayloadHash       string `json:"payload_hash,omitempty"`
}

// PulseBenefitReceipt reserves source_ref before quota is changed. The
// dedicated unique key keeps conflict semantics correct even when concurrent
// requests carry different user IDs, while BenefitChangeRecord keeps the real
// target_id required by the benefit audit trail.
type PulseBenefitReceipt struct {
	Id          int    `json:"id"`
	SourceRef   string `json:"source_ref" gorm:"type:varchar(255);not null;uniqueIndex"`
	PayloadHash string `json:"payload_hash" gorm:"type:char(64);not null"`
	UserId      int    `json:"user_id" gorm:"not null;index"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index"`
}

func (r *PulseBenefitReceipt) BeforeCreate(tx *gorm.DB) error {
	r.CreatedAt = common.GetTimestamp()
	return nil
}

type PulseBenefitResult struct {
	SourceRef   string `json:"source_ref"`
	Status      string `json:"status"`
	Applied     bool   `json:"applied"`
	RolledBack  bool   `json:"rolled_back"`
	PayloadHash string `json:"payload_hash,omitempty"`
}

// GrantPulseBenefit applies a Pulse grant and records it in the same database
// transaction as the quota mutation. The record is inserted before the quota
// update, so a duplicate-key race cannot apply the quota twice.
func GrantPulseBenefit(req PulseBenefitGrantRequest) (PulseBenefitResult, error) {
	if err := validatePulseGrant(req); err != nil {
		return PulseBenefitResult{}, err
	}
	fingerprint, err := pulseBenefitFingerprint(req)
	if err != nil {
		return PulseBenefitResult{}, err
	}
	if supplied := strings.TrimSpace(req.PayloadHash); supplied != "" && supplied != fingerprint {
		return PulseBenefitResult{}, ErrPulseBenefitConflict
	}

	result := PulseBenefitResult{SourceRef: req.SourceRef, PayloadHash: fingerprint}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockPulseBenefitReceiverTx(tx); err != nil {
			return err
		}
		receipt, findErr := findPulseReceiptTx(tx, req.SourceRef)
		if findErr != nil {
			return findErr
		}
		if receipt != nil {
			if receipt.PayloadHash != fingerprint || receipt.UserId != req.UserID {
				return ErrPulseBenefitConflict
			}
			grant, findErr := findPulseGrantTx(tx, req.SourceRef)
			if findErr != nil {
				return findErr
			}
			if grant == nil || grant.PayloadHash != fingerprint || grant.UserId != req.UserID {
				return errors.New("pulse benefit receipt has no matching grant")
			}
			result, findErr = pulseBenefitResultTx(tx, grant, PulseBenefitStatusAlreadyApplied)
			if findErr != nil {
				return findErr
			}
			return nil
		}

		// Upgrade compatibility: register a pre-receipt grant without changing
		// quota, then continue with the same idempotency contract.
		existing, findErr := findPulseGrantTx(tx, req.SourceRef)
		if findErr != nil {
			return findErr
		}
		if existing != nil {
			if existing.PayloadHash == "" || existing.PayloadHash != fingerprint || existing.UserId != req.UserID {
				return ErrPulseBenefitConflict
			}
			if err := createPulseReceiptTx(tx, req.SourceRef, fingerprint, req.UserID); err != nil {
				return err
			}
			result, findErr = pulseBenefitResultTx(tx, existing, PulseBenefitStatusAlreadyApplied)
			if findErr != nil {
				return findErr
			}
			return nil
		}
		policy, policyErr := loadPulseBenefitPolicy()
		if policyErr != nil {
			return policyErr
		}
		if req.RewardType != "newapi_quota" {
			return errors.New("invalid pulse benefit reward_type")
		}
		if err := validatePulseBenefitRecipientTx(tx, req.UserID, req.Amount); err != nil {
			return err
		}
		if err := reservePulseBenefitQuotaTx(tx, req, policy); err != nil {
			return err
		}
		if err := createPulseReceiptTx(tx, req.SourceRef, fingerprint, req.UserID); err != nil {
			return err
		}

		record := &BenefitChangeRecord{
			BenefitType: BenefitTypeQuota,
			Action:      BenefitActionGrant,
			SourceType:  BenefitSourcePulseReward,
			SourceRef:   req.SourceRef,
			UserId:      req.UserID,
			TargetType:  BenefitTargetUserQuota,
			TargetId:    req.UserID,
			PayloadHash: fingerprint,
			Detail: marshalBenefitDetail(&QuotaBenefitDetail{
				QuotaDelta: req.Amount,
				Context:    "pulse_reward:" + req.RewardType,
			}),
		}
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		if err := GrantUserQuotaTx(tx, req.UserID, req.Amount, 0, CashGrantSource{Kind: "gift", Ref: "pulse:" + req.SourceRef}); err != nil {
			return err
		}
		result.Status = PulseBenefitStatusApplied
		result.Applied = true
		return nil
	})
	if err == nil {
		if result.Status == PulseBenefitStatusApplied {
			// Invalidate instead of incrementing: a concurrent reader can have
			// loaded the committed balance already, making HINCRBY count twice.
			if cacheErr := invalidateUserCache(req.UserID); cacheErr != nil {
				common.SysError("failed to invalidate Pulse grant recipient cache: " + cacheErr.Error())
			}
		}
		syncPulseBenefitLogsAfterCommit(req.SourceRef)
		return result, nil
	}
	if !errors.Is(err, errPulseBenefitDuplicate) {
		return PulseBenefitResult{}, err
	}
	// A concurrent transaction won the unique source_ref race. Read its
	// committed fingerprint after the failed transaction has rolled back.
	receipt, findErr := findPulseReceiptTx(DB, req.SourceRef)
	if findErr != nil {
		return PulseBenefitResult{}, findErr
	}
	if receipt == nil {
		return PulseBenefitResult{}, err
	}
	if receipt.PayloadHash != fingerprint || receipt.UserId != req.UserID {
		return PulseBenefitResult{}, ErrPulseBenefitConflict
	}
	grant, findErr := findPulseGrantTx(DB, req.SourceRef)
	if findErr != nil {
		return PulseBenefitResult{}, findErr
	}
	if grant == nil || grant.PayloadHash != fingerprint || grant.UserId != req.UserID {
		return PulseBenefitResult{}, errors.New("pulse benefit receipt has no matching grant")
	}
	result, err = pulseBenefitResultTx(DB, grant, PulseBenefitStatusAlreadyApplied)
	if err == nil {
		syncPulseBenefitLogsAfterCommit(req.SourceRef)
	}
	return result, err
}

// QueryPulseBenefit returns the state for a stable source_ref. It is safe to
// call after a timeout and never creates a new source reference.
func QueryPulseBenefit(sourceRef string) (PulseBenefitResult, error) {
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceRef == "" {
		return PulseBenefitResult{}, errors.New("pulse benefit source_ref is required")
	}
	grant, err := findPulseGrantTx(DB, sourceRef)
	if err != nil {
		return PulseBenefitResult{}, err
	}
	if grant == nil {
		return PulseBenefitResult{SourceRef: sourceRef, Status: PulseBenefitStatusNotFound}, ErrPulseBenefitNotFound
	}
	return pulseBenefitResultTx(DB, grant, PulseBenefitStatusApplied)
}

func pulseBenefitResultTx(tx *gorm.DB, grant *BenefitChangeRecord, activeStatus string) (PulseBenefitResult, error) {
	if tx == nil {
		tx = DB
	}
	var rollback BenefitChangeRecord
	if err := tx.Where("origin_record_id = ? AND action = ?", grant.Id, BenefitActionRollback).First(&rollback).Error; err == nil {
		return PulseBenefitResult{
			SourceRef: grant.SourceRef, Status: PulseBenefitStatusRolledBack,
			RolledBack: true, PayloadHash: grant.PayloadHash,
		}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return PulseBenefitResult{}, err
	}
	return PulseBenefitResult{
		SourceRef: grant.SourceRef, Status: activeStatus,
		Applied: true, PayloadHash: grant.PayloadHash,
	}, nil
}

// RollbackPulseBenefit reuses the original source_ref. The generic rollback
// engine appends a reversal record and is idempotent by the grant record ID.
func RollbackPulseBenefit(sourceRef, reason string) (PulseBenefitResult, error) {
	sourceRef = strings.TrimSpace(sourceRef)
	reason = strings.TrimSpace(reason)
	if sourceRef == "" || reason == "" {
		return PulseBenefitResult{}, errors.New("pulse benefit rollback requires source_ref and reason")
	}
	grant, err := findPulseGrantTx(DB, sourceRef)
	if err != nil {
		return PulseBenefitResult{}, err
	}
	if grant == nil {
		return PulseBenefitResult{}, ErrPulseBenefitNotFound
	}
	_, _, err = rollbackBenefitsBySource("pulse_reward", grant.Id, BenefitSourcePulseReward, sourceRef, reason)
	if err != nil {
		return PulseBenefitResult{}, err
	}
	if cacheErr := invalidateUserCache(grant.UserId); cacheErr != nil {
		common.SysError("failed to invalidate Pulse rollback recipient cache: " + cacheErr.Error())
	}
	syncPulseBenefitLogsAfterCommit(sourceRef)
	result, err := QueryPulseBenefit(sourceRef)
	if err != nil {
		return PulseBenefitResult{}, err
	}
	if !result.RolledBack || result.Applied {
		return PulseBenefitResult{}, errors.New("pulse benefit rollback state is inconsistent")
	}
	return result, nil
}

var errPulseBenefitDuplicate = errors.New("pulse benefit duplicate")

func validatePulseGrant(req PulseBenefitGrantRequest) error {
	if strings.TrimSpace(req.GrantID) == "" || req.UserID <= 0 || req.Amount <= 0 || req.TransferableQuota || strings.TrimSpace(req.SourceRef) == "" || strings.TrimSpace(req.RewardType) == "" {
		return errors.New("invalid pulse benefit grant")
	}
	if req.GrantID != req.SourceRef {
		return errors.New("pulse grant_id must equal source_ref")
	}
	return nil
}

// Keep the JSON shape identical to Pulse's settlement outbox payload. The
// server computes this value instead of trusting a caller-supplied hash.
func pulseBenefitFingerprint(req PulseBenefitGrantRequest) (string, error) {
	payload, err := common.Marshal(struct {
		GrantID           string `json:"grant_id"`
		UserID            int    `json:"user_id"`
		Amount            int    `json:"amount"`
		TransferableQuota bool   `json:"transferable_quota"`
		SourceRef         string `json:"source_ref"`
		RewardType        string `json:"reward_type"`
	}{
		GrantID: req.GrantID, UserID: req.UserID, Amount: req.Amount,
		TransferableQuota: false, SourceRef: req.SourceRef, RewardType: req.RewardType,
	})
	if err != nil {
		return "", err
	}
	canonical, err := canonicalPulseBenefitJSON(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return fmt.Sprintf("%x", digest[:]), nil
}

// canonicalPulseBenefitJSON makes the fingerprint independent of JSON object
// key order and insignificant whitespace. UseNumber is required so future
// payload fields cannot lose integer precision through float64 decoding.
func canonicalPulseBenefitJSON(payload []byte) ([]byte, error) {
	var value any
	if err := common.DecodeJsonUseNumberStrict(bytes.NewReader(payload), &value); err != nil {
		return nil, err
	}
	// Keep common.Marshal as the project's JSON boundary; encoding/json sorts
	// map keys deterministically during this marshal.
	return common.Marshal(value)
}

func createPulseReceiptTx(tx *gorm.DB, sourceRef, payloadHash string, userID int) error {
	receipt := &PulseBenefitReceipt{SourceRef: sourceRef, PayloadHash: payloadHash, UserId: userID}
	if err := tx.Create(receipt).Error; err != nil {
		if isBenefitDuplicateKeyErr(err) {
			return fmt.Errorf("%w: %v", errPulseBenefitDuplicate, err)
		}
		return err
	}
	return nil
}

func findPulseReceiptTx(tx *gorm.DB, sourceRef string) (*PulseBenefitReceipt, error) {
	if tx == nil {
		tx = DB
	}
	query := tx.Where("source_ref = ?", sourceRef)
	if tx != DB && !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var receipt PulseBenefitReceipt
	if err := query.First(&receipt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &receipt, nil
}

func findPulseGrantTx(tx *gorm.DB, sourceRef string) (*BenefitChangeRecord, error) {
	if tx == nil {
		tx = DB
	}
	query := tx.Where("source_type = ? AND source_ref = ? AND action = ?", BenefitSourcePulseReward, sourceRef, BenefitActionGrant)
	if tx != DB && !common.UsingSQLite {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var grant BenefitChangeRecord
	if err := query.First(&grant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &grant, nil
}
