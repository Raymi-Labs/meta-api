package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const AnnouncementOptionKey = "console_setting.announcements"

// Identity tombstones survive edits, deletion and non-email publications. The
// lock row provides a real write lock on SQLite, MySQL and PostgreSQL alike.
type AnnouncementEmailIdentity struct {
	Key      string `gorm:"primaryKey;type:char(64)"`
	Revision int64
}

type AnnouncementEmailJob struct {
	ID        int    `json:"id"`
	Identity  string `json:"-" gorm:"type:char(64);uniqueIndex"`
	Content   string `json:"content" gorm:"type:text"`
	Extra     string `json:"-" gorm:"type:text"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime:false"`
	CreatedBy int    `json:"-"`
	Total     int    `json:"total"`
}

// Each address is private, belongs to a submission-time snapshot and is never
// returned by the management API or written to SQL logs.
type AnnouncementEmailDelivery struct {
	ID         int
	JobID      int    `gorm:"uniqueIndex:idx_announcement_recipient,priority:1;index"`
	EmailHash  string `gorm:"type:char(64);uniqueIndex:idx_announcement_recipient,priority:2"`
	Email      string `gorm:"type:varchar(254)" json:"-"`
	UserID     int    `json:"-"`
	Status     string `gorm:"type:varchar(16);index:idx_announcement_queue,priority:1"`
	DueAt      int64  `gorm:"index:idx_announcement_queue,priority:2"`
	LeaseUntil int64
	Attempts   int
}

type AnnouncementEmailJobSummary struct {
	AnnouncementEmailJob
	Pending   int `json:"pending"`
	Sending   int `json:"sending"`
	Submitted int `json:"submitted"`
	Failed    int `json:"failed"`
	Unknown   int `json:"unknown"`
	Skipped   int `json:"skipped"`
}

type announcementPublication struct{ id, semantic, content, extra string }

var announcementPublicationMutex sync.Mutex

func announcementDigest(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

func parseAnnouncementPublications(raw string, strict bool) ([]announcementPublication, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if strict {
		if err := console_setting.ValidateConsoleSettings(raw, "Announcements"); err != nil {
			return nil, err
		}
	}
	var rows []map[string]any
	if err := common.UnmarshalJsonStr(raw, &rows); err != nil {
		return nil, err
	}
	result := make([]announcementPublication, 0, len(rows))
	ids := map[string]bool{}
	for _, row := range rows {
		content, _ := row["content"].(string)
		extra, _ := row["extra"].(string)
		date, _ := row["publishDate"].(string)
		published, _ := time.Parse(time.RFC3339, date)
		semantic, err := common.Marshal([]string{strings.TrimSpace(content), strings.TrimSpace(extra), published.UTC().Format(time.RFC3339Nano)})
		if err != nil {
			return nil, err
		}
		item := announcementPublication{content: content, extra: extra, semantic: announcementDigest("content:" + string(semantic))}
		var id string
		switch v := row["id"].(type) {
		case nil:
		case string:
			id = strings.TrimSpace(v)
		case float64:
			if v <= 0 || v != float64(int64(v)) || v > 9007199254740991 {
				if strict {
					return nil, errors.New("公告 ID 无效")
				}
				break
			}
			id = strconv.FormatInt(int64(v), 10)
		default:
			if strict {
				return nil, errors.New("公告 ID 无效")
			}
		}
		if strict && len(id) > 128 {
			return nil, errors.New("公告 ID 过长")
		}
		if id != "" {
			if strict && ids[id] {
				return nil, errors.New("公告 ID 不能重复")
			}
			ids[id] = true
			item.id = announcementDigest("id:" + id)
		}
		result = append(result, item)
	}
	return result, nil
}

func announcementPrivateDB() *gorm.DB {
	return DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
}

func markAnnouncementSeen(tx *gorm.DB, item announcementPublication) error {
	keys := []string{item.semantic}
	if item.id != "" {
		keys = append(keys, item.id)
	}
	for _, key := range keys {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&AnnouncementEmailIdentity{Key: key}).Error; err != nil {
			return err
		}
	}
	return nil
}

// SaveAnnouncementPublications commits both the announcement and all delivery
// snapshots, or neither. SMTP readiness is checked only for genuinely new mail.
func SaveAnnouncementPublications(raw string, emailSync bool, actor int, smtpReady bool) (int, error) {
	items, err := parseAnnouncementPublications(raw, true)
	if err != nil {
		return 0, err
	}
	announcementPublicationMutex.Lock()
	defer announcementPublicationMutex.Unlock()
	queued := 0
	err = announcementPrivateDB().Transaction(func(tx *gorm.DB) error {
		lock := AnnouncementEmailIdentity{Key: announcementDigest("publication-lock")}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lock).Error; err != nil {
			return err
		}
		if err := tx.Model(&lock).UpdateColumn("revision", gorm.Expr("revision + 1")).Error; err != nil {
			return err
		}
		var previous Option
		err := tx.Where(&Option{Key: AnnouncementOptionKey}).First(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		oldItems, err := parseAnnouncementPublications(previous.Value, false)
		if err != nil {
			return fmt.Errorf("原公告数据无效，请先修复: %w", err)
		}
		for index, item := range oldItems {
			// The existing editor assigns index+1 to legacy rows without IDs.
			// Recognize that identity even when the administrator edits the text.
			if item.id == "" {
				item.id = announcementDigest("id:" + strconv.Itoa(index+1))
			}
			if err := markAnnouncementSeen(tx, item); err != nil {
				return err
			}
		}
		var recipients []AnnouncementEmailDelivery
		loadedRecipients := false
		for _, item := range items {
			keys := []string{item.semantic}
			if item.id != "" {
				keys = append(keys, item.id)
			}
			var seen int64
			if err := tx.Model(&AnnouncementEmailIdentity{}).Where(clause.IN{Column: "key", Values: stringInterfaces(keys)}).Count(&seen).Error; err != nil {
				return err
			}
			if emailSync && seen == 0 {
				if queued >= 10 {
					return errors.New("单次最多同步 10 条新公告，请分批发布或取消邮件同步")
				}
				if !smtpReady {
					return errors.New("SMTP 未配置完整，请先配置邮件服务或取消邮件同步")
				}
				if !loadedRecipients {
					recipients, err = snapshotAnnouncementRecipients(tx)
					if err != nil {
						return err
					}
					loadedRecipients = true
				}
				identity := item.id
				if identity == "" {
					identity = item.semantic
				}
				job := AnnouncementEmailJob{Identity: identity, Content: item.content, Extra: item.extra, CreatedAt: time.Now().Unix(), CreatedBy: actor, Total: len(recipients)}
				if err := tx.Create(&job).Error; err != nil {
					return err
				}
				for start := 0; start < len(recipients); start += 200 {
					end := min(start+200, len(recipients))
					batch := append([]AnnouncementEmailDelivery(nil), recipients[start:end]...)
					for i := range batch {
						batch[i].JobID = job.ID
					}
					if err := tx.Create(&batch).Error; err != nil {
						return err
					}
				}
				queued++
			}
			if err := markAnnouncementSeen(tx, item); err != nil {
				return err
			}
		}
		return tx.Save(&Option{Key: AnnouncementOptionKey, Value: raw}).Error
	})
	if err != nil {
		return 0, err
	}
	return queued, updateOptionMap(AnnouncementOptionKey, raw)
}

func snapshotAnnouncementRecipients(tx *gorm.DB) ([]AnnouncementEmailDelivery, error) {
	var recipients []AnnouncementEmailDelivery
	seen := map[string]bool{}
	lastID := 0
	for {
		var users []User
		if err := tx.Select("id", "email").Where("status = ? AND id > ? AND email <> ?", common.UserStatusEnabled, lastID, "").Order("id").Limit(500).Find(&users).Error; err != nil {
			return nil, err
		}
		if len(users) == 0 {
			return recipients, nil
		}
		for _, user := range users {
			lastID = user.Id
			email := common.NormalizeEmailAddress(user.Email)
			if common.ValidateEmailAddress(email) != nil {
				continue
			}
			hash := announcementDigest(strings.ToLower(email))
			if seen[hash] {
				continue
			}
			seen[hash] = true
			recipients = append(recipients, AnnouncementEmailDelivery{Email: email, EmailHash: hash, UserID: user.Id, Status: "pending"})
		}
	}
}

func stringInterfaces(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

// Claim is a compare-and-swap, so multiple application instances cannot submit
// the same recipient. Expired claims become unknown, never pending again.
func ClaimAnnouncementEmail(now int64) (*AnnouncementEmailDelivery, error) {
	db := announcementPrivateDB()
	if err := db.Model(&AnnouncementEmailDelivery{}).Where("status = ? AND lease_until <= ?", "sending", now).UpdateColumn("status", "unknown").Error; err != nil {
		return nil, err
	}
	for i := 0; i < 8; i++ {
		var item AnnouncementEmailDelivery
		err := db.Where("status = ? AND due_at <= ?", "pending", now).Order("id").First(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		// One globally shared SMTP slot per second, even with several masters.
		slot := AnnouncementEmailIdentity{Key: announcementDigest("smtp-rate-slot")}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&slot).Error; err != nil {
			return nil, err
		}
		reserved := db.Model(&slot).Where("revision <= ?", now).UpdateColumn("revision", now+1)
		if reserved.Error != nil {
			return nil, reserved.Error
		}
		if reserved.RowsAffected == 0 {
			return nil, nil
		}
		lease := now + 120
		result := db.Model(&AnnouncementEmailDelivery{}).Where("id = ? AND status = ? AND due_at <= ?", item.ID, "pending", now).
			Updates(map[string]any{"status": "sending", "lease_until": lease, "attempts": gorm.Expr("attempts + 1")})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			item.Status, item.LeaseUntil, item.Attempts = "sending", lease, item.Attempts+1
			return &item, nil
		}
	}
	return nil, nil
}

func FinishAnnouncementEmail(item *AnnouncementEmailDelivery, status string, now int64, retry bool) error {
	if status != "submitted" && status != "unknown" && status != "failed" && status != "skipped" {
		return errors.New("invalid announcement email outcome")
	}
	due := int64(0)
	if status == "failed" && retry && item.Attempts < 3 {
		status, due = "pending", now+int64(item.Attempts)*60
	}
	return announcementPrivateDB().Model(&AnnouncementEmailDelivery{}).
		Where("id = ? AND status = ? AND lease_until = ? AND attempts = ?", item.ID, "sending", item.LeaseUntil, item.Attempts).
		Updates(map[string]any{"status": status, "due_at": due, "lease_until": 0}).Error
}

func GetAnnouncementEmailJob(id int) (*AnnouncementEmailJob, error) {
	var job AnnouncementEmailJob
	err := announcementPrivateDB().First(&job, id).Error
	return &job, err
}

func AnnouncementEmailRecipientEligible(item *AnnouncementEmailDelivery) (bool, error) {
	var user User
	err := announcementPrivateDB().Select("id", "email", "status").First(&user, item.UserID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return user.Status == common.UserStatusEnabled && strings.EqualFold(common.NormalizeEmailAddress(user.Email), item.Email), nil
}

func ListAnnouncementEmailJobs(offset, limit int) ([]AnnouncementEmailJobSummary, int64, error) {
	db := announcementPrivateDB()
	var total int64
	if err := db.Model(&AnnouncementEmailJob{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var jobs []AnnouncementEmailJob
	if err := db.Order("id DESC").Offset(offset).Limit(limit).Find(&jobs).Error; err != nil {
		return nil, 0, err
	}
	items := make([]AnnouncementEmailJobSummary, 0, len(jobs))
	for _, job := range jobs {
		item := AnnouncementEmailJobSummary{AnnouncementEmailJob: job}
		var counts []struct {
			Status string
			Count  int
		}
		if err := db.Model(&AnnouncementEmailDelivery{}).Select("status, COUNT(*) AS count").Where("job_id = ?", job.ID).Group("status").Scan(&counts).Error; err != nil {
			return nil, 0, err
		}
		for _, count := range counts {
			switch count.Status {
			case "pending":
				item.Pending = count.Count
			case "sending":
				item.Sending = count.Count
			case "submitted":
				item.Submitted = count.Count
			case "failed":
				item.Failed = count.Count
			case "unknown":
				item.Unknown = count.Count
			case "skipped":
				item.Skipped = count.Count
			}
		}
		items = append(items, item)
	}
	return items, total, nil
}

// Keep completed audit summaries/address snapshots for 90 days. Identity
// tombstones deliberately remain, so cleanup cannot re-enable mass delivery.
func CleanupAnnouncementEmailJobs(now int64) error {
	db := announcementPrivateDB()
	var jobs []AnnouncementEmailJob
	if err := db.Where("created_at < ?", now-90*86400).Order("id").Limit(100).Find(&jobs).Error; err != nil {
		return err
	}
	for _, job := range jobs {
		if err := db.Transaction(func(tx *gorm.DB) error {
			var active int64
			if err := tx.Model(&AnnouncementEmailDelivery{}).Where("job_id = ? AND status IN ?", job.ID, []string{"pending", "sending"}).Count(&active).Error; err != nil {
				return err
			}
			if active > 0 {
				return nil
			}
			if err := tx.Where("job_id = ?", job.ID).Delete(&AnnouncementEmailDelivery{}).Error; err != nil {
				return err
			}
			return tx.Delete(&job).Error
		}); err != nil {
			return err
		}
	}
	return nil
}
