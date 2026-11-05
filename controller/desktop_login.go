package controller

import (
	"crypto/subtle"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func desktopHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Referrer-Policy", "no-referrer")
}
func desktopFailure(c *gin.Context, status int) {
	c.JSON(status, gin.H{"success": false, "message": "桌面授权不可用、已过期或已处理，请从客户端重新发起"})
}
func desktopProviderEnabled(name string) bool {
	p := oauth.GetProvider(name)
	return (name == "github" || name == "google") && p != nil && p.IsEnabled()
}
func DesktopLoginCapabilities(c *gin.Context) {
	desktopHeaders(c)
	providers := []string{}
	for _, p := range []string{"github", "google"} {
		if desktopProviderEnabled(p) {
			providers = append(providers, p)
		}
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"protocol_version": 1, "providers": providers, "expires_in": 300, "interval": 3}})
}
func StartDesktopLogin(c *gin.Context) {
	desktopHeaders(c)
	var req struct {
		Challenge string `json:"challenge"`
		Provider  string `json:"provider"`
	}
	if c.ShouldBindJSON(&req) != nil || !desktopProviderEnabled(req.Provider) {
		desktopFailure(c, 400)
		return
	}
	id, g, err := model.NewDesktopLogin(req.Challenge, req.Provider)
	if err != nil {
		desktopFailure(c, 400)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"request_id": id, "display_code": g.DisplayCode, "expires_in": 300, "interval": 3}})
}
func DesktopLoginInfo(c *gin.Context) {
	desktopHeaders(c)
	id := c.Query("request_id")
	g, err := model.GetDesktopLogin(id)
	if err != nil || g.Status != "pending" || !desktopProviderEnabled(g.Provider) {
		desktopFailure(c, 410)
		return
	}
	session := sessions.Default(c)
	csrf, err := model.DesktopLoginToken()
	if err != nil {
		desktopFailure(c, 500)
		return
	}
	// Bound to both this browser session and this request; another tab must reload.
	session.Set("desktop_login_request", id)
	session.Set("desktop_login_csrf", csrf)
	if err = session.Save(); err != nil {
		desktopFailure(c, 500)
		return
	}
	data := gin.H{"display_code": g.DisplayCode, "provider": g.Provider, "expires_at": g.ExpiresAt, "csrf": csrf, "logged_in": false}
	if uid, ok := session.Get("id").(int); ok && uid > 0 {
		u := model.User{Id: uid}
		if u.FillUserById() == nil && u.Status == common.UserStatusEnabled {
			data["logged_in"] = true
			data["username"] = u.Username
			data["user_id"] = uid
		}
	}
	c.JSON(200, gin.H{"success": true, "data": data})
}
func ApproveDesktopLogin(c *gin.Context) {
	desktopHeaders(c)
	var req struct {
		RequestID string `json:"request_id"`
		CSRF      string `json:"csrf"`
		Code      string `json:"display_code"`
		UserID    int    `json:"user_id"`
		Deny      bool   `json:"deny"`
	}
	if c.ShouldBindJSON(&req) != nil {
		desktopFailure(c, 400)
		return
	}
	origin, err := url.Parse(system_setting.ServerAddress)
	if err != nil || origin.Scheme != "https" || c.GetHeader("Origin") != origin.Scheme+"://"+origin.Host {
		desktopFailure(c, 403)
		return
	}
	s := sessions.Default(c)
	uid, ok := s.Get("id").(int)
	csrf, _ := s.Get("desktop_login_csrf").(string)
	if !ok || uid <= 0 || uid != req.UserID || s.Get("desktop_login_request") != req.RequestID || csrf == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(req.CSRF)) != 1 {
		desktopFailure(c, 403)
		return
	}
	u := model.User{Id: uid}
	if u.FillUserById() != nil || u.Status != common.UserStatusEnabled {
		desktopFailure(c, 403)
		return
	}
	g, err := model.GetDesktopLogin(req.RequestID)
	if err != nil || !desktopProviderEnabled(g.Provider) {
		desktopFailure(c, 410)
		return
	}
	if err = model.ApproveDesktopLogin(req.RequestID, uid, req.Code, req.Deny); err != nil {
		desktopFailure(c, 409)
		return
	}
	s.Delete("desktop_login_request")
	s.Delete("desktop_login_csrf")
	_ = s.Save()
	c.JSON(200, gin.H{"success": true, "data": gin.H{"approved": !req.Deny}})
}
func ExchangeDesktopLogin(c *gin.Context) {
	desktopHeaders(c)
	var req struct {
		RequestID string `json:"request_id"`
		Verifier  string `json:"verifier"`
		Cancel    bool   `json:"cancel"`
	}
	if c.ShouldBindJSON(&req) != nil {
		desktopFailure(c, 400)
		return
	}
	if req.Cancel {
		if model.CancelDesktopLogin(req.RequestID, req.Verifier) != nil {
			desktopFailure(c, 410)
			return
		}
		c.JSON(200, gin.H{"success": true, "data": gin.H{"status": "cancelled"}})
		return
	}
	g, err := model.DesktopLoginWithProof(req.RequestID, req.Verifier)
	if err != nil || !desktopProviderEnabled(g.Provider) {
		desktopFailure(c, 410)
		return
	}
	if g.Status == "pending" {
		c.JSON(200, gin.H{"success": true, "data": gin.H{"status": "pending"}})
		return
	}
	if g.Status != "approved" {
		desktopFailure(c, 410)
		return
	}
	u := model.User{Id: g.UserID}
	if u.FillUserById() != nil || u.Status != common.UserStatusEnabled {
		desktopFailure(c, 403)
		return
	}
	if model.ConsumeDesktopLogin(g) != nil {
		desktopFailure(c, 410)
		return
	}
	// Never import the browser's cookie. Mint a separate desktop session.
	sessions.Default(c).Clear()
	setupLogin(&u, c)
}
