package middleware

import (
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// AppealAuth 申诉会话鉴权中间件
// 用于保护申诉工单提交接口，确保请求来自有效的申诉会话
func AppealAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)

		// 检查申诉会话
		appealUserID := session.Get("appeal_user_id")
		appealExpiresAt := session.Get("appeal_expires_at")

		if appealUserID == nil || appealExpiresAt == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "申诉会话无效或已过期，请重新登录",
			})
			c.Abort()
			return
		}

		userID, ok1 := appealUserID.(int)
		expiresAt, ok2 := appealExpiresAt.(int64)
		if !ok1 || !ok2 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "申诉会话数据格式错误",
			})
			c.Abort()
			return
		}

		// 检查是否过期
		if time.Now().Unix() > expiresAt {
			session.Delete("appeal_user_id")
			session.Delete("appeal_expires_at")
			_ = session.Save()
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "申诉会话已过期，请重新登录",
			})
			c.Abort()
			return
		}

		// 将用户ID存入上下文
		c.Set("appeal_user_id", userID)
		c.Next()
	}
}
