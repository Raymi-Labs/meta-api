package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetSelfQuotaNotices(c *gin.Context) { listQuotaNotices(c, c.GetInt("id")) }
func GetUserQuotaNotices(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"success": false, "message": "无效用户 ID"})
		return
	}
	listQuotaNotices(c, id)
}
func listQuotaNotices(c *gin.Context, userID int) {
	if userID <= 0 {
		c.JSON(401, gin.H{"success": false, "message": "请先登录"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		page = 10000
	}
	rows, total, err := model.ListQuotaNotices(userID, (page-1)*10, 10)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"items": rows, "total": total, "page": page}})
}
