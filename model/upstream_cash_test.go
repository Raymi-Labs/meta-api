package model

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCashTestDB(t *testing.T) {
	t.Helper()
	setupPaymentRiskCaseTestDB(t)
	require.NoError(t, DB.AutoMigrate(cashAccountingModels()...))
	require.NoError(t, DB.AutoMigrate(&UpstreamAccount{}, &UpstreamAccountSnapshot{}, &Channel{}))
	old := cashAccountingEnabledAt.Swap(1)
	t.Cleanup(func() { cashAccountingEnabledAt.Store(old) })
	require.NoError(t, DB.Create(&CashAccountingConfig{ID: 1, EnabledAt: 1, EnabledBy: 1}).Error)
}

func cashGrantTest(t *testing.T, u *User, quota int, kind string) CashFundingLot {
	t.Helper()
	ref := common.GetUUID()
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return GrantUserQuotaTx(tx, u.Id, quota, quota, CashGrantSource{Kind: kind, Ref: ref})
	}))
	var lot CashFundingLot
	require.NoError(t, DB.Where("user_id = ? AND source_ref = ?", u.Id, ref).First(&lot).Error)
	return lot
}

func cashReceiptTest(t *testing.T, lot CashFundingLot, amount string, at int64) CashFundingReceipt {
	t.Helper()
	r, err := SaveCashFundingReceipt(lot.ID, 1, CashReceiptInput{RequestID: common.GetUUID(), Currency: "CNY", GrossAmount: amount, FeeAmount: "0", FXRate: "1", ReceivedAt: at, Evidence: "test-bank-record"})
	require.NoError(t, err)
	return *r
}

func TestCashWalletFIFORefundAndIdempotency(t *testing.T) {
	setupCashTestDB(t)
	u := createPaymentRiskCaseTestUser(t, "cash-fifo")
	paid := cashGrantTest(t, u, 100, "topup")
	gift := cashGrantTest(t, u, 100, "gift")
	_, err := AdjustPulseWalletReservation(u.Id, "cash-partial", 150, "reserved", true)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, err = AdjustPulseWalletReservation(u.Id, "cash-partial", 75, "settled", true)
		require.NoError(t, err)
	}
	require.NoError(t, DB.First(&paid, paid.ID).Error)
	require.NoError(t, DB.First(&gift, gift.ID).Error)
	require.EqualValues(t, 25, paid.RemainingQuota)
	require.EqualValues(t, 100, gift.RemainingQuota)
	require.EqualValues(t, 125, readFundingUser(t, u.Id).Quota)
	var allocations []CashFundingAllocation
	require.NoError(t, DB.Where("quota > 0").Find(&allocations).Error)
	require.Len(t, allocations, 1)
	require.EqualValues(t, 75, allocations[0].Quota)
	require.Equal(t, "settled", allocations[0].Status)
	_, err = AdjustPulseWalletReservation(u.Id, "cash-full-refund", 90, "reserved", true)
	require.NoError(t, err)
	_, err = AdjustPulseWalletReservation(u.Id, "cash-full-refund", 0, "refunded", true)
	require.NoError(t, err)
	require.NoError(t, DB.First(&paid, paid.ID).Error)
	require.NoError(t, DB.First(&gift, gift.ID).Error)
	require.EqualValues(t, 25, paid.RemainingQuota)
	require.EqualValues(t, 100, gift.RemainingQuota)
	var remaining int64
	require.NoError(t, DB.Model(&CashFundingLot{}).Select("SUM(remaining_quota)").Scan(&remaining).Error)
	require.EqualValues(t, readFundingUser(t, u.Id).Quota, remaining)
}

func TestCashInvalidationAndUntrackedWalletNeverRestoreProof(t *testing.T) {
	setupCashTestDB(t)
	u := createPaymentRiskCaseTestUser(t, "cash-invalid")
	paid := cashGrantTest(t, u, 100, "topup")
	cashReceiptTest(t, paid, "10", common.GetTimestamp()-60)
	_, err := AdjustPulseWalletReservation(u.Id, "inflight-cash", 60, "reserved", true)
	require.NoError(t, err)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error { return InvalidatePulsePaidFundingTx(tx, u.Id, "test") }))
	_, err = AdjustPulseWalletReservation(u.Id, "inflight-cash", 0, "refunded", true)
	require.NoError(t, err)
	require.NoError(t, DB.First(&paid, paid.ID).Error)
	require.Zero(t, paid.RemainingQuota)
	var lots []CashFundingLot
	require.NoError(t, DB.Where("remaining_quota > 0").Find(&lots).Error)
	var available int64
	for _, l := range lots {
		require.Zero(t, l.ReceiptID)
		available += l.RemainingQuota
	}
	require.EqualValues(t, 100, available)
	// A direct mutation outside the observed transaction invalidates remaining basis.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", u.Id).Update("quota", 130).Error)
	_, err = AdjustPulseWalletReservation(u.Id, "after-untracked", 10, "settled", true)
	require.NoError(t, err)
	var state CashWalletState
	require.NoError(t, DB.First(&state, "user_id = ?", u.Id).Error)
	require.GreaterOrEqual(t, state.Epoch, int64(3))
	require.EqualValues(t, 120, state.Quota)
}

func TestCashWalletConcurrentAndAuditWriteRollback(t *testing.T) {
	setupCashTestDB(t)
	u := createPaymentRiskCaseTestUser(t, "cash-parallel")
	cashGrantTest(t, u, 100, "topup")
	var wg sync.WaitGroup
	failures := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := AdjustPulseWalletReservation(u.Id, fmt.Sprintf("cash-parallel-%d", i), 10, "settled", true)
			failures <- e
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.Zero(t, readFundingUser(t, u.Id).Quota)
	var sum int64
	require.NoError(t, DB.Model(&CashFundingAllocation{}).Select("SUM(quota)").Scan(&sum).Error)
	require.EqualValues(t, 100, sum)
	// The transaction cannot commit a quota grant when its audit lot is missing.
	require.NoError(t, DB.Migrator().DropTable(&CashFundingLot{}))
	require.Error(t, GrantUserQuota(u.Id, 20, 20))
	require.Zero(t, readFundingUser(t, u.Id).Quota)
}

func TestCashReceiptsRevisionsAndPayloadBinding(t *testing.T) {
	setupCashTestDB(t)
	u := createPaymentRiskCaseTestUser(t, "cash-receipts")
	lot := cashGrantTest(t, u, 100, "topup")
	input := CashReceiptInput{RequestID: common.GetUUID(), Currency: "USD", GrossAmount: "10", FeeAmount: "1", FXRate: "7", ReceivedAt: common.GetTimestamp() - 3600, Evidence: "bank/001"}
	a, err := SaveCashFundingReceipt(lot.ID, 1, input)
	require.NoError(t, err)
	require.EqualValues(t, 63000000, a.NetCNYMicros)
	b, err := SaveCashFundingReceipt(lot.ID, 1, input)
	require.NoError(t, err)
	require.Equal(t, a.ID, b.ID)
	input.GrossAmount = "11"
	_, err = SaveCashFundingReceipt(lot.ID, 1, input)
	require.Error(t, err)
	input.RequestID = common.GetUUID()
	input.PreviousID = a.ID
	_, err = SaveCashFundingReceipt(lot.ID, 1, input)
	require.Error(t, err) // revision requires reason
	input.Note = "bank correction"
	b, err = SaveCashFundingReceipt(lot.ID, 1, input)
	require.NoError(t, err)
	input.RequestID = common.GetUUID()
	_, err = SaveCashFundingReceipt(lot.ID, 1, input)
	require.Error(t, err) // stale version
	require.Equal(t, a.ID, b.PreviousID)
	rows, total, err := ListCashFundingReceipts(lot.ID, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, rows, 2)
	_, err = SaveCashFundingReceipt(cashGrantTest(t, u, 100, "gift").ID, 1, input)
	require.Error(t, err)
	for _, amount := range []string{"NaN", "-1", "1e3", "0.0000000001", "9999999999999"} {
		_, err = cashDecimal(amount)
		require.Error(t, err)
	}
	_, err = cashConvert("1", "2", "USD", "7", true)
	require.Error(t, err)
	_, err = cashConvert("1", "0", "CNY", "7", false)
	require.Error(t, err)
}

func cashCostFixture() (time.Time, time.Time, []CashUpstreamEntry, []CashActivityDay, map[string]bool) {
	start, _ := time.Parse("2006-01-02", "2026-10-01")
	end := start.AddDate(0, 0, 2)
	entries := []CashUpstreamEntry{{ID: 1, Kind: "opening", Date: "2026-10-01", CreditsNanos: 100e9, CostCNYMicros: 600e6}, {ID: 2, Kind: "purchase", Date: "2026-10-01", CreditsNanos: 100e9, CostCNYMicros: 800e6}}
	activity := []CashActivityDay{{Date: "2026-10-01", UsageNanos: 40e9}, {Date: "2026-10-01", KeyHash: "site", UsageNanos: 10e9}, {Date: "2026-10-02", UsageNanos: 20e9}, {Date: "2026-10-02", KeyHash: "site", UsageNanos: 20e9}}
	coverage := map[string]bool{}
	for _, day := range []string{"2026-10-01", "2026-10-02"} {
		for _, hash := range []string{"", "site"} {
			coverage[cashHash(hash, day)] = true
		}
	}
	return start, end, entries, activity, coverage
}

func TestCashCostWeightedAverageExternalSpendAndCoverage(t *testing.T) {
	s, e, entries, activity, coverage := cashCostFixture()
	cost, err := calculateCashCosts(s, e, entries, activity, coverage, map[string]bool{"site": true})
	require.NoError(t, err)
	require.EqualValues(t, 280e6, cost["2026-10-01"].totalCost)
	require.EqualValues(t, 140e6, cost["2026-10-02"].totalCost)
	require.True(t, cost["2026-10-02"].known)
	require.EqualValues(t, 70e6, cashMulDiv(cost["2026-10-01"].totalCost, 10e9, 40e9))
	delete(coverage, cashHash("", "2026-10-01"))
	cost, err = calculateCashCosts(s, e, entries, activity, coverage, map[string]bool{"site": true})
	require.NoError(t, err)
	require.False(t, cost["2026-10-01"].known)
	require.False(t, cost["2026-10-02"].known)
	entries = append(entries, CashUpstreamEntry{ID: 3, Kind: "opening", Date: "2026-10-02", CreditsNanos: 160e9, CostCNYMicros: 1120e6})
	cost, err = calculateCashCosts(s, e, entries, activity, coverage, map[string]bool{"site": true})
	require.NoError(t, err)
	require.True(t, cost["2026-10-02"].known)
	activity[3].BYOKNanos = 1e9
	cost, err = calculateCashCosts(s, e, entries, activity, coverage, map[string]bool{"site": true})
	require.NoError(t, err)
	require.False(t, cost["2026-10-02"].known)
	entries = append(entries, CashUpstreamEntry{ID: 4, Kind: "byok", Date: "2026-10-02", CostCNYMicros: 7e6})
	cost, err = calculateCashCosts(s, e, entries, activity, coverage, map[string]bool{"site": true})
	require.NoError(t, err)
	require.True(t, cost["2026-10-02"].known)
	require.EqualValues(t, 7e6, cost["2026-10-02"].byokExpense)
}

func cashReportFixture(t *testing.T) (*UpstreamAccount, *User, CashFundingLot, string) {
	t.Helper()
	account := &UpstreamAccount{Name: "cash-test", AccountType: UpstreamAccountTypeOpenRouter, Enabled: true}
	require.NoError(t, DB.Create(account).Error)
	day := cashDay(common.GetTimestamp() - 86400)
	start, _ := time.Parse("2006-01-02", day)
	u := createPaymentRiskCaseTestUser(t, "cash-report")
	lot := cashGrantTest(t, u, 3, "topup")
	cashReceiptTest(t, lot, "1", start.Unix())
	hash := cashKeyHash("test-call-key")
	require.NoError(t, DB.Create(&CashAccountBinding{KeyHash: hash, AccountID: account.Id, SiteOnly: true}).Error)
	for _, scope := range []string{"", hash} {
		require.NoError(t, DB.Create(&CashDayCoverage{ID: cashHash(scope, day), AccountID: account.Id, KeyHash: scope, Date: day}).Error)
		require.NoError(t, DB.Create(&CashActivityDay{ID: cashHash(scope, day, "m"), AccountID: account.Id, KeyHash: scope, Date: day, Model: "m", UsageNanos: 1e9, Requests: 3}).Error)
	}
	require.NoError(t, DB.Create(&CashUpstreamEntry{AccountID: account.Id, RequestKey: common.GetUUID(), Kind: "opening", Date: day, CreditsNanos: 1e9, CostCNYMicros: 500000}).Error)
	for i := 0; i < 3; i++ {
		r, err := AdjustPulseWalletReservation(u.Id, fmt.Sprintf("report-%d", i), 1, "settled", true)
		require.NoError(t, err)
		require.NoError(t, DB.Model(&r).Update("updated_at", start.Unix()+10).Error)
		require.NoError(t, DB.Create(&CashUsage{RequestID: r.RequestID, ReservationID: r.ID, UserID: u.Id, ChannelID: 1, KeyHash: hash, Model: "m", FundingSource: "wallet", Quota: 1, OccurredAt: start.Unix() + 10}).Error)
	}
	return account, u, lot, day
}

func TestCashReportConservationRevisionsAndUnknown(t *testing.T) {
	setupCashTestDB(t)
	account, _, lot, day := cashReportFixture(t)
	r, err := GetCashReport(context.Background(), account.Id, day, day)
	require.NoError(t, err)
	require.NotNil(t, r.Summary.Profit)
	require.Equal(t, 0.5, *r.Summary.Profit)
	require.Equal(t, 1.0, *r.Summary.Income)
	require.Equal(t, "verified", r.Summary.Status)
	require.Equal(t, 1.0, r.CashFlow.SiteConfirmedReceipts)
	require.Len(t, r.Models, 1)
	require.Equal(t, *r.Summary.Profit, *r.Models[0].Profit)
	require.Nil(t, r.Channels[0].Cost)
	require.NoError(t, DB.First(&lot, lot.ID).Error)
	_, err = SaveCashFundingReceipt(lot.ID, 1, CashReceiptInput{RequestID: common.GetUUID(), PreviousID: lot.ReceiptID, Currency: "CNY", GrossAmount: "2", FeeAmount: "0", FXRate: "1", ReceivedAt: time.Now().Add(-24 * time.Hour).Unix(), Evidence: "corrected", Note: "correction"})
	require.NoError(t, err)
	r, err = GetCashReport(context.Background(), account.Id, day, day)
	require.NoError(t, err)
	require.Equal(t, 2.0, *r.Summary.Income)
	require.Equal(t, 2.0, r.CashFlow.SiteConfirmedReceipts)
	require.EqualValues(t, 1, r.CashFlow.SiteReceiptCount)
	require.NoError(t, DB.Create(&PaymentRiskCase{TradeNo: lot.SourceRef, Status: PaymentRiskStatusOpen, Source: "stripe", RecordType: "topup"}).Error)
	r, err = GetCashReport(context.Background(), account.Id, day, day)
	require.NoError(t, err)
	require.Nil(t, r.Summary.Income)
	require.Nil(t, r.Summary.Profit)
	require.NoError(t, DB.Model(&CashAccountSync{}).Create(&CashAccountSync{AccountID: account.Id, Error: "sync failed"}).Error)
	r, err = GetCashReport(context.Background(), account.Id, day, day)
	require.NoError(t, err)
	require.Nil(t, r.Summary.Cost)
}

func TestCashReportMissingScopeAndCaptureStart(t *testing.T) {
	setupCashTestDB(t)
	account, _, _, day := cashReportFixture(t)
	require.NoError(t, DB.Model(&CashAccountBinding{}).Where("account_id = ?", account.Id).Update("site_only", false).Error)
	r, err := GetCashReport(context.Background(), account.Id, day, day)
	require.NoError(t, err)
	require.Nil(t, r.Summary.Profit)
	require.NoError(t, DB.Model(&CashAccountBinding{}).Where("account_id = ?", account.Id).Update("site_only", true).Error)
	require.NoError(t, DB.Model(&CashAccountingConfig{}).Where("id = 1").Update("enabled_at", common.GetTimestamp()).Error)
	r, err = GetCashReport(context.Background(), account.Id, day, day)
	require.NoError(t, err)
	require.Nil(t, r.Summary.Profit)
	require.Error(t, DeleteUpstreamAccount(account.Id))
	_, _, err = cashReportRange(day, cashDay(common.GetTimestamp()), common.GetTimestamp())
	require.Error(t, err)
}

func TestCashUsageSurvivesDisabledLogs(t *testing.T) {
	setupCashTestDB(t)
	u := createPaymentRiskCaseTestUser(t, "cash-log")
	cashGrantTest(t, u, 20, "gift")
	r, err := AdjustPulseWalletReservation(u.Id, "cash-log-disabled", 10, "settled", true)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.RequestIdKey, r.RequestID)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenRouter)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-call-key")
	previous := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = previous })
	for i := 0; i < 2; i++ {
		RecordConsumeLog(c, u.Id, RecordConsumeLogParams{ChannelId: 1, ModelName: "alias", Quota: 10, Other: map[string]interface{}{"upstream_model_name": "actual-model"}})
	}
	var rows []CashUsage
	require.NoError(t, DB.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "actual-model", rows[0].Model)
	require.Equal(t, cashKeyHash("test-call-key"), rows[0].KeyHash)
	require.Equal(t, r.ID, rows[0].ReservationID)
}

func TestCashSyncAtomicReplacementAndEmptyCoverage(t *testing.T) {
	setupCashTestDB(t)
	a := &UpstreamAccount{Name: "sync", AccountType: UpstreamAccountTypeOpenRouter, Enabled: true, AccessToken: "test-secret"}
	require.NoError(t, DB.Create(a).Error)
	hash := cashKeyHash("test-call-key")
	require.NoError(t, DB.Create(&CashAccountBinding{AccountID: a.Id, KeyHash: hash, SiteOnly: true}).Error)
	day := cashDay(common.GetTimestamp() - 86400)
	fail := false
	empty := false
	client := &http.Client{Transport: openRouterTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/keys" {
			body := `{"data":[]}`
			if r.URL.Query().Get("offset") == "0" {
				body = fmt.Sprintf(`{"data":[{"hash":%q}]}`, hash)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		}
		body := fmt.Sprintf(`{"data":[{"date":%q,"model":"m","usage":1,"requests":1}]}`, day+" 00:00:00")
		status := 200
		if fail && r.URL.Query().Get("api_key_hash") != "" {
			status = 500
			body = "secret-response-must-not-leak"
		}
		if empty {
			body = `{"data":[]}`
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	for i := 0; i < 2; i++ {
		require.NoError(t, syncCashAccountData(context.Background(), client, a, common.GetTimestamp()))
	}
	var count int64
	require.NoError(t, DB.Model(&CashActivityDay{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	require.NoError(t, DB.Model(&CashDayCoverage{}).Count(&count).Error)
	require.EqualValues(t, 60, count)
	fail = true
	require.Error(t, syncCashAccountData(context.Background(), client, a, common.GetTimestamp()))
	require.NoError(t, DB.Model(&CashActivityDay{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	fail = false
	empty = true
	require.NoError(t, syncCashAccountData(context.Background(), client, a, common.GetTimestamp()))
	require.NoError(t, DB.Model(&CashActivityDay{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCashUpstreamJournalRefundsAndVoids(t *testing.T) {
	setupCashTestDB(t)
	a := &UpstreamAccount{Name: "journal", AccountType: UpstreamAccountTypeOpenRouter, Enabled: true}
	require.NoError(t, DB.Create(a).Error)
	input := CashEntryInput{RequestID: common.GetUUID(), Kind: "opening", Date: cashDay(common.GetTimestamp() - 86400), Credits: "10", Currency: "CNY", GrossAmount: "70", FeeAmount: "0", FXRate: "1", Reference: "opening", Evidence: "bank"}
	first, err := SaveCashUpstreamEntry(a.Id, 1, input)
	require.NoError(t, err)
	replay, err := SaveCashUpstreamEntry(a.Id, 1, input)
	require.NoError(t, err)
	require.Equal(t, first.ID, replay.ID)
	input.RequestID = common.GetUUID()
	_, err = SaveCashUpstreamEntry(a.Id, 1, input)
	require.Error(t, err)
	input.Reference = "different-opening"
	_, err = SaveCashUpstreamEntry(a.Id, 1, input)
	require.Error(t, err)
	require.Error(t, VoidCashUpstreamEntry(a.Id, first.ID, 1, ""))
	require.Error(t, VoidCashUpstreamEntry(a.Id+1, first.ID, 1, "wrong account"))
	require.NoError(t, VoidCashUpstreamEntry(a.Id, first.ID, 1, "corrected"))
	require.NoError(t, VoidCashUpstreamEntry(a.Id, first.ID, 1, "retry"))
	_, err = SaveCashUpstreamEntry(a.Id, 1, input)
	require.NoError(t, err)
	input.RequestID = common.GetUUID()
	input.Kind = "refund"
	input.Reference = "refund"
	input.GrossAmount = "7"
	input.FeeAmount = "1"
	input.Credits = "1"
	refund, err := SaveCashUpstreamEntry(a.Id, 1, input)
	require.NoError(t, err)
	require.EqualValues(t, 6e6, refund.CostCNYMicros)
	rows, total, err := ListCashUpstreamEntries(a.Id, 0, 1)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, rows, 1)
	require.Error(t, DeleteUpstreamAccount(a.Id))
}

func TestCashReadPathScopeValidation(t *testing.T) {
	hash := cashKeyHash("test")
	for _, path := range []string{"/api/v1/activity?api_key_hash=" + hash, "/api/v1/keys?include_disabled=true&include_expired=true&offset=0"} {
		require.True(t, validOpenRouterAccountReadPath(path), path)
	}
	for _, path := range []string{"//attacker.invalid/api/v1/keys", "/api/v1/activity?api_key_hash=bad", "/api/v1/credits?api_key_hash=" + hash, "/api/v1/keys?include_disabled=true&include_expired=true&offset=-1", "/api/v1/keys?include_disabled=true&include_expired=true&offset=10001", "/api/v1/activity?api_key_hash=" + hash + "&api_key_hash=" + hash} {
		require.False(t, validOpenRouterAccountReadPath(path), path)
	}
}

func TestCashFullUpstreamRefundRecognizesFee(t *testing.T) {
	s, _ := time.Parse("2006-01-02", "2026-10-01")
	entries := []CashUpstreamEntry{{ID: 1, Kind: "opening", Date: "2026-10-01", CreditsNanos: 10e9, CostCNYMicros: 70e6}, {ID: 2, Kind: "refund", Date: "2026-10-01", CreditsNanos: 10e9, CostCNYMicros: 69e6}}
	coverage := map[string]bool{cashHash("", "2026-10-01"): true, cashHash("site", "2026-10-01"): true}
	days, err := calculateCashCosts(s, s.AddDate(0, 0, 1), entries, nil, coverage, map[string]bool{"site": true})
	require.NoError(t, err)
	require.True(t, days["2026-10-01"].known)
	require.EqualValues(t, 1e6, days["2026-10-01"].expense)
}
