package common

import (
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"
)

// InviteTierSettings 定义六档位的返佣配置与晋级规则。
type InviteTierSettings struct {
	Enabled                  bool  `json:"enabled"`
	ActivatedAt              int64 `json:"activated_at"`
	RequireIdentityForPayout bool  `json:"require_identity_for_payout"`
	CommissionerRetention    int   `json:"commissioner_retention"`

	// 返利池比例（如 0.16 = 16%）
	PoolRatio float64 `json:"pool_ratio"`

	// 六档一级比例（普通、晋级A、晋级B、宣传专员、认证成员、宣传部长）
	FirstLevelRates [6]float64 `json:"first_level_rates"`

	// 二级矩阵：[上级身份 0-5][下级档位 0-3] → 二级比例
	// 成员（4）有两行：成员位 + 部长位（由所属部长收取）
	// 部长（5）合并取值
	SecondLevelMatrix [6][4]float64 `json:"second_level_matrix"`
	LeaderSlotRates   [4]float64    `json:"leader_slot_rates"`

	// 部门团队分成比例（一级为认证成员时，由所属部长提取）
	DepartmentCommissionRate float64 `json:"department_commission_rate"`

	// 达标奖金比例（单笔 ≥ 达标门槛时触发，宣传专员/部长可得）
	QualifiedBonusRate      float64 `json:"qualified_bonus_rate"`
	QualifiedThresholdMoney float64 `json:"qualified_threshold_money"` // 人民币元

	// 二级月基数上限（人民币元）
	SecondLevelMonthlyCapMoney float64 `json:"second_level_monthly_cap_money"`

	// 晋级人数门槛（普通 → A → B，每月新增邀请成功用户数）
	PromotionThresholds [2]int `json:"promotion_thresholds"` // [A门槛, B门槛]

	// 注册奖励（六档，按站内展示单位，如 0.2 = 0.2 闪）
	RegisterRewards [6]float64 `json:"register_rewards"`

	// 最低提现额（六档，人民币分）
	MinWithdrawalAmounts [6]int `json:"min_withdrawal_amounts"`
}

var inviteTierSettings atomic.Value
var defaultInviteTierSettings InviteTierSettings

func init() {
	// 内部规则默认值（仓库只存配置，不抄原文说明与容量表）
	defaultInviteTierSettings = InviteTierSettings{
		Enabled:               false,
		CommissionerRetention: 30,
		PoolRatio:             0.16,
		FirstLevelRates: [6]float64{
			0.05,  // 0: 普通推广者
			0.07,  // 1: 晋级档 A
			0.10,  // 2: 晋级档 B
			0.11,  // 3: 宣传专员
			0.125, // 4: 认证成员
			0.13,  // 5: 宣传部长
		},
		// 二级矩阵：[上级身份][下级档位 0=普通 1=A 2=B 3=专员]
		SecondLevelMatrix: [6][4]float64{
			{0.03, 0.03, 0.03, 0.03}, // 0: 普通推广者作为二级
			{0.04, 0.04, 0.04, 0.04}, // 1: 晋级档 A
			{0.04, 0.04, 0.04, 0.04}, // 2: 晋级档 B
			{0.07, 0.06, 0.05, 0.04}, // 3: 宣传专员
			{0.07, 0.05, 0.04, 0.02}, // 4: 认证成员（成员位）
			{0.10, 0.08, 0.06, 0.04}, // 5: 部长（成员位 + 部长位合并）
		},
		LeaderSlotRates:            [4]float64{0.03, 0.03, 0.02, 0.02},
		DepartmentCommissionRate:   0.035,
		QualifiedBonusRate:         0.01,
		QualifiedThresholdMoney:    50.0,
		SecondLevelMonthlyCapMoney: 200.0,
		PromotionThresholds:        [2]int{10, 30},
		RegisterRewards: [6]float64{
			0.2, // 普通 = 0.2 闪（站内展示单位）
			1.0, // 晋级档 A
			1.0, // 晋级档 B
			2.0, // 宣传专员
			2.0, // 认证成员
			2.0, // 宣传部长
		},
		MinWithdrawalAmounts: [6]int{
			5000, // 普通 = 50 元（人民币分）
			5000, // 晋级档 A
			5000, // 晋级档 B
			100,  // 宣传专员 = 1 元
			100,  // 认证成员
			100,  // 宣传部长
		},
	}
	inviteTierSettings.Store(defaultInviteTierSettings)
}

func GetInviteTierSettings() InviteTierSettings {
	return inviteTierSettings.Load().(InviteTierSettings)
}

func ValidateInviteTierSettings(s InviteTierSettings) error {
	if s.CommissionerRetention < 0 || s.ActivatedAt < 0 {
		return fmt.Errorf("invalid tier lifecycle settings")
	}
	for _, rate := range s.LeaderSlotRates {
		if rate < 0 || rate > 1 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return fmt.Errorf("invalid leader slot rate")
		}
	}
	// 池比例在 0-1 之间
	if s.PoolRatio < 0 || s.PoolRatio > 1 || math.IsNaN(s.PoolRatio) || math.IsInf(s.PoolRatio, 0) {
		return fmt.Errorf("返利池比例必须在 0 到 1 之间")
	}

	// 一级比例均在 0-1
	for i, rate := range s.FirstLevelRates {
		if rate < 0 || rate > 1 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return fmt.Errorf("档位 %d 一级比例必须在 0 到 1 之间", i)
		}
	}

	// 二级矩阵所有值在 0-1
	for i := 0; i < 6; i++ {
		for j := 0; j < 4; j++ {
			rate := s.SecondLevelMatrix[i][j]
			if rate < 0 || rate > 1 || math.IsNaN(rate) || math.IsInf(rate, 0) {
				return fmt.Errorf("二级矩阵 [%d][%d] 必须在 0 到 1 之间", i, j)
			}
		}
	}

	// 部门分成、达标奖金在 0-1
	if s.DepartmentCommissionRate < 0 || s.DepartmentCommissionRate > 1 ||
		math.IsNaN(s.DepartmentCommissionRate) || math.IsInf(s.DepartmentCommissionRate, 0) {
		return fmt.Errorf("部门团队分成比例必须在 0 到 1 之间")
	}
	if s.QualifiedBonusRate < 0 || s.QualifiedBonusRate > 1 ||
		math.IsNaN(s.QualifiedBonusRate) || math.IsInf(s.QualifiedBonusRate, 0) {
		return fmt.Errorf("达标奖金比例必须在 0 到 1 之间")
	}

	// 门槛与上限非负
	if s.QualifiedThresholdMoney < 0 || math.IsNaN(s.QualifiedThresholdMoney) || math.IsInf(s.QualifiedThresholdMoney, 0) {
		return fmt.Errorf("达标门槛金额不能为负")
	}
	if s.SecondLevelMonthlyCapMoney < 0 || s.SecondLevelMonthlyCapMoney >= float64(math.MaxInt64)/100 || math.IsNaN(s.SecondLevelMonthlyCapMoney) || math.IsInf(s.SecondLevelMonthlyCapMoney, 0) {
		return fmt.Errorf("二级月基数上限不能为负")
	}

	// 晋级门槛非负且 A < B
	if s.PromotionThresholds[0] < 0 || s.PromotionThresholds[1] < 0 {
		return fmt.Errorf("晋级人数门槛不能为负")
	}
	if s.PromotionThresholds[0] >= s.PromotionThresholds[1] {
		return fmt.Errorf("晋级档 A 门槛必须小于 B 门槛")
	}

	// 注册奖励非负
	for i, reward := range s.RegisterRewards {
		if reward < 0 || reward >= float64(math.MaxInt64)/math.Max(1, QuotaPerUnit) || math.IsNaN(reward) || math.IsInf(reward, 0) {
			return fmt.Errorf("档位 %d 注册奖励不能为负", i)
		}
	}

	// 最低提现额非负
	for i, amount := range s.MinWithdrawalAmounts {
		if amount < 0 {
			return fmt.Errorf("档位 %d 最低提现额不能为负", i)
		}
	}

	// 组合校验：每种组合合计不超过池比例
	// 1. 普通/晋级档/宣传专员（有上游）：一级 + 二级 + 奖金预留
	for firstTier := 0; firstTier <= 3; firstTier++ {
		firstRate := s.FirstLevelRates[firstTier]
		bonusReserve := 0.0
		if firstTier == 3 {
			bonusReserve = s.QualifiedBonusRate
		}
		for secondTier := 0; secondTier <= 5; secondTier++ {
			var secondRate float64
			if secondTier <= 3 {
				secondRate = s.SecondLevelMatrix[secondTier][firstTier]
			} else if secondTier == 4 {
				// 认证成员：成员位 + 部长位
				secondRate = s.SecondLevelMatrix[4][firstTier]
				// 部长位来自表格第二行对应列
				secondRate += s.LeaderSlotRates[firstTier]
			} else {
				// 部长合并值
				secondRate = s.SecondLevelMatrix[5][firstTier]
			}
			total := firstRate + secondRate + bonusReserve
			if total > s.PoolRatio+1e-9 {
				return fmt.Errorf("组合超池：一级档位 %d + 二级身份 %d 合计 %.4f 超过池比例 %.4f",
					firstTier, secondTier, total, s.PoolRatio)
			}
		}
	}

	// 2. 认证成员（无上游）：12.5% + 部长团队分成 3.5%
	memberTotal := s.FirstLevelRates[4] + s.DepartmentCommissionRate
	if memberTotal > s.PoolRatio+1e-9 {
		return fmt.Errorf("认证成员 + 部长团队分成合计 %.4f 超过池比例 %.4f", memberTotal, s.PoolRatio)
	}

	// 3. 宣传部长（无上游）：13% + 达标奖金 1%
	leaderTotal := s.FirstLevelRates[5] + s.QualifiedBonusRate
	if leaderTotal > s.PoolRatio+1e-9 {
		return fmt.Errorf("宣传部长 + 达标奖金合计 %.4f 超过池比例 %.4f", leaderTotal, s.PoolRatio)
	}

	return nil
}

func ParseInviteTierSettings(raw string) (InviteTierSettings, error) {
	s := GetInviteTierSettings()
	var fields map[string]json.RawMessage
	if err := UnmarshalJsonStr(raw, &fields); err != nil || fields == nil {
		return s, fmt.Errorf("档位配置必须是 JSON 对象")
	}
	// Reject truncated arrays: encoding/json otherwise silently pads fixed arrays.
	lengths := map[string]int{"first_level_rates": 6, "leader_slot_rates": 4, "promotion_thresholds": 2, "register_rewards": 6, "min_withdrawal_amounts": 6, "second_level_matrix": 6}
	for name, value := range fields {
		if string(value) == "null" {
			return s, fmt.Errorf("配置 %s 不能为 null", name)
		}
		if n, ok := lengths[name]; ok {
			var items []json.RawMessage
			if err := Unmarshal(value, &items); err != nil || len(items) != n {
				return s, fmt.Errorf("配置 %s 长度必须是 %d", name, n)
			}
			if name == "second_level_matrix" {
				for _, row := range items {
					var cols []float64
					if err := Unmarshal(row, &cols); err != nil || len(cols) != 4 {
						return s, fmt.Errorf("二级矩阵必须是 6 行 4 列")
					}
				}
			}
		}
	}
	if err := UnmarshalJsonStr(raw, &s); err != nil {
		return s, err
	}

	if err := ValidateInviteTierSettings(s); err != nil {
		return InviteTierSettings{}, err
	}
	return s, nil
}

func SetInviteTierSettings(s InviteTierSettings) error {
	if err := ValidateInviteTierSettings(s); err != nil {
		return err
	}
	inviteTierSettings.Store(s)
	return nil
}

func UpdateInviteTierSettingsByJSONString(raw string) error {
	s, err := ParseInviteTierSettings(raw)
	if err != nil {
		return err
	}
	return SetInviteTierSettings(s)
}

func InviteTierSettings2JSONString() string {
	raw, err := Marshal(GetInviteTierSettings())
	if err != nil {
		raw, _ = Marshal(defaultInviteTierSettings)
	}
	return string(raw)
}

// Lifecycle controls survive disabling new tier allocations.
func InviteTierLifecycleActive() bool {
	s := GetInviteTierSettings()
	return s.Enabled || s.ActivatedAt > 0
}
