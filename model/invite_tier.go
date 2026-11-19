package model

// PromoterProfile 推广者档案表（每用户最多一行）。
//
// 记录人工档位（专员/成员/部长）及管理员覆盖档位。
// 业绩档（普通/A/B）由查询逻辑实时计算，不存储。
type PromoterProfile struct {
	Id     int `json:"id"`
	UserId int `json:"user_id" gorm:"uniqueIndex;not null"`

	// 人工档位：3=宣传专员 4=认证成员 5=宣传部长，0=无人工档位
	ManualTier int `json:"manual_tier" gorm:"not null;default:0;index"`

	// 人工档位生效月份（YYYY-MM），用于专员考核与降档
	ManualTierEffectiveMonth string `json:"manual_tier_effective_month" gorm:"type:varchar(7);default:''"`

	// 所属部门 ID（仅认证成员有效，部长此字段为 0）
	DepartmentId int `json:"department_id" gorm:"not null;default:0;index"`

	// 上游切断标记与时间（签约成员/部长后永久切断原有上游）
	UpstreamCut   bool  `json:"upstream_cut" gorm:"not null;default:false"`
	UpstreamCutAt int64 `json:"upstream_cut_at" gorm:"not null;default:0"`

	// 管理员覆盖档位及截止时间（优先于人工档位与业绩档）
	AdminOverrideTier       int   `json:"admin_override_tier" gorm:"not null;default:0"`
	AdminOverrideExpiresAt  int64 `json:"admin_override_expires_at" gorm:"not null;default:0"`
	AdminOverrideAssignedBy int   `json:"admin_override_assigned_by" gorm:"not null;default:0"`

	// 提现冻结标记
	WithdrawalFrozen   bool  `json:"withdrawal_frozen" gorm:"not null;default:false"`
	WithdrawalFrozenAt int64 `json:"withdrawal_frozen_at" gorm:"not null;default:0"`

	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

// InviteeQualification 被邀请人资格表（每个被邀请人一行）。
//
// 记录首次成为"有效用户"的时间与来源支付。
// 驱动晋级人数统计、注册奖励与成员 30 日无新增检测。
type InviteeQualification struct {
	Id         int `json:"id"`
	InviteeId  int `json:"invitee_id" gorm:"uniqueIndex;not null"` // 被邀请人 user_id
	InviterId  int `json:"inviter_id" gorm:"index;not null"`       // 直接上级 user_id
	SourceType int `json:"source_type" gorm:"not null;default:1"`  // 1=充值 2=其他

	// 来源支付订单（撤销时失效，可重新获得资格）
	SourceTopupTradeNo string `json:"source_topup_trade_no" gorm:"type:varchar(255);default:''"`

	// 首次有效时间与月份（用于晋级人数统计与注册奖励）
	QualifiedAt    int64  `json:"qualified_at" gorm:"not null;index"`
	QualifiedMonth string `json:"qualified_month" gorm:"type:varchar(7);not null;index"` // YYYY-MM

	// 资格状态（1=有效 0=已失效，支付撤销时失效）
	Status int `json:"status" gorm:"not null;default:1;index"`

	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (InviteeQualification) TableName() string {
	return "invitee_qualifications"
}

// Department 宣传部门表。
type Department struct {
	Id           int    `json:"id"`
	LeaderUserId int    `json:"leader_user_id" gorm:"not null;index"` // 部长 user_id
	Name         string `json:"name" gorm:"type:varchar(64);default:''"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// DepartmentMember 部门成员表。
type DepartmentMember struct {
	Id           int   `json:"id"`
	DepartmentId int   `json:"department_id" gorm:"not null;index"`
	UserId       int   `json:"user_id" gorm:"not null;uniqueIndex:idx_department_member_user"` // 成员 user_id
	JoinedAt     int64 `json:"joined_at" gorm:"not null"`
	LeftAt       int64 `json:"left_at" gorm:"not null;default:0"` // 0=在职，非 0=已退出
	CreatedAt    int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

// InviteCommissionStatement 返佣周期账单表。
//
// 记录周/月结算的汇总账单，台账在 InviteCommissionLedger。
type InviteCommissionStatement struct {
	Id            int `json:"id"`
	InviterUserId int `json:"inviter_user_id" gorm:"not null;index;uniqueIndex:idx_statement_inviter_period"`

	// 周期类型：1=周结 2=月结
	PeriodType int `json:"period_type" gorm:"not null;index;uniqueIndex:idx_statement_inviter_period"`

	// 周期键：周结为 "YYYY-WW"（ISO 周），月结为 "YYYY-MM"
	PeriodKey string `json:"period_key" gorm:"type:varchar(10);not null;index;uniqueIndex:idx_statement_inviter_period"`

	// 出账与发放时间
	IssuedAt   int64 `json:"issued_at" gorm:"not null;default:0"`
	ReleasedAt int64 `json:"released_at" gorm:"not null;default:0"`

	// 应发/实发/扣减/结转欠款金额（bigint，人民币分或内部额度，待确认单位）
	DueAmount       int64 `json:"due_amount" gorm:"type:bigint;not null;default:0"`
	PaidAmount      int64 `json:"paid_amount" gorm:"type:bigint;not null;default:0"`
	DeductedAmount  int64 `json:"deducted_amount" gorm:"type:bigint;not null;default:0"`
	CarryOverAmount int64 `json:"carry_over_amount" gorm:"type:bigint;not null;default:0"`

	// 状态：1=open 2=issued 3=paid 4=frozen
	Status int `json:"status" gorm:"not null;default:1;index"`

	// 异议截止时间与备注
	DisputeDeadline int64  `json:"dispute_deadline" gorm:"not null;default:0"`
	Remarks         string `json:"remarks" gorm:"type:text"`

	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

// InviteCommissionDebt 推广欠款表。
//
// 记录已结算台账被撤销时余额不足的差额，后续结算时优先抵扣。
type InviteCommissionDebt struct {
	Id            int   `json:"id"`
	InviterUserId int   `json:"inviter_user_id" gorm:"not null;uniqueIndex:idx_debt_inviter"`
	DebtAmount    int64 `json:"debt_amount" gorm:"type:bigint;not null;default:0"` // 欠款额（正数）
	PaidAmount    int64 `json:"paid_amount" gorm:"type:bigint;not null;default:0"` // 已抵扣额

	// 欠款来源（1=支付撤销 2=提前发放后违规 3=其他）
	SourceType int    `json:"source_type" gorm:"not null;default:1"`
	SourceRef  string `json:"source_ref" gorm:"type:varchar(255);default:''"` // 来源订单号或台账 ID

	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

// SecondLevelMonthlyUsage 二级月基数占用表。
//
// 记录 (inviter_user_id, invitee_user_id, month) 的已用基数，
// 用于限制"每个二级用户每月前 200 元"。
type SecondLevelMonthlyUsage struct {
	UsedBaseCents   int64   `json:"used_base_cents" gorm:"type:bigint;not null;default:0"`
	Id              int     `json:"id"`
	InviterUserId   int     `json:"inviter_user_id" gorm:"not null;uniqueIndex:idx_second_level_monthly"`       // 二级收款人
	InviteeUserId   int     `json:"invitee_user_id" gorm:"not null;uniqueIndex:idx_second_level_monthly"`       // 二级用户（一级的下级）
	Month           string  `json:"month" gorm:"type:varchar(7);not null;uniqueIndex:idx_second_level_monthly"` // YYYY-MM
	UsedMoneyAmount float64 `json:"used_money_amount" gorm:"type:decimal(10,2);not null;default:0"`             // 已用基数（人民币元）
	UsedQuotaAmount int64   `json:"used_quota_amount" gorm:"type:bigint;not null;default:0"`                    // 已用基数（内部额度）
	CreatedAt       int64   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64   `json:"updated_at" gorm:"autoUpdateTime"`
}

// QualifiedBonusOnceGuard 达标奖金唯一触发保护表。
//
// 用可空唯一键保证每个被邀请人只触发一次达标奖金。
// 撤销时释放唯一键，允许下一笔达标充值重新触发。
type QualifiedBonusOnceGuard struct {
	Id                         int    `json:"id"`
	InviteeUserId              int    `json:"invitee_user_id" gorm:"uniqueIndex;not null"`                      // 被邀请人 user_id
	FirstQualifiedTopupTradeNo string `json:"first_qualified_topup_trade_no" gorm:"type:varchar(255);not null"` // 首次触发的订单号
	InviterUserId              int    `json:"inviter_user_id" gorm:"not null;index"`                            // 收款的上级 user_id
	LedgerId                   int    `json:"ledger_id" gorm:"not null;index"`                                  // 对应台账 ID
	CreatedAt                  int64  `json:"created_at" gorm:"autoCreateTime"`
}
