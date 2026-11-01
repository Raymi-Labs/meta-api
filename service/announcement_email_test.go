package service

import (
	"errors"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAnnouncementEmailSMTPOutcomes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status string
		retry  bool
	}{
		{nil, "submitted", false},
		{&common.SMTPOutcomeUnknown{Err: errors.New("lost DATA reply")}, "unknown", false},
		{&textproto.Error{Code: 450, Msg: "temporary"}, "failed", true},
		{&textproto.Error{Code: 550, Msg: "rejected"}, "failed", false},
		{&net.DNSError{Err: "timeout", IsTimeout: true}, "failed", true},
		{errors.New("configuration error"), "failed", false},
	} {
		status, retry := announcementEmailOutcome(tc.err)
		require.Equal(t, tc.status, status)
		require.Equal(t, tc.retry, retry)
	}
	subject, content := announcementEmailContent(&model.AnnouncementEmailJob{Content: "<script>alert('x')</script>\n维护", Extra: "<img src=x onerror=alert(1)>"})
	require.Contains(t, subject, "平台公告")
	require.NotContains(t, content, "<script>")
	require.Contains(t, content, "&lt;script&gt;")
	require.NotContains(t, content, "<img")
}

func TestAnnouncementEmailWorkerRecoveryAndEligibility(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	old := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = old; _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AnnouncementEmailJob{}, &model.AnnouncementEmailIdentity{}, &model.AnnouncementEmailDelivery{}))
	u := model.User{Username: "worker-recipient", Email: "worker@example.invalid", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&u).Error)
	job := model.AnnouncementEmailJob{Identity: "worker-job", Content: "维护公告", CreatedAt: time.Now().Unix(), Total: 3}
	require.NoError(t, db.Create(&job).Error)
	for _, hash := range []string{"first", "second", "third"} {
		require.NoError(t, db.Create(&model.AnnouncementEmailDelivery{JobID: job.ID, UserID: u.Id, Email: u.Email, EmailHash: hash, Status: "pending"}).Error)
	}
	now := time.Now().Unix()
	sent := 0
	send := func(subject, receiver, body string) error {
		sent++
		require.Equal(t, u.Email, receiver)
		return &common.SMTPOutcomeUnknown{Err: errors.New("reply lost")}
	}
	require.NoError(t, processAnnouncementEmail(now, send))
	require.Equal(t, 1, sent)
	// A fresh worker resumes the next pending record and never replays unknown.
	require.NoError(t, processAnnouncementEmail(now+1, func(_, _, _ string) error { sent++; return nil }))
	require.Equal(t, 2, sent)
	require.NoError(t, db.Model(&u).UpdateColumn("status", common.UserStatusDisabled).Error)
	require.NoError(t, processAnnouncementEmail(now+2, send))
	require.Equal(t, 2, sent)
	jobs, _, err := model.ListAnnouncementEmailJobs(0, 10)
	require.NoError(t, err)
	require.Equal(t, 1, jobs[0].Unknown)
	require.Equal(t, 1, jobs[0].Submitted)
	require.Equal(t, 1, jobs[0].Skipped)
}

func TestAnnouncementEmailBrandAndSignature(t *testing.T) {
	oldName := common.SystemName
	common.SystemName = "元衡 API"
	t.Cleanup(func() { common.SystemName = oldName })
	for _, tc := range []struct{ name, body, extra, want string }{
		{"no signature", "维护内容", "", "维护内容"},
		{"old signature", "维护内容\n\nMeta-Api 团队\r\n", "", "维护内容"},
		{"current signature", "维护内容\nMeta-API团队", "", "维护内容"},
		{"extra signature", "维护内容", "附加信息\nMeta-Api 团队", "附加信息"},
		{"body mention", "Meta-Api 团队将在凌晨维护", "", "Meta-Api 团队将在凌晨维护"},
		{"middle mention", "Meta-Api 团队\n将在凌晨维护", "", "Meta-Api 团队\n将在凌晨维护"},
		{"HTML", "<script>内容</script>", "<img src=x>", "&lt;script&gt;内容&lt;/script&gt;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := &model.AnnouncementEmailJob{Content: tc.body, Extra: tc.extra}
			subject, body := announcementEmailContent(job)
			require.Equal(t, "[Meta-API团队] 新平台公告", subject)
			require.Contains(t, body, "<h2>Meta-API团队</h2>")
			require.Contains(t, body, "<p>Meta-API团队</p>")
			require.Equal(t, 2, strings.Count(body, "Meta-API团队"))
			require.Contains(t, body, tc.want)
			require.NotContains(t, body, "元衡 API")
			require.NotContains(t, body, "<script>")
			require.NotContains(t, body, "<img")
			require.Equal(t, tc.body, job.Content)
			require.Equal(t, tc.extra, job.Extra)
		})
	}
}
