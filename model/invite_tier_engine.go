package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// getTierForCommission 返回推广者在指定月份的生效档位（用于返佣计算）
// 档位判定：max(上月业绩档, 本月实时业绩档)
// 实现：达标当月享受→次月考核→第三月降档
func getTierForCommission(tx *gorm.DB, userId int, month string) (int, error) {
	if tx == nil {
		tx = DB
	}

	// 读取档案中的手动档位和管理员覆盖
	profile := &PromoterProfile{}
	profileErr := tx.Where("user_id = ?", userId).First(profile).Error
	if profileErr == nil {
		if profile.Cleared {
			return 0, nil
		}
		if (profile.OverrideActive || profile.AdminOverrideTier > 0) && (profile.AdminOverrideExpiresAt == 0 || profile.AdminOverrideExpiresAt > time.Now().Unix()) {
			return profile.AdminOverrideTier, nil
		}
		if profile.ManualTier > 0 {
			if profile.ManualTier != 3 || profile.ExemptAssessment || common.GetInviteTierSettings().CommissionerRetention == 0 {
				return profile.ManualTier, nil
			}
			effectiveMonth := profile.ManualTierEffectiveMonth
			if effectiveMonth == "" && profile.CreatedAt > 0 {
				effectiveMonth = InviteBusinessTime(time.Unix(profile.CreatedAt, 0)).Format("2006-01")
			}
			effective, e := time.Parse("2006-01", effectiveMonth)
			current, ce := time.Parse("2006-01", month)
			if e != nil || ce != nil {
				return 0, ErrInviteInvalid
			}
			if current.Before(effective.AddDate(0, 2, 0)) {
				return 3, nil
			}
			var count int64
			if e := tx.Model(&InviteeQualification{}).Where("inviter_id = ? AND qualified_month = ? AND status = 1", userId, getPrevMonth(month)).Count(&count).Error; e != nil {
				return 0, e
			}
			if count >= int64(common.GetInviteTierSettings().CommissionerRetention) {
				return 3, nil
			}
		}
	}

	if profileErr != nil && !errors.Is(profileErr, gorm.ErrRecordNotFound) {
		return 0, profileErr
	}
	// 计算上月和本月的业绩档位
	prevMonth := getPrevMonth(month)
	prevTier, err := calculatePerformanceTier(tx, userId, prevMonth)
	if err != nil {
		return 0, err
	}
	currTier, err := calculatePerformanceTier(tx, userId, month)
	if err != nil {
		return 0, err
	}

	// 取较高档位
	if prevTier > currTier {
		return prevTier, nil
	}
	return currTier, nil
}

// calculatePerformanceTier 计算用户在指定月份新增的合格被邀请人数对应的业绩档位
func calculatePerformanceTier(tx *gorm.DB, userId int, month string) (int, error) {
	settings := common.GetInviteTierSettings()

	var count int64
	err := tx.Model(&InviteeQualification{}).
		Where("inviter_id = ? AND qualified_month = ? AND status = ?", userId, month, 1).
		Count(&count).Error
	if err != nil {
		return 0, err
	}

	// 根据 PromotionThresholds 判定档位
	// [10, 30]：≥30 晋级到 2(B档)，≥10 晋级到 1(A档)，其他 0(普通)
	if int(count) >= settings.PromotionThresholds[1] {
		return 2, nil // B档
	}
	if int(count) >= settings.PromotionThresholds[0] {
		return 1, nil // A档
	}
	return 0, nil // 普通推广者
}

// getPrevMonth 返回上一个月的字符串（格式：2026-11）
func getPrevMonth(month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return month
	}
	prev := t.AddDate(0, -1, 0)
	return prev.Format("2006-01")
}

// getEffectiveSecondLevelTier 返回二级推广者的有效档位身份（用于查询二级矩阵）
// 返回值保留 0–5 六种身份，不将成员或部长折叠为专员。
func getEffectiveSecondLevelTier(tx *gorm.DB, secondLevelUserId int, month string) (int, error) {
	tier, err := getTierForCommission(tx, secondLevelUserId, month)
	if err != nil {
		return 0, err
	}

	return tier, nil
}

// snapshotPromoterTier 在返佣台账创建时记录推广者档位快照
func snapshotPromoterTier(tx *gorm.DB, userId int, month string) (int, error) {
	tier, err := getTierForCommission(tx, userId, month)
	if err != nil {
		return 0, err
	}
	return tier, nil
}

// getCommissionRate 根据档位返回一级返佣比例
func getCommissionRate(tier int) (float64, error) {
	settings := common.GetInviteTierSettings()
	if tier < 0 || tier >= len(settings.FirstLevelRates) {
		return 0, fmt.Errorf("invalid tier: %d", tier)
	}
	return settings.FirstLevelRates[tier], nil
}

// getSecondLevelCommissionRate 根据一级档位和二级档位身份返回二级返佣比例
func getSecondLevelCommissionRate(firstTier int, secondIdentity int) (float64, error) {
	settings := common.GetInviteTierSettings()
	if firstTier < 0 || firstTier > 5 || secondIdentity < 0 || secondIdentity > 5 {
		return 0, fmt.Errorf("invalid commission tiers: %d/%d", firstTier, secondIdentity)
	}
	return settings.SecondLevelMatrix[secondIdentity][min(firstTier, 3)], nil
}
