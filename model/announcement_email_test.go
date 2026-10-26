package model

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func announcementTestDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "mysql":
		dsn := os.Getenv("TEST_COMPAT_MYSQL_DSN")
		if dsn == "" {
			t.Skip("disposable MySQL not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_COMPAT_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("disposable PostgreSQL not configured")
		}
		driver = postgres.Open(dsn)
	default:
		driver = sqlite.Open(":memory:")
	}
	db, err := gorm.Open(driver, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	if dialect == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
	}
	oldDB, oldMap := DB, common.OptionMap
	oldAnnouncements := console_setting.GetConsoleSetting().Announcements
	DB, common.OptionMap = db, map[string]string{}
	t.Cleanup(func() {
		DB, common.OptionMap = oldDB, oldMap
		console_setting.GetConsoleSetting().Announcements = oldAnnouncements
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&Option{}, &User{}, &AnnouncementEmailIdentity{}, &AnnouncementEmailJob{}, &AnnouncementEmailDelivery{}))
	// These environment variables must point to disposable test databases.
	for _, table := range []any{&AnnouncementEmailDelivery{}, &AnnouncementEmailJob{}, &AnnouncementEmailIdentity{}, &Option{}, &User{}} {
		require.NoError(t, db.Unscoped().Where("1 = 1").Delete(table).Error)
	}
	return db
}

func announcementJSON(t *testing.T, rows ...map[string]any) string {
	t.Helper()
	if rows == nil {
		rows = []map[string]any{}
	}
	b, err := common.Marshal(rows)
	require.NoError(t, err)
	return string(b)
}
func announcementRow(id any, content string) map[string]any {
	row := map[string]any{"content": content, "publishDate": "2026-10-26T12:00:00+08:00", "type": "warning", "extra": ""}
	if id != nil {
		row["id"] = id
	}
	return row
}

func TestAnnouncementEmailDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := announcementTestDB(t, dialect)
			for i, email := range []string{"first@EXAMPLE.invalid", "FIRST@example.invalid", "second@example.invalid", "bad\r\nBcc:private@example.invalid", "disabled@example.invalid", "deleted@example.invalid", ""} {
				u := User{Username: fmt.Sprintf("announcement%d", i), AffCode: fmt.Sprintf("announcement%d", i), Email: email, Status: common.UserStatusEnabled}
				if i == 4 {
					u.Status = common.UserStatusDisabled
				}
				require.NoError(t, db.Create(&u).Error)
				if i == 5 {
					require.NoError(t, db.Delete(&u).Error)
				}
			}
			legacy := announcementJSON(t, announcementRow(nil, "历史公告"))
			require.NoError(t, db.Create(&Option{Key: AnnouncementOptionKey, Value: legacy}).Error)
			edited := announcementJSON(t, announcementRow(1, "已编辑历史公告"))
			n, err := SaveAnnouncementPublications(edited, true, 1, false)
			require.NoError(t, err)
			require.Zero(t, n)
			newRaw := announcementJSON(t, announcementRow(1, "已编辑历史公告"), announcementRow("uuid-a", "维护公告"))
			_, err = SaveAnnouncementPublications(newRaw, true, 1, false)
			require.ErrorContains(t, err, "SMTP")
			var opt Option
			require.NoError(t, db.Where(&Option{Key: AnnouncementOptionKey}).First(&opt).Error)
			require.Equal(t, edited, opt.Value)
			n, err = SaveAnnouncementPublications(newRaw, true, 1, true)
			require.NoError(t, err)
			require.Equal(t, 1, n)
			jobs, total, err := ListAnnouncementEmailJobs(0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Equal(t, 2, jobs[0].Total)
			require.Equal(t, 2, jobs[0].Pending)
			wire, err := common.Marshal(jobs)
			require.NoError(t, err)
			require.NotContains(t, string(wire), "example.invalid")
			n, err = SaveAnnouncementPublications(newRaw, true, 1, true)
			require.NoError(t, err)
			require.Zero(t, n)
			_, err = SaveAnnouncementPublications("[]", false, 1, false)
			require.NoError(t, err)
			n, err = SaveAnnouncementPublications(newRaw, true, 1, true)
			require.NoError(t, err)
			require.Zero(t, n)
			alias := announcementJSON(t, announcementRow("new-uuid-same-content", "维护公告"))
			n, err = SaveAnnouncementPublications(alias, true, 1, true)
			require.NoError(t, err)
			require.Zero(t, n)
			withoutEmail := announcementJSON(t, announcementRow("uuid-b", "不发邮件的公告"))
			n, err = SaveAnnouncementPublications(withoutEmail, false, 1, false)
			require.NoError(t, err)
			require.Zero(t, n)
			n, err = SaveAnnouncementPublications(withoutEmail, true, 1, true)
			require.NoError(t, err)
			require.Zero(t, n)
			// An unavailable outbox rolls the public announcement update back.
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("announcement_test_failure", func(tx *gorm.DB) {
				if tx.Statement.Table == "announcement_email_deliveries" {
					tx.AddError(fmt.Errorf("forced delivery write failure"))
				}
			}))
			_, err = SaveAnnouncementPublications(announcementJSON(t, announcementRow("uuid-c", "回滚公告")), true, 1, true)
			require.Error(t, err)
			require.NoError(t, db.Callback().Create().Remove("announcement_test_failure"))
			require.NoError(t, db.Where(&Option{Key: AnnouncementOptionKey}).First(&opt).Error)
			require.Equal(t, withoutEmail, opt.Value)
			// Global rate slot + CAS allow one claim in the same second.
			now := time.Now().Unix()
			var claims atomic.Int32
			var claimed *AnnouncementEmailDelivery
			var mu sync.Mutex
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					row, e := ClaimAnnouncementEmail(now)
					require.NoError(t, e)
					if row != nil {
						claims.Add(1)
						mu.Lock()
						claimed = row
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			require.EqualValues(t, 1, claims.Load())
			require.NotNil(t, claimed)
			require.NoError(t, FinishAnnouncementEmail(claimed, "submitted", now, false))
			second, err := ClaimAnnouncementEmail(now + 1)
			require.NoError(t, err)
			require.NotNil(t, second)
			// Process loss after claim is conservatively unknown, including a late completion.
			row, err := ClaimAnnouncementEmail(now + 122)
			require.NoError(t, err)
			require.Nil(t, row)
			require.NoError(t, FinishAnnouncementEmail(second, "submitted", now+123, false))
			jobs, _, err = ListAnnouncementEmailJobs(0, 10)
			require.NoError(t, err)
			require.Equal(t, 1, jobs[0].Submitted)
			require.Equal(t, 1, jobs[0].Unknown)
			// Retention removes addresses/audit rows while preserving dedup tombstones.
			require.NoError(t, CleanupAnnouncementEmailJobs(now+91*86400))
			jobs, total, err = ListAnnouncementEmailJobs(0, 10)
			require.NoError(t, err)
			require.Zero(t, total)
			require.Empty(t, jobs)
			n, err = SaveAnnouncementPublications(newRaw, true, 1, true)
			require.NoError(t, err)
			require.Zero(t, n)
		})
	}
}

func TestAnnouncementEmailValidationAndRetry(t *testing.T) {
	db := announcementTestDB(t, "sqlite")
	_, err := SaveAnnouncementPublications(announcementJSON(t, announcementRow("same", "a"), announcementRow("same", "b")), true, 1, true)
	require.ErrorContains(t, err, "ID 不能重复")
	var tooMany []map[string]any
	for i := range 11 {
		tooMany = append(tooMany, announcementRow(fmt.Sprintf("limit-%d", i), fmt.Sprintf("公告%d", i)))
	}
	_, err = SaveAnnouncementPublications(announcementJSON(t, tooMany...), true, 1, true)
	require.ErrorContains(t, err, "单次最多同步 10 条")
	var jobs int64
	require.NoError(t, db.Model(&AnnouncementEmailJob{}).Count(&jobs).Error)
	require.Zero(t, jobs)
	u := User{Username: "recipient", Email: "mail@example.invalid", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&u).Error)
	n, err := SaveAnnouncementPublications(announcementJSON(t, announcementRow("one", "第一"), announcementRow("two", "第二")), true, 1, true)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	now := time.Now().Unix()
	item, err := ClaimAnnouncementEmail(now)
	require.NoError(t, err)
	require.NotNil(t, item)
	eligible, err := AnnouncementEmailRecipientEligible(item)
	require.NoError(t, err)
	require.True(t, eligible)
	require.NoError(t, db.Model(&u).UpdateColumn("email", "changed@example.invalid").Error)
	eligible, err = AnnouncementEmailRecipientEligible(item)
	require.NoError(t, err)
	require.False(t, eligible)
	require.NoError(t, FinishAnnouncementEmail(item, "failed", now, true))
	for attempt := 2; attempt <= 3; attempt++ {
		item, err = ClaimAnnouncementEmail(now + int64(attempt)*180)
		require.NoError(t, err)
		require.NotNil(t, item)
		require.Equal(t, attempt, item.Attempts)
		require.NoError(t, FinishAnnouncementEmail(item, "failed", now+int64(attempt)*180, true))
	}
	var finished AnnouncementEmailDelivery
	require.NoError(t, db.First(&finished, item.ID).Error)
	require.Equal(t, "failed", finished.Status)
	page, total, err := ListAnnouncementEmailJobs(1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, page, 1)
}

func TestAnnouncementEmailLegacyFalsyIDEditing(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   any
	}{{"zero", 0}, {"empty_string", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			db := announcementTestDB(t, "sqlite")
			// Historical falsy IDs were accepted by the old backend and are
			// repaired to index+1 by the editor, including an edited body.
			require.NoError(t, db.Create(&Option{Key: AnnouncementOptionKey, Value: announcementJSON(t, announcementRow(tc.id, "旧公告"))}).Error)
			queued, err := SaveAnnouncementPublications(announcementJSON(t, announcementRow(1, "修复并编辑旧公告")), true, 1, false)
			require.NoError(t, err)
			require.Zero(t, queued)
			var total int64
			require.NoError(t, db.Model(&AnnouncementEmailJob{}).Count(&total).Error)
			require.Zero(t, total)
		})
	}
}
