package model

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	QuotaNoticeWindow     = int64(24 * time.Hour / time.Millisecond)
	quotaNoticeMinute     = int64(time.Minute / time.Millisecond)
	QuotaNoticePending    = "pending"
	QuotaNoticeSending    = "sending"
	QuotaNoticeSubmitted  = "submitted"
	QuotaNoticeUnknown    = "unknown"
	QuotaNoticeFailed     = "failed"
	QuotaNoticeRetry      = "retry_wait"
	QuotaNoticeSkipped    = "skipped"
	QuotaNoticeSuppressed = "suppressed"
)

// One durable lock and cadence per user, including every funding resource.
type QuotaNoticeAccount struct {
	UserID          int `gorm:"primaryKey;autoIncrement:false"`
	Revision        int64
	Resources       string `gorm:"type:text"`
	Stage           int
	LastSubmittedAt int64
	NextCheckAt     int64 `gorm:"index"`
	LastActivityAt  int64 `gorm:"index"`
}

type QuotaNoticeResource struct {
	Scope      string   `json:"scope"`
	Source     string   `json:"source"`
	ResourceID int      `json:"resource_id"`
	FirstAt    int64    `json:"first_at"`
	LastAt     int64    `json:"last_at"`
	SuccessAt  int64    `json:"success_at"`
	Count      int64    `json:"count"`
	Available  int64    `json:"available"`
	Required   int64    `json:"required"`
	Known      bool     `json:"known"`
	TokenID    int      `json:"token_id"`
	TokenName  string   `json:"token_name"`
	ModelName  string   `json:"model_name"`
	RequestID  string   `json:"request_id"`
	TokenIDs   []int    `json:"token_ids,omitempty"`
	ModelNames []string `json:"model_names,omitempty"`
}

// Short lived, indexed deduplication and sliding-window fallback. Never query
// the unbounded consumption log to decide whether a notification is eligible.
type QuotaNoticeSignal struct {
	ID     string `gorm:"primaryKey;type:char(64)"`
	UserID int    `gorm:"index:idx_quota_signal_window,priority:1"`
	At     int64  `gorm:"index:idx_quota_signal_window,priority:2;index"`
	Scope  string `gorm:"type:varchar(96)"`
}

type QuotaNotice struct {
	ID          int    `json:"id"`
	UserID      int    `json:"user_id" gorm:"index:idx_quota_notice_user,priority:1"`
	Status      string `json:"status" gorm:"type:varchar(24);index:idx_quota_notice_queue,priority:1"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime:false;index:idx_quota_notice_user,priority:2;index"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime:false"`
	SlotAt      int64  `json:"-"`
	DueAt       int64  `json:"next_attempt_at" gorm:"index:idx_quota_notice_queue,priority:2"`
	LeaseUntil  int64  `json:"-"`
	Attempts    int    `json:"attempts"`
	SubmittedAt int64  `json:"submitted_at"`
	Recipient   string `json:"recipient" gorm:"type:varchar(128)"`
	Medium      string `json:"medium" gorm:"type:varchar(16)"`
	Reason      string `json:"reason" gorm:"type:varchar(128)"`
	Summary     string `json:"summary" gorm:"type:text"`
}

type QuotaNoticeWindowStats struct{ Count, FirstAt, LastAt int64 }
type QuotaNoticeStatsReader func(userID int, r QuotaNoticeResource, since, now int64) (QuotaNoticeWindowStats, bool)

func withQuotaNoticeAccount(userID int, fn func(*gorm.DB, *QuotaNoticeAccount) error) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		a := QuotaNoticeAccount{UserID: userID, Resources: "{}"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&a).Error; err != nil {
			return err
		}
		// A real write serializes this user's claims on all three SQL engines,
		// including SQLite (which does not support SELECT FOR UPDATE).
		if err := tx.Model(&QuotaNoticeAccount{}).Where("user_id = ?", userID).UpdateColumn("revision", gorm.Expr("revision + 1")).Error; err != nil {
			return err
		}
		if err := tx.First(&a, "user_id = ?", userID).Error; err != nil {
			return err
		}
		if err := fn(tx, &a); err != nil {
			return err
		}
		return tx.Save(&a).Error
	})
}

func quotaNoticeResources(a *QuotaNoticeAccount) (map[string]QuotaNoticeResource, error) {
	resources := map[string]QuotaNoticeResource{}
	if a.Resources != "" {
		if err := common.UnmarshalJsonStr(a.Resources, &resources); err != nil {
			return nil, err
		}
	}
	return resources, nil
}

func saveQuotaNoticeResources(a *QuotaNoticeAccount, resources map[string]QuotaNoticeResource) error {
	b, err := common.Marshal(resources)
	if err != nil {
		return err
	}
	// MySQL TEXT has a 64 KiB byte limit, independent of character count.
	// Keep recent evidence within the same portable TEXT contract on every DB.
	if len(b) > 56*1024 {
		for key, r := range resources {
			r.TokenIDs = nil
			r.ModelNames = nil
			resources[key] = r
		}
		b, err = common.Marshal(resources)
		if err != nil {
			return err
		}
	}
	for len(b) > 56*1024 && len(resources) > 1 {
		var oldest string
		var at int64
		for key, r := range resources {
			if oldest == "" || r.LastAt < at {
				oldest = key
				at = r.LastAt
			}
		}
		delete(resources, oldest)
		b, err = common.Marshal(resources)
		if err != nil {
			return err
		}
	}
	if len(b) > 56*1024 {
		return fmt.Errorf("quota notice evidence exceeds byte budget")
	}
	a.Resources = string(b)
	return nil
}

func pruneQuotaNoticeResources(resources map[string]QuotaNoticeResource, now int64) {
	for k, r := range resources {
		if now-r.LastAt >= 48*60*quotaNoticeMinute || (r.SuccessAt > r.LastAt && now-r.LastAt >= 10*quotaNoticeMinute) {
			delete(resources, k)
		}
	}
}

// RecordQuotaNoticeSignal receives an immutable snapshot, never a Gin context.
func RecordQuotaNoticeSignal(userID int, r QuotaNoticeResource, success bool) (bool, int64, error) {
	if userID <= 0 || r.Scope == "" || r.RequestID == "" {
		return false, 0, nil
	}
	if success {
		var existing QuotaNoticeAccount
		result := DB.Select("user_id").Where("user_id = ?", userID).Limit(1).Find(&existing)
		if result.Error != nil {
			return false, 0, result.Error
		}
		if result.RowsAffected == 0 {
			return false, 0, nil
		}
	}
	inserted := false
	var total int64
	err := withQuotaNoticeAccount(userID, func(tx *gorm.DB, a *QuotaNoticeAccount) error {
		resources, err := quotaNoticeResources(a)
		if err != nil {
			return err
		}
		if success {
			old, ok := resources[r.Scope]
			if ok && r.LastAt > old.SuccessAt {
				old.SuccessAt = r.LastAt
				resources[r.Scope] = old
			}
			return saveQuotaNoticeResources(a, resources)
		}
		pruneQuotaNoticeResources(resources, r.LastAt)
		if len(resources) == 0 {
			a.Stage = 0
			a.LastSubmittedAt = 0
			a.NextCheckAt = 0
		}
		if _, ok := resources[r.Scope]; !ok && len(resources) >= 64 {
			return nil
		}
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", userID, r.Scope, r.RequestID))))
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&QuotaNoticeSignal{ID: id, UserID: userID, Scope: r.Scope, At: r.LastAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		old := resources[r.Scope]
		if old.FirstAt == 0 || r.LastAt < old.FirstAt {
			old.FirstAt = r.LastAt
		}
		if r.LastAt >= old.LastAt {
			r.FirstAt = old.FirstAt
			r.SuccessAt = old.SuccessAt
			r.Count = old.Count
			r.TokenIDs = old.TokenIDs
			r.ModelNames = old.ModelNames
			old = r
		}
		if r.TokenID > 0 && len(old.TokenIDs) < 64 && !slices.Contains(old.TokenIDs, r.TokenID) {
			old.TokenIDs = append(old.TokenIDs, r.TokenID)
		}
		if r.ModelName != "" && len(old.ModelNames) < 64 && !slices.Contains(old.ModelNames, r.ModelName) {
			old.ModelNames = append(old.ModelNames, r.ModelName)
		}
		old.Count++
		total = old.Count
		inserted = true
		resources[r.Scope] = old
		if r.LastAt > a.LastActivityAt {
			a.LastActivityAt = r.LastAt
		}
		return saveQuotaNoticeResources(a, resources)
	})
	return inserted, total, err
}

func quotaNoticeStats(tx *gorm.DB, userID int, r QuotaNoticeResource, since, now int64, cache QuotaNoticeStatsReader) (QuotaNoticeWindowStats, error) {
	if cache != nil {
		if stats, ok := cache(userID, r, since, now); ok {
			return stats, nil
		}
	}
	var row struct {
		Count   int64
		FirstAt int64
		LastAt  int64
	}
	err := tx.Model(&QuotaNoticeSignal{}).Select("COUNT(*) AS count, COALESCE(MIN(at), 0) AS first_at, COALESCE(MAX(at), 0) AS last_at").Where("user_id = ? AND scope = ? AND at > ? AND at <= ?", userID, r.Scope, since, now).Scan(&row).Error
	return QuotaNoticeWindowStats{row.Count, row.FirstAt, row.LastAt}, err
}

func eligibleQuotaNotice(tx *gorm.DB, a *QuotaNoticeAccount, resources map[string]QuotaNoticeResource, now int64, cache QuotaNoticeStatsReader) (bool, error) {
	if a.Stage > 0 && now-a.LastSubmittedAt < 30*quotaNoticeMinute {
		return false, nil
	}
	if a.Stage > 1 && now-a.LastSubmittedAt < 6*60*quotaNoticeMinute {
		return false, nil
	}
	var recent int64
	var latest int64
	for _, r := range resources {
		since := now - 5*quotaNoticeMinute
		if a.Stage > 0 {
			since = max(now-10*quotaNoticeMinute, a.LastSubmittedAt)
		}
		stats, err := quotaNoticeStats(tx, a.UserID, r, since, now, cache)
		if err != nil {
			return false, err
		}
		if a.Stage == 0 && stats.Count >= 3 && stats.LastAt-stats.FirstAt >= quotaNoticeMinute {
			return true, nil
		}
		recent += stats.Count
		latest = max(latest, stats.LastAt)
	}
	return a.Stage > 0 && recent >= 3 && now-latest <= 5*quotaNoticeMinute, nil
}

func quotaNoticeSlots(tx *gorm.DB, userID int, now int64) (int64, int64, error) {
	var rows []QuotaNotice
	err := tx.Select("slot_at").Where("user_id = ? AND ((status IN ? AND slot_at > ?) OR status IN ?)", userID, []string{QuotaNoticeSubmitted, QuotaNoticeUnknown}, now-QuotaNoticeWindow, []string{QuotaNoticePending, QuotaNoticeSending}).Find(&rows).Error
	var oldest int64
	for _, row := range rows {
		if oldest == 0 || row.SlotAt < oldest {
			oldest = row.SlotAt
		}
	}
	return int64(len(rows)), oldest, err
}

// PrepareQuotaNotice atomically reserves a rolling-window slot. Limits are
// checked again for retries; neither midnight nor a new incident resets them.
func PrepareQuotaNotice(userID int, now int64, cache QuotaNoticeStatsReader) (*QuotaNotice, error) {
	var prepared *QuotaNotice
	err := withQuotaNoticeAccount(userID, func(tx *gorm.DB, a *QuotaNoticeAccount) error {
		if now < a.NextCheckAt {
			return nil
		}
		resources, err := quotaNoticeResources(a)
		if err != nil {
			return err
		}
		pruneQuotaNoticeResources(resources, now)
		if err = saveQuotaNoticeResources(a, resources); err != nil {
			return err
		}
		if len(resources) == 0 {
			a.Stage = 0
			a.LastSubmittedAt = 0
			a.LastActivityAt = 0
			if err = tx.Model(&QuotaNotice{}).Where("user_id = ? AND status = ?", userID, QuotaNoticeRetry).Updates(map[string]interface{}{"status": QuotaNoticeSkipped, "reason": "inactive_or_recovered", "updated_at": now, "due_at": 0}).Error; err != nil {
				return err
			}
			a.NextCheckAt = now + quotaNoticeMinute
			return nil
		}
		var active int64
		if err = tx.Model(&QuotaNotice{}).Where("user_id = ? AND status IN ?", userID, []string{QuotaNoticePending, QuotaNoticeSending}).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return nil
		}
		eligible, err := eligibleQuotaNotice(tx, a, resources, now, cache)
		if err != nil {
			return err
		}
		if !eligible {
			if err = tx.Model(&QuotaNotice{}).Where("user_id = ? AND status = ? AND due_at <= ?", userID, QuotaNoticeRetry, now).Updates(map[string]interface{}{"status": QuotaNoticeSkipped, "reason": "inactive_or_recovered", "updated_at": now, "due_at": 0}).Error; err != nil {
				return err
			}
			a.NextCheckAt = now + 15000
			return nil
		}
		var retry QuotaNotice
		if err = tx.Where("user_id = ? AND status = ?", userID, QuotaNoticeRetry).Order("id desc").Limit(1).Find(&retry).Error; err != nil {
			return err
		}
		if retry.ID > 0 && now < retry.DueAt {
			a.NextCheckAt = retry.DueAt
			return nil
		}
		count, oldest, err := quotaNoticeSlots(tx, userID, now)
		if err != nil {
			return err
		}
		if count >= 4 {
			list := make([]QuotaNoticeResource, 0, len(resources))
			for _, r := range resources {
				list = append(list, r)
			}
			summary, err := common.Marshal(list)
			if err != nil {
				return err
			}
			a.NextCheckAt = max(now+quotaNoticeMinute, oldest+QuotaNoticeWindow)
			n := QuotaNotice{UserID: userID, Status: QuotaNoticeSuppressed, Summary: string(summary), Reason: "rolling_24h_limit", CreatedAt: now, UpdatedAt: now, DueAt: a.NextCheckAt}
			return tx.Create(&n).Error
		}
		list := make([]QuotaNoticeResource, 0, len(resources))
		for _, r := range resources {
			list = append(list, r)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].LastAt > list[j].LastAt })
		b, err := common.Marshal(list)
		if err != nil {
			return err
		}
		n := QuotaNotice{UserID: userID, Status: QuotaNoticePending, CreatedAt: now, UpdatedAt: now, SlotAt: now, DueAt: now, Summary: string(b)}
		if retry.ID > 0 {
			n = retry
			n.Status = QuotaNoticePending
			n.SlotAt = now
			n.UpdatedAt = now
			n.DueAt = now
			n.Summary = string(b)
			err = tx.Save(&n).Error
		} else {
			err = tx.Create(&n).Error
		}
		if err != nil {
			return err
		}
		prepared = &n
		a.NextCheckAt = now + 15000
		return nil
	})
	return prepared, err
}

// Claim checks resource activity and recovery again immediately before sending.
func ClaimQuotaNotice(id int, now int64) (*QuotaNotice, error) {
	var n QuotaNotice
	if err := DB.First(&n, id).Error; err != nil {
		return nil, err
	}
	var claimed *QuotaNotice
	err := withQuotaNoticeAccount(n.UserID, func(tx *gorm.DB, a *QuotaNoticeAccount) error {
		if err := tx.First(&n, id).Error; err != nil {
			return err
		}
		if n.Status != QuotaNoticePending {
			return nil
		}
		resources, err := quotaNoticeResources(a)
		if err != nil {
			return err
		}
		pruneQuotaNoticeResources(resources, now)
		ok, err := eligibleQuotaNotice(tx, a, resources, now, nil)
		if err != nil {
			return err
		}
		if !ok {
			n.Status = QuotaNoticeSkipped
			n.Reason = "inactive_or_recovered"
			n.DueAt = 0
			n.UpdatedAt = now
			return tx.Save(&n).Error
		}
		list := make([]QuotaNoticeResource, 0, len(resources))
		for _, r := range resources {
			list = append(list, r)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].LastAt > list[j].LastAt })
		summary, err := common.Marshal(list)
		if err != nil {
			return err
		}
		n.Summary = string(summary)
		n.Status = QuotaNoticeSending
		n.Attempts++
		n.LeaseUntil = now + 2*quotaNoticeMinute
		n.UpdatedAt = now
		if err = tx.Save(&n).Error; err != nil {
			return err
		}
		claimed = &n
		return nil
	})
	return claimed, err
}

func FinishQuotaNotice(id int, now int64, status, reason, medium, recipient string) error {
	var n QuotaNotice
	if err := DB.First(&n, id).Error; err != nil {
		return err
	}
	return withQuotaNoticeAccount(n.UserID, func(tx *gorm.DB, a *QuotaNoticeAccount) error {
		if err := tx.First(&n, id).Error; err != nil {
			return err
		}
		if n.Status != QuotaNoticeSending {
			return nil
		}
		n.Status = status
		n.Reason = reason
		n.Medium = medium
		n.Recipient = recipient
		n.UpdatedAt = now
		n.LeaseUntil = 0
		n.DueAt = 0
		switch status {
		case QuotaNoticeSubmitted, QuotaNoticeUnknown:
			n.SlotAt = now
			a.Stage++
			a.LastSubmittedAt = now
			if status == QuotaNoticeSubmitted {
				n.SubmittedAt = now
			}
			a.NextCheckAt = now + 30*quotaNoticeMinute
			if a.Stage > 1 {
				a.NextCheckAt = now + 6*60*quotaNoticeMinute
			}
		case QuotaNoticeFailed:
			if n.Attempts < 3 {
				n.Status = QuotaNoticeRetry
				n.DueAt = now + quotaNoticeMinute
				if n.Attempts == 2 {
					n.DueAt = now + 5*quotaNoticeMinute
				}
				a.NextCheckAt = n.DueAt
			} else {
				a.NextCheckAt = now + 6*60*quotaNoticeMinute
			}
		default:
			a.NextCheckAt = now + 30*quotaNoticeMinute
		}
		return tx.Save(&n).Error
	})
}

func ListQuotaNotices(userID, offset, limit int) ([]QuotaNotice, int64, error) {
	q := DB.Model(&QuotaNotice{}).Where("user_id = ?", userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []QuotaNotice
	err := q.Order("id desc").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}

func CleanupQuotaNotices(now int64) error {
	// Chunk deletes to avoid long transactions. Only notification-owned data.
	var ids []string
	if err := DB.Model(&QuotaNoticeSignal{}).Where("at < ?", now-20*quotaNoticeMinute).Limit(1000).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) > 0 {
		if err := DB.Where("id IN ?", ids).Delete(&QuotaNoticeSignal{}).Error; err != nil {
			return err
		}
	}
	var noticeIDs []int
	if err := DB.Model(&QuotaNotice{}).Where("created_at < ? AND status NOT IN ?", now-90*QuotaNoticeWindow, []string{QuotaNoticePending, QuotaNoticeSending, QuotaNoticeRetry}).Limit(1000).Pluck("id", &noticeIDs).Error; err != nil {
		return err
	}
	if len(noticeIDs) > 0 {
		return DB.Where("id IN ?", noticeIDs).Delete(&QuotaNotice{}).Error
	}
	return nil
}

func RecoverQuotaNoticeLeases(now int64) error {
	var rows []QuotaNotice
	if err := DB.Select("id").Where("status = ? AND lease_until <= ?", QuotaNoticeSending, now).Limit(100).Find(&rows).Error; err != nil {
		return err
	}
	for _, n := range rows {
		if err := FinishQuotaNotice(n.ID, now, QuotaNoticeUnknown, "sender_interrupted", "", ""); err != nil {
			return err
		}
	}
	return nil
}
