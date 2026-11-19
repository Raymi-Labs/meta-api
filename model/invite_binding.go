package model

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const inviteBindingRollScale = 10_000

type inviteBindingOutcome string

const (
	inviteBindingOutcomeNoInviter          inviteBindingOutcome = "no_inviter"
	inviteBindingOutcomeInvalidInviter     inviteBindingOutcome = "invalid_inviter"
	inviteBindingOutcomeGuaranteed         inviteBindingOutcome = "guaranteed"
	inviteBindingOutcomeProbabilityBound   inviteBindingOutcome = "probability_bound"
	inviteBindingOutcomeProbabilitySkipped inviteBindingOutcome = "probability_skipped"
	inviteBindingOutcomeRandomErrorSkipped inviteBindingOutcome = "random_error_skipped"
)

type inviteBindingDecision struct {
	RequestedInviterID int
	EffectiveInviterID int
	Threshold          int
	RateAfterThreshold int
	AffCountBefore     int
	InviterRewardQuota int
	InviteeRewardQuota int
	Outcome            inviteBindingOutcome
	RandomError        error
}

var inviteBindingRoll = func() (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(inviteBindingRollScale))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func updateInviterForBinding(tx *gorm.DB, inviterID int, inviterRewardQuota int, extraWhere string, extraArgs ...any) (bool, error) {
	query := tx.Model(&User{}).
		Where("id = ? AND status = ?", inviterID, common.UserStatusEnabled)
	if extraWhere != "" {
		query = query.Where(extraWhere, extraArgs...)
	}
	result := query.Updates(map[string]any{
		"aff_count":   gorm.Expr("aff_count + 1"),
		"aff_quota":   gorm.Expr("aff_quota + ?", inviterRewardQuota),
		"aff_history": gorm.Expr("aff_history + ?", inviterRewardQuota),
	})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func decideInviteBindingWithTx(tx *gorm.DB, inviterID int) (inviteBindingDecision, error) {
	return decideInviteBindingWithRewardsTx(tx, inviterID, common.QuotaForInviter, common.QuotaForInvitee)
}

func decideInviteBindingWithoutRewardsWithTx(tx *gorm.DB, inviterID int) (inviteBindingDecision, error) {
	return decideInviteBindingWithRewardsTx(tx, inviterID, 0, 0)
}

func decideInviteBindingWithRewardsTx(tx *gorm.DB, inviterID int, inviterRewardQuota int, inviteeRewardQuota int) (inviteBindingDecision, error) {
	if common.GetInviteTierSettings().Enabled {
		inviterRewardQuota = 0
	}
	settings := common.GetInviteBindingSettings()
	decision := inviteBindingDecision{
		RequestedInviterID: inviterID,
		Threshold:          settings.Threshold,
		RateAfterThreshold: settings.RateAfterThreshold,
		InviterRewardQuota: inviterRewardQuota,
		InviteeRewardQuota: inviteeRewardQuota,
		Outcome:            inviteBindingOutcomeNoInviter,
	}
	if inviterID <= 0 {
		return decision, nil
	}

	if settings.Threshold == 0 {
		bound, err := updateInviterForBinding(tx, inviterID, decision.InviterRewardQuota, "")
		if err != nil {
			return decision, err
		}
		if !bound {
			decision.Outcome = inviteBindingOutcomeInvalidInviter
			return decision, nil
		}
		decision.EffectiveInviterID = inviterID
		decision.Outcome = inviteBindingOutcomeGuaranteed
		return decision, nil
	}

	// The conditional update atomically assigns the remaining guaranteed slots.
	// Concurrent registrations crossing the threshold cannot all observe the same slot.
	bound, err := updateInviterForBinding(tx, inviterID, decision.InviterRewardQuota, "aff_count < ?", settings.Threshold)
	if err != nil {
		return decision, err
	}
	if bound {
		decision.EffectiveInviterID = inviterID
		decision.Outcome = inviteBindingOutcomeGuaranteed
		return decision, nil
	}

	var inviterState struct {
		AffCount int `gorm:"column:aff_count"`
	}
	result := tx.Model(&User{}).
		Select("aff_count").
		Where("id = ? AND status = ?", inviterID, common.UserStatusEnabled).
		Take(&inviterState)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			decision.Outcome = inviteBindingOutcomeInvalidInviter
			return decision, nil
		}
		return decision, result.Error
	}
	decision.AffCountBefore = inviterState.AffCount

	// A concurrent administrative repair may have lowered aff_count after the
	// conditional update. Retry once so a remaining guaranteed slot is not lost.
	if inviterState.AffCount < settings.Threshold {
		bound, err = updateInviterForBinding(tx, inviterID, decision.InviterRewardQuota, "aff_count < ?", settings.Threshold)
		if err != nil {
			return decision, err
		}
		if bound {
			decision.EffectiveInviterID = inviterID
			decision.Outcome = inviteBindingOutcomeGuaranteed
			return decision, nil
		}
	}

	if settings.RateAfterThreshold <= common.InviteBindingRateMin {
		decision.Outcome = inviteBindingOutcomeProbabilitySkipped
		return decision, nil
	}
	if settings.RateAfterThreshold < common.InviteBindingRateMax {
		roll, rollErr := inviteBindingRoll()
		if rollErr != nil {
			decision.Outcome = inviteBindingOutcomeRandomErrorSkipped
			decision.RandomError = rollErr
			return decision, nil
		}
		if roll < 0 || roll >= inviteBindingRollScale {
			decision.Outcome = inviteBindingOutcomeRandomErrorSkipped
			decision.RandomError = fmt.Errorf("invite binding roll out of range: %d", roll)
			return decision, nil
		}
		if roll >= settings.RateAfterThreshold*(inviteBindingRollScale/100) {
			decision.Outcome = inviteBindingOutcomeProbabilitySkipped
			return decision, nil
		}
	}

	bound, err = updateInviterForBinding(tx, inviterID, decision.InviterRewardQuota, "")
	if err != nil {
		return decision, err
	}
	if !bound {
		decision.Outcome = inviteBindingOutcomeInvalidInviter
		return decision, nil
	}
	decision.EffectiveInviterID = inviterID
	decision.Outcome = inviteBindingOutcomeProbabilityBound
	return decision, nil
}
