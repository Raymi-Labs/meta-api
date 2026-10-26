package service

import (
	"errors"
	"net"
	"net/textproto"
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
