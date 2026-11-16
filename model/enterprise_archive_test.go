package model

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func enterpriseArchiveDB(t *testing.T, dialect string) {
	t.Helper()
	enterpriseTestDB(t, dialect)
	require.NoError(t, DB.AutoMigrate(&Task{}, &Midjourney{}))
	var existingIDs []int
	require.NoError(t, DB.Model(&Enterprise{}).Pluck("id", &existingIDs).Error)
	t.Cleanup(func() {
		var ids []int
		query := DB.Model(&Enterprise{})
		if len(existingIDs) > 0 {
			query = query.Where("id NOT IN ?", existingIDs)
		}
		require.NoError(t, query.Pluck("id", &ids).Error)
		if len(ids) == 0 {
			return
		}
		for _, cleanup := range []struct {
			row    any
			column string
		}{
			{&Task{}, "enterprise_id"}, {&Midjourney{}, "enterprise_id"}, {&EnterpriseReservation{}, "enterprise_id"},
			{&EnterpriseTokenBudget{}, "enterprise_id"}, {&EnterpriseQuotaRequest{}, "enterprise_id"}, {&EnterpriseLedger{}, "enterprise_id"},
			{&EnterpriseAuditEvent{}, "enterprise_id"}, {&EnterpriseInvitation{}, "enterprise_id"}, {&Token{}, "enterprise_id"},
			{&EnterpriseMember{}, "enterprise_id"}, {&Enterprise{}, "id"},
		} {
			query := DB.Where(cleanup.column+" IN ?", ids)
			if _, ok := cleanup.row.(*Token); ok {
				query = DB.Unscoped().Where(cleanup.column+" IN ?", ids)
			}
			require.NoError(t, query.Delete(cleanup.row).Error, fmt.Sprintf("cleanup %T", cleanup.row))
		}
	})
}

func TestEnterpriseArchiveCloseout(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseArchiveDB(t, dialect)
			ent, ownerMember, owner := enterpriseTestOrg(t)
			other, _, _ := enterpriseTestOrg(t)
			former, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			require.NoError(t, DisableEnterpriseMember(EnterpriseActor{UserId: owner.Id}, ent.Id, former.Id, "离职"))
			_, inv, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: owner.Id}, ent.Id, "archive@example.invalid", EnterpriseRoleMember, 0)
			require.NoError(t, err)
			token := &Token{UserId: owner.Id, EnterpriseId: ent.Id, MembershipId: ownerMember.Id, Key: common.GetUUID(), Name: "历史令牌", Status: common.TokenStatusEnabled}
			require.NoError(t, CreateEnterpriseToken(token, 1000))
			personal := &Token{UserId: owner.Id, Key: common.GetUUID(), Status: common.TokenStatusEnabled}
			require.NoError(t, personal.Insert())
			_, err = CreditEnterprise(1, ent.Id, 1000, common.GetUUID(), "测试入账")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: owner.Id}, ent.Id, ownerMember.Id, 800, ""))
			sub := enterpriseTestSubject(ownerMember, token.Id)
			_, err = AdjustEnterpriseReservation(sub, "archive-"+common.GetUUID(), 299, EnterpriseReservationSettled, false)
			require.NoError(t, err)
			_, err = AdjustEnterprise(owner.Id, ent.Id, -1000, "撤回测试入账")
			require.NoError(t, err)
			require.NoError(t, SetEnterpriseStatus(owner.Id, ent.Id, EnterpriseStatusFrozen, "结束测试"))
			preview, err := PreviewEnterpriseArchive(ent.Id)
			require.NoError(t, err)
			require.EqualValues(t, -299, preview.Remaining)
			require.True(t, preview.Balance.Balanced)
			require.Contains(t, preview.Blockers, "remaining_balance")
			_, err = ArchiveEnterprise(owner.Id, ent.Id, ent.Name, "归档")
			require.ErrorIs(t, err, ErrEnterpriseArchiveBlocked)
			require.ErrorIs(t, owner.Delete(), ErrEnterpriseOwnerRequired)
			_, err = AdjustEnterprise(owner.Id, ent.Id, 299, "测试消费核销，保留真实消费")
			require.NoError(t, err)
			preview, err = PreviewEnterpriseArchive(ent.Id)
			require.NoError(t, err)
			require.True(t, preview.CanArchive)
			require.EqualValues(t, 501, preview.Balance.MemberAvailable)
			archived, err := ArchiveEnterprise(owner.Id, ent.Id, ent.Name, "验收结束")
			require.NoError(t, err)
			require.Equal(t, EnterpriseStatusArchived, archived.Status)
			require.Positive(t, archived.ArchivedAt)
			balance := enterpriseTestBalanced(t, ent.Id)
			require.Zero(t, balance.Unallocated)
			require.Zero(t, balance.MemberAvailable)
			require.EqualValues(t, 299, balance.Consumed)
			require.EqualValues(t, 299, balance.NetCredited)
			current, err := GetActiveEnterpriseMemberByUser(owner.Id)
			require.NoError(t, err)
			require.Nil(t, current)
			require.NoError(t, DB.First(&former, former.Id).Error)
			require.Equal(t, EnterpriseMemberStatusDisabled, former.Status)
			require.Positive(t, former.ArchivedAt)
			require.NoError(t, DB.First(&inv, inv.Id).Error)
			require.Equal(t, EnterpriseInvitationRevoked, inv.Status)
			var stored Token
			require.NoError(t, DB.First(&stored, token.Id).Error)
			require.Equal(t, common.TokenStatusDisabled, stored.Status)
			stored = Token{}
			require.NoError(t, DB.First(&stored, personal.Id).Error)
			require.Equal(t, common.TokenStatusEnabled, stored.Status)
			require.ErrorIs(t, token.Update(), ErrEnterpriseArchived, "stale enabled token cannot overwrite archive")
			_, err = ApplyEnterpriseCharge(enterpriseTestSubject(former, 0), common.GetUUID(), EnterpriseChargeTaskAdjust, -1)
			require.ErrorIs(t, err, ErrEnterpriseArchived, "former member callbacks also sealed")
			_, err = AdjustEnterpriseReservation(sub, common.GetUUID(), 0, EnterpriseReservationReserved, false)
			require.ErrorIs(t, err, ErrEnterpriseArchived)
			require.ErrorIs(t, SetEnterpriseStatus(1, ent.Id, EnterpriseStatusActive, ""), ErrEnterpriseArchived)
			require.ErrorIs(t, SetEnterpriseStatus(1, ent.Id, EnterpriseStatusFrozen, ""), ErrEnterpriseArchived)
			require.ErrorIs(t, UpdateEnterpriseByPlatform(1, ent.Id, "新名称", "default"), ErrEnterpriseArchived)
			_, err = CreditEnterprise(1, ent.Id, 1, common.GetUUID(), "")
			require.ErrorIs(t, err, ErrEnterpriseArchived)
			_, err = AdjustEnterprise(1, ent.Id, 1, "修改")
			require.ErrorIs(t, err, ErrEnterpriseArchived)
			_, err = ArchiveEnterprise(1, ent.Id, ent.Name, "重复提交")
			require.NoError(t, err)
			var audits int64
			require.NoError(t, DB.Model(&EnterpriseAuditEvent{}).Where("enterprise_id = ? AND action = ?", ent.Id, "enterprise.archive").Count(&audits).Error)
			require.EqualValues(t, 1, audits)
			_, count, err := ListEnterprises(ent.Name, 0, 10)
			require.NoError(t, err)
			require.Zero(t, count)
			items, count, err := ListEnterprisesByStatus(ent.Name, EnterpriseStatusArchived, 0, 1)
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			require.Equal(t, ent.Id, items[0].Id)
			_, count, err = ListEnterpriseUsage(EnterpriseUsageFilter{EnterpriseId: ent.Id, Status: "all"}, 0, 20)
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			var actualOwner User
			require.NoError(t, DB.First(&actualOwner, owner.Id).Error)
			require.Equal(t, owner.Quota, actualOwner.Quota)
			require.EqualValues(t, 0, enterpriseTestBalanced(t, other.Id).NetCredited)
			// Released user can join another enterprise; its old ownership ID remains history.
			next, err := CreateEnterprise(1, "新企业", "default", "", owner.Id)
			require.NoError(t, err)
			require.ErrorIs(t, owner.Delete(), ErrEnterpriseOwnerRequired)
			require.NoError(t, SetEnterpriseStatus(1, next.Id, EnterpriseStatusFrozen, ""))
			_, err = ArchiveEnterprise(1, next.Id, next.Name, "结束")
			require.NoError(t, err)
			require.NoError(t, owner.Delete())
			history, err := GetEnterpriseByID(ent.Id)
			require.NoError(t, err)
			require.Equal(t, owner.Id, history.OwnerUserId)
		})
	}
}

func TestEnterpriseArchivePreconditions(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseArchiveDB(t, dialect)
			cases := []struct {
				name, blocker string
				mutate        func(*Enterprise, *EnterpriseMember)
			}{
				{"active", "not_frozen", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, SetEnterpriseStatus(1, e.Id, EnterpriseStatusActive, ""))
				}},
				{"positive_unit", "remaining_balance", func(e *Enterprise, m *EnterpriseMember) {
					_, err := AdjustEnterprise(1, e.Id, 1, "fixture")
					require.NoError(t, err)
				}},
				{"negative_unit", "remaining_balance", func(e *Enterprise, m *EnterpriseMember) {
					_, err := AdjustEnterprise(1, e.Id, -1, "fixture")
					require.NoError(t, err)
				}},
				{"unbalanced", "unbalanced", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, DB.Model(e).Update("net_credited", 1).Error)
				}},
				{"zero_reservation", "open_reservations", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, DB.Create(&EnterpriseReservation{EnterpriseId: e.Id, MemberId: m.Id, RequestId: common.GetUUID(), Status: EnterpriseReservationReserved}).Error)
				}},
				{"zero_review", "open_reservations", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, DB.Create(&EnterpriseReservation{EnterpriseId: e.Id, MemberId: m.Id, RequestId: common.GetUUID(), Status: EnterpriseReservationReview}).Error)
				}},
				{"orphan_reserved", "open_reservations", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, DB.Model(m).Updates(map[string]any{"reserved": 1, "available": -1}).Error)
				}},
				{"video_running", "pending_tasks", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, DB.Create(&Task{EnterpriseId: e.Id, Status: TaskStatusInProgress, EnterpriseInitialRecorded: true}).Error)
				}},
				{"video_pending_bill", "pending_tasks", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, DB.Create(&Task{EnterpriseId: e.Id, Status: TaskStatusFailure, EnterpriseBillingPending: true, EnterpriseInitialRecorded: true}).Error)
				}},
				{"video_missing_ack", "pending_tasks", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, DB.Create(&Task{EnterpriseId: e.Id, Status: TaskStatusSuccess}).Error)
				}},
				{"mj_unknown", "pending_midjourney", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, DB.Create(&Midjourney{EnterpriseId: e.Id, EnterpriseBillingState: MJEnterpriseUnknown}).Error)
				}},
				{"mj_pending_bill", "pending_midjourney", func(e *Enterprise, m *EnterpriseMember) {
					require.NoError(t, DB.Create(&Midjourney{EnterpriseId: e.Id, Status: "SUCCESS", EnterpriseBillingState: MJEnterpriseAccepted, EnterpriseBillingSettled: true}).Error)
				}},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					ent, member, _ := enterpriseTestOrg(t)
					require.NoError(t, SetEnterpriseStatus(1, ent.Id, EnterpriseStatusFrozen, ""))
					tc.mutate(ent, member)
					p, err := PreviewEnterpriseArchive(ent.Id)
					require.NoError(t, err)
					require.False(t, p.CanArchive)
					require.Contains(t, p.Blockers, tc.blocker)
					_, err = ArchiveEnterprise(1, ent.Id, ent.Name, "fixture")
					require.ErrorIs(t, err, ErrEnterpriseArchiveBlocked)
					fresh, err := GetActiveEnterpriseMemberByUser(member.UserId)
					require.NoError(t, err)
					require.NotNil(t, fresh)
				})
			}
		})
	}
}

func TestEnterpriseArchiveConfirmationRollbackAndGroupRelease(t *testing.T) {
	enterpriseArchiveDB(t, "sqlite")
	ent, member, owner := enterpriseTestOrg(t)
	require.NoError(t, SetEnterpriseStatus(1, ent.Id, EnterpriseStatusFrozen, ""))
	for _, input := range []struct {
		name, reason string
		want         error
	}{{ent.Name + " ", "reason", ErrEnterpriseArchiveName}, {ent.Name, "  ", ErrEnterpriseInvalidInput}} {
		_, err := ArchiveEnterprise(1, ent.Id, input.name, input.reason)
		require.ErrorIs(t, err, input.want)
	}
	callback := "archive_audit_failure"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if ev, ok := tx.Statement.Dest.(*EnterpriseAuditEvent); ok && ev.Action == "enterprise.archive" {
			tx.AddError(errors.New("injected audit failure"))
		}
	}))
	_, err := ArchiveEnterprise(1, ent.Id, ent.Name, "fixture")
	require.ErrorContains(t, err, "injected audit failure")
	require.NoError(t, DB.Callback().Create().Remove(callback))
	fresh, err := GetEnterpriseByID(ent.Id)
	require.NoError(t, err)
	require.Equal(t, EnterpriseStatusFrozen, fresh.Status)
	require.NoError(t, DB.First(member, member.Id).Error)
	require.Zero(t, member.ArchivedAt)
	require.NotNil(t, member.ActiveUserId)
	require.Error(t, DB.Transaction(func(tx *gorm.DB) error { return validateEnterpriseGroupReferencesTx(tx, "{}") }))
	_, err = ArchiveEnterprise(1, ent.Id, ent.Name, "fixture")
	require.NoError(t, err)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return validateEnterpriseGroupReferencesTx(tx, "{}") }))
	lateTask := &Task{EnterpriseId: ent.Id, EnterpriseMemberId: member.Id, UserId: owner.Id}
	require.ErrorIs(t, lateTask.Insert(), ErrEnterpriseArchived)
	mj := &Midjourney{EnterpriseId: ent.Id, EnterpriseMemberId: member.Id, UserId: owner.Id, BillingSource: EnterpriseQuotaSource, BillingRequestId: common.GetUUID(), BillingTokenId: 1}
	require.ErrorIs(t, CreateEnterpriseMJIntent(mj), ErrEnterpriseArchived)
	lateToken := &Token{EnterpriseId: ent.Id, MembershipId: member.Id, UserId: owner.Id, Key: common.GetUUID()}
	require.ErrorIs(t, CreateEnterpriseToken(lateToken, 0), ErrEnterpriseArchived)
}

func TestEnterpriseArchiveRaces(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseArchiveDB(t, dialect)
			for round := 0; round < 8; round++ {
				ent, member, owner := enterpriseTestOrg(t)
				require.NoError(t, SetEnterpriseStatus(1, ent.Id, EnterpriseStatusFrozen, ""))
				start := make(chan struct{})
				var wg sync.WaitGroup
				errs := make([]error, 4)
				operations := []func() error{
					func() error { _, err := ArchiveEnterprise(1, ent.Id, ent.Name, "race"); return err },
					func() error {
						_, err := AdjustEnterpriseReservation(enterpriseTestSubject(member, 0), common.GetUUID(), 0, EnterpriseReservationReserved, false)
						return err
					},
					func() error {
						return (&Task{EnterpriseId: ent.Id, EnterpriseMemberId: member.Id, UserId: owner.Id, Status: TaskStatusInProgress}).Insert()
					},
					func() error { _, err := ArchiveEnterprise(1, ent.Id, ent.Name, "duplicate"); return err },
				}
				for i, operation := range operations {
					wg.Add(1)
					go func(i int, operation func() error) { defer wg.Done(); <-start; errs[i] = operation() }(i, operation)
				}
				close(start)
				wg.Wait()
				for _, err := range errs {
					require.True(t, err == nil || errors.Is(err, ErrEnterpriseArchived) || errors.Is(err, ErrEnterpriseArchiveBlocked), fmt.Sprint(err))
				}
				fresh, err := GetEnterpriseByID(ent.Id)
				require.NoError(t, err)
				if fresh.Status == EnterpriseStatusArchived {
					require.ErrorIs(t, errs[1], ErrEnterpriseArchived)
					require.ErrorIs(t, errs[2], ErrEnterpriseArchived)
				} else {
					require.ErrorIs(t, errs[0], ErrEnterpriseArchiveBlocked)
					require.ErrorIs(t, errs[3], ErrEnterpriseArchiveBlocked)
				}
			}
		})
	}
}

func TestEnterpriseArchiveCompletedAsyncTasks(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseArchiveDB(t, dialect)
			ent, member, owner := enterpriseTestOrg(t)
			token := &Token{UserId: owner.Id, EnterpriseId: ent.Id, MembershipId: member.Id, Key: common.GetUUID()}
			require.NoError(t, CreateEnterpriseToken(token, 0))
			sub := enterpriseTestSubject(member, token.Id)
			requestID := common.GetUUID()
			_, err := AdjustEnterpriseReservation(sub, requestID, 0, EnterpriseReservationReserved, false)
			require.NoError(t, err)
			task := &Task{UserId: owner.Id, EnterpriseId: ent.Id, EnterpriseMemberId: member.Id, Status: TaskStatusSuccess,
				EnterpriseBillingPending: true, PrivateData: TaskPrivateData{RequestId: requestID, BillingSource: EnterpriseQuotaSource, TokenId: token.Id}}
			require.NoError(t, task.Insert())
			mj := &Midjourney{UserId: owner.Id, EnterpriseId: ent.Id, EnterpriseMemberId: member.Id, BillingSource: EnterpriseQuotaSource,
				BillingTokenId: token.Id, BillingRequestId: common.GetUUID()}
			require.NoError(t, CreateEnterpriseMJIntent(mj))
			_, err = AdjustEnterpriseReservation(sub, mj.BillingRequestId, 0, EnterpriseReservationReserved, false)
			require.NoError(t, err)
			mj.MjId, mj.Status = common.GetUUID(), "SUCCESS"
			require.NoError(t, SaveEnterpriseMJSubmission(mj, true))
			require.NoError(t, SetEnterpriseStatus(1, ent.Id, EnterpriseStatusFrozen, "fixture"))
			p, err := PreviewEnterpriseArchive(ent.Id)
			require.NoError(t, err)
			require.EqualValues(t, 1, p.PendingTasks)
			require.EqualValues(t, 1, p.PendingMidjourney)
			_, err = FinalizeEnterpriseTask(task.ID)
			require.NoError(t, err, "frozen enterprise must still settle accepted work")
			require.NoError(t, SettleEnterpriseMJSubmission(mj.Id, 0))
			_, err = FinalizeEnterpriseMJ(mj.Id)
			require.NoError(t, err)
			p, err = PreviewEnterpriseArchive(ent.Id)
			require.NoError(t, err)
			require.True(t, p.CanArchive)
			_, err = ArchiveEnterprise(1, ent.Id, ent.Name, "结算完成")
			require.NoError(t, err)
			// Terminal CAS updates cannot resurrect a finished archived job.
			task.Status = TaskStatusInProgress
			won, err := task.UpdateWithStatus(TaskStatusSuccess)
			require.NoError(t, err)
			require.False(t, won)
			mj.Status = "IN_PROGRESS"
			won, err = mj.UpdateWithStatus("SUCCESS")
			require.NoError(t, err)
			require.False(t, won)
		})
	}
}

func TestEnterpriseArchiveIncrementalMigration(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			for _, original := range []any{
				&Enterprise{Id: 1, Name: "已有企业", Status: EnterpriseStatusFrozen, OwnerUserId: 7, NetCredited: 299, Unallocated: -1},
				&EnterpriseMember{Id: 1, EnterpriseId: 1, UserId: 7, ActiveUserId: common.GetPointer(7), Status: EnterpriseMemberStatusActive, Available: 300, Consumed: 0},
			} {
				table := "ent_archive_migrate_" + common.GetUUID()[:12]
				kind := reflect.TypeOf(original).Elem()
				fields := []reflect.StructField{}
				currentFields := []reflect.StructField{}
				for i := 0; i < kind.NumField(); i++ {
					field := kind.Field(i)
					// Named indexes are schema-global in SQLite/PostgreSQL. Keep
					// the real contract but give this isolated table its own name.
					field.Tag = reflect.StructTag(strings.ReplaceAll(string(field.Tag), "idx_enterprise_member_ent", "idx_"+table))
					currentFields = append(currentFields, field)
					if field.Name != "ArchivedAt" && field.Name != "ArchivedBy" && field.Name != "ArchiveReason" {
						fields = append(fields, field)
					}
				}
				require.NoError(t, DB.Table(table).AutoMigrate(reflect.New(reflect.StructOf(fields)).Interface()))
				t.Cleanup(func() { require.NoError(t, DB.Migrator().DropTable(table)) })
				require.NoError(t, DB.Table(table).Omit("ArchivedAt", "ArchivedBy", "ArchiveReason").Create(original).Error)
				for i := 0; i < 2; i++ {
					require.NoError(t, DB.Table(table).AutoMigrate(reflect.New(reflect.StructOf(currentFields)).Interface()))
				}
				got := reflect.New(kind).Interface()
				require.NoError(t, DB.Table(table).First(got).Error)
				require.Equal(t, original, got, "migration must not archive existing data or alter money")
			}
		})
	}
}
