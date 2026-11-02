package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetCashAccountingConfig() (CashAccountingConfig, error) {
	config := CashAccountingConfig{ID: 1}
	err := DB.First(&config, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return config, nil
	}
	return config, err
}

func cashHasAccountHistory(tx *gorm.DB, id int) (bool, error) {
	for _, table := range []any{&CashAccountBinding{}, &CashUpstreamEntry{}, &CashDayCoverage{}} {
		var count int64
		if err := tx.Model(table).Where("account_id = ?", id).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

// A rotated management credential must still own every historical invocation
// key. Financial account identity cannot silently change with a credential edit.
func validateCashCredentialRotation(tx *gorm.DB, account *UpstreamAccount) error {
	var bindings []CashAccountBinding
	if err := tx.Where("account_id = ?", account.Id).Find(&bindings).Error; err != nil {
		return err
	}
	if len(bindings) == 0 {
		return errors.New("请先关联此账户的调用密钥，再更换管理凭据以核实账户归属")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	remote, err := cashRemoteKeyHashes(ctx, newOpenRouterAccountClient(), account)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		if !remote[binding.KeyHash] {
			return errors.New("新管理凭据不拥有原账户的调用密钥，不能合并财务记录")
		}
	}
	return nil
}

func EnableCashAccounting(actor int) (CashAccountingConfig, error) {
	config := CashAccountingConfig{ID: 1, EnabledAt: common.GetTimestamp(), EnabledBy: actor}
	if actor <= 0 {
		return config, errors.New("无效的操作人")
	}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&config).Error; err != nil {
		return config, err
	}
	config, err := GetCashAccountingConfig()
	if err == nil {
		cashAccountingEnabledAt.Store(config.EnabledAt)
	}
	return config, err
}

var cashServiceOnce sync.Once

func StartCashAccountingService() {
	cashServiceOnce.Do(func() {
		refresh := func() {
			config, err := GetCashAccountingConfig()
			if err == nil {
				cashAccountingEnabledAt.Store(config.EnabledAt)
			}
		}
		refresh()
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				refresh()
				if cashAccountingEnabledAt.Load() == 0 {
					continue
				}
				var bindings []CashAccountBinding
				if err := DB.Find(&bindings).Error; err != nil {
					continue
				}
				seen := map[int]bool{}
				for _, b := range bindings {
					if seen[b.AccountID] {
						continue
					}
					seen[b.AccountID] = true
					var status CashAccountSync
					if err := DB.Where("account_id = ?", b.AccountID).First(&status).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
						continue
					}
					if status.LastAttempt > common.GetTimestamp()-900 {
						continue
					}
					ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
					_, _ = SyncCashAccount(ctx, b.AccountID)
					cancel()
				}
			}
		}()
	})
}

type CashBindingCandidate struct {
	KeyHash  string              `json:"key_hash"`
	Channels []CashChannelLabel  `json:"channels"`
	Binding  *CashAccountBinding `json:"binding"`
}
type CashChannelLabel struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func cashAccountForRead(id int) (*UpstreamAccount, error) {
	account, err := getUpstreamAccountByID(id)
	if err != nil {
		return nil, err
	}
	if account.AccountType != UpstreamAccountTypeOpenRouter {
		return nil, ErrUpstreamAccountTypeUnsupported
	}
	return account, nil
}

func cashRemoteKeyHashes(ctx context.Context, client *http.Client, account *UpstreamAccount) (map[string]bool, error) {
	remote := map[string]bool{}
	for offset := 0; offset <= 10000; {
		var response struct {
			Data *[]struct {
				Hash string `json:"hash"`
			} `json:"data"`
		}
		if err := openRouterAccountRequest(ctx, client, account.remoteObserverConfig(), fmt.Sprintf("/api/v1/keys?include_disabled=true&include_expired=true&offset=%d", offset), &response); err != nil {
			return nil, err
		}
		if response.Data == nil {
			return nil, errors.New("OpenRouter keys missing")
		}
		rows := *response.Data
		if len(rows) == 0 {
			break
		}
		fresh := 0
		for _, row := range rows {
			if !openRouterKeyHashPattern.MatchString(row.Hash) {
				return nil, errors.New("OpenRouter key metadata invalid")
			}
			if !remote[row.Hash] {
				fresh++
			}
			remote[row.Hash] = true
		}
		if fresh == 0 {
			return nil, errors.New("OpenRouter key pagination did not advance")
		}
		offset += len(rows)
		if offset >= 10000 {
			return nil, errors.New("OpenRouter 密钥数量超出单次核对上限")
		}
	}
	return remote, nil
}

func cashKeyCandidates(ctx context.Context, client *http.Client, account *UpstreamAccount) ([]CashBindingCandidate, error) {
	if !account.Enabled {
		return nil, errors.New("OpenRouter account disabled")
	}
	remote, err := cashRemoteKeyHashes(ctx, client, account)
	if err != nil {
		return nil, err
	}
	var channels []Channel
	if err := DB.Select("id", "name", "key", "channel_info").Where("type = ?", constant.ChannelTypeOpenRouter).Find(&channels).Error; err != nil {
		return nil, err
	}
	var bindings []CashAccountBinding
	if err := DB.Where("account_id = ?", account.Id).Find(&bindings).Error; err != nil {
		return nil, err
	}
	byKey := map[string]*CashBindingCandidate{}
	for i := range channels {
		ch := &channels[i]
		seen := map[string]bool{}
		for _, key := range ch.GetKeys() {
			hash := cashKeyHash(key)
			if !remote[hash] || seen[hash] {
				continue
			}
			seen[hash] = true
			if byKey[hash] == nil {
				byKey[hash] = &CashBindingCandidate{KeyHash: hash, Channels: []CashChannelLabel{}}
			}
			byKey[hash].Channels = append(byKey[hash].Channels, CashChannelLabel{ID: ch.Id, Name: ch.Name})
		}
	}
	for i := range bindings {
		b := &bindings[i]
		if byKey[b.KeyHash] == nil {
			byKey[b.KeyHash] = &CashBindingCandidate{KeyHash: b.KeyHash, Channels: []CashChannelLabel{}}
		}
		byKey[b.KeyHash].Binding = b
	}
	result := make([]CashBindingCandidate, 0, len(byKey))
	for _, v := range byKey {
		result = append(result, *v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].KeyHash < result[j].KeyHash })
	return result, nil
}

func GetCashBindingCandidates(ctx context.Context, accountID int) ([]CashBindingCandidate, error) {
	a, err := cashAccountForRead(accountID)
	if err != nil {
		return nil, err
	}
	return cashKeyCandidates(ctx, newOpenRouterAccountClient(), a)
}

func SaveCashBinding(ctx context.Context, accountID, actor int, keyHash string, siteOnly bool) (*CashAccountBinding, error) {
	if cashAccountingEnabledAt.Load() == 0 {
		return nil, errors.New("请先启用现金收益核算")
	}
	if actor <= 0 || !openRouterKeyHashPattern.MatchString(keyHash) {
		return nil, errors.New("无效的密钥指纹")
	}
	candidates, err := GetCashBindingCandidates(ctx, accountID)
	if err != nil {
		return nil, err
	}
	matched := false
	for _, c := range candidates {
		if c.KeyHash == keyHash && len(c.Channels) > 0 {
			matched = true
		}
	}
	if !matched {
		return nil, errors.New("该密钥无法同时关联此 OpenRouter 账户和本站渠道")
	}
	result := CashAccountBinding{KeyHash: keyHash, AccountID: accountID, SiteOnly: siteOnly, CreatedAt: common.GetTimestamp(), CreatedBy: actor}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if _, err := cashLockAccount(tx, accountID); err != nil {
			return err
		}
		var old CashAccountBinding
		err := tx.Where("key_hash = ?", keyHash).First(&old).Error
		if err == nil {
			if old.AccountID != accountID {
				return errors.New("该推理密钥已绑定其他账户，不能重复归集收入和成本")
			}
			result = old
			if old.SiteOnly == siteOnly {
				return nil
			}
			result.SiteOnly = siteOnly
			if err = tx.Save(&result).Error; err != nil {
				return err
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			var count int64
			if err = tx.Model(&CashAccountBinding{}).Where("account_id = ?", accountID).Count(&count).Error; err != nil {
				return err
			}
			if count >= 10 {
				return errors.New("每个账户最多绑定十个推理密钥")
			}
			if err = tx.Create(&result).Error; err != nil {
				return err
			}
		} else {
			return err
		}
		return tx.Create(&CashBindingChange{AccountID: accountID, KeyHash: keyHash, SiteOnly: siteOnly, CreatedBy: actor, CreatedAt: common.GetTimestamp()}).Error
	})
	return &result, err
}

func cashFetchActivity(ctx context.Context, client *http.Client, config UpstreamAccountRemoteConfig, hash string) ([]OpenRouterAccountActivity, error) {
	path := "/api/v1/activity"
	if hash != "" {
		path += "?api_key_hash=" + hash
	}
	var response struct {
		Data *[]OpenRouterAccountActivity `json:"data"`
	}
	if err := openRouterAccountRequest(ctx, client, config, path, &response); err != nil {
		return nil, err
	}
	if response.Data == nil {
		return nil, errors.New("OpenRouter activity missing")
	}
	for i := range *response.Data {
		r := &(*response.Data)[i]
		day, err := normalizeOpenRouterActivityDate(r.Date)
		if err != nil || r.Usage == nil || *r.Usage < 0 || r.BYOKUsageInference < 0 || r.Requests < 0 || len(r.Model) > 255 || len(r.ProviderName) > 128 || len(r.EndpointID) > 128 {
			return nil, errors.New("OpenRouter activity invalid")
		}
		r.Date = day
	}
	return *response.Data, nil
}

func SyncCashAccount(ctx context.Context, accountID int) (*CashAccountSync, error) {
	account, err := cashAccountForRead(accountID)
	if err != nil {
		return nil, err
	}
	if !account.Enabled {
		return nil, errors.New("OpenRouter account disabled")
	}
	now := common.GetTimestamp()
	status := CashAccountSync{AccountID: accountID}
	if err = DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&status).Error; err != nil {
		return nil, err
	}
	token := common.GetUUID()
	lease := DB.Model(&CashAccountSync{}).Where("account_id = ? AND lease_until <= ?", accountID, now).Updates(map[string]any{"lease_until": now + 300, "lease_token": token, "last_attempt": now})
	if lease.Error != nil {
		return nil, lease.Error
	}
	if lease.RowsAffected != 1 {
		return nil, errors.New("财务同步正在进行，请稍后刷新")
	}
	err = syncCashAccountData(ctx, newOpenRouterAccountClient(), account, now)
	updates := map[string]any{"lease_until": 0, "lease_token": "", "error": ""}
	if err != nil {
		updates["error"] = "OpenRouter 财务同步失败，保留上次成功数据"
	} else {
		updates["last_success"] = now
	}
	if finishErr := DB.Model(&CashAccountSync{}).Where("account_id = ? AND lease_token = ?", accountID, token).Updates(updates).Error; finishErr != nil {
		return nil, finishErr
	}
	if readErr := DB.Where("account_id = ?", accountID).First(&status).Error; readErr != nil {
		return nil, readErr
	}
	return &status, err
}

func syncCashAccountData(ctx context.Context, client *http.Client, account *UpstreamAccount, now int64) error {
	var bindings []CashAccountBinding
	if err := DB.Where("account_id = ?", account.Id).Find(&bindings).Error; err != nil {
		return err
	}
	if len(bindings) > 10 {
		return errors.New("too many cash scopes")
	}
	if len(bindings) > 0 {
		remote, err := cashRemoteKeyHashes(ctx, client, account)
		if err != nil {
			return err
		}
		for _, b := range bindings {
			if !remote[b.KeyHash] {
				return errors.New("已关联的调用密钥不再属于此管理账户，请核对凭据")
			}
		}
	}
	today, _ := time.Parse("2006-01-02", cashDay(now))
	start := today.AddDate(0, 0, -30).Format("2006-01-02")
	hashes := []string{""}
	for _, b := range bindings {
		hashes = append(hashes, b.KeyHash)
	}
	rows := map[string]CashActivityDay{}
	coverage := []CashDayCoverage{}
	for _, hash := range hashes {
		activity, err := cashFetchActivity(ctx, client, account.remoteObserverConfig(), hash)
		if err != nil {
			return err
		}
		for _, r := range activity {
			if r.Date < start || r.Date >= today.Format("2006-01-02") {
				return errors.New("OpenRouter activity outside completed UTC window")
			}
			usage, err := cashScaled(decimal.NewFromFloat(*r.Usage), 9)
			if err != nil {
				return err
			}
			byok, err := cashScaled(decimal.NewFromFloat(r.BYOKUsageInference), 9)
			if err != nil {
				return err
			}
			id := cashHash(fmt.Sprint(account.Id), hash, r.Date, r.Model, r.ProviderName, r.EndpointID)
			row := rows[id]
			row.ID, row.AccountID, row.KeyHash, row.Date, row.Model, row.Provider, row.EndpointID, row.SyncedAt = id, account.Id, hash, r.Date, r.Model, r.ProviderName, r.EndpointID, now
			if row.UsageNanos > 1e18-usage || row.BYOKNanos > 1e18-byok || row.Requests > 1e15-r.Requests {
				return errors.New("OpenRouter activity totals too large")
			}
			row.UsageNanos += usage
			row.BYOKNanos += byok
			row.Requests += r.Requests
			rows[id] = row
		}
		for d := today.AddDate(0, 0, -30); d.Before(today); d = d.AddDate(0, 0, 1) {
			day := d.Format("2006-01-02")
			coverage = append(coverage, CashDayCoverage{ID: cashHash(fmt.Sprint(account.Id), hash, day), AccountID: account.Id, KeyHash: hash, Date: day, SyncedAt: now})
		}
	}
	items := make([]CashActivityDay, 0, len(rows))
	for _, r := range rows {
		items = append(items, r)
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := cashLockAccount(tx, account.Id); err != nil {
			return err
		}
		if err := tx.Where("account_id = ? AND date >= ? AND date < ?", account.Id, start, today.Format("2006-01-02")).Delete(&CashActivityDay{}).Error; err != nil {
			return err
		}
		if len(items) > 0 {
			if err := tx.CreateInBatches(items, 100).Error; err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"synced_at"})}).CreateInBatches(coverage, 100).Error
	})
}

type CashEntryInput struct {
	RequestID   string `json:"request_id"`
	Kind        string `json:"kind"`
	Date        string `json:"date"`
	Credits     string `json:"credits"`
	Currency    string `json:"currency"`
	GrossAmount string `json:"gross_amount"`
	FeeAmount   string `json:"fee_amount"`
	FXRate      string `json:"fx_rate"`
	Reference   string `json:"reference"`
	Evidence    string `json:"evidence"`
}

func SaveCashUpstreamEntry(accountID, actor int, input CashEntryInput) (*CashUpstreamEntry, error) {
	if !validWalletRedemptionRequestID(input.RequestID) || actor <= 0 {
		return nil, errors.New("无效的账单请求")
	}
	if _, err := time.Parse("2006-01-02", input.Date); err != nil || input.Date > cashDay(common.GetTimestamp()) {
		return nil, errors.New("请选择有效的 UTC 账单日期")
	}
	switch input.Kind {
	case "opening", "purchase", "refund", "expense", "byok":
	default:
		return nil, errors.New("无效的账单类型")
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	input.Reference = strings.TrimSpace(input.Reference)
	input.Evidence = strings.TrimSpace(input.Evidence)
	if input.Reference == "" || len(input.Reference) > 128 || input.Evidence == "" || len(input.Evidence) > 255 {
		return nil, errors.New("请填写账单编号和凭证引用")
	}
	credits, err := cashDecimal(input.Credits)
	if err != nil {
		return nil, err
	}
	nanos, err := cashScaled(credits, 9)
	if err != nil {
		return nil, err
	}
	if (input.Kind == "purchase" || input.Kind == "refund") && nanos == 0 {
		return nil, errors.New("购入或退回的 credits 必须大于零")
	}
	if (input.Kind == "expense" || input.Kind == "byok") && nanos != 0 {
		return nil, errors.New("直接费用不应填写 credits")
	}
	cost, err := cashConvert(input.GrossAmount, input.FeeAmount, input.Currency, input.FXRate, input.Kind == "refund")
	if err != nil {
		return nil, err
	}
	if input.Kind == "opening" && nanos == 0 && cost != 0 {
		return nil, errors.New("零期初 credits 的剩余现金基础必须为零")
	}
	payload, _ := common.Marshal(input)
	row := CashUpstreamEntry{AccountID: accountID, RequestKey: cashHash(fmt.Sprint(actor), input.RequestID), PayloadHash: cashHash(fmt.Sprint(accountID), string(payload)), Kind: input.Kind, Date: input.Date, CreditsNanos: nanos,
		Currency: input.Currency, GrossAmount: input.GrossAmount, FeeAmount: input.FeeAmount, FXRate: input.FXRate, CostCNYMicros: cost, Reference: input.Reference, Evidence: input.Evidence, CreatedBy: actor, CreatedAt: common.GetTimestamp()}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if _, err := cashLockAccount(tx, accountID); err != nil {
			return err
		}
		var old CashUpstreamEntry
		err := tx.Where("request_key = ?", row.RequestKey).First(&old).Error
		if err == nil {
			if old.PayloadHash != row.PayloadHash {
				return errors.New("重复请求的账单内容不一致")
			}
			row = old
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err = tx.Model(&CashUpstreamEntry{}).Where("account_id = ? AND reference = ? AND voided_at = 0", accountID, input.Reference).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("该账单编号已录入，请勿重复记账")
		}
		if input.Kind == "opening" {
			if err = tx.Model(&CashUpstreamEntry{}).Where("account_id = ? AND kind = ? AND date = ? AND voided_at = 0", accountID, "opening", input.Date).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("该日期已有期初记录，更正时请先作废原记录")
			}
		}
		return tx.Create(&row).Error
	})
	return &row, err
}

func VoidCashUpstreamEntry(accountID int, id int64, actor int, reason string) error {
	reason = strings.TrimSpace(reason)
	if actor <= 0 || reason == "" || len(reason) > 255 {
		return errors.New("请填写作废原因")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := cashLockAccount(tx, accountID); err != nil {
			return err
		}
		var row CashUpstreamEntry
		if err := tx.Where("id = ? AND account_id = ?", id, accountID).First(&row).Error; err != nil {
			return err
		}
		if row.VoidedAt > 0 {
			return nil
		}
		return tx.Model(&row).Updates(map[string]any{"voided_at": common.GetTimestamp(), "voided_by": actor, "void_reason": reason}).Error
	})
}

func ListCashUpstreamEntries(accountID, offset, limit int) ([]CashUpstreamEntry, int64, error) {
	var total int64
	var rows []CashUpstreamEntry
	q := DB.Model(&CashUpstreamEntry{}).Where("account_id = ?", accountID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("date desc, id desc").Offset(max(offset, 0)).Limit(min(max(limit, 1), 100)).Find(&rows).Error
	if rows == nil {
		rows = []CashUpstreamEntry{}
	}
	return rows, total, err
}
