package service

import (
	"errors"
	"fmt"
	"html"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var announcementEmailOnce sync.Once

func AnnouncementSMTPReady() bool {
	from := common.SMTPFrom
	if from == "" {
		from = common.SMTPAccount
	}
	return strings.TrimSpace(common.SMTPServer) != "" && common.SMTPPort > 0 && common.SMTPPort <= 65535 && common.ValidateEmailAddress(from) == nil
}

func announcementEmailContent(job *model.AnnouncementEmailJob) (string, string) {
	// Treat announcement Markdown/HTML as text in email. This preserves every
	// character without allowing announcement content to inject email markup.
	content := `<div style="font-family:Arial,sans-serif;line-height:1.7;max-width:720px;margin:auto"><h2>` + html.EscapeString(common.SystemName) + ` 平台公告</h2><div style="white-space:pre-wrap">` + html.EscapeString(job.Content) + `</div>`
	if job.Extra != "" {
		content += `<div style="white-space:pre-wrap;margin-top:16px">` + html.EscapeString(job.Extra) + `</div>`
	}
	content += `<p style="color:#666">您收到此邮件，是因为您是本平台的注册用户。</p></div>`
	return fmt.Sprintf("[%s] 新平台公告", common.SystemName), content
}

func announcementEmailOutcome(err error) (status string, retry bool) {
	if err == nil {
		return "submitted", false
	}
	var unknown *common.SMTPOutcomeUnknown
	if errors.As(err, &unknown) {
		return "unknown", false
	}
	var smtpError *textproto.Error
	if errors.As(err, &smtpError) {
		return "failed", smtpError.Code >= 400 && smtpError.Code < 500
	}
	var networkError net.Error
	return "failed", errors.As(err, &networkError)
}

func processAnnouncementEmail(now int64, send func(string, string, string) error) error {
	item, err := model.ClaimAnnouncementEmail(now)
	if err != nil || item == nil {
		return err
	}
	eligible, err := model.AnnouncementEmailRecipientEligible(item)
	if err != nil {
		return model.FinishAnnouncementEmail(item, "failed", now, true)
	}
	if !eligible {
		return model.FinishAnnouncementEmail(item, "skipped", now, false)
	}
	job, err := model.GetAnnouncementEmailJob(item.JobID)
	if err != nil {
		return model.FinishAnnouncementEmail(item, "failed", now, true)
	}
	subject, content := announcementEmailContent(job)
	status, retry := announcementEmailOutcome(send(subject, item.Email, content))
	return model.FinishAnnouncementEmail(item, status, time.Now().Unix(), retry)
}

func StartAnnouncementEmailTask() {
	announcementEmailOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			var lastCleanup int64
			for now := range ticker.C {
				if now.Unix()-lastCleanup >= 3600 {
					lastCleanup = now.Unix()
					if err := model.CleanupAnnouncementEmailJobs(now.Unix()); err != nil {
						common.SysError("announcement email cleanup failed")
					}
				}
				if err := processAnnouncementEmail(now.Unix(), common.SendEmailTracked); err != nil {
					// Do not include SQL errors, SMTP replies or recipient addresses.
					common.SysError("announcement email queue processing failed")
				}
			}
		}()
	})
}
