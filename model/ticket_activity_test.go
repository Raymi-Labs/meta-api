package model

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
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
			t.Run("read_snapshot_keeps_later_reply_unread", func(t *testing.T) {
				ticket := create(TicketStatusOpen)
				id := ticket.ID
				readLocked, releaseRead := make(chan struct{}), make(chan struct{})
				require.NoError(t, DB.Callback().Query().After("gorm:query").Register("ticket_read_barrier", func(tx *gorm.DB) {
					if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "ticket_messages" {
						close(readLocked)
						<-releaseRead
					}
				}))
				readDone := make(chan error, 1)
				go func() { _, err := ReadTicketMessages(ctx, ticket, true); readDone <- err }()
				<-readLocked
				writeDone := make(chan error, 1)
				go func() {
					writeDone <- TicketReplyAndTransition(ctx, id, 900001, TicketSenderUser, "arrived during read", false, "", TicketStatusOpen, nil)
				}()
				close(releaseRead)
				require.NoError(t, <-readDone)
				require.NoError(t, <-writeDone)
				require.NoError(t, DB.Callback().Query().Remove("ticket_read_barrier"))
				got, err := GetTicket(ctx, id)
				require.NoError(t, err)
				require.True(t, got.StaffUnread, "new reply must not be acknowledged by the earlier read")
			})
			t.Run("successful_creation_quota_is_atomic_and_rolls_back", func(t *testing.T) {
				require.NoError(t, DB.AutoMigrate(&User{}))
				user := &User{Username: common.GetUUID()[:12], AffCode: common.GetUUID()[:12], Status: 1}
				require.NoError(t, DB.Create(user).Error)
				t.Cleanup(func() {
					var ids []int
					require.NoError(t, DB.Model(&Ticket{}).Where("user_id = ?", user.Id).Pluck("id", &ids).Error)
					if len(ids) > 0 {
						require.NoError(t, DB.Where("ticket_id IN ?", ids).Delete(&TicketMessage{}).Error)
					}
					require.NoError(t, DB.Where("user_id = ?", user.Id).Delete(&Ticket{}).Error)
					require.NoError(t, DB.Unscoped().Delete(user).Error)
				})
				for _, category := range []string{TicketCategoryAPI, TicketCategoryAppeal} {
					_, err := CreateTicket(ctx, user.Id, "   ", category, "", "test")
					require.Error(t, err)
				}
				// A failed first-message insert must roll back both ticket and quota.
				require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("ticket_fail_message", func(tx *gorm.DB) {
					if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "ticket_messages" {
						tx.AddError(errors.New("fixture insert failure"))
					}
				}))
				_, err := CreateTicket(ctx, user.Id, "failed", TicketCategoryAppeal, "", "body")
				require.Error(t, err)
				require.NoError(t, DB.Callback().Create().Remove("ticket_fail_message"))
				for _, category := range []string{TicketCategoryAPI, TicketCategoryAppeal} {
					var wg sync.WaitGroup
					results := make(chan error, 8)
					for i := 0; i < 8; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							_, err := CreateTicket(ctx, user.Id, "concurrent", category, "", "body")
							results <- err
						}()
					}
					wg.Wait()
					close(results)
					successes := 0
					for err := range results {
						if err == nil {
							successes++
						}
					}
					limit, window := 3, int64(3600)
					if category == TicketCategoryAppeal {
						limit, window = 1, 86400
					}
					require.Equal(t, limit, successes)
					require.NoError(t, DB.Model(&Ticket{}).Where("user_id = ? AND category = ?", user.Id, category).Update("status", TicketStatusClosed).Error)
					_, err = CreateTicket(ctx, user.Id, "still limited", category, "", "body")
					require.ErrorContains(t, err, "操作过于频繁")
					require.NoError(t, DB.Model(&Ticket{}).Where("user_id = ? AND category = ?", user.Id, category).Update("created_at", time.Now().Unix()-window-1).Error)
					_, err = CreateTicket(ctx, user.Id, "expired window", category, "", "body")
					require.NoError(t, err)
				}
			})
			t.Run("attachment_delete_requires_owner_and_open_state", func(t *testing.T) {
				ticket := create(TicketStatusOpen)
				for _, tc := range []struct {
					uploader, actor                  int
					admin, closed, internal, allowed bool
				}{
					{900001, 900001, false, false, false, true},
					{900002, 900001, false, false, false, false},
					{900001, 900003, false, false, false, false},
					{900001, 900002, true, false, false, true},
					{900001, 900001, false, true, false, false},
					{900001, 900002, true, true, false, false},
					{900001, 900001, false, false, true, false},
				} {
					status := TicketStatusOpen
					if tc.closed {
						status = TicketStatusClosed
					}
					require.NoError(t, DB.Model(ticket).Update("status", status).Error)
					attachment := &TicketAttachment{TicketID: ticket.ID, UploadedByID: tc.uploader}
					if tc.internal {
						msg := &TicketMessage{TicketID: ticket.ID, IsInternal: true}
						require.NoError(t, DB.Create(msg).Error)
						attachment.MessageID = &msg.ID
					}
					require.NoError(t, DB.Create(attachment).Error)
					_, err := DeleteTicketAttachment(ctx, attachment.ID, tc.actor, tc.admin)
					if tc.allowed {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
						_, err = GetTicketAttachment(ctx, attachment.ID)
						require.NoError(t, err)
					}
				}
			})

			t.Run("same_second_replies_and_notes", func(t *testing.T) {
				for _, internal := range []bool{false, true} {
					ticket := create(TicketStatusOpen)
					role := TicketSenderUser
					if internal {
						role = TicketSenderStaff
					}
					for i := 0; i < 20; i++ {
						require.NoError(t, TicketReplyAndTransition(ctx, ticket.ID, 900001, role, "same second", internal, "", TicketStatusOpen, nil))
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
						errs <- TicketReplyAndTransition(ctx, ticket.ID, 900002, TicketSenderStaff, "note", true, "", TicketStatusOpen, nil)
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
				require.ErrorContains(t, TicketReplyAndTransition(ctx, ticket.ID, 900001, TicketSenderUser, "stale", false, "", TicketStatusOpen, nil), "状态已变更")
				require.ErrorIs(t, TicketReplyAndTransition(ctx, ticket.ID, 900001, TicketSenderUser, "closed", false, "", TicketStatusClosed, nil), ErrTicketClosed)
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
