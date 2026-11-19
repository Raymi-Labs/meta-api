package service

import (
	"context"
	"fmt"
	"html"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	NotifyTypeTicketNew   = "ticket_new"
	NotifyTypeTicketReply = "ticket_reply"

	ticketAutoTransitionInterval  = 30 * time.Minute
	ticketAutoTransitionBatchSize = 200
	ticketWaitingUserDays         = 7
	ticketResolvedDays            = 7
)

var (
	ticketNotifications         sync.WaitGroup
	ticketAutoTransitionOnce    sync.Once
	ticketAutoTransitionRunning atomic.Bool
)

func RedactTicketText(text string, limit int) string {
	text = desktopFeedbackHeaders.ReplaceAllString(text, "[REDACTED]")
	text = desktopFeedbackPrivateKey.ReplaceAllString(text, "[REDACTED]")
	return truncateContentSafetyText(strings.TrimSpace(text), limit)
}

func ticketLink(ticketID int, isAppeal bool) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	if isAppeal {
		return base + "/login"
	}
	return fmt.Sprintf("%s/console/tickets?id=%d", base, ticketID)
}

func staffTicketLink(ticketID int) string {
	return fmt.Sprintf("%s/console/ticket-admin?id=%d", strings.TrimRight(system_setting.ServerAddress, "/"), ticketID)
}

// WaitTicketNotifications drains queued notifications after request producers stop.
func WaitTicketNotifications() { ticketNotifications.Wait() }

func queueTicketNotification(run func()) {
	ticketNotifications.Add(1)
	gopool.Go(func() { defer ticketNotifications.Done(); run() })
}

func sendTicketNotification(userID int, email string, settings dto.UserSetting, notify dto.Notify) {
	if err := NotifyUser(userID, email, settings, notify); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("ticket notification failed: user=%d type=%s err=%v", userID, notify.Type, err))
	}
}

func notifyTicketCreated(ticketID int, subject, category string) {
	root := model.GetRootUser()
	if root == nil || root.Id <= 0 || root.Status != common.UserStatusEnabled {
		return
	}
	settings := root.GetSetting()
	medium := settings.NotifyType
	if medium == "" {
		medium = dto.NotifyTypeEmail
	}

	content := fmt.Sprintf("用户提交了新工单 #%d\n标题：%s\n分类：%s\n链接：%s", ticketID, subject, formatTicketCategory(category), staffTicketLink(ticketID))
	if medium == "" || medium == dto.NotifyTypeEmail {
		content = strings.ReplaceAll(html.EscapeString(content), "\n", "<br/>")
	}

	notify := dto.NewNotify(NotifyTypeTicketNew, "新工单提醒", content, nil)
	queueTicketNotification(func() {
		sendTicketNotification(root.Id, root.Email, settings, notify)
	})
}

func notifyTicketReply(ticket *model.Ticket, recipientID int, recipientEmail string, recipientSettings dto.UserSetting) {
	medium := recipientSettings.NotifyType
	if medium == "" {
		medium = dto.NotifyTypeEmail
	}

	isAppeal := ticket.Category == model.TicketCategoryAppeal
	link := ticketLink(ticket.ID, isAppeal)
	if recipientID != ticket.UserID {
		link = staffTicketLink(ticket.ID)
	}
	content := fmt.Sprintf("工单 #%d 有新回复\n标题：%s\n链接：%s", ticket.ID, ticket.Subject, link)
	if medium == "" || medium == dto.NotifyTypeEmail {
		content = strings.ReplaceAll(html.EscapeString(content), "\n", "<br/>")
	}

	notify := dto.NewNotify(NotifyTypeTicketReply, "工单回复提醒", content, nil)
	queueTicketNotification(func() {
		sendTicketNotification(recipientID, recipientEmail, recipientSettings, notify)
	})
}

func NotifyTicketUserReply(ticket *model.Ticket) {
	if ticket.Assignee > 0 {
		user, err := model.GetUserById(ticket.Assignee, false)
		if err == nil && user.Status == common.UserStatusEnabled {
			notifyTicketReply(ticket, user.Id, user.Email, user.GetSetting())
			return
		}
	}

	root := model.GetRootUser()
	if root != nil && root.Id > 0 && root.Status == common.UserStatusEnabled {
		notifyTicketReply(ticket, root.Id, root.Email, root.GetSetting())
	}
}

func NotifyTicketStaffReply(ticket *model.Ticket) {
	user, err := model.GetUserById(ticket.UserID, false)
	if err != nil || (user.Status != common.UserStatusEnabled && ticket.Category != model.TicketCategoryAppeal) {
		return
	}
	notifyTicketReply(ticket, user.Id, user.Email, user.GetSetting())
}

func NotifyTicketCreated(ticketID int, subject, category string) {
	queueTicketNotification(func() {
		notifyTicketCreated(ticketID, subject, category)
	})
}

func StartTicketAutoTransitionTask() {
	ticketAutoTransitionOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			ticker := time.NewTicker(ticketAutoTransitionInterval)
			defer ticker.Stop()
			runTicketAutoTransitionLoop(context.Background(), ticker.C, func() {
				runTicketAutoTransitionOnce(time.Now())
			})
		})
	})
}

func runTicketAutoTransitionLoop(ctx context.Context, ticks <-chan time.Time, run func()) {
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
		run()
	}
}

func runTicketAutoTransitionOnce(now time.Time) {
	if !ticketAutoTransitionRunning.CompareAndSwap(false, true) {
		return
	}
	defer ticketAutoTransitionRunning.Store(false)

	ctx := context.Background()
	waitingUserThreshold := now.Add(-ticketWaitingUserDays * 24 * time.Hour).Unix()
	resolvedThreshold := now.Add(-ticketResolvedDays * 24 * time.Hour).Unix()

	waitingTickets, err := model.ListTicketsForAutoTransition(ctx, model.TicketStatusWaitingUser, waitingUserThreshold, ticketAutoTransitionBatchSize)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("ticket auto-transition: failed to list waiting_user tickets: %v", err))
	} else {
		for _, ticket := range waitingTickets {
			if err := model.AutoTransitionTicketStatus(ctx, ticket.ID, model.TicketStatusWaitingUser, model.TicketStatusResolved, waitingUserThreshold); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("ticket auto-transition: failed to transition ticket %d from waiting_user to resolved: %v", ticket.ID, err))
			}
		}
	}

	resolvedTickets, err := model.ListTicketsForAutoTransition(ctx, model.TicketStatusResolved, resolvedThreshold, ticketAutoTransitionBatchSize)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("ticket auto-transition: failed to list resolved tickets: %v", err))
	} else {
		for _, ticket := range resolvedTickets {
			if err := model.AutoTransitionTicketStatus(ctx, ticket.ID, model.TicketStatusResolved, model.TicketStatusClosed, resolvedThreshold); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("ticket auto-transition: failed to transition ticket %d from resolved to closed: %v", ticket.ID, err))
			}
		}
	}
}

func formatTicketCategory(category string) string {
	switch category {
	case model.TicketCategoryBilling:
		return "计费"
	case model.TicketCategoryAPI:
		return "接口"
	case model.TicketCategoryAccount:
		return "账号"
	case model.TicketCategoryInvoice:
		return "发票"
	case model.TicketCategoryEnterprise:
		return "企业"
	case model.TicketCategoryAppeal:
		return "申诉"
	case model.TicketCategoryOther:
		return "其他"
	default:
		return category
	}
}
