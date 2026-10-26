package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func ListAnnouncementEmailJobs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	page = max(1, min(page, 10000))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if size < 1 {
		size = 10
	}
	size = min(size, 100)
	items, total, err := model.ListAnnouncementEmailJobs((page-1)*size, size)
	if err != nil {
		c.JSON(200, gin.H{"success": false, "message": "公告邮件任务读取失败"})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"items": items, "total": total, "page": page, "page_size": size}})
}
