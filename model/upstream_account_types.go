package model

import (
	"fmt"
	"math"
	"time"
)

const (
	UpstreamAccountTypeNewAPI                  = "newapi"
	UpstreamAccountTypeSub2API                 = "sub2api"
	UpstreamAccountTypeOpenRouter              = "openrouter"
	openRouterAccountBaseURL                   = "https://openrouter.ai"
	UpstreamAccountResourceDisplayBoth         = "both"
	UpstreamAccountResourceDisplayWallet       = "wallet"
	UpstreamAccountResourceDisplaySubscription = "subscription"
)

type UpstreamAccountRemoteConfig struct {
	Enabled              bool   `json:"enabled,omitempty"`
	AccountType          string `json:"account_type,omitempty"`
	BaseURL              string `json:"base_url,omitempty"`
	UserID               int    `json:"user_id,omitempty"`
	AccessToken          string `json:"access_token,omitempty"`
	AccessTokenMasked    string `json:"access_token_masked,omitempty"`
	AccessTokenEncrypted string `json:"access_token_encrypted,omitempty"`
	Email                string `json:"email,omitempty"`
	Password             string `json:"password,omitempty"`
	PasswordMasked       string `json:"password_masked,omitempty"`
	PasswordEncrypted    string `json:"password_encrypted,omitempty"`
}

type UpstreamAccountSubscriptionSnapshot struct {
	ID             int    `json:"id,omitempty"`
	SubscriptionID int    `json:"subscription_id"`
	PlanID         int    `json:"plan_id"`
	AmountTotal    int64  `json:"amount_total"`
	AmountUsed     int64  `json:"amount_used"`
	LastResetTime  int64  `json:"last_reset_time"`
	NextResetTime  int64  `json:"next_reset_time"`
	StartTime      int64  `json:"start_time"`
	EndTime        int64  `json:"end_time"`
	Status         string `json:"status,omitempty"`
}

type UpstreamAccountState struct {
	BatchId                      string  `json:"batch_id"`
	BatchName                    string  `json:"batch_name"`
	Enabled                      bool    `json:"enabled"`
	Configured                   bool    `json:"configured"`
	Status                       string  `json:"status"`
	ErrorMessage                 string  `json:"error_message,omitempty"`
	LastSyncedAt                 int64   `json:"last_synced_at"`
	LastSuccessAt                int64   `json:"last_success_at"`
	PeriodUsedUSD                float64 `json:"period_used_usd"`
	WalletBalanceUSD             float64 `json:"wallet_balance_usd"`
	WalletQuotaUSD               float64 `json:"wallet_quota_usd"`
	WalletUsedTotalUSD           float64 `json:"wallet_used_total_usd"`
	WalletUsedQuotaUSD           float64 `json:"wallet_used_quota_usd"`
	SubscriptionRemainingUSD     float64 `json:"subscription_remaining_quota_usd"`
	SubscriptionTotalQuotaUSD    float64 `json:"subscription_total_quota_usd"`
	SubscriptionUsedQuotaUSD     float64 `json:"subscription_used_quota_usd"`
	SubscriptionCount            int     `json:"subscription_count"`
	SubscriptionEarliestExpireAt int64   `json:"subscription_earliest_expire_at"`
	HasSubscriptionData          bool    `json:"has_subscription_data"`
	SubscriptionHasUnlimited     bool    `json:"subscription_has_unlimited"`
	RemoteQuotaPerUnit           float64 `json:"remote_quota_per_unit"`
	QuotaPerUnitMismatch         bool    `json:"quota_per_unit_mismatch"`
	BaselineReady                bool    `json:"baseline_ready"`
	SnapshotCount                int     `json:"snapshot_count"`
}

type UpstreamAccountSubscription struct {
	SubscriptionID    int     `json:"subscription_id"`
	PlanID            int     `json:"plan_id"`
	TotalQuotaUSD     float64 `json:"total_quota_usd"`
	UsedQuotaUSD      float64 `json:"used_quota_usd"`
	RemainingQuotaUSD float64 `json:"remaining_quota_usd"`
	HasUnlimited      bool    `json:"has_unlimited"`
	LastResetTime     int64   `json:"last_reset_time"`
	NextResetTime     int64   `json:"next_reset_time"`
	StartTime         int64   `json:"start_time"`
	EndTime           int64   `json:"end_time"`
	Status            string  `json:"status,omitempty"`
}

type UpstreamAccountTrendPoint struct {
	Bucket          string  `json:"bucket"`
	BucketTimestamp int64   `json:"bucket_timestamp"`
	PeriodUsedUSD   float64 `json:"period_used_usd"`
}

type UpstreamAccountTrend struct {
	Account               UpstreamAccountOption         `json:"account"`
	Points                []UpstreamAccountTrendPoint   `json:"points"`
	Subscriptions         []UpstreamAccountSubscription `json:"subscriptions,omitempty"`
	StartTimestamp        int64                         `json:"start_timestamp"`
	EndTimestamp          int64                         `json:"end_timestamp"`
	Granularity           string                        `json:"granularity"`
	CustomIntervalMinutes int                           `json:"custom_interval_minutes"`
	Warnings              []string                      `json:"warnings,omitempty"`
}

type UpstreamAccountSnapshotSubject struct {
	Id   string
	Name string
}

func roundUpstreamAccountAmount(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func buildUpstreamAccountBucket(timestamp int64, granularity string, customIntervalMinutes int) (int64, string) {
	t := time.Unix(timestamp, 0).In(time.Local)
	switch granularity {
	case "hour":
		bucket := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
		return bucket.Unix(), bucket.Format("2006-01-02 15:00")
	case "month":
		bucket := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
		return bucket.Unix(), bucket.Format("2006-01")
	case "week":
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		bucket := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).AddDate(0, 0, -(weekday - 1))
		year, week := bucket.ISOWeek()
		return bucket.Unix(), fmt.Sprintf("%d-W%02d", year, week)
	case "custom":
		minutes := customIntervalMinutes
		if minutes <= 0 {
			minutes = 15
		}
		current := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())
		minutesSinceMidnight := current.Hour()*60 + current.Minute()
		bucketMinutes := (minutesSinceMidnight / minutes) * minutes
		bucket := time.Date(current.Year(), current.Month(), current.Day(), bucketMinutes/60, bucketMinutes%60, 0, 0, current.Location())
		return bucket.Unix(), bucket.Format("2006-01-02 15:04")
	default:
		bucket := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		return bucket.Unix(), bucket.Format("2006-01-02")
	}
}
