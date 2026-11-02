package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// Sentinel errors for i18n translation (upstream account)
var (
	ErrUpstreamAccountTypeUnsupported = errors.New("upstream_account:account_type_unsupported")
	ErrUpstreamAccountNameEmpty       = errors.New("upstream_account:account_name_empty")
	ErrUpstreamAccountInvalid         = errors.New("upstream_account:account_invalid")
	ErrUpstreamAccountTokenEmpty      = errors.New("upstream_account:account_token_empty")
	ErrUpstreamAccountEmailEmpty      = errors.New("upstream_account:account_email_empty")
	ErrUpstreamAccountPasswordEmpty   = errors.New("upstream_account:account_password_empty")
)

const upstreamAccountSnapshotComboID = "wallet"

type UpstreamAccount struct {
	Id                   int    `json:"id"`
	Name                 string `json:"name" gorm:"type:varchar(128);not null"`
	Remark               string `json:"remark,omitempty" gorm:"type:text"`
	AccountType          string `json:"account_type" gorm:"type:varchar(24);index;not null"`
	BaseURL              string `json:"base_url" gorm:"type:varchar(255);not null"`
	UserID               int    `json:"user_id" gorm:"index;not null"`
	Email                string `json:"email,omitempty" gorm:"type:varchar(255);index"`
	AccessToken          string `json:"access_token,omitempty" gorm:"-"`
	AccessTokenMasked    string `json:"access_token_masked,omitempty" gorm:"-"`
	AccessTokenEncrypted string `json:"-" gorm:"type:text;not null"`
	Password             string `json:"password,omitempty" gorm:"-"`
	PasswordMasked       string `json:"password_masked,omitempty" gorm:"-"`
	PasswordEncrypted    string `json:"-" gorm:"type:text"`
	Enabled              bool   `json:"enabled" gorm:"default:true"`
	ResourceDisplayMode  string `json:"resource_display_mode" gorm:"type:varchar(24);default:both"`
	CreatedAt            int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt            int64  `json:"updated_at" gorm:"bigint;index"`
}

func (UpstreamAccount) TableName() string {
	return "profit_board_upstream_accounts"
}

type UpstreamAccountOption struct {
	Id                           int     `json:"id"`
	Name                         string  `json:"name"`
	Remark                       string  `json:"remark,omitempty"`
	AccountType                  string  `json:"account_type"`
	BaseURL                      string  `json:"base_url"`
	UserID                       int     `json:"user_id"`
	Email                        string  `json:"email,omitempty"`
	Enabled                      bool    `json:"enabled"`
	ResourceDisplayMode          string  `json:"resource_display_mode"`
	AccessTokenMasked            string  `json:"access_token_masked,omitempty"`
	PasswordMasked               string  `json:"password_masked,omitempty"`
	Status                       string  `json:"status,omitempty"`
	ErrorMessage                 string  `json:"error_message,omitempty"`
	LastSyncedAt                 int64   `json:"last_synced_at"`
	LastSuccessAt                int64   `json:"last_success_at"`
	WalletBalanceUSD             float64 `json:"wallet_balance_usd"`
	WalletQuotaUSD               float64 `json:"wallet_quota_usd"`
	WalletUsedTotalUSD           float64 `json:"wallet_used_total_usd"`
	WalletUsedQuotaUSD           float64 `json:"wallet_used_quota_usd"`
	PeriodUsedUSD                float64 `json:"period_used_usd"`
	SubscriptionRemainingUSD     float64 `json:"subscription_remaining_quota_usd"`
	SubscriptionTotalQuotaUSD    float64 `json:"subscription_total_quota_usd"`
	SubscriptionUsedQuotaUSD     float64 `json:"subscription_used_quota_usd"`
	SubscriptionCount            int     `json:"subscription_count"`
	SubscriptionEarliestExpireAt int64   `json:"subscription_earliest_expire_at"`
	HasSubscriptionData          bool    `json:"has_subscription_data"`
	SubscriptionHasUnlimited     bool    `json:"subscription_has_unlimited"`
	ObservedCostUSD              float64 `json:"observed_cost_usd"`
	RemoteQuotaPerUnit           float64 `json:"remote_quota_per_unit"`
	QuotaPerUnitMismatch         bool    `json:"quota_per_unit_mismatch"`
	BaselineReady                bool    `json:"baseline_ready"`
	SnapshotCount                int     `json:"snapshot_count"`
}

type upstreamAccountSubscriptionSummary struct {
	RemainingUSD     float64
	TotalUSD         float64
	UsedUSD          float64
	Count            int
	EarliestExpireAt int64
	HasData          bool
	HasUnlimited     bool
	Details          []UpstreamAccountSubscription
}

func summarizeUpstreamAccountSubscriptions(subscriptions []UpstreamAccountSubscriptionSnapshot) upstreamAccountSubscriptionSummary {
	summary := upstreamAccountSubscriptionSummary{
		Details: make([]UpstreamAccountSubscription, 0, len(subscriptions)),
	}
	if len(subscriptions) == 0 {
		return summary
	}
	summary.HasData = true
	summary.Count = len(subscriptions)
	for _, item := range subscriptions {
		usedUSD := upstreamAccountQuotaToUSD(item.AmountUsed)
		totalUSD := 0.0
		remainingUSD := 0.0
		hasUnlimited := item.AmountTotal <= 0
		if hasUnlimited {
			summary.HasUnlimited = true
		} else {
			totalUSD = upstreamAccountQuotaToUSD(item.AmountTotal)
			remainingQuota := item.AmountTotal - item.AmountUsed
			if remainingQuota < 0 {
				remainingQuota = 0
			}
			remainingUSD = upstreamAccountQuotaToUSD(remainingQuota)
			summary.TotalUSD += totalUSD
			summary.RemainingUSD += remainingUSD
		}
		summary.UsedUSD += usedUSD
		if item.EndTime > 0 && (summary.EarliestExpireAt == 0 || item.EndTime < summary.EarliestExpireAt) {
			summary.EarliestExpireAt = item.EndTime
		}
		summary.Details = append(summary.Details, UpstreamAccountSubscription{
			SubscriptionID:    item.SubscriptionID,
			PlanID:            item.PlanID,
			TotalQuotaUSD:     roundUpstreamAccountAmount(totalUSD),
			UsedQuotaUSD:      roundUpstreamAccountAmount(usedUSD),
			RemainingQuotaUSD: roundUpstreamAccountAmount(remainingUSD),
			HasUnlimited:      hasUnlimited,
			LastResetTime:     item.LastResetTime,
			NextResetTime:     item.NextResetTime,
			StartTime:         item.StartTime,
			EndTime:           item.EndTime,
			Status:            item.Status,
		})
	}
	summary.TotalUSD = roundUpstreamAccountAmount(summary.TotalUSD)
	summary.UsedUSD = roundUpstreamAccountAmount(summary.UsedUSD)
	summary.RemainingUSD = roundUpstreamAccountAmount(summary.RemainingUSD)
	sort.Slice(summary.Details, func(i, j int) bool {
		left := summary.Details[i]
		right := summary.Details[j]
		if left.EndTime == 0 && right.EndTime != 0 {
			return false
		}
		if left.EndTime != 0 && right.EndTime == 0 {
			return true
		}
		if left.EndTime != right.EndTime {
			return left.EndTime < right.EndTime
		}
		return left.SubscriptionID < right.SubscriptionID
	})
	return summary
}

func (a *UpstreamAccount) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if a.CreatedAt <= 0 {
		a.CreatedAt = now
	}
	if a.UpdatedAt <= 0 {
		a.UpdatedAt = now
	}
	return nil
}

func (a *UpstreamAccount) BeforeUpdate(tx *gorm.DB) error {
	a.UpdatedAt = common.GetTimestamp()
	return nil
}

func normalizeUpstreamAccountResourceDisplayMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case UpstreamAccountResourceDisplayWallet:
		return UpstreamAccountResourceDisplayWallet
	case UpstreamAccountResourceDisplaySubscription:
		return UpstreamAccountResourceDisplaySubscription
	default:
		return UpstreamAccountResourceDisplayBoth
	}
}

func normalizeUpstreamAccount(account UpstreamAccount) UpstreamAccount {
	account.Name = strings.TrimSpace(account.Name)
	account.Remark = strings.TrimSpace(account.Remark)
	account.AccountType = strings.ToLower(strings.TrimSpace(account.AccountType))
	if account.AccountType == "" {
		account.AccountType = UpstreamAccountTypeNewAPI
	}
	account.BaseURL = strings.TrimRight(strings.TrimSpace(account.BaseURL), "/")
	account.Email = strings.TrimSpace(account.Email)
	account.AccessToken = strings.TrimSpace(account.AccessToken)
	account.Password = strings.TrimSpace(account.Password)
	if account.UserID < 0 {
		account.UserID = 0
	}
	account.ResourceDisplayMode = normalizeUpstreamAccountResourceDisplayMode(account.ResourceDisplayMode)
	if account.AccountType == UpstreamAccountTypeOpenRouter {
		account.BaseURL = openRouterAccountBaseURL
	}
	if account.AccountType == UpstreamAccountTypeSub2API || account.AccountType == UpstreamAccountTypeOpenRouter {
		account.UserID = 0
		account.ResourceDisplayMode = UpstreamAccountResourceDisplayWallet
	}
	return account
}

func validateUpstreamAccount(account UpstreamAccount, requireSecret bool) error {
	account = normalizeUpstreamAccount(account)
	if account.Name == "" {
		return ErrUpstreamAccountNameEmpty
	}
	switch account.AccountType {
	case UpstreamAccountTypeNewAPI, UpstreamAccountTypeOpenRouter:
		config := UpstreamAccountRemoteConfig{
			Enabled:              true,
			AccountType:          account.AccountType,
			BaseURL:              account.BaseURL,
			UserID:               account.UserID,
			AccessToken:          account.AccessToken,
			AccessTokenEncrypted: account.AccessTokenEncrypted,
		}
		if !requireSecret {
			config.AccessToken = ""
		}
		if err := validateUpstreamAccountRemoteConfig(config); err != nil {
			return err
		}
	case UpstreamAccountTypeSub2API:
		if account.Email == "" {
			return ErrUpstreamAccountEmailEmpty
		}
		config := UpstreamAccountRemoteConfig{
			Enabled:           true,
			AccountType:       UpstreamAccountTypeSub2API,
			BaseURL:           account.BaseURL,
			Email:             account.Email,
			Password:          account.Password,
			PasswordEncrypted: account.PasswordEncrypted,
		}
		if !requireSecret {
			config.Password = ""
		}
		if err := validateUpstreamAccountRemoteConfig(config); err != nil {
			if errors.Is(err, ErrUpstreamAccountRemoteMissingPassword) {
				return ErrUpstreamAccountPasswordEmpty
			}
			return err
		}
	default:
		return ErrUpstreamAccountTypeUnsupported
	}
	return nil
}

func (a UpstreamAccount) remoteObserverConfig() UpstreamAccountRemoteConfig {
	a = normalizeUpstreamAccount(a)
	config := UpstreamAccountRemoteConfig{
		Enabled:           a.Enabled,
		AccountType:       a.AccountType,
		BaseURL:           a.BaseURL,
		UserID:            a.UserID,
		Email:             a.Email,
		Password:          a.Password,
		PasswordEncrypted: a.PasswordEncrypted,
	}
	if a.AccountType == UpstreamAccountTypeNewAPI || a.AccountType == UpstreamAccountTypeOpenRouter {
		config.AccessToken = a.AccessToken
		config.AccessTokenEncrypted = a.AccessTokenEncrypted
	}
	return normalizeUpstreamAccountRemoteConfig(config)
}

func upstreamAccountSnapshotSignature(accountID int) string {
	return fmt.Sprintf("profit_board_account:%d", accountID)
}

func upstreamAccountSnapshotBatch(account UpstreamAccount) UpstreamAccountSnapshotSubject {
	return UpstreamAccountSnapshotSubject{
		Id:   upstreamAccountSnapshotComboID,
		Name: account.Name,
	}
}

func buildUpstreamAccountOption(
	account UpstreamAccount,
	state UpstreamAccountState,
) UpstreamAccountOption {
	account = normalizeUpstreamAccount(account)
	return UpstreamAccountOption{
		Id:                           account.Id,
		Name:                         account.Name,
		Remark:                       account.Remark,
		AccountType:                  account.AccountType,
		BaseURL:                      account.BaseURL,
		UserID:                       account.UserID,
		Email:                        account.Email,
		Enabled:                      account.Enabled,
		ResourceDisplayMode:          account.ResourceDisplayMode,
		AccessTokenMasked:            maskUpstreamAccountRemoteSecret(statefulUpstreamAccountToken(account)),
		PasswordMasked:               maskUpstreamAccountRemoteSecret(statefulUpstreamAccountPassword(account)),
		Status:                       state.Status,
		ErrorMessage:                 state.ErrorMessage,
		LastSyncedAt:                 state.LastSyncedAt,
		LastSuccessAt:                state.LastSuccessAt,
		WalletBalanceUSD:             state.WalletBalanceUSD,
		WalletQuotaUSD:               state.WalletQuotaUSD,
		WalletUsedTotalUSD:           state.WalletUsedTotalUSD,
		WalletUsedQuotaUSD:           state.WalletUsedQuotaUSD,
		PeriodUsedUSD:                state.PeriodUsedUSD,
		SubscriptionRemainingUSD:     state.SubscriptionRemainingUSD,
		SubscriptionTotalQuotaUSD:    state.SubscriptionTotalQuotaUSD,
		SubscriptionUsedQuotaUSD:     state.SubscriptionUsedQuotaUSD,
		SubscriptionCount:            state.SubscriptionCount,
		SubscriptionEarliestExpireAt: state.SubscriptionEarliestExpireAt,
		HasSubscriptionData:          state.HasSubscriptionData,
		SubscriptionHasUnlimited:     state.SubscriptionHasUnlimited,
		ObservedCostUSD:              state.PeriodUsedUSD,
		RemoteQuotaPerUnit:           state.RemoteQuotaPerUnit,
		QuotaPerUnitMismatch:         state.QuotaPerUnitMismatch,
		BaselineReady:                state.BaselineReady,
		SnapshotCount:                state.SnapshotCount,
	}
}

func statefulUpstreamAccountToken(account UpstreamAccount) string {
	if account.AccessToken != "" {
		return account.AccessToken
	}
	if account.AccessTokenEncrypted == "" {
		return ""
	}
	decrypted, err := decryptUpstreamAccountRemoteSecret(account.AccessTokenEncrypted)
	if err != nil {
		return ""
	}
	return decrypted
}

func statefulUpstreamAccountPassword(account UpstreamAccount) string {
	if account.Password != "" {
		return account.Password
	}
	if account.PasswordEncrypted == "" {
		return ""
	}
	decrypted, err := decryptUpstreamAccountRemoteSecret(account.PasswordEncrypted)
	if err != nil {
		return ""
	}
	return decrypted
}

func getUpstreamAccountByID(id int) (*UpstreamAccount, error) {
	if id <= 0 {
		return nil, ErrUpstreamAccountInvalid
	}
	account := &UpstreamAccount{}
	if err := DB.First(account, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return account, nil
}

func listUpstreamAccounts() ([]UpstreamAccount, error) {
	accounts := make([]UpstreamAccount, 0)
	if err := DB.Order("updated_at desc, id desc").Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func buildUpstreamAccountState(account UpstreamAccount, periodUsedUSD float64) (UpstreamAccountState, error) {
	signature := upstreamAccountSnapshotSignature(account.Id)
	config := account.remoteObserverConfig()
	batch := upstreamAccountSnapshotBatch(account)
	latestAny, err := getLatestUpstreamAccountSnapshot(signature, upstreamAccountSnapshotComboID)
	if err != nil {
		return UpstreamAccountState{}, err
	}
	latestSuccess, err := getLatestUpstreamAccountRemoteSuccessSnapshot(signature, upstreamAccountSnapshotComboID, upstreamAccountRemoteObserverConfigHash(config))
	if err != nil {
		return UpstreamAccountState{}, err
	}
	return buildUpstreamAccountSnapshotState(signature, batch, config, latestAny, latestSuccess, periodUsedUSD), nil
}

func GetUpstreamAccountOptions() ([]UpstreamAccountOption, error) {
	accounts, err := listUpstreamAccounts()
	if err != nil {
		return nil, err
	}
	options := make([]UpstreamAccountOption, 0, len(accounts))
	for _, account := range accounts {
		state, stateErr := buildUpstreamAccountState(account, 0)
		if stateErr != nil {
			return nil, stateErr
		}
		options = append(options, buildUpstreamAccountOption(account, state))
	}
	sortUpstreamAccountOptions(options)
	return options, nil
}

func SaveUpstreamAccount(account UpstreamAccount) (*UpstreamAccountOption, error) {
	account = normalizeUpstreamAccount(account)
	var existing UpstreamAccount
	if account.Id > 0 {
		if err := DB.First(&existing, "id = ?", account.Id).Error; err != nil {
			return nil, err
		}
		existing = normalizeUpstreamAccount(existing)
		if account.AccessTokenEncrypted == "" {
			account.AccessTokenEncrypted = existing.AccessTokenEncrypted
		}
		if account.PasswordEncrypted == "" {
			account.PasswordEncrypted = existing.PasswordEncrypted
		}
	}
	requireSecret := account.Id == 0 ||
		((account.AccountType == UpstreamAccountTypeNewAPI || account.AccountType == UpstreamAccountTypeOpenRouter) && (account.AccessToken != "" || account.AccessTokenEncrypted == "")) ||
		(account.AccountType == UpstreamAccountTypeSub2API && (account.Password != "" || account.PasswordEncrypted == ""))
	if err := validateUpstreamAccount(account, requireSecret); err != nil {
		return nil, err
	}
	if err := prepareUpstreamAccountSecrets(&account, existing); err != nil {
		return nil, err
	}
	account.AccessToken = ""
	account.Password = ""
	if account.Id == 0 {
		if err := DB.Create(&account).Error; err != nil {
			return nil, err
		}
	} else {
		existing.Name = account.Name
		existing.Remark = account.Remark
		existing.AccountType = account.AccountType
		existing.BaseURL = account.BaseURL
		existing.UserID = account.UserID
		existing.Email = account.Email
		existing.AccessTokenEncrypted = account.AccessTokenEncrypted
		existing.PasswordEncrypted = account.PasswordEncrypted
		existing.Enabled = account.Enabled
		existing.ResourceDisplayMode = account.ResourceDisplayMode
		if err := DB.Save(&existing).Error; err != nil {
			return nil, err
		}
		account = existing
	}
	state, err := buildUpstreamAccountState(account, 0)
	if err != nil {
		return nil, err
	}
	option := buildUpstreamAccountOption(account, state)
	return &option, nil
}

func prepareUpstreamAccountSecrets(account *UpstreamAccount, existing UpstreamAccount) error {
	switch account.AccountType {
	case UpstreamAccountTypeNewAPI, UpstreamAccountTypeOpenRouter:
		account.PasswordEncrypted = ""
		account.Email = ""
		switch {
		case account.AccessToken != "":
			encrypted, err := encryptUpstreamAccountRemoteSecret(account.AccessToken)
			if err != nil {
				return err
			}
			account.AccessTokenEncrypted = encrypted
		case existing.AccountType == account.AccountType && existing.AccessTokenEncrypted != "":
			account.AccessTokenEncrypted = existing.AccessTokenEncrypted
		default:
			return ErrUpstreamAccountTokenEmpty
		}
	case UpstreamAccountTypeSub2API:
		account.AccessTokenEncrypted = ""
		account.UserID = 0
		account.ResourceDisplayMode = UpstreamAccountResourceDisplayWallet
		switch {
		case account.Password != "":
			encrypted, err := encryptUpstreamAccountRemoteSecret(account.Password)
			if err != nil {
				return err
			}
			account.PasswordEncrypted = encrypted
		case existing.AccountType == UpstreamAccountTypeSub2API && existing.PasswordEncrypted != "":
			account.PasswordEncrypted = existing.PasswordEncrypted
		default:
			return ErrUpstreamAccountPasswordEmpty
		}
	default:
		return ErrUpstreamAccountTypeUnsupported
	}
	return nil
}

func DeleteUpstreamAccount(id int) error {
	account, err := getUpstreamAccountByID(id)
	if err != nil {
		return err
	}
	signature := upstreamAccountSnapshotSignature(account.Id)
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("selection_signature = ?", signature).Delete(&UpstreamAccountSnapshot{}).Error; err != nil {
			return err
		}
		return tx.Delete(account).Error
	})
}

func SyncUpstreamAccount(id int, force bool) (*UpstreamAccountOption, error) {
	account, err := getUpstreamAccountByID(id)
	if err != nil {
		return nil, err
	}
	config := account.remoteObserverConfig()
	config.Enabled = true
	signature := upstreamAccountSnapshotSignature(account.Id)
	batch := upstreamAccountSnapshotBatch(*account)
	latestAny, latestSuccess, err := syncUpstreamAccountRemoteObserverSnapshot(signature, batch, config, force)
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	periodUsedUSD, usageErr := calculateUpstreamAccountPeriodUsedUSD(
		*account,
		latestSuccess,
		now-7*24*60*60,
		now,
	)
	if usageErr != nil {
		return nil, usageErr
	}
	state := buildUpstreamAccountSnapshotState(
		signature,
		batch,
		config,
		latestAny,
		latestSuccess,
		periodUsedUSD,
	)
	option := buildUpstreamAccountOption(*account, state)
	return &option, nil
}

func SyncAllUpstreamAccounts(force bool) ([]UpstreamAccountOption, error) {
	accounts, err := listUpstreamAccounts()
	if err != nil {
		return nil, err
	}
	options := make([]UpstreamAccountOption, 0, len(accounts))
	for _, account := range accounts {
		if account.Enabled {
			option, syncErr := SyncUpstreamAccount(account.Id, force)
			if syncErr != nil {
				return nil, syncErr
			}
			options = append(options, *option)
			continue
		}
		state, stateErr := buildUpstreamAccountState(account, 0)
		if stateErr != nil {
			return nil, stateErr
		}
		options = append(options, buildUpstreamAccountOption(account, state))
	}
	sortUpstreamAccountOptions(options)
	return options, nil
}

func calculateUpstreamAccountPeriodUsedUSD(
	account UpstreamAccount,
	latestSuccess *UpstreamAccountSnapshot,
	startTimestamp int64,
	endTimestamp int64,
) (float64, error) {
	config := account.remoteObserverConfig()
	config.Enabled = true
	signature := upstreamAccountSnapshotSignature(account.Id)
	configHash := upstreamAccountRemoteObserverConfigHash(config)
	if configHash == "" && latestSuccess != nil {
		configHash = latestSuccess.ConfigHash
	}
	effectiveEndTimestamp := endTimestamp
	if latestSuccess != nil && latestSuccess.SyncedAt > effectiveEndTimestamp {
		effectiveEndTimestamp = latestSuccess.SyncedAt
	}
	snapshots, err := listUpstreamAccountRemoteSuccessSnapshots(
		signature,
		upstreamAccountSnapshotComboID,
		configHash,
		startTimestamp,
		effectiveEndTimestamp,
	)
	if err != nil {
		return 0, err
	}
	if len(snapshots) < 2 || common.QuotaPerUnit <= 0 {
		return 0, nil
	}
	totalUsedQuota := int64(0)
	for index := 1; index < len(snapshots); index++ {
		deltaQuota, _ := upstreamAccountRemoteSnapshotDeltaForConfig(config, snapshots[index-1], snapshots[index])
		if deltaQuota > 0 {
			totalUsedQuota += deltaQuota
		}
	}
	return roundUpstreamAccountAmount(float64(totalUsedQuota) / common.QuotaPerUnit), nil
}

func GetUpstreamAccountTrend(id int, startTimestamp int64, endTimestamp int64, granularity string, customIntervalMinutes int) (*UpstreamAccountTrend, error) {
	now := common.GetTimestamp()
	if endTimestamp <= 0 {
		endTimestamp = now
	}
	if startTimestamp <= 0 || startTimestamp >= endTimestamp {
		startTimestamp = endTimestamp - 7*24*60*60
	}
	if strings.TrimSpace(granularity) == "" {
		granularity = "day"
	}
	if customIntervalMinutes <= 0 {
		customIntervalMinutes = 15
	}
	account, err := getUpstreamAccountByID(id)
	if err != nil {
		return nil, err
	}
	config := account.remoteObserverConfig()
	signature := upstreamAccountSnapshotSignature(account.Id)
	configHash := upstreamAccountRemoteObserverConfigHash(config)
	if configHash == "" {
		latestSuccess, latestErr := getLatestUpstreamAccountRemoteSuccessSnapshot(signature, upstreamAccountSnapshotComboID, "")
		if latestErr != nil {
			return nil, latestErr
		}
		if latestSuccess != nil {
			configHash = latestSuccess.ConfigHash
		}
	}
	snapshots, err := listUpstreamAccountRemoteSuccessSnapshots(signature, upstreamAccountSnapshotComboID, configHash, startTimestamp, endTimestamp)
	if err != nil {
		return nil, err
	}
	if len(snapshots) < 2 {
		snapshots, err = listUpstreamAccountRemoteSuccessSnapshots(signature, upstreamAccountSnapshotComboID, configHash, 0, endTimestamp)
		if err != nil {
			return nil, err
		}
	}
	bucketUsedUSD := make(map[int64]float64)
	bucketLabels := make(map[int64]string)
	warnings := make([]string, 0)
	totalUsedUSD := 0.0
	if len(snapshots) >= 2 {
		for index := 1; index < len(snapshots); index++ {
			deltaQuota, deltaWarnings := upstreamAccountRemoteSnapshotDeltaForConfig(config, snapshots[index-1], snapshots[index])
			for _, warning := range deltaWarnings {
				warnings = append(warnings, fmt.Sprintf("%s：%s", account.Name, warning))
			}
			if deltaQuota <= 0 {
				continue
			}
			periodUsedUSD := float64(deltaQuota) / common.QuotaPerUnit
			totalUsedUSD += periodUsedUSD
			bucketTimestamp, bucketLabel := buildUpstreamAccountBucket(snapshots[index].SyncedAt, granularity, customIntervalMinutes)
			bucketUsedUSD[bucketTimestamp] += periodUsedUSD
			bucketLabels[bucketTimestamp] = bucketLabel
		}
	}
	state, err := buildUpstreamAccountState(*account, roundUpstreamAccountAmount(totalUsedUSD))
	if err != nil {
		return nil, err
	}
	option := buildUpstreamAccountOption(*account, state)
	subscriptionSummary := summarizeUpstreamAccountSubscriptions(nil)
	if latestSuccess, latestErr := getLatestUpstreamAccountRemoteSuccessSnapshot(signature, upstreamAccountSnapshotComboID, configHash); latestErr != nil {
		return nil, latestErr
	} else if latestSuccess != nil {
		subscriptionSummary = summarizeUpstreamAccountSubscriptions(parseUpstreamAccountRemoteSubscriptions(latestSuccess.SubscriptionStates))
	}
	points := make([]UpstreamAccountTrendPoint, 0, len(bucketUsedUSD))
	for bucketTimestamp, periodUsedUSD := range bucketUsedUSD {
		points = append(points, UpstreamAccountTrendPoint{
			Bucket:          bucketLabels[bucketTimestamp],
			BucketTimestamp: bucketTimestamp,
			PeriodUsedUSD:   roundUpstreamAccountAmount(periodUsedUSD),
		})
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].BucketTimestamp < points[j].BucketTimestamp
	})
	return &UpstreamAccountTrend{
		Account:               option,
		Points:                points,
		Subscriptions:         subscriptionSummary.Details,
		StartTimestamp:        startTimestamp,
		EndTimestamp:          endTimestamp,
		Granularity:           granularity,
		CustomIntervalMinutes: customIntervalMinutes,
		Warnings:              uniqueUpstreamAccountWarnings(warnings),
	}, nil
}

func sortUpstreamAccountOptions(options []UpstreamAccountOption) {
	sort.Slice(options, func(i, j int) bool {
		if options[i].Enabled != options[j].Enabled {
			return options[i].Enabled
		}
		return options[i].Name < options[j].Name
	})
}
