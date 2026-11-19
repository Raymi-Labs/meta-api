package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

func TestTicketFailedAppealPreservesDailySlot(t *testing.T) {
	a := setupTicketAPI(t)
	u := a.user("appeal-review", 2)
	first := a.callAppeal(u.Id, "POST", "/api/appeal/tickets", map[string]any{"subject": "   ", "body": "valid appeal"})
	second := a.callAppeal(u.Id, "POST", "/api/appeal/tickets", map[string]any{"subject": "corrected title", "body": "valid appeal"})
	var count int64
	require.NoError(t, model.DB.Model(&model.Ticket{}).Count(&count).Error)
	require.False(t, first.Success)
	require.True(t, second.Success, second.Message)
	require.EqualValues(t, 1, count)
	t.Logf("first=%q corrected=%q saved_tickets=%d", first.Message, second.Message, count)
}
func TestTicketOwnerCannotDeleteStaffEvidenceAfterClosure(t *testing.T) {
	a := setupTicketAPI(t)
	u := a.user("delete-review", 1)
	admin := a.user("delete-admin", 1)
	ctx := context.Background()
	tk, err := model.CreateTicket(ctx, u.Id, "evidence", "api", "", "initial")
	require.NoError(t, err)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	require.NoError(t, mw.WriteField("ticket_id", strconv.Itoa(tk.ID)))
	part, err := mw.CreateFormFile("file", "staff.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("staff investigation evidence"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest("POST", "/api/ticket/attachment/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", strconv.Itoa(admin.Id))
	req.Header.Set("X-Test-Role", "10")
	req.Header.Set("New-Api-User", strconv.Itoa(admin.Id))
	w := httptest.NewRecorder()
	a.engine.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var upload struct {
		Success bool
		Data    model.TicketAttachment
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &upload))
	require.True(t, upload.Success, w.Body.String())
	at, err := model.GetTicketAttachment(ctx, upload.Data.ID)
	require.NoError(t, err)
	path := at.StoragePath
	a.ok(admin.Id, 10, "PUT", fmt.Sprintf("/api/admin/ticket/%d/status", tk.ID), map[string]any{"status": "closed"})
	result := a.call(u.Id, 1, "DELETE", fmt.Sprintf("/api/ticket/attachment/%d", at.ID), nil)
	require.False(t, result.Success)
	require.Equal(t, 403, result.Code)
	_, err = os.Stat(path)
	require.NoError(t, err)
	_, err = model.GetTicketAttachment(ctx, at.ID)
	require.NoError(t, err)
	t.Logf("closed ticket owner deletion rejected: success=%v", result.Success)
}
func TestTicketFailedDetailPreservesUnread(t *testing.T) {
	for _, mode := range []string{"user", "admin", "appeal"} {
		t.Run(mode, func(t *testing.T) {
			a := setupTicketAPI(t)
			u := a.user("read-review", 1)
			admin := a.user("read-admin", 1)
			ctx := context.Background()
			category := "api"
			if mode == "appeal" {
				category = "appeal"
				require.NoError(t, model.DB.Model(u).Update("status", 2).Error)
			}
			tk, err := model.CreateTicket(ctx, u.Id, "read", category, "", "unseen message")
			require.NoError(t, err)
			require.NoError(t, model.DB.Model(tk).Updates(map[string]any{"user_unread": true, "staff_unread": true}).Error)
			require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("review_fail_messages", func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "ticket_messages" {
					tx.AddError(errors.New("simulated message query outage"))
				}
			}))
			path := fmt.Sprintf("/api/ticket/%d", tk.ID)
			id, role := u.Id, 1
			if mode == "admin" {
				path = fmt.Sprintf("/api/admin/ticket/%d", tk.ID)
				id, role = admin.Id, 10
			}
			var response ticketAPIResponse
			if mode == "appeal" {
				response = a.callAppeal(u.Id, "GET", fmt.Sprintf("/api/appeal/tickets/%d", tk.ID), nil)
			} else {
				response = a.call(id, role, "GET", path, nil)
			}
			require.False(t, response.Success)
			require.NoError(t, model.DB.Callback().Query().Remove("review_fail_messages"))
			got, err := model.GetTicket(ctx, tk.ID)
			require.NoError(t, err)
			unread := got.UserUnread
			if mode == "admin" {
				unread = got.StaffUnread
			}
			require.True(t, unread)
			t.Logf("detail_success=%v unread_after_failed_load=%v message=%q", response.Success, unread, response.Message)
		})
	}
}
