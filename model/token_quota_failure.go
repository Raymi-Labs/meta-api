package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"strconv"
)

const TokenQuotaFailureThreshold = 3

// RecordTokenQuotaOutcome serializes final outcomes on the token row. A manual
// token edit starts a new generation; old in-flight outcomes are then ignored.
func RecordTokenQuotaOutcome(id int, version int64, insufficient bool) error {
	if id <= 0 {
		return nil
	}
	var disabled bool
	var token Token
	err := DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&Token{}).Where("id = ? AND quota_failure_version = ? AND status IN ?", id, version, []int{common.TokenStatusEnabled, common.TokenStatusExhausted})
		if !insufficient {
			return query.Where("quota_failure_count > 0").UpdateColumn("quota_failure_count", 0).Error
		}
		result := query.Where("quota_failure_count < ?", TokenQuotaFailureThreshold).UpdateColumn("quota_failure_count", gorm.Expr("quota_failure_count + 1"))
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		if err := tx.First(&token, id).Error; err != nil {
			return err
		}
		if token.QuotaFailureCount < TokenQuotaFailureThreshold {
			return nil
		}
		if err := tx.Model(&token).UpdateColumn("status", common.TokenStatusDisabled).Error; err != nil {
			return err
		}
		disabled = true
		return nil
	})
	if err != nil {
		return err
	}
	if disabled {
		common.SysLog("token automatically disabled after consecutive local quota failures, token_id=" + strconv.Itoa(id))
		if common.RedisEnabled {
			return cacheDeleteToken(token.Key)
		}
	}
	return nil
}
