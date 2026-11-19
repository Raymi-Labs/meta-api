package model

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Uses the existing disposable database DSNs; never run against a business DB.
func TestTicketDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			quotaNoticeTestDB(t, dialect)
			require.NoError(t, DB.AutoMigrate(&Ticket{}, &TicketMessage{}, &TicketAttachment{}))
			ctx := context.Background()
			create := func(status string) *Ticket {
				now := time.Now().Unix()
				ticket := &Ticket{UserID: 900001, Subject: "activity regression", Category: "api", Status: status, CreatedAt: now, UpdatedAt: now, LastMessageAt: now, StatusUpdatedAt: now}
				require.NoError(t, DB.Create(ticket).Error)
				t.Cleanup(func() {
					require.NoError(t, DB.Where("ticket_id = ?", ticket.ID).Delete(&TicketAttachment{}).Error)
					require.NoError(t, DB.Where("ticket_id = ?", ticket.ID).Delete(&TicketMessage{}).Error)
					require.NoError(t, DB.Delete(ticket).Error)
				})
				return ticket
			}
			t.Run("same_second_replies_and_notes", func(t *testing.T) {
				for _, internal := range []bool{false, true} {
					ticket := create(TicketStatusOpen)
					role := TicketSenderUser
					if internal {
						role = TicketSenderStaff
					}
					for i := 0; i < 20; i++ {
						require.NoError(t, TicketReplyAndTransition(ctx, ticket.ID, 900001, role, "same second", internal, "", TicketStatusOpen))
					}
					var count int64
					require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.ID).Count(&count).Error)
					require.EqualValues(t, 20, count)
				}
			})
			t.Run("idempotent_metadata_updates", func(t *testing.T) {
				ticket := create(TicketStatusOpen)
				for i := 0; i < 5; i++ {
					require.NoError(t, UpdateTicketAssignee(ctx, ticket.ID, 0))
					require.NoError(t, UpdateTicketCategory(ctx, ticket.ID, ticket.Category))
				}
				require.NoError(t, DB.Delete(ticket).Error)
				require.ErrorIs(t, UpdateTicketAssignee(ctx, ticket.ID, 0), ErrTicketNotFound)
				require.ErrorIs(t, UpdateTicketCategory(ctx, ticket.ID, ticket.Category), ErrTicketNotFound)
			})
			t.Run("concurrent_notes_preserve_every_message", func(t *testing.T) {
				ticket := create(TicketStatusOpen)
				var wg sync.WaitGroup
				errs := make(chan error, 10)
				for i := 0; i < 10; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						errs <- TicketReplyAndTransition(ctx, ticket.ID, 900002, TicketSenderStaff, "note", true, "", TicketStatusOpen)
					}()
				}
				wg.Wait()
				close(errs)
				for err := range errs {
					require.NoError(t, err)
				}
				var count int64
				require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.ID).Count(&count).Error)
				require.EqualValues(t, 10, count)
			})
			t.Run("closed_and_conflicting_status_do_not_persist_messages", func(t *testing.T) {
				ticket := create(TicketStatusClosed)
				require.ErrorContains(t, TicketReplyAndTransition(ctx, ticket.ID, 900001, TicketSenderUser, "stale", false, "", TicketStatusOpen), "状态已变更")
				require.ErrorIs(t, TicketReplyAndTransition(ctx, ticket.ID, 900001, TicketSenderUser, "closed", false, "", TicketStatusClosed), ErrTicketClosed)
				var count int64
				require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.ID).Count(&count).Error)
				require.Zero(t, count)
			})
			t.Run("attachments_reopen_and_prevent_stale_auto_transition", func(t *testing.T) {
				for _, status := range []string{TicketStatusWaitingUser, TicketStatusResolved} {
					ticket := create(status)
					stale := time.Now().Add(-8 * 24 * time.Hour).Unix()
					require.NoError(t, DB.Model(ticket).Updates(map[string]any{"last_message_at": stale, "status_updated_at": stale, "updated_at": stale}).Error)
					_, err := CreateTicketAttachment(ctx, &TicketAttachment{TicketID: ticket.ID, UploadedByID: ticket.UserID, Filename: "evidence.txt"})
					require.NoError(t, err)
					require.NoError(t, AutoTransitionTicketStatus(ctx, ticket.ID, status, TicketStatusClosed, time.Now().Add(-7*24*time.Hour).Unix()))
					got, err := GetTicket(ctx, ticket.ID)
					require.NoError(t, err)
					require.Equal(t, TicketStatusOpen, got.Status)
					require.True(t, got.StaffUnread)
					require.Greater(t, got.LastMessageAt, stale)
					var events int64
					require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ? AND event = ?", ticket.ID, "status_open").Count(&events).Error)
					require.EqualValues(t, 1, events)
				}
			})
			t.Run("staff_public_and_internal_attachment_visibility", func(t *testing.T) {
				for _, internal := range []bool{false, true} {
					ticket := create(TicketStatusResolved)
					message := &TicketMessage{TicketID: ticket.ID, SenderID: 900002, SenderRole: TicketSenderStaff, IsInternal: internal, Body: "evidence"}
					require.NoError(t, DB.Create(message).Error)
					_, err := CreateTicketAttachment(ctx, &TicketAttachment{TicketID: ticket.ID, UploadedByID: 900002, MessageID: &message.ID, Filename: "staff.txt"})
					require.NoError(t, err)
					got, err := GetTicket(ctx, ticket.ID)
					require.NoError(t, err)
					require.Equal(t, !internal, got.UserUnread)
					if internal {
						require.Equal(t, TicketStatusResolved, got.Status)
					} else {
						require.Equal(t, TicketStatusWaitingUser, got.Status)
					}
				}
			})
			t.Run("failed_upload_keeps_activity_unchanged", func(t *testing.T) {
				ticket := create(TicketStatusResolved)
				for i := 0; i < 10; i++ {
					require.NoError(t, DB.Create(&TicketAttachment{TicketID: ticket.ID}).Error)
				}
				_, err := CreateTicketAttachment(ctx, &TicketAttachment{TicketID: ticket.ID, UploadedByID: ticket.UserID})
				require.ErrorContains(t, err, "最多10")
				got, err := GetTicket(ctx, ticket.ID)
				require.NoError(t, err)
				require.Equal(t, ticket.Status, got.Status)
				require.Equal(t, ticket.LastMessageAt, got.LastMessageAt)
				require.False(t, got.StaffUnread)
			})
		})
	}
}
