package model

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupUpstreamAccountTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	originDB := DB
	originLogDB := LOG_DB
	originSQLite := common.UsingSQLite
	originMySQL := common.UsingMySQL
	originPostgres := common.UsingPostgreSQL
	DB = db
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	initCol()

	t.Cleanup(func() {
		DB = originDB
		LOG_DB = originLogDB
		common.UsingSQLite = originSQLite
		common.UsingMySQL = originMySQL
		common.UsingPostgreSQL = originPostgres
		initCol()
	})

	if err := db.AutoMigrate(&UpstreamAccount{}, &UpstreamAccountSnapshot{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.AutoMigrate(cashAccountingModels()...); err != nil {
		t.Fatal(err)
	}
	return db
}

func allowUpstreamAccountTestPrivateFetch(t *testing.T) {
	t.Helper()
	fetchSetting := system_setting.GetFetchSetting()
	origin := *fetchSetting
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() {
		*fetchSetting = origin
	})
}

func seedUpstreamAccountSnapshot(
	t *testing.T,
	signature string,
	config UpstreamAccountRemoteConfig,
	syncedAt int64,
	walletQuota int64,
	walletUsedQuota int64,
	subscriptions []UpstreamAccountSubscriptionSnapshot,
) {
	t.Helper()
	payload, err := common.Marshal(subscriptions)
	if err != nil {
		t.Fatalf("marshal subscriptions: %v", err)
	}
	snapshot := UpstreamAccountSnapshot{
		SelectionSignature: signature,
		ComboId:            upstreamAccountSnapshotComboID,
		ConfigHash:         upstreamAccountRemoteObserverConfigHash(config),
		Status:             upstreamAccountRemoteSnapshotStatusSuccess,
		RemoteQuotaPerUnit: common.QuotaPerUnit,
		WalletQuota:        walletQuota,
		WalletUsedQuota:    walletUsedQuota,
		SubscriptionStates: string(payload),
		SyncedAt:           syncedAt,
		CreatedAt:          syncedAt,
	}
	if err := DB.Create(&snapshot).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
}

func TestUpstreamAccountKeepsLegacyStorageCompatibility(t *testing.T) {
	if got := (UpstreamAccount{}).TableName(); got != "profit_board_upstream_accounts" {
		t.Fatalf("unexpected account table: %s", got)
	}
	if got := (UpstreamAccountSnapshot{}).TableName(); got != "profit_board_remote_snapshots" {
		t.Fatalf("unexpected snapshot table: %s", got)
	}
	if got := upstreamAccountSnapshotSignature(42); got != "profit_board_account:42" {
		t.Fatalf("unexpected snapshot signature: %s", got)
	}
}

func TestNormalizeAndValidateUpstreamAccountTypes(t *testing.T) {
	allowUpstreamAccountTestPrivateFetch(t)

	newAPI := normalizeUpstreamAccount(UpstreamAccount{
		Name:                "  primary  ",
		AccountType:         " NEWAPI ",
		BaseURL:             " https://api.example.com/// ",
		UserID:              7,
		AccessToken:         " token ",
		ResourceDisplayMode: "invalid",
	})
	if newAPI.Name != "primary" || newAPI.AccountType != UpstreamAccountTypeNewAPI || newAPI.BaseURL != "https://api.example.com" {
		t.Fatalf("unexpected new-api normalization: %+v", newAPI)
	}
	if newAPI.ResourceDisplayMode != UpstreamAccountResourceDisplayBoth {
		t.Fatalf("unexpected display mode: %s", newAPI.ResourceDisplayMode)
	}
	if err := validateUpstreamAccount(newAPI, true); err != nil {
		t.Fatalf("validate new-api account: %v", err)
	}

	sub2api := normalizeUpstreamAccount(UpstreamAccount{
		Name:                "sub2api",
		AccountType:         " SUB2API ",
		BaseURL:             "https://sub.example.com/",
		UserID:              99,
		Email:               " user@example.com ",
		Password:            " password ",
		ResourceDisplayMode: UpstreamAccountResourceDisplaySubscription,
	})
	if sub2api.UserID != 0 || sub2api.ResourceDisplayMode != UpstreamAccountResourceDisplayWallet {
		t.Fatalf("unexpected sub2api normalization: %+v", sub2api)
	}
	if err := validateUpstreamAccount(sub2api, true); err != nil {
		t.Fatalf("validate sub2api account: %v", err)
	}

	if err := validateUpstreamAccount(UpstreamAccount{}, true); !errors.Is(err, ErrUpstreamAccountNameEmpty) {
		t.Fatalf("expected empty-name error, got %v", err)
	}
	if err := validateUpstreamAccount(UpstreamAccount{Name: "unsupported", AccountType: "other"}, true); !errors.Is(err, ErrUpstreamAccountTypeUnsupported) {
		t.Fatalf("expected unsupported-type error, got %v", err)
	}
}

func TestSaveUpstreamAccountEncryptsAndPreservesSecret(t *testing.T) {
	db := setupUpstreamAccountTestDB(t)
	allowUpstreamAccountTestPrivateFetch(t)

	saved, err := SaveUpstreamAccount(UpstreamAccount{
		Name:                "primary",
		AccountType:         UpstreamAccountTypeNewAPI,
		BaseURL:             "https://api.example.com/",
		UserID:              7,
		AccessToken:         "token-secret-value",
		Enabled:             true,
		ResourceDisplayMode: UpstreamAccountResourceDisplayBoth,
	})
	if err != nil {
		t.Fatalf("save account: %v", err)
	}
	if saved.AccessTokenMasked != "toke****alue" {
		t.Fatalf("unexpected token mask: %s", saved.AccessTokenMasked)
	}

	stored := UpstreamAccount{}
	if err := db.First(&stored, saved.Id).Error; err != nil {
		t.Fatalf("load account: %v", err)
	}
	if stored.AccessTokenEncrypted == "" || stored.AccessTokenEncrypted == "token-secret-value" {
		t.Fatalf("token was not encrypted: %q", stored.AccessTokenEncrypted)
	}
	plain, err := decryptUpstreamAccountRemoteSecret(stored.AccessTokenEncrypted)
	if err != nil || plain != "token-secret-value" {
		t.Fatalf("decrypt token: value=%q err=%v", plain, err)
	}

	updated, err := SaveUpstreamAccount(UpstreamAccount{
		Id:                  saved.Id,
		Name:                "primary-updated",
		AccountType:         UpstreamAccountTypeNewAPI,
		BaseURL:             "https://api.example.com",
		UserID:              7,
		Enabled:             true,
		ResourceDisplayMode: UpstreamAccountResourceDisplayWallet,
	})
	if err != nil {
		t.Fatalf("update account without token: %v", err)
	}
	if updated.AccessTokenMasked != saved.AccessTokenMasked {
		t.Fatalf("token was not preserved: before=%s after=%s", saved.AccessTokenMasked, updated.AccessTokenMasked)
	}
}

func TestUpstreamAccountSnapshotDeltaByAccountType(t *testing.T) {
	newAPIDelta, warnings := upstreamAccountRemoteSnapshotDeltaForConfig(
		UpstreamAccountRemoteConfig{AccountType: UpstreamAccountTypeNewAPI},
		UpstreamAccountSnapshot{WalletUsedQuota: 100},
		UpstreamAccountSnapshot{WalletUsedQuota: 160},
	)
	if newAPIDelta != 60 || len(warnings) != 0 {
		t.Fatalf("unexpected new-api delta: delta=%d warnings=%v", newAPIDelta, warnings)
	}

	sub2Delta, warnings := upstreamAccountRemoteSnapshotDeltaForConfig(
		UpstreamAccountRemoteConfig{AccountType: UpstreamAccountTypeSub2API},
		UpstreamAccountSnapshot{WalletQuota: 600},
		UpstreamAccountSnapshot{WalletQuota: 520},
	)
	if sub2Delta != 80 || len(warnings) != 0 {
		t.Fatalf("unexpected sub2api delta: delta=%d warnings=%v", sub2Delta, warnings)
	}

	rollbackDelta, warnings := upstreamAccountRemoteSnapshotDeltaForConfig(
		UpstreamAccountRemoteConfig{AccountType: UpstreamAccountTypeNewAPI},
		UpstreamAccountSnapshot{WalletUsedQuota: 160},
		UpstreamAccountSnapshot{WalletUsedQuota: 120},
	)
	if rollbackDelta != 0 || len(warnings) != 1 {
		t.Fatalf("unexpected rollback handling: delta=%d warnings=%v", rollbackDelta, warnings)
	}
}

func TestGetUpstreamAccountTrendAggregatesSnapshots(t *testing.T) {
	setupUpstreamAccountTestDB(t)
	allowUpstreamAccountTestPrivateFetch(t)

	originQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1000
	t.Cleanup(func() {
		common.QuotaPerUnit = originQuotaPerUnit
	})

	saved, err := SaveUpstreamAccount(UpstreamAccount{
		Name:                "trend-account",
		AccountType:         UpstreamAccountTypeNewAPI,
		BaseURL:             "https://api.example.com",
		UserID:              8,
		AccessToken:         "trend-token",
		Enabled:             true,
		ResourceDisplayMode: UpstreamAccountResourceDisplayBoth,
	})
	if err != nil {
		t.Fatalf("save account: %v", err)
	}
	account, err := getUpstreamAccountByID(saved.Id)
	if err != nil {
		t.Fatalf("load account: %v", err)
	}
	config := account.remoteObserverConfig()
	signature := upstreamAccountSnapshotSignature(account.Id)
	first := time.Date(2026, time.July, 27, 10, 0, 0, 0, time.Local).Unix()
	second := time.Date(2026, time.July, 28, 10, 0, 0, 0, time.Local).Unix()
	third := time.Date(2026, time.July, 29, 10, 0, 0, 0, time.Local).Unix()
	seedUpstreamAccountSnapshot(t, signature, config, first, 500, 100, nil)
	seedUpstreamAccountSnapshot(t, signature, config, second, 500, 160, nil)
	seedUpstreamAccountSnapshot(t, signature, config, third, 500, 210, []UpstreamAccountSubscriptionSnapshot{{
		SubscriptionID: 1,
		PlanID:         2,
		AmountTotal:    1000,
		AmountUsed:     250,
		EndTime:        third + 86400,
		Status:         "active",
	}})

	trend, err := GetUpstreamAccountTrend(saved.Id, first-60, third+60, "day", 0)
	if err != nil {
		t.Fatalf("get trend: %v", err)
	}
	if math.Abs(trend.Account.PeriodUsedUSD-0.11) > 0.000001 {
		t.Fatalf("unexpected period usage: %+v", trend.Account)
	}
	if !trend.Account.BaselineReady || trend.Account.SnapshotCount != 3 {
		t.Fatalf("unexpected baseline state: %+v", trend.Account)
	}
	if len(trend.Points) != 2 {
		t.Fatalf("unexpected trend points: %+v", trend.Points)
	}
	pointTotal := 0.0
	for _, point := range trend.Points {
		pointTotal += point.PeriodUsedUSD
	}
	if math.Abs(pointTotal-0.11) > 0.000001 {
		t.Fatalf("unexpected trend total: %.6f", pointTotal)
	}
	if len(trend.Subscriptions) != 1 || math.Abs(trend.Subscriptions[0].RemainingQuotaUSD-0.75) > 0.000001 {
		t.Fatalf("unexpected subscriptions: %+v", trend.Subscriptions)
	}
}

func TestDeleteUpstreamAccountRemovesOnlyItsSnapshots(t *testing.T) {
	db := setupUpstreamAccountTestDB(t)
	allowUpstreamAccountTestPrivateFetch(t)

	saved, err := SaveUpstreamAccount(UpstreamAccount{
		Name:        "delete-account",
		AccountType: UpstreamAccountTypeNewAPI,
		BaseURL:     "https://api.example.com",
		UserID:      9,
		AccessToken: "delete-token",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("save account: %v", err)
	}
	targetSignature := upstreamAccountSnapshotSignature(saved.Id)
	for _, signature := range []string{targetSignature, "profit_board_account:other"} {
		if err := db.Create(&UpstreamAccountSnapshot{
			SelectionSignature: signature,
			ComboId:            upstreamAccountSnapshotComboID,
			ConfigHash:         "hash",
			Status:             upstreamAccountRemoteSnapshotStatusSuccess,
			SyncedAt:           common.GetTimestamp(),
		}).Error; err != nil {
			t.Fatalf("seed snapshot: %v", err)
		}
	}

	if err := DeleteUpstreamAccount(saved.Id); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	if err := db.First(&UpstreamAccount{}, saved.Id).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("account still exists or query failed: %v", err)
	}
	var targetCount int64
	if err := db.Model(&UpstreamAccountSnapshot{}).Where("selection_signature = ?", targetSignature).Count(&targetCount).Error; err != nil {
		t.Fatalf("count target snapshots: %v", err)
	}
	if targetCount != 0 {
		t.Fatalf("target snapshots were not deleted: %d", targetCount)
	}
	var unrelatedCount int64
	if err := db.Model(&UpstreamAccountSnapshot{}).Where("selection_signature = ?", "profit_board_account:other").Count(&unrelatedCount).Error; err != nil {
		t.Fatalf("count unrelated snapshots: %v", err)
	}
	if unrelatedCount != 1 {
		t.Fatalf("unrelated snapshots changed: %d", unrelatedCount)
	}
}
