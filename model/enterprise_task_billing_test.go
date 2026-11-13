package model

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEnterpriseTaskFinalizationMatrix(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			require.NoError(t, DB.AutoMigrate(&Task{}))
			ent, _, owner := enterpriseTestOrg(t)
			member, user := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			_, err := CreditEnterprise(1, ent.Id, 10000, common.GetUUID(), "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: owner.Id}, ent.Id, member.Id, 8000, ""))
			mint := func(amount int) *Task {
				token := &Token{UserId: user.Id, EnterpriseId: ent.Id, MembershipId: member.Id, Key: common.GetUUID(), Name: "task", Status: common.TokenStatusEnabled, ExpiredTime: -1}
				require.NoError(t, CreateEnterpriseToken(token, 0))
				id := common.GetUUID()
				sub := EnterpriseSubject{EnterpriseId: ent.Id, MemberId: member.Id, UserId: user.Id, TokenId: token.Id, ModelName: "video-fixture"}
				_, err := AdjustEnterpriseReservation(sub, id, amount, EnterpriseReservationReserved, true)
				require.NoError(t, err)
				_, err = AdjustEnterpriseReservation(sub, id, amount, EnterpriseReservationSettled, false)
				require.NoError(t, err)

				task := &Task{TaskID: common.GetUUID(), EnterpriseId: ent.Id, EnterpriseMemberId: member.Id, UserId: user.Id, Quota: amount, EnterpriseInitialQuota: int64(amount), Status: TaskStatusInProgress, PrivateData: TaskPrivateData{RequestId: id, BillingSource: EnterpriseQuotaSource, TokenId: token.Id}}
				require.NoError(t, task.Insert())
				return task
			}
			for _, target := range []int{0, 200, 900} {
				t.Run(fmt.Sprint(target), func(t *testing.T) {
					task := mint(500)
					task.Status = TaskStatusSuccess
					task.EnterpriseBillingPending = true
					task.EnterpriseFinalQuota = int64(target)
					if target == 0 {
						task.Status = TaskStatusFailure
					}
					won, err := task.UpdateWithStatus(TaskStatusInProgress)
					require.NoError(t, err)
					require.True(t, won)
					var wg sync.WaitGroup
					results := make(chan error, 6)
					changed := make(chan bool, 6)
					for i := 0; i < 6; i++ {
						wg.Add(1)
						go func() { defer wg.Done(); r, e := FinalizeEnterpriseTask(task.ID); results <- e; changed <- r.Changed }()
					}
					wg.Wait()
					close(results)
					close(changed)
					for e := range results {
						require.NoError(t, e)
					}
					n := 0
					for c := range changed {
						if c {
							n++
						}
					}
					require.Equal(t, 1, n)
					var fresh Task
					require.NoError(t, DB.First(&fresh, task.ID).Error)
					require.Equal(t, target, fresh.Quota)
					require.False(t, fresh.EnterpriseBillingPending)
					var token Token
					require.NoError(t, DB.First(&token, task.PrivateData.TokenId).Error)
					require.Equal(t, target, token.UsedQuota)
					enterpriseTestBalanced(t, ent.Id)
				})
			}
			// Terminal intent survives a failed billing transaction and can be replayed after restart.
			task := mint(300)
			task.Status = TaskStatusFailure
			won, err := task.UpdateWithStatus(TaskStatusInProgress)
			require.NoError(t, err)
			require.True(t, won)
			request := task.PrivateData.RequestId
			require.NoError(t, DB.Model(&EnterpriseReservation{}).Where("request_id = ?", request).Update("status", EnterpriseReservationRefunded).Error)
			_, err = FinalizeEnterpriseTask(task.ID)
			require.Error(t, err)
			var pending Task
			require.NoError(t, DB.First(&pending, task.ID).Error)
			require.True(t, pending.EnterpriseBillingPending)
			require.NoError(t, DB.Model(&EnterpriseReservation{}).Where("request_id = ?", request).Update("status", EnterpriseReservationSettled).Error)
			require.NoError(t, DisableEnterpriseMember(EnterpriseActor{UserId: owner.Id}, ent.Id, member.Id, "left"))
			require.NoError(t, DB.Delete(&Token{}, task.PrivateData.TokenId).Error)
			count, err := ReconcileEnterpriseTasks(500)
			require.NoError(t, err)
			require.Positive(t, count)
			enterpriseTestBalanced(t, ent.Id)
			var personal User
			require.NoError(t, DB.First(&personal, user.Id).Error)
			require.Equal(t, user.Quota, personal.Quota)
			require.Zero(t, personal.UsedQuota)
		})
	}
}

func TestEnterpriseTaskCorrectionRollsBackWhenAcknowledgementFails(t *testing.T) {
	enterpriseTestDB(t, "sqlite")
	require.NoError(t, DB.AutoMigrate(&Task{}))
	ent, member, owner := enterpriseTestOrg(t)
	_, err := CreditEnterprise(1, ent.Id, 1000, common.GetUUID(), "")
	require.NoError(t, err)
	require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: owner.Id}, ent.Id, member.Id, 1000, ""))
	token := &Token{UserId: owner.Id, EnterpriseId: ent.Id, MembershipId: member.Id, Key: common.GetUUID()}
	require.NoError(t, CreateEnterpriseToken(token, 0))
	id := common.GetUUID()
	sub := EnterpriseSubject{EnterpriseId: ent.Id, MemberId: member.Id, UserId: owner.Id, TokenId: token.Id}
	_, err = AdjustEnterpriseReservation(sub, id, 300, EnterpriseReservationReserved, true)
	require.NoError(t, err)
	_, err = AdjustEnterpriseReservation(sub, id, 300, EnterpriseReservationSettled, false)
	require.NoError(t, err)
	task := &Task{TaskID: common.GetUUID(), UserId: owner.Id, EnterpriseId: ent.Id, EnterpriseMemberId: member.Id, EnterpriseInitialQuota: 300, EnterpriseFinalQuota: 0, EnterpriseBillingPending: true, Quota: 300, Status: TaskStatusFailure, PrivateData: TaskPrivateData{RequestId: id, TokenId: token.Id, BillingSource: EnterpriseQuotaSource}}
	require.NoError(t, task.Insert())
	name := "enterprise_task_ack_failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			tx.AddError(fmt.Errorf("fixture ack failure"))
		}
	}))
	_, err = FinalizeEnterpriseTask(task.ID)
	require.Error(t, err)
	require.NoError(t, DB.Callback().Update().Remove(name))
	var saved EnterpriseMember
	require.NoError(t, DB.First(&saved, member.Id).Error)
	require.EqualValues(t, 700, saved.Available)
	var count int64
	require.NoError(t, DB.Model(&EnterpriseReservation{}).Where("kind = ? AND enterprise_id = ?", EnterpriseChargeTaskAdjust, ent.Id).Count(&count).Error)
	require.Zero(t, count)
	_, err = FinalizeEnterpriseTask(task.ID)
	require.NoError(t, err)
	require.NoError(t, DB.First(&saved, member.Id).Error)
	require.EqualValues(t, 1000, saved.Available)
	enterpriseTestBalanced(t, ent.Id)
}

func TestEnterpriseTaskPayerMigrationPreservesPersonalHistory(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			table := "task_payer_migration_" + common.GetUUID()[:10]
			fields := []reflect.StructField{}
			kind := reflect.TypeOf(Task{})
			for i := 0; i < kind.NumField(); i++ {
				field := kind.Field(i)
				if !strings.HasPrefix(field.Name, "Enterprise") {
					fields = append(fields, field)
				}
			}
			legacy := reflect.New(reflect.StructOf(fields)).Interface()
			require.NoError(t, DB.Table(table).AutoMigrate(legacy))
			t.Cleanup(func() { require.NoError(t, DB.Migrator().DropTable(table)) })
			original := Task{UserId: 999, TaskID: common.GetUUID(), Quota: 875, Status: TaskStatusSuccess, PrivateData: TaskPrivateData{BillingSource: "wallet", RequestId: "fixture-history"}}
			require.NoError(t, DB.Table(table).Omit("EnterpriseId", "EnterpriseMemberId", "EnterpriseBillingPending", "EnterpriseFinalQuota", "EnterpriseInitialQuota", "EnterpriseInitialRecorded").Create(&original).Error)
			for i := 0; i < 2; i++ {
				require.NoError(t, DB.Table(table).AutoMigrate(&Task{}))
			}
			var stored Task
			require.NoError(t, DB.Table(table).First(&stored, original.ID).Error)
			require.Zero(t, stored.EnterpriseId)
			require.Zero(t, stored.EnterpriseMemberId)
			require.False(t, stored.EnterpriseBillingPending)
			require.Equal(t, 875, stored.Quota)
			require.Equal(t, original.PrivateData, stored.PrivateData)
			require.Equal(t, TaskStatus(TaskStatusSuccess), stored.Status)
		})
	}
}

func TestEnterpriseMJPayerMigrationPreservesPersonalHistory(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			table := "mj_payer_migration_" + common.GetUUID()[:10]
			fields := []reflect.StructField{}
			omit := []string{}
			kind := reflect.TypeOf(Midjourney{})
			for i := 0; i < kind.NumField(); i++ {
				field := kind.Field(i)
				if strings.HasPrefix(field.Name, "Enterprise") || strings.HasPrefix(field.Name, "Billing") || strings.HasPrefix(field.Name, "Polling") {
					omit = append(omit, field.Name)
				} else {
					fields = append(fields, field)
				}
			}
			require.NoError(t, DB.Table(table).AutoMigrate(reflect.New(reflect.StructOf(fields)).Interface()))
			t.Cleanup(func() { require.NoError(t, DB.Migrator().DropTable(table)) })
			old := Midjourney{UserId: 999, MjId: "fixture-old", Quota: 123, Status: "SUCCESS", Progress: "100%", Prompt: "synthetic fixture"}
			require.NoError(t, DB.Table(table).Omit(omit...).Create(&old).Error)
			for i := 0; i < 2; i++ {
				require.NoError(t, DB.Table(table).AutoMigrate(&Midjourney{}))
			}
			var got Midjourney
			require.NoError(t, DB.Table(table).First(&got, old.Id).Error)
			require.Zero(t, got.EnterpriseId)
			require.Empty(t, got.BillingSource)
			require.Nil(t, got.EnterpriseTaskKey)
			require.Empty(t, got.PollingKey)
			require.Equal(t, old.Quota, got.Quota)
			require.Equal(t, old.Status, got.Status)
			require.Equal(t, old.Prompt, got.Prompt)
		})
	}
}
