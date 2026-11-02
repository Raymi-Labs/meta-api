package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestOpenRouterFinanceRoutesRejectNonAdmin(t *testing.T) {
	oldLimit := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() { common.GlobalApiRateLimitEnable = oldLimit })
	for _, role := range []int{0, common.RoleCommonUser} {
		r := gin.New()
		r.Use(sessions.Sessions("test", cookie.NewStore([]byte("local-test-session-key"))))
		r.Use(func(c *gin.Context) {
			if role != 0 {
				session := sessions.Default(c)
				session.Set("username", "local-test-user")
				session.Set("role", role)
				session.Set("id", 1)
				session.Set("status", common.UserStatusEnabled)
			}
			c.Next()
		})
		SetApiRouter(r)
		for _, path := range []string{"/api/upstream_accounts/1/openrouter/finance", "/api/upstream_accounts/1/cash/report", "/api/upstream_accounts/1/cash/bindings", "/api/upstream_accounts/1/cash/entries", "/api/cash_accounting/funding", "/api/cash_accounting/funding/1/receipts"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("New-Api-User", "1")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var response struct {
				Success bool `json:"success"`
			}
			if err := common.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Success {
				t.Fatalf("role=%d path=%s authorization failed: %s err=%v", role, path, w.Body.String(), err)
			}
		}
	}
}

func TestCashWritesRejectOrdinaryAdmins(t *testing.T) {
	oldLimit := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() { common.GlobalApiRateLimitEnable = oldLimit })
	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("local-test-session-key"))))
	r.Use(func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("username", "cash-admin")
		s.Set("role", common.RoleAdminUser)
		s.Set("id", 1)
		s.Set("status", common.UserStatusEnabled)
		c.Next()
	})
	SetApiRouter(r)
	for _, path := range []string{"/api/cash_accounting/enable", "/api/cash_accounting/funding/1/receipt", "/api/upstream_accounts/1/cash/bindings", "/api/upstream_accounts/1/cash/entries", "/api/upstream_accounts/1/cash/entries/1/void"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("New-Api-User", "1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var response struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		if err := common.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Success || response.Message != "无权进行此操作，权限不足" {
			t.Fatalf("root boundary failed %s: %s", path, w.Body.String())
		}
	}
}
