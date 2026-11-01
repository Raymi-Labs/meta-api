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

const announcementEmailTeam = "Meta-API团队"

var announcementEmailOnce sync.Once

func AnnouncementSMTPReady() bool {
	from := common.SMTPFrom
	if from == "" {
		from = common.SMTPAccount
	}
	return strings.TrimSpace(common.SMTPServer) != "" && common.SMTPPort > 0 && common.SMTPPort <= 65535 && common.ValidateEmailAddress(from) == nil
}

func announcementEmailContent(job *model.AnnouncementEmailJob) (string, string) {
	// Treat announcement Markdown/HTML as text in email, normalizing only the
	// trailing team signature without allowing content to inject email markup.
	content := `<div style="font-family:Arial,sans-serif;line-height:1.7;max-width:720px;margin:auto"><h2>` + announcementEmailTeam + `</h2><div style="white-space:pre-wrap">` + html.EscapeString(announcementEmailText(job.Content)) + `</div>`
	if job.Extra != "" {
		content += `<div style="white-space:pre-wrap;margin-top:16px">` + html.EscapeString(announcementEmailText(job.Extra)) + `</div>`
	}
	content += `<p>` + announcementEmailTeam + `</p><p style="color:#666">您收到此邮件，是因为您是本平台的注册用户。</p></div>`
	return fmt.Sprintf("[%s] 新平台公告", announcementEmailTeam), content
}

// Remove only a standalone trailing team signature; retain mentions in the body
// and leave the stored website announcement and delivery snapshot untouched.
func announcementEmailText(text string) string {
	trimmed := strings.TrimRight(text, " \t\r\n")
	lineStart := strings.LastIndex(trimmed, "\n") + 1
	lastLine := strings.TrimSpace(trimmed[lineStart:])
	if strings.EqualFold(strings.ReplaceAll(lastLine, " ", ""), announcementEmailTeam) {
		return strings.TrimRight(trimmed[:lineStart], " \t\r\n")
	}
	return text
}

func sendAnnouncementEmail(subject, receiver, content string) error {
	return common.SendEmailTrackedWithSenderName(subject, receiver, content, announcementEmailTeam)
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
				if err := processAnnouncementEmail(now.Unix(), sendAnnouncementEmail); err != nil {
					// Do not include SQL errors, SMTP replies or recipient addresses.
					common.SysError("announcement email queue processing failed")
				}
			}
		}()
	})
}
