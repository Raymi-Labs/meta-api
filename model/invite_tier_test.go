package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTierTest(t *testing.T) {
	setupInviteCommissionTopUpTest(t)
	original := common.GetInviteTierSettings()
	t.Cleanup(func() { require.NoError(t, common.SetInviteTierSettings(original)) })
	settings := original
	settings.Enabled = true
	require.NoError(t, common.SetInviteTierSettings(settings))
	require.NoError(t, DB.AutoMigrate(&PromoterProfile{}, &InviteeQualification{}, &InviteCommissionAllocation{}, &QualifiedBonusOnceGuard{}, &SecondLevelMonthlyUsage{}, &Department{}, &DepartmentMember{}))
}
func TestInviteTierMatrixAndReplay(t *testing.T) {
	for x := 0; x < 6; x++ {
		for y := 0; y < 6; y++ {
			t.Run(fmt.Sprintf("%d_%d", x, y), func(t *testing.T) {
				setupTierTest(t)
				leader := createInviteCommissionTestUser(t, "leader", 0)
				second := createInviteCommissionTestUser(t, "second", 0)
				first := createInviteCommissionTestUser(t, "first", second.Id)
				buyer := createInviteCommissionTestUser(t, "buyer", first.Id)
				require.NoError(t, DB.Create(&PromoterProfile{UserId: first.Id, ManualTier: x}).Error)
				require.NoError(t, DB.Create(&PromoterProfile{UserId: second.Id, ManualTier: y}).Error)
				require.NoError(t, DB.Create(&PromoterProfile{UserId: leader.Id, ManualTier: 5}).Error)
				dept := Department{LeaderUserId: leader.Id}
				require.NoError(t, DB.Create(&dept).Error)
				for _, u := range []int{first.Id, second.Id} {
					require.NoError(t, DB.Create(&DepartmentMember{UserId: u, DepartmentId: dept.Id}).Error)
				}
				run := func(trade string) error {
					return DB.Transaction(func(tx *gorm.DB) error {
						return enqueueTierBasedCommission(tx, trade, buyer.Id, time.Now().Unix(), 100000, decimal.NewFromInt(100))
					})
				}
				require.NoError(t, run("matrix"))
				require.NoError(t, run("matrix"))
				var rows []InviteCommissionLedger
				require.NoError(t, DB.Where("topup_trade_no = ?", "matrix").Find(&rows).Error)
				s := common.GetInviteTierSettings()
				expected := map[int]int64{1: decimal.NewFromInt(100000).Mul(decimal.NewFromFloat(s.FirstLevelRates[x])).IntPart()}
				if x == 3 || x == 5 {
					expected[3] = 1000
				}
				if x == 4 {
					expected[4] = 3500
				}
				if x < 4 {
					expected[2] = decimal.NewFromInt(100000).Mul(decimal.NewFromFloat(s.SecondLevelMatrix[y][x])).IntPart()
					if y == 4 {
						expected[5] = decimal.NewFromInt(100000).Mul(decimal.NewFromFloat(s.LeaderSlotRates[x])).IntPart()
					}
				}
				require.Len(t, rows, len(expected))
				for _, row := range rows {
					require.Equal(t, int64(expected[row.CommissionLevel]), row.CommissionQuota)
					if row.CommissionLevel == 1 || row.CommissionLevel == 3 {
						require.Equal(t, buyer.Id, row.DirectInviteeUserId)
					}
				}
				var usage SecondLevelMonthlyUsage
				if x < 4 {
					require.NoError(t, DB.First(&usage).Error)
					require.Equal(t, int64(10000), usage.UsedBaseCents)
				}
			})
		}
	}
}
func TestInviteTierMonthlyBaseAndBonusGuard(t *testing.T) {
	setupTierTest(t)
	second := createInviteCommissionTestUser(t, "second_cap", 0)
	first := createInviteCommissionTestUser(t, "first_cap", second.Id)
	buyer := createInviteCommissionTestUser(t, "buyer_cap", first.Id)
	require.NoError(t, DB.Create(&PromoterProfile{UserId: first.Id, ManualTier: 3}).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return enqueueTierBasedCommission(tx, fmt.Sprintf("cap%d", i), buyer.Id, time.Now().Unix(), 150000, decimal.NewFromInt(150))
		}))
	}
	var rows []InviteCommissionLedger
	require.NoError(t, DB.Where("commission_level = 2").Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, int64(int64(4500)), rows[0].CommissionQuota)
	require.Equal(t, int64(int64(1500)), rows[1].CommissionQuota)
	var count int64
	require.NoError(t, DB.Model(&InviteCommissionLedger{}).Where("commission_level = 3").Count(&count).Error)
	require.Equal(t, int64(1), count)
}
func TestInviteTierMatrixInvalidIndices(t *testing.T) {
	for _, pair := range [][2]int{{-1, 0}, {6, 0}, {0, -1}, {0, 6}} {
		_, err := getSecondLevelCommissionRate(pair[0], pair[1])
		require.Error(t, err)
	}
}
func TestInviteTierOldIndexMigration(t *testing.T) {
	setupTierTest(t)
	require.NoError(t, DB.Exec("CREATE UNIQUE INDEX idx_invite_commission_trade_inviter ON invite_commission_ledgers (topup_trade_no, inviter_user_id)").Error)
	require.NoError(t, migrateInviteCommissionUniqueIndex(DB))
	require.False(t, DB.Migrator().HasIndex(&InviteCommissionLedger{}, "idx_invite_commission_trade_inviter"))
	require.True(t, DB.Migrator().HasIndex(&InviteCommissionLedger{}, "idx_invite_commission_trade_inviter_level"))
	require.NoError(t, migrateInviteCommissionUniqueIndex(DB))
}

func TestInviteTierReversalReleasesBaseOnce(t *testing.T) {
	setupTierTest(t)
	second := createInviteCommissionTestUser(t, "second_reverse", 0)
	first := createInviteCommissionTestUser(t, "first_reverse", second.Id)
	buyer := createInviteCommissionTestUser(t, "buyer_reverse", first.Id)
	require.NoError(t, DB.Create(&PromoterProfile{UserId: first.Id, ManualTier: 3}).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return enqueueTierBasedCommission(tx, "reverse", buyer.Id, time.Now().Unix(), 200000, decimal.NewFromInt(200))
	}))
	for i := 0; i < 2; i++ {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { _, err := reverseInviteCommissionsByTradeNoTx(tx, "reverse"); return err }))
	}
	var usage SecondLevelMonthlyUsage
	require.NoError(t, DB.First(&usage).Error)
	require.Zero(t, usage.UsedBaseCents)
	var guards int64
	require.NoError(t, DB.Model(&QualifiedBonusOnceGuard{}).Count(&guards).Error)
	require.Zero(t, guards)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return enqueueTierBasedCommission(tx, "after_reverse", buyer.Id, time.Now().Unix(), 200000, decimal.NewFromInt(200))
	}))
	require.NoError(t, DB.First(&usage).Error)
	require.Equal(t, int64(20000), usage.UsedBaseCents)
}

func TestInviteTierOverrideExpiryAndDatabaseFailure(t *testing.T) {
	setupTierTest(t)
	user := createInviteCommissionTestUser(t, "expiry", 0)
	require.NoError(t, DB.Create(&PromoterProfile{UserId: user.Id, AdminOverrideTier: 5, AdminOverrideExpiresAt: time.Now().Unix() - 1}).Error)
	tier, err := getTierForCommission(DB, user.Id, "2026-11")
	require.NoError(t, err)
	require.Zero(t, tier)
	require.NoError(t, DB.Migrator().DropTable(&InviteeQualification{}))
	_, err = getTierForCommission(DB, user.Id, "2026-11")
	require.Error(t, err)
}

func TestInviteTierCurrencyAndOverflow(t *testing.T) {
	setupTierTest(t)
	q, err := calculateTopUpBaseQuota(&TopUp{Status: common.TopUpStatusSuccess, PaidCurrency: "CNY", PaidMoney: 50})
	require.NoError(t, err)
	require.Equal(t, int64(50000), q)
	_, err = calculateTopUpBaseQuota(&TopUp{Status: common.TopUpStatusSuccess, PaidCurrency: "USD", PaidMoney: 50})
	require.Error(t, err)
	_, err = inviteBaseQuota(1e30)
	require.Error(t, err)
}

func TestInviteTierCrossDatabase(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			setupTierTest(t)
			db := capabilitySyncTestDB(t, dialect)
			common.UsingSQLite = false
			common.UsingMySQL = dialect == "mysql"
			common.UsingPostgreSQL = dialect == "postgres"
			initCol()
			require.NoError(t, db.AutoMigrate(&User{}, &InviteCommissionLedger{}, &PromoterProfile{}, &InviteeQualification{}, &InviteCommissionAllocation{}, &QualifiedBonusOnceGuard{}, &SecondLevelMonthlyUsage{}, &Department{}, &DepartmentMember{}))
			require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_invite_commission_trade_inviter ON invite_commission_ledgers (topup_trade_no, inviter_user_id)").Error)
			require.NoError(t, migrateInviteCommissionUniqueIndex(db))
			require.NoError(t, migrateInviteCommissionUniqueIndex(db))
			second := createInviteCommissionTestUser(t, "cross_second", 0)
			first := createInviteCommissionTestUser(t, "cross_first", second.Id)
			buyer := createInviteCommissionTestUser(t, "cross_buyer", first.Id)
			require.NoError(t, db.Create(&PromoterProfile{UserId: first.Id, ManualTier: 3}).Error)
			errors := make(chan error, 8)
			for i := 0; i < 8; i++ {
				go func() {
					errors <- db.Transaction(func(tx *gorm.DB) error {
						return enqueueTierBasedCommission(tx, "concurrent", buyer.Id, time.Now().Unix(), 5000000000, decimal.NewFromInt(100))
					})
				}()
			}
			for i := 0; i < 8; i++ {
				require.NoError(t, <-errors)
			}
			var usage SecondLevelMonthlyUsage
			require.NoError(t, db.First(&usage).Error)
			require.Equal(t, int64(10000), usage.UsedBaseCents)
			var rows []InviteCommissionLedger
			require.NoError(t, db.Find(&rows).Error)
			require.Len(t, rows, 3)
			for _, row := range rows {
				require.Equal(t, int64(5000000000), row.BaseQuota)
			}
			// Different payment sources must contend on the same remaining base.
			for i := 0; i < 8; i++ {
				go func(i int) {
					errors <- db.Transaction(func(tx *gorm.DB) error {
						return enqueueTierBasedCommission(tx, fmt.Sprintf("different_%d", i), buyer.Id, time.Now().Unix(), 100000, decimal.NewFromInt(100))
					})
				}(i)
			}
			for i := 0; i < 8; i++ {
				require.NoError(t, <-errors)
			}
			require.NoError(t, db.First(&usage).Error)
			require.Equal(t, int64(20000), usage.UsedBaseCents)

		})
	}
}

func TestInviteTierOptionPatchPersistsFullSnapshot(t *testing.T) {
	setupTierTest(t)
	s := common.GetInviteTierSettings()
	s.Enabled = false
	require.NoError(t, common.SetInviteTierSettings(s))
	common.OptionMapRWMutex.Lock()
	old := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() { common.OptionMapRWMutex.Lock(); common.OptionMap = old; common.OptionMapRWMutex.Unlock() })
	require.NoError(t, UpdateOption("InviteTierSettings", `{"second_level_monthly_cap_money":300}`))
	require.NoError(t, UpdateOption("InviteTierSettings", `{"qualified_threshold_money":60}`))
	var stored Option
	require.NoError(t, DB.First(&stored, "key = ?", "InviteTierSettings").Error)
	require.NoError(t, common.SetInviteTierSettings(s))
	require.NoError(t, common.UpdateInviteTierSettingsByJSONString(stored.Value))
	require.Equal(t, 300.0, common.GetInviteTierSettings().SecondLevelMonthlyCapMoney)
	require.Equal(t, 60.0, common.GetInviteTierSettings().QualifiedThresholdMoney)
	require.Error(t, UpdateOption("InviteTierSettings", `{"enabled":true}`))
	var after Option
	require.NoError(t, DB.First(&after, "key = ?", "InviteTierSettings").Error)
	require.Equal(t, stored.Value, after.Value)
}
