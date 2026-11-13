package model

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func mjBillingFixture(t *testing.T) (*EnterpriseMember, *Token) {
	t.Helper()
	enterpriseTestDB(t, "sqlite")
	require.NoError(t, DB.AutoMigrate(&Midjourney{}))
	ent, member, user := enterpriseTestOrg(t)
	_, err := CreditEnterprise(1, ent.Id, 10000, "mj-fixture", "")
	require.NoError(t, err)
	require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: user.Id}, ent.Id, member.Id, 1000, ""))
	token := &Token{UserId: user.Id, Key: common.GetUUID(), EnterpriseId: ent.Id, MembershipId: member.Id, UnlimitedQuota: true, RemainQuota: 19}
	require.NoError(t, DB.Create(token).Error)
	return member, token
}

func mjIntent(t *testing.T, member *EnterpriseMember, token *Token, quota int) *Midjourney {
	t.Helper()
	task := &Midjourney{UserId: member.UserId, EnterpriseId: member.EnterpriseId, EnterpriseMemberId: member.Id,
		BillingSource: EnterpriseQuotaSource, BillingRequestId: common.GetUUID(), BillingTokenId: token.Id, Quota: quota, ChannelId: 17, Progress: "0%"}
	require.NoError(t, CreateEnterpriseMJIntent(task))
	_, err := AdjustEnterpriseReservation(task.EnterpriseMJSubject(), task.BillingRequestId, quota, EnterpriseReservationReserved, true)
	require.NoError(t, err)
	return task
}

func assertMJBalances(t *testing.T, member *EnterpriseMember, token *Token, available, reserved, consumed int64) {
	t.Helper()
	var m EnterpriseMember
	require.NoError(t, DB.First(&m, member.Id).Error)
	require.Equal(t, available, m.Available)
	require.Equal(t, reserved, m.Reserved)
	require.Equal(t, consumed, m.Consumed)
	var tok Token
	require.NoError(t, DB.Unscoped().First(&tok, token.Id).Error)
	require.EqualValues(t, consumed, tok.UsedQuota)
	require.Equal(t, 19, tok.RemainQuota, "enterprise settlement must not change personal token balance")
	var budget EnterpriseTokenBudget
	require.NoError(t, DB.Where("token_id = ?", token.Id).First(&budget).Error)
	require.Equal(t, reserved+consumed, budget.UsedQuota)
	var user User
	require.NoError(t, DB.First(&user, member.UserId).Error)
	require.Zero(t, user.Quota, "enterprise funds must never enter the personal wallet")
	enterpriseTestBalanced(t, member.EnterpriseId)
}

func TestEnterpriseMJRestartSettlementAndRefund(t *testing.T) {
	member, token := mjBillingFixture(t)
	task := mjIntent(t, member, token, 100)
	n, err := ReconcileEnterpriseMJ(10)
	require.NoError(t, err)
	require.Zero(t, n)
	assertMJBalances(t, member, token, 900, 100, 0)
	require.Empty(t, GetAllUnFinishTasks(), "unknown submission must not be marked failed by polling")
	task.MjId, task.Code = "upstream-17", 22
	require.NoError(t, SaveEnterpriseMJSubmission(task, true))
	// Simulate a crash after committing the original ledger, before task acknowledgement.
	_, err = AdjustEnterpriseReservation(task.EnterpriseMJSubject(), task.BillingRequestId, 100, EnterpriseReservationSettled, false)
	require.NoError(t, err)
	n, err = ReconcileEnterpriseMJ(10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	assertMJBalances(t, member, token, 900, 0, 100)
	task.Status, task.Progress = "FAILURE", "100%"
	won, err := task.UpdateWithStatus("")
	require.NoError(t, err)
	require.True(t, won)
	// The original member may leave and the token may be deleted while work runs.
	require.NoError(t, DB.Model(&EnterpriseMember{}).Where("id = ?", member.Id).Update("status", EnterpriseMemberStatusLeft).Error)
	require.NoError(t, DB.Delete(token).Error)
	n, err = ReconcileEnterpriseMJ(10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	for i := 0; i < 3; i++ {
		_, err = FinalizeEnterpriseMJ(task.Id)
		require.NoError(t, err)
	}
	assertMJBalances(t, member, token, 1000, 0, 0)
	var count int64
	require.NoError(t, DB.Model(&EnterpriseReservation{}).Where("request_id LIKE ?", "mj-final:%").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestEnterpriseMJCorrectionRollbackAndConcurrentRetry(t *testing.T) {
	member, token := mjBillingFixture(t)
	task := mjIntent(t, member, token, 120)
	task.MjId, task.Status, task.Progress = "failed-task", "FAILURE", "100%"
	require.NoError(t, SaveEnterpriseMJSubmission(task, true))
	require.NoError(t, SettleEnterpriseMJSubmission(task.Id, 120))
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("mj-test-fail-ack", func(tx *gorm.DB) {
		if tx.Statement.Table == "midjourneys" {
			if updates, ok := tx.Statement.Dest.(map[string]interface{}); ok && updates["enterprise_billing_finalized"] == true {
				tx.AddError(errors.New("simulated acknowledgement failure"))
			}
		}
	}))
	_, err := FinalizeEnterpriseMJ(task.Id)
	require.Error(t, err)
	require.NoError(t, DB.Callback().Update().Remove("mj-test-fail-ack"))
	assertMJBalances(t, member, token, 880, 0, 120)
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := FinalizeEnterpriseMJ(task.Id); results <- err }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	assertMJBalances(t, member, token, 1000, 0, 0)
}

func TestEnterpriseMJDuplicateAndZeroCharge(t *testing.T) {
	member, token := mjBillingFixture(t)
	first := mjIntent(t, member, token, 80)
	first.MjId, first.Code = "same-result", 1
	require.NoError(t, SaveEnterpriseMJSubmission(first, true))
	require.NoError(t, SettleEnterpriseMJSubmission(first.Id, 80))
	for _, code := range []int{21, 22, 1} {
		duplicate := mjIntent(t, member, token, 80)
		duplicate.MjId, duplicate.Code = first.MjId, code
		require.NoError(t, SaveEnterpriseMJSubmission(duplicate, true))
		require.Equal(t, MJEnterpriseRejected, duplicate.EnterpriseBillingState)
		require.Empty(t, duplicate.MjId)
		_, err := ReconcileEnterpriseMJ(10)
		require.NoError(t, err)
	}
	free := mjIntent(t, member, token, 0)
	free.MjId, free.Status, free.Progress = "free-dialog", "FAILURE", "100%"
	require.NoError(t, SaveEnterpriseMJSubmission(free, true))
	_, err := ReconcileEnterpriseMJ(10)
	require.NoError(t, err)
	assertMJBalances(t, member, token, 920, 0, 80)
	var correction EnterpriseReservation
	require.NoError(t, DB.Where("request_id LIKE ? AND quota = ?", "mj-final:%", 0).First(&correction).Error)
	require.Zero(t, correction.Quota, "zero charge still gets a durable correction receipt")
}

func TestEnterpriseMJSubjectQueriesAndTerminalCAS(t *testing.T) {
	member, token := mjBillingFixture(t)
	personal := &Midjourney{UserId: member.UserId, MjId: "personal", Buttons: `[{"customId":"personal-button"}]`}
	require.NoError(t, personal.Insert())
	task := mjIntent(t, member, token, 50)
	task.MjId, task.Buttons = "enterprise", `[{"customId":"company-button"}]`
	require.NoError(t, SaveEnterpriseMJSubmission(task, true))
	person := MJTaskSubject{UserId: member.UserId}
	company := MJTaskSubject{UserId: member.UserId, EnterpriseId: member.EnterpriseId, MemberId: member.Id}
	require.Nil(t, GetMJTaskForSubject(person, task.MjId))
	require.Nil(t, GetByMJId(member.UserId, task.MjId))
	require.Nil(t, GetMJTaskForSubject(company, personal.MjId))
	require.Len(t, GetMJTasksForSubject(person, []string{task.MjId, personal.MjId}), 1)
	require.Len(t, GetMJTasksForSubject(company, []string{task.MjId, personal.MjId}), 1)
	require.NotNil(t, GetMJTaskForCustomID(company, "company-button"))
	require.Nil(t, GetMJTaskForCustomID(person, "company-button"))
	for _, sub := range []MJTaskSubject{{UserId: member.UserId + 1, EnterpriseId: member.EnterpriseId, MemberId: member.Id}, {UserId: member.UserId, EnterpriseId: member.EnterpriseId, MemberId: member.Id + 1}, {UserId: member.UserId, MemberId: member.Id}} {
		require.Nil(t, GetMJTaskForSubject(sub, task.MjId), fmt.Sprintf("scope: %+v", sub))
	}
	require.NoError(t, SettleEnterpriseMJSubmission(task.Id, 50))
	task.Status, task.Progress = "SUCCESS", "100%"
	won, err := task.UpdateWithStatus("")
	require.NoError(t, err)
	require.True(t, won)
	// A stale task object must not overwrite settled acknowledgement.
	var current Midjourney
	require.NoError(t, DB.First(&current, task.Id).Error)
	require.True(t, current.EnterpriseBillingSettled)
	task.Status = "FAILURE"
	won, err = task.UpdateWithStatus("SUCCESS")
	require.NoError(t, err)
	require.False(t, won)
	_, err = ReconcileEnterpriseMJ(10)
	require.NoError(t, err)
	assertMJBalances(t, member, token, 950, 0, 50)
}

func TestEnterpriseMJPollOnlyTrustedTerminalStatus(t *testing.T) {
	member, token := mjBillingFixture(t)
	task := mjIntent(t, member, token, 40)
	task.MjId, task.Status, task.Progress = "still-working", "IN_PROGRESS", "100%"
	require.NoError(t, SaveEnterpriseMJSubmission(task, true))
	require.Len(t, GetAllUnFinishTasks(), 1, "100 percent is not proof of enterprise terminal status")
	require.NoError(t, MjBulkUpdatePersonalByChannel(task.ChannelId, []string{task.MjId}, map[string]any{"status": "FAILURE", "progress": "100%"}))
	var row Midjourney
	require.NoError(t, DB.First(&row, task.Id).Error)
	require.Equal(t, "IN_PROGRESS", row.Status, "channel lookup failure must not become an enterprise refund")
	_, err := ReconcileEnterpriseMJ(10)
	require.NoError(t, err)
	assertMJBalances(t, member, token, 960, 0, 40)
}

func TestEnterpriseMJPollingBatchesIsolateOwnerPayerAndCredential(t *testing.T) {
	base := Midjourney{Id: 1, UserId: 10, EnterpriseId: 20, EnterpriseMemberId: 30, BillingSource: EnterpriseQuotaSource, ChannelId: 40, MjId: "same-id", PollingKey: "fixture-key-a", PollingBaseURL: "https://fixture.invalid"}
	records := []*Midjourney{}
	for i := 0; i < 7; i++ {
		copy := base
		copy.Id = i + 1
		records = append(records, &copy)
	}
	// Same namespace duplicate must be retained, not overwritten.
	records[2].PollingKey = "fixture-key-b"
	records[3].UserId, records[3].EnterpriseMemberId = 11, 31
	records[4].EnterpriseId, records[4].EnterpriseMemberId, records[4].BillingSource = 0, 0, ""
	records[5].PollingBaseURL = "https://other-fixture.invalid"
	records[6].PollingKey = "" // never fall back to channel-wide multi-key credentials
	batches, err := BuildMJPollingBatches(records)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "fixture-key")
	require.Len(t, batches, 5)
	seen := map[int]int{}
	for _, batch := range batches {
		require.Equal(t, []string{"same-id"}, batch.TaskIDs)
		for _, task := range batch.TasksByID["same-id"] {
			seen[task.Id]++
		}
		encoded, err := common.Marshal(batch)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "fixture-key")
	}
	for i := 1; i <= 6; i++ {
		require.Equal(t, 1, seen[i])
	}
	require.Zero(t, seen[7])
	require.Len(t, batches[0].TasksByID["same-id"], 2)
	channel := &Channel{Id: 40, Key: "rotated-a\nrotated-b"}
	for _, batch := range batches {
		endpoint, key, err := batch.PollingTransport(channel)
		require.NoError(t, err)
		require.Contains(t, endpoint, "fixture.invalid")
		require.Contains(t, key, "fixture-key")
		require.NotContains(t, key, "rotated")
	}
	legacy, err := BuildMJPollingBatches([]*Midjourney{{UserId: 10, ChannelId: 40, MjId: "legacy"}})
	require.NoError(t, err)
	require.Len(t, legacy, 1)
	_, key, err := legacy[0].PollingTransport(channel)
	require.NoError(t, err)
	require.Equal(t, channel.Key, key)
}

func TestEnterpriseMJCredentialNamespaceDedupAndPersistence(t *testing.T) {
	member, token := mjBillingFixture(t)
	makeTask := func(key string) *Midjourney {
		task := &Midjourney{UserId: member.UserId, EnterpriseId: member.EnterpriseId, EnterpriseMemberId: member.Id, BillingSource: EnterpriseQuotaSource,
			BillingRequestId: common.GetUUID(), BillingTokenId: token.Id, Quota: 20, ChannelId: 17, Progress: "0%", PollingKey: key, PollingBaseURL: "https://fixture.invalid"}
		require.NoError(t, CreateEnterpriseMJIntent(task))
		_, err := AdjustEnterpriseReservation(task.EnterpriseMJSubject(), task.BillingRequestId, 20, EnterpriseReservationReserved, true)
		require.NoError(t, err)
		task.MjId = "same-id"
		require.NoError(t, SaveEnterpriseMJSubmission(task, true))
		return task
	}
	first := makeTask("fixture-key-a")
	second := makeTask("fixture-key-b")
	require.Equal(t, MJEnterpriseAccepted, first.EnterpriseBillingState)
	require.Equal(t, MJEnterpriseAccepted, second.EnterpriseBillingState)
	duplicate := makeTask("fixture-key-a")
	require.Equal(t, MJEnterpriseRejected, duplicate.EnterpriseBillingState)
	var persisted Midjourney
	require.NoError(t, DB.First(&persisted, first.Id).Error)
	require.Equal(t, first.PollingKey, persisted.PollingKey)
	require.Equal(t, first.PollingBaseURL, persisted.PollingBaseURL)
	data, err := common.Marshal(persisted)
	require.NoError(t, err)
	require.NotContains(t, string(data), "fixture-key")
	require.NotContains(t, string(data), "fixture.invalid")
	// Both accepted rows survive restart and settle independently; duplicate is zero.
	_, err = ReconcileEnterpriseMJ(10)
	require.NoError(t, err)
	assertMJBalances(t, member, token, 960, 0, 40)
	sub := MJTaskSubject{UserId: member.UserId, EnterpriseId: member.EnterpriseId, MemberId: member.Id}
	require.Nil(t, GetMJTaskForSubject(sub, "same-id"), "ambiguous account namespace must not pick the first task")
	// Polling never mutates the credential snapshot or settlement fields.
	persisted.Status = "SUCCESS"
	persisted.PollingKey = "tampered"
	_, err = persisted.UpdateWithStatus("")
	require.NoError(t, err)
	var unchanged Midjourney
	require.NoError(t, DB.First(&unchanged, first.Id).Error)
	require.Equal(t, "fixture-key-a", unchanged.PollingKey)
}

func TestEnterpriseMJChannelCorrectionSharesTransaction(t *testing.T) {
	member, token := mjBillingFixture(t)
	require.NoError(t, DB.AutoMigrate(&Channel{}))
	require.NoError(t, DB.Create(&Channel{Id: 17, UsedQuota: 100}).Error)
	task := mjIntent(t, member, token, 100)
	task.MjId, task.Status, task.Progress = "failed-channel-task", "FAILURE", "100%"
	require.NoError(t, SaveEnterpriseMJSubmission(task, true))
	require.NoError(t, SettleEnterpriseMJSubmission(task.Id, 100))
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("mj-channel-correction-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			tx.AddError(errors.New("simulated channel correction failure"))
		}
	}))
	_, err := FinalizeEnterpriseMJ(task.Id)
	require.Error(t, err)
	require.NoError(t, DB.Callback().Update().Remove("mj-channel-correction-failure"))
	assertMJBalances(t, member, token, 900, 0, 100)
	var channel Channel
	require.NoError(t, DB.First(&channel, 17).Error)
	require.EqualValues(t, 100, channel.UsedQuota)
	var current Midjourney
	require.NoError(t, DB.First(&current, task.Id).Error)
	require.False(t, current.EnterpriseBillingFinalized)
	receipt, err := FinalizeEnterpriseMJ(task.Id)
	require.NoError(t, err)
	require.True(t, receipt.Changed)
	for i := 0; i < 3; i++ {
		_, err := FinalizeEnterpriseMJ(task.Id)
		require.NoError(t, err)
	}
	assertMJBalances(t, member, token, 1000, 0, 0)
	require.NoError(t, DB.First(&channel, 17).Error)
	require.Zero(t, channel.UsedQuota)
}
