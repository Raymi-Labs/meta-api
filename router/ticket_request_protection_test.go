package router

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestTicketRoutesInheritGlobalLimit(t *testing.T) {
	oldNum, oldDuration := common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration
	common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = 1, 3600
	t.Cleanup(func() { common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = oldNum, oldDuration })
	a := setupTicketAPI(t, true)
	user, admin, appeal := a.user("limit-user", 1), a.user("limit-admin", 1), a.user("limit-appeal", 2)
	for i, tc := range []struct {
		path     string
		id, role int
		appeal   bool
	}{
		{"/api/ticket/unread", user.Id, 1, false},
		{"/api/admin/ticket/unread", admin.Id, 10, false},
		{"/api/appeal/session", appeal.Id, 1, true},
		{"/api/ticket/999999/attachments", user.Id, 1, false},
		{"/api/admin/ticket/999999/attachments", admin.Id, 10, false},
		{"/api/appeal/tickets/999999/attachments", appeal.Id, 1, true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			for attempt := 0; attempt < 2; attempt++ {
				req := httptest.NewRequest("GET", tc.path, nil)
				req.RemoteAddr = fmt.Sprintf("192.0.2.%d:12345", i+1)
				if tc.appeal {
					req.Header.Set("X-Test-Appeal-User", strconv.Itoa(tc.id))
				} else {
					req.Header.Set("X-Test-User", strconv.Itoa(tc.id))
					req.Header.Set("X-Test-Role", strconv.Itoa(tc.role))
					req.Header.Set("New-Api-User", strconv.Itoa(tc.id))
				}
				w := httptest.NewRecorder()
				a.engine.ServeHTTP(w, req)
				if attempt == 0 {
					require.NotEqual(t, 429, w.Code)
				} else {
					require.Equal(t, 429, w.Code)
					require.Equal(t, "3600", w.Header().Get("Retry-After"))
					require.Equal(t, "GA", w.Header().Get("X-RateLimit-Scope"))
				}
			}
		})
	}
}

func TestTicketJSONEnvelopeLimit(t *testing.T) {
	a := setupTicketAPI(t)
	user, admin, appeal := a.user("body-user", 1), a.user("body-admin", 1), a.user("body-appeal", 2)
	for _, tc := range []struct {
		method, path string
		id, role     int
		appeal       bool
	}{
		{"POST", "/api/ticket", user.Id, 1, false},
		{"POST", "/api/ticket/1/reply", user.Id, 1, false},
		{"POST", "/api/ticket/1/close", user.Id, 1, false},
		{"POST", "/api/appeal/tickets", appeal.Id, 1, true},
		{"POST", "/api/appeal/tickets/1/reply", appeal.Id, 1, true},
		{"POST", "/api/admin/ticket/1/reply", admin.Id, 10, false},
		{"PUT", "/api/admin/ticket/1/status", admin.Id, 10, false},
		{"PUT", "/api/admin/ticket/1/assign", admin.Id, 10, false},
	} {
		for _, chunked := range []bool{false, true} {
			for _, payload := range []string{`{"subject":"test","body":"test","category":"api","padding":"` + strings.Repeat("x", 2*1024*1024) + `"}`, `{}` + strings.Repeat(" ", 64*1024)} {
				req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(payload))
				if chunked {
					req.ContentLength = -1
					req.TransferEncoding = []string{"chunked"}
				}
				req.Header.Set("Content-Type", "application/json")
				if tc.appeal {
					req.Header.Set("X-Test-Appeal-User", strconv.Itoa(tc.id))
				} else {
					req.Header.Set("X-Test-User", strconv.Itoa(tc.id))
					req.Header.Set("X-Test-Role", strconv.Itoa(tc.role))
					req.Header.Set("New-Api-User", strconv.Itoa(tc.id))
				}
				w := httptest.NewRecorder()
				a.engine.ServeHTTP(w, req)
				require.Equal(t, 413, w.Code, "%s chunked=%v", tc.path, chunked)
			}
		}
	}
	// Worst-case JSON escapes still fit the 5000-rune message contract. Rejected
	// envelopes must not consume the account's three valid creations per hour.
	for i := 0; i < 3; i++ {
		payload := `{"subject":"valid","category":"api","body":"` + strings.Repeat(`\ud83d\ude00`, 5000) + `"}`
		req := httptest.NewRequest("POST", "/api/ticket", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", strconv.Itoa(user.Id))
		req.Header.Set("X-Test-Role", "1")
		req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
		w := httptest.NewRecorder()
		a.engine.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code)
		var response ticketAPIResponse
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success, w.Body.String())
	}
}
