package service

import (
	"context"
	"errors"
	"net"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

type NotificationDelivery struct{ Status, Reason, Recipient string }

// The caller already owns a durable quota slot. Unlike NotifyUser, a missing
// destination is a typed skip and a nil transport error means submission only.
func SendUserNotificationTracked(email string, settings dto.UserSetting, data dto.Notify) NotificationDelivery {
	medium := settings.NotifyType
	if medium == "" {
		medium = dto.NotifyTypeEmail
	}
	result := NotificationDelivery{Status: model.QuotaNoticeSkipped, Reason: "no_destination"}
	switch medium {
	case dto.NotifyTypeEmail:
		if settings.NotificationEmail != "" {
			email = settings.NotificationEmail
		}
		if email == "" {
			return result
		}
		result.Recipient = common.MaskEmail(email)
	case dto.NotifyTypeWebhook:
		if settings.WebhookUrl == "" {
			return result
		}
		result.Recipient = "Webhook"
	case dto.NotifyTypeBark:
		if settings.BarkUrl == "" {
			return result
		}
		result.Recipient = "Bark"
	case dto.NotifyTypeGotify:
		if settings.GotifyUrl == "" || settings.GotifyToken == "" {
			return result
		}
		result.Recipient = "Gotify"
	default:
		result.Reason = "unsupported_medium"
		return result
	}
	var err error
	var wroteHeaders atomic.Bool
	if medium == dto.NotifyTypeEmail {
		err = common.SendEmailTracked(data.Title, email, data.Content)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteHeaders: func() { wroteHeaders.Store(true) }})
		err = dispatchUserNotificationContext(ctx, 0, email, settings, data)
	}
	if err == nil {
		result.Status = model.QuotaNoticeSubmitted
		result.Reason = "accepted_by_service"
		return result
	}
	result.Status = model.QuotaNoticeFailed
	result.Reason = "notification_service_failed"
	var unknown *common.SMTPOutcomeUnknown
	var network net.Error
	if errors.As(err, &unknown) || (medium != dto.NotifyTypeEmail && wroteHeaders.Load() && (errors.As(err, &network) || strings.Contains(err.Error(), "EOF"))) {
		result.Status = model.QuotaNoticeUnknown
		result.Reason = "submission_outcome_unknown"
	}
	return result
}
