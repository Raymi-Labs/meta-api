package router

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTicketAdversarialMultilineAndFilter(t *testing.T) {
	a := setupTicketAPI(t)
	u := a.user("review", 1)
	admin := a.user("admin", 1)
	r := a.ok(u.Id, 1, "POST", "/api/ticket", map[string]any{"subject": "test", "category": "api", "body": "line1\nline2\tcode"})
	id := int(r.object()["id"].(float64))
	a.ok(admin.Id, 10, "POST", fmt.Sprintf("/api/admin/ticket/%d/reply", id), map[string]any{"body": "answer\nmore"})
	a.ok(u.Id, 1, "POST", fmt.Sprintf("/api/ticket/%d/reply", id), map[string]any{"body": "thanks\nnext"})
	r = a.ok(admin.Id, 10, "GET", "/api/admin/ticket?status=closed", nil)
	require.EqualValues(t, 0, r.object()["total"])
	r = a.ok(admin.Id, 10, "GET", "/api/admin/ticket?status=open", nil)
	require.EqualValues(t, 1, r.object()["total"])
	require.False(t, a.call(u.Id, 1, "POST", "/api/ticket", map[string]any{"subject": "test", "category": "api", "body": "\n\t"}).Success)
}

func TestTicketAttachmentActivityThroughRoutes(t *testing.T) {
	for _, appeal := range []bool{false, true} {
		t.Run(fmt.Sprintf("appeal=%v", appeal), func(t *testing.T) {
			a := setupTicketAPI(t)
			u := a.user("attachment-owner", 1)
			category, base := "api", "/api/ticket"
			if appeal {
				category, base = "appeal", "/api/appeal"
				require.NoError(t, model.DB.Model(u).Update("status", 2).Error)
			}
			tk, err := model.CreateTicket(context.Background(), u.Id, "evidence", category, "", "test")
			require.NoError(t, err)
			require.NoError(t, model.DB.Model(tk).Updates(map[string]any{"status": "resolved", "staff_unread": false}).Error)
			setAuth := func(req *http.Request) {
				if appeal {
					req.Header.Set("X-Test-Appeal-User", strconv.Itoa(u.Id))
				} else {
					req.Header.Set("X-Test-User", strconv.Itoa(u.Id))
					req.Header.Set("X-Test-Role", "1")
					req.Header.Set("New-Api-User", strconv.Itoa(u.Id))
				}
			}
			content := bytes.Repeat([]byte("evidence\n"), 128*1024)
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("ticket_id", strconv.Itoa(tk.ID)))
			part, err := writer.CreateFormFile("file", "evidence.txt")
			require.NoError(t, err)
			_, err = part.Write(content)
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			req := httptest.NewRequest("POST", base+"/attachment/upload", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			setAuth(req)
			w := httptest.NewRecorder()
			a.engine.ServeHTTP(w, req)
			require.Equal(t, 200, w.Code, w.Body.String())
			var response struct {
				Success bool
				Data    model.TicketAttachment
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
			require.True(t, response.Success)
			got, err := model.GetTicket(context.Background(), tk.ID)
			require.NoError(t, err)
			require.Equal(t, "open", got.Status)
			require.True(t, got.StaffUnread)
			// Inherited gzip middleware must preserve authenticated binary downloads.
			req = httptest.NewRequest("GET", fmt.Sprintf("%s/attachment/%d/download", base, response.Data.ID), nil)
			req.Header.Set("Accept-Encoding", "gzip")
			setAuth(req)
			w = httptest.NewRecorder()
			a.engine.ServeHTTP(w, req)
			require.Equal(t, 200, w.Code)
			var reader io.Reader = w.Body
			if w.Header().Get("Content-Encoding") == "gzip" {
				gz, err := gzip.NewReader(w.Body)
				require.NoError(t, err)
				defer gz.Close()
				reader = gz
			}
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, content, data)
			require.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
		})
	}
}

func TestTicketAppealOrdinaryAndPayload(t *testing.T) {
	a := setupTicketAPI(t)
	u := a.user("review", 1)
	_, err := model.CreateTicket(context.Background(), u.Id, "ordinary", "api", "", "test")
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(u).Update("status", 2).Error)
	r := a.callAppeal(u.Id, "POST", "/api/appeal/tickets", map[string]any{"subject": "appeal", "body": "test"})
	require.True(t, r.Success, r.Message)
	id := int(r.object()["id"].(float64))
	require.NoError(t, model.UpdateTicketStatus(context.Background(), id, model.TicketStatusResolved, model.TicketStatusOpen))
	common.ResetTicketRateLimiter()
	r = a.callAppeal(u.Id, "POST", "/api/appeal/tickets", map[string]any{"subject": "second", "body": "test"})
	require.False(t, r.Success)
	require.NoError(t, model.DB.Model(u).Update("status", 1).Error)
	r = a.callAppeal(u.Id, "GET", "/api/appeal/session", nil)
	require.Equal(t, 401, r.Code)
}

func TestTicketAttachmentAdminAndInternalBoundary(t *testing.T) {
	a := setupTicketAPI(t)
	u := a.user("review", 1)
	admin := a.user("admin", 1)
	other := a.user("other", 1)
	tk, err := model.CreateTicket(context.Background(), u.Id, "test", "api", "", "test")
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "evidence.txt")
	require.NoError(t, os.WriteFile(path, []byte("evidence"), 0600))
	att, err := model.CreateTicketAttachment(context.Background(), &model.TicketAttachment{TicketID: tk.ID, Filename: "evidence.txt", MimeType: "text/plain", Size: 8, StoragePath: path, UploadedByID: u.Id})
	require.NoError(t, err)
	request := func(uid, role int, base string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", fmt.Sprintf("%s/attachment/%d/download", base, att.ID), nil)
		req.Header.Set("X-Test-User", strconv.Itoa(uid))
		req.Header.Set("X-Test-Role", strconv.Itoa(role))
		req.Header.Set("New-Api-User", strconv.Itoa(uid))
		w := httptest.NewRecorder()
		a.engine.ServeHTTP(w, req)
		return w
	}
	w := request(admin.Id, 10, "/api/admin/ticket")
	require.Equal(t, 200, w.Code)
	require.Equal(t, "evidence", w.Body.String())
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, 403, request(other.Id, 1, "/api/ticket").Code)
	msg, err := model.CreateTicketMessage(context.Background(), tk.ID, admin.Id, "staff", "secret", true)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(att).Update("message_id", msg.ID).Error)
	require.Equal(t, 404, request(u.Id, 1, "/api/ticket").Code)
	require.Equal(t, 200, request(admin.Id, 10, "/api/admin/ticket").Code)
	require.NoError(t, model.DB.Model(admin).Update("role", 1).Error)
	require.Equal(t, 403, request(admin.Id, 10, "/api/admin/ticket").Code)
}

func TestTicketAutoTransitionRechecksActivity(t *testing.T) {
	a := setupTicketAPI(t)
	u := a.user("review", 1)
	ctx := context.Background()
	tk, err := model.CreateTicket(ctx, u.Id, "test", "api", "", "test")
	require.NoError(t, err)
	old := time.Now().Add(-8 * 24 * time.Hour).Unix()
	threshold := time.Now().Add(-7 * 24 * time.Hour).Unix()
	require.NoError(t, model.DB.Model(tk).Updates(map[string]any{"status": "resolved", "status_updated_at": old, "last_message_at": old}).Error)
	items, err := model.ListTicketsForAutoTransition(ctx, "resolved", threshold, 200)
	require.NoError(t, err)
	require.Len(t, items, 1)
	_, err = model.CreateTicketMessage(ctx, tk.ID, 1, "staff", "internal activity", true)
	require.NoError(t, err)
	require.NoError(t, model.AutoTransitionTicketStatus(ctx, tk.ID, "resolved", "closed", threshold))
	current, err := model.GetTicket(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, "resolved", current.Status)
}

func TestTicketAttachmentUploadBoundaries(t *testing.T) {
	a := setupTicketAPI(t)
	u := a.user("review", 1)
	tk, err := model.CreateTicket(context.Background(), u.Id, "test", "api", "", "test")
	require.NoError(t, err)
	upload := func(content []byte, name, contentType string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("ticket_id", strconv.Itoa(tk.ID)))
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, name))
		h.Set("Content-Type", contentType)
		part, err := writer.CreatePart(h)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		req := httptest.NewRequest("POST", "/api/ticket/attachment/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-Test-User", strconv.Itoa(u.Id))
		req.Header.Set("X-Test-Role", "1")
		req.Header.Set("New-Api-User", strconv.Itoa(u.Id))
		w := httptest.NewRecorder()
		a.engine.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, 400, upload([]byte("<html><script>alert(1)</script></html>"), "fake.png", "image/png").Code)
	require.Equal(t, 400, upload(bytes.Repeat([]byte("x"), 5*1024*1024+1), "large.txt", "text/plain").Code)
	// An adversarial suffix must not panic or become part of the storage path.
	w := upload([]byte("plain text evidence"), "file."+strings.Repeat("a", 220), "image/png")
	require.Equal(t, 200, w.Code)
	var resp struct {
		Data model.TicketAttachment `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "text/plain", resp.Data.MimeType)
	require.NoError(t, model.UpdateTicketStatus(context.Background(), tk.ID, "closed", "open"))
	require.Equal(t, 400, upload([]byte("late"), "late.txt", "text/plain").Code)
}
