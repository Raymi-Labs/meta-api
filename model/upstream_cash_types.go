package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Cash accounting is an opt-in analytical ledger. It never changes quota prices,
// Pulse paid-allowance eligibility, routing, or upstream payment instructions.
var cashAccountingEnabledAt atomic.Int64

type CashAccountingConfig struct {
	ID        int   `json:"id" gorm:"primaryKey"`
	EnabledAt int64 `json:"enabled_at"`
	EnabledBy int   `json:"enabled_by"`
}

type CashWalletState struct {
	UserID int   `gorm:"primaryKey;autoIncrement:false"`
	Quota  int64 `gorm:"not null"`
	Epoch  int64 `gorm:"not null"`
}

type CashFundingLot struct {
	ID             int64  `json:"id"`
	UserID         int    `json:"user_id" gorm:"index:idx_cash_lot_wallet,priority:1;not null"`
	SourceType     string `json:"source_type" gorm:"type:varchar(32);not null"`
	SourceRef      string `json:"source_ref" gorm:"type:varchar(255);not null"`
	SourceKey      string `json:"-" gorm:"type:char(64);uniqueIndex;not null"`
	TotalQuota     int64  `json:"total_quota" gorm:"not null"`
	RemainingQuota int64  `json:"remaining_quota" gorm:"not null"`
	Epoch          int64  `json:"epoch" gorm:"index:idx_cash_lot_wallet,priority:2;not null"`
	ReceiptID      int64  `json:"receipt_id" gorm:"not null;default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"index;not null"`
}

// Receipt revisions are immutable. Lot.ReceiptID selects the latest audited
// basis; old receipts remain queryable and every report identifies its basis.
type CashFundingReceipt struct {
	ID           int64  `json:"id"`
	LotID        int64  `json:"lot_id" gorm:"index;not null"`
	PreviousID   int64  `json:"previous_id"`
	RequestKey   string `json:"-" gorm:"type:char(64);uniqueIndex;not null"`
	PayloadHash  string `json:"-" gorm:"type:char(64);not null"`
	Currency     string `json:"currency" gorm:"type:varchar(3);not null"`
	GrossAmount  string `json:"gross_amount" gorm:"type:varchar(32);not null"`
	FeeAmount    string `json:"fee_amount" gorm:"type:varchar(32);not null"`
	FXRate       string `json:"fx_rate" gorm:"type:varchar(32);not null"`
	NetCNYMicros int64  `json:"net_cny_micros" gorm:"not null"`
	ReceivedAt   int64  `json:"received_at" gorm:"index;not null"`
	Evidence     string `json:"evidence" gorm:"type:varchar(255);not null"`
	Note         string `json:"note" gorm:"type:varchar(255);not null"`
	CreatedBy    int    `json:"created_by" gorm:"not null"`
	CreatedAt    int64  `json:"created_at" gorm:"not null"`
}

type CashFundingAllocation struct {
	ID            int64  `json:"id"`
	ReservationID string `json:"reservation_id" gorm:"type:varchar(64);index;not null"`
	UserID        int    `json:"user_id" gorm:"index;not null"`
	LotID         int64  `json:"lot_id" gorm:"index;not null"`
	Quota         int64  `json:"quota" gorm:"not null"`
	Epoch         int64  `json:"epoch" gorm:"not null"`
	Status        string `json:"status" gorm:"type:varchar(16);index;not null"`
	CreatedAt     int64  `json:"created_at" gorm:"not null"`
}

// Minimal durable usage metadata; no prompts, user names, IPs, API secrets or
// upstream bodies. Logging retention and LogConsumeEnabled do not govern it.
type CashUsage struct {
	ID            int64  `json:"id"`
	RequestID     string `json:"request_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	ReservationID string `json:"reservation_id" gorm:"type:varchar(64);index;not null"`
	UserID        int    `json:"user_id" gorm:"index;not null"`
	ChannelID     int    `json:"channel_id" gorm:"index;not null"`
	KeyHash       string `json:"-" gorm:"type:char(64);index:idx_cash_usage_key_time,priority:1;not null"`
	Model         string `json:"model" gorm:"type:varchar(255);not null"`
	FundingSource string `json:"funding_source" gorm:"type:varchar(32);not null"`
	Quota         int64  `json:"quota" gorm:"not null"`
	OccurredAt    int64  `json:"occurred_at" gorm:"index:idx_cash_usage_key_time,priority:2;index;not null"`
}

type CashAccountBinding struct {
	KeyHash   string `json:"key_hash" gorm:"primaryKey;type:char(64)"`
	AccountID int    `json:"account_id" gorm:"index;not null"`
	SiteOnly  bool   `json:"site_only" gorm:"not null"`
	CreatedBy int    `json:"created_by" gorm:"not null"`
	CreatedAt int64  `json:"created_at" gorm:"not null"`
}

type CashBindingChange struct {
	ID        int64  `json:"id"`
	AccountID int    `json:"account_id" gorm:"index;not null"`
	KeyHash   string `json:"key_hash" gorm:"type:char(64);not null"`
	SiteOnly  bool   `json:"site_only" gorm:"not null"`
	CreatedBy int    `json:"created_by" gorm:"not null"`
	CreatedAt int64  `json:"created_at" gorm:"not null"`
}

// Day rows include a total-account scope (empty KeyHash) and selected key
// scopes. Empty days have coverage rows too, so failure is never confused with 0.
type CashActivityDay struct {
	ID         string `json:"id" gorm:"primaryKey;type:char(64)"`
	AccountID  int    `json:"account_id" gorm:"index:idx_cash_day_account_date,priority:1;not null"`
	KeyHash    string `json:"-" gorm:"type:varchar(64);not null"`
	Date       string `json:"date" gorm:"type:varchar(10);index:idx_cash_day_account_date,priority:2;not null"`
	Model      string `json:"model" gorm:"type:varchar(255);not null"`
	Provider   string `json:"provider" gorm:"type:varchar(128);not null"`
	EndpointID string `json:"endpoint_id" gorm:"type:varchar(128);not null"`
	UsageNanos int64  `json:"usage_nanos" gorm:"not null"`
	BYOKNanos  int64  `json:"byok_nanos" gorm:"not null"`
	Requests   int64  `json:"requests" gorm:"not null"`
	SyncedAt   int64  `json:"synced_at" gorm:"not null"`
}

type CashDayCoverage struct {
	ID        string `gorm:"primaryKey;type:char(64)"`
	AccountID int    `gorm:"index:idx_cash_coverage_account_date,priority:1;not null"`
	KeyHash   string `gorm:"type:varchar(64);not null"`
	Date      string `gorm:"type:varchar(10);index:idx_cash_coverage_account_date,priority:2;not null"`
	SyncedAt  int64  `gorm:"not null"`
}

type CashAccountSync struct {
	AccountID   int    `json:"account_id" gorm:"primaryKey;autoIncrement:false"`
	LastAttempt int64  `json:"last_attempt"`
	LastSuccess int64  `json:"last_success"`
	Error       string `json:"error" gorm:"type:varchar(255)"`
	LeaseUntil  int64  `json:"-"`
	LeaseToken  string `json:"-" gorm:"type:varchar(64)"`
}

type CashUpstreamEntry struct {
	ID            int64  `json:"id"`
	AccountID     int    `json:"account_id" gorm:"index:idx_cash_entry_account_date,priority:1;not null"`
	RequestKey    string `json:"-" gorm:"type:char(64);uniqueIndex;not null"`
	PayloadHash   string `json:"-" gorm:"type:char(64);not null"`
	Kind          string `json:"kind" gorm:"type:varchar(16);not null"` // opening/purchase/refund/expense/byok
	Date          string `json:"date" gorm:"type:varchar(10);index:idx_cash_entry_account_date,priority:2;not null"`
	CreditsNanos  int64  `json:"credits_nanos" gorm:"not null"`
	Currency      string `json:"currency" gorm:"type:varchar(3);not null"`
	GrossAmount   string `json:"gross_amount" gorm:"type:varchar(32);not null"`
	FeeAmount     string `json:"fee_amount" gorm:"type:varchar(32);not null"`
	FXRate        string `json:"fx_rate" gorm:"type:varchar(32);not null"`
	CostCNYMicros int64  `json:"cost_cny_micros" gorm:"not null"`
	Reference     string `json:"reference" gorm:"type:varchar(128);not null"`
	Evidence      string `json:"evidence" gorm:"type:varchar(255);not null"`
	CreatedBy     int    `json:"created_by" gorm:"not null"`
	CreatedAt     int64  `json:"created_at" gorm:"not null"`
	VoidedBy      int    `json:"voided_by"`
	VoidedAt      int64  `json:"voided_at" gorm:"not null;default:0"`
	VoidReason    string `json:"void_reason" gorm:"type:varchar(255)"`
}

func cashAccountingModels() []any {
	return []any{&CashAccountingConfig{}, &CashWalletState{}, &CashFundingLot{}, &CashFundingReceipt{}, &CashFundingAllocation{}, &CashUsage{}, &CashAccountBinding{}, &CashBindingChange{}, &CashActivityDay{}, &CashDayCoverage{}, &CashAccountSync{}, &CashUpstreamEntry{}}
}

func cashHash(parts ...string) string {
	encoded, _ := common.Marshal(parts)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func cashKeyHash(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
}

var cashDecimalPattern = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,9})?$`)
var cashCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func cashDecimal(s string) (decimal.Decimal, error) {
	if !cashDecimalPattern.MatchString(s) {
		return decimal.Zero, errors.New("金额格式无效，最多支持九位小数")
	}
	return decimal.NewFromString(s)
}

func cashScaled(d decimal.Decimal, scale int32) (int64, error) {
	v := d.Shift(scale).Round(0)
	if v.Abs().GreaterThan(decimal.NewFromInt(1_000_000_000_000_000_000)) {
		return 0, errors.New("金额超出允许范围")
	}
	return v.IntPart(), nil
}

func cashConvert(gross, fee, currency, rate string, income bool) (int64, error) {
	g, err := cashDecimal(gross)
	if err != nil {
		return 0, err
	}
	f, err := cashDecimal(fee)
	if err != nil {
		return 0, err
	}
	r, err := cashDecimal(rate)
	if err != nil {
		return 0, err
	}
	if !cashCurrencyPattern.MatchString(currency) || !r.IsPositive() || (currency == "CNY" && !r.Equal(decimal.NewFromInt(1))) {
		return 0, errors.New("币种或实际汇率无效")
	}
	if income {
		if f.GreaterThan(g) {
			return 0, errors.New("支付手续费不能超过实收金额")
		}
		g = g.Sub(f)
	} else {
		g = g.Add(f)
	}
	return cashScaled(g.Mul(r), 6)
}

func cashMulDiv(a, b, divisor int64) int64 {
	if divisor <= 0 {
		return 0
	}
	v := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	v.Quo(v, big.NewInt(divisor))
	return v.Int64() // callers bound inputs and the quotient to stored monetary limits
}

func cashDay(t int64) string          { return time.Unix(t, 0).UTC().Format("2006-01-02") }
func cashAmount(micros int64) float64 { return float64(micros) / 1e6 }

func cashLockAccount(tx *gorm.DB, id int) (*UpstreamAccount, error) {
	if common.UsingSQLite {
		if err := tx.Model(&UpstreamAccount{}).Where("id = ?", id).UpdateColumn("updated_at", gorm.Expr("updated_at")).Error; err != nil {
			return nil, err
		}
	}
	var account UpstreamAccount
	q := tx.Where("id = ?", id)
	if !common.UsingSQLite {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.First(&account).Error; err != nil {
		return nil, err
	}
	if account.AccountType != UpstreamAccountTypeOpenRouter {
		return nil, ErrUpstreamAccountTypeUnsupported
	}
	return &account, nil
}
