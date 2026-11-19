package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// AppealAuth never grants ordinary user scope and rechecks account state.
func AppealAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		id, okID := session.Get("appeal_user_id").(int)
		expires, okExpires := session.Get("appeal_expires_at").(int64)
		if okID && okExpires && id > 0 && time.Now().Unix() < expires {
			user, err := model.GetUserById(id, false)
			if err == nil && user.Status == common.UserStatusDisabled {
				c.Set("appeal_user_id", id)
				c.Next()
				return
			}
		}
		session.Delete("appeal_user_id")
		session.Delete("appeal_expires_at")
		_ = session.Save()
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "申诉会话无效或已过期，请重新登录"})
	}
}

// TicketAccountAuth rejects stale sessions after bans or administrative demotion.
func TicketAccountAuth(minRole int) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := model.GetUserById(c.GetInt("id"), false)
		if err != nil || user.Status != common.UserStatusEnabled || user.Role < minRole {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "账号状态或权限已变更，请重新登录"})
			return
		}
		c.Set("role", user.Role)
		c.Next()
	}
}
