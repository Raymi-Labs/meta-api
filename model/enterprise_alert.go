package model

import (
	"time"
)

// 低额提醒只发一次：靠条件更新抢占资格，多节点、并发请求同时触发也只有一个成功。
// 成员获得新额度、企业入账或改提醒线时资格复位（见划拨、入账与设置更新）。

// ClaimEnterpriseMemberLowBalance 成员可用额度低于 threshold 且尚未提醒过时返回 true。
func ClaimEnterpriseMemberLowBalance(memberID int, threshold int64) (bool, error) {
	if memberID <= 0 || threshold <= 0 {
		return false, nil
	}
	res := DB.Model(&EnterpriseMember{}).
		Where("id = ? AND status = ? AND low_balance_notified_at = 0 AND available < ?", memberID, EnterpriseMemberStatusActive, threshold).
		UpdateColumn("low_balance_notified_at", time.Now().Unix())
	return res.RowsAffected == 1, res.Error
}

// ClaimEnterpriseLowBalances 返回本轮需要提醒的企业：未分配余额低于企业自己设的提醒线、
// 尚未提醒、且未冻结。余额回到提醒线以上（回收、调账）或提醒线被关掉的企业先复位，
// 下次再跌破时重新提醒。
func ClaimEnterpriseLowBalances(limit int) ([]Enterprise, error) {
	if err := DB.Model(&Enterprise{}).
		Where("low_balance_notified_at > 0 AND (low_balance_threshold <= 0 OR unallocated >= low_balance_threshold)").
		UpdateColumn("low_balance_notified_at", 0).Error; err != nil {
		return nil, err
	}
	const due = "status = ? AND low_balance_threshold > 0 AND low_balance_notified_at = 0 AND unallocated < low_balance_threshold"
	var candidates []Enterprise
	if err := DB.Where(due, EnterpriseStatusActive).Order("id ASC").Limit(limit).Find(&candidates).Error; err != nil {
		return nil, err
	}
	claimed := make([]Enterprise, 0, len(candidates))
	for _, ent := range candidates {
		res := DB.Model(&Enterprise{}).Where("id = ? AND "+due, ent.Id, EnterpriseStatusActive).
			UpdateColumn("low_balance_notified_at", time.Now().Unix())
		if res.Error != nil {
			return claimed, res.Error
		}
		if res.RowsAffected == 1 {
			claimed = append(claimed, ent)
		}
	}
	return claimed, nil
}
