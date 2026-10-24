package controller

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// InviteRedirect retains registration even when the optional analytics write fails.
func InviteRedirect(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex, nofollow")
	purpose := strings.ToLower(c.GetHeader("Purpose") + " " + c.GetHeader("Sec-Purpose"))
	if len(code) > 0 && len(code) <= 32 && !strings.Contains(purpose, "prefetch") && !strings.Contains(purpose, "prerender") {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		userID, err := model.GetUserIdByAffCodeWithContext(ctx, code)
		if err == nil {
			if err = model.RecordInvitePromotionClick(ctx, userID, c.Query("src"), time.Now()); err != nil {
				// Do not log the URL, invitation code or visitor identity.
				common.SysError("invite promotion click aggregation failed")
			}
		}
	}
	c.Redirect(http.StatusFound, "/register?aff="+url.QueryEscape(code))
}

func GetSelfInvitePromotion(c *gin.Context) {
	stats, err := model.GetInvitePromotionStats(c.Request.Context(), c.GetInt("id"), time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

func GetAdminInvitePromotion(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的 id"})
		return
	}
	stats, err := model.GetInvitePromotionStats(c.Request.Context(), id, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}
