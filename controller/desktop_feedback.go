package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const desktopFeedbackBodyLimit = 128 * 1024

func readDesktopFeedbackBody(c *gin.Context, destination interface{}) bool {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, desktopFeedbackBodyLimit+1))
	if len(body) > desktopFeedbackBodyLimit {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "message": "反馈内容过大"})
		return false
	}
	if err != nil || common.Unmarshal(body, destination) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "反馈内容格式无效"})
		return false
	}
	return true
}

func CreateDesktopFeedback(c *gin.Context) {
	var input dto.DesktopFeedbackRequest
	if !readDesktopFeedbackBody(c, &input) {
		return
	}
	row, err := service.NormalizeDesktopFeedback(input, c.GetInt("id"), time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	stored, err := model.CreateDesktopFeedback(c.Request.Context(), row)
	if err != nil {
		desktopFeedbackFailure(c, err)
		return
	}
	common.ApiSuccess(c, dto.DesktopFeedbackReceipt{ID: stored.ID, Status: stored.Status, CreatedAt: stored.CreatedAt})
}

func desktopFeedbackFailure(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "反馈记录不存在"})
		return
	}
	// Database errors can contain submitted data; neither log nor return them.
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "暂时无法处理反馈，请稍后重试"})
}

func GetDesktopFeedbackList(c *gin.Context) {
	filter := model.DesktopFeedbackFilter{Status: c.Query("status"), Platform: c.Query("platform"), AppVersion: c.Query("appVersion")}
	if value := c.Query("userId"); value != "" {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "用户编号无效"})
			return
		}
		filter.UserID = id
	}
	if (filter.Status != "" && !model.ValidDesktopFeedbackStatus(filter.Status)) || (filter.Platform != "" && !service.ValidDesktopFeedbackPlatform(filter.Platform)) || len(filter.AppVersion) > 32 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "反馈筛选条件无效"})
		return
	}
	page := common.GetPageQuery(c)
	if page.Page < 1 || page.Page > 1000000 || page.PageSize < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "分页参数无效"})
		return
	}
	rows, total, err := model.ListDesktopFeedback(c.Request.Context(), filter, page.GetStartIdx(), page.PageSize)
	if err != nil {
		desktopFeedbackFailure(c, err)
		return
	}
	items := make([]*dto.DesktopFeedbackDetail, 0, len(rows))
	for _, row := range rows {
		item, err := service.DesktopFeedbackDetail(&row)
		if err != nil {
			desktopFeedbackFailure(c, err)
			return
		}
		items = append(items, item)
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	common.ApiSuccess(c, page)
}

func desktopFeedbackID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "反馈编号无效"})
		return 0, false
	}
	return id, true
}

func GetDesktopFeedback(c *gin.Context) {
	id, ok := desktopFeedbackID(c)
	if !ok {
		return
	}
	row, err := model.GetDesktopFeedback(c.Request.Context(), id)
	if err != nil {
		desktopFeedbackFailure(c, err)
		return
	}
	detail, err := service.DesktopFeedbackDetail(row)
	if err != nil {
		desktopFeedbackFailure(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func UpdateDesktopFeedback(c *gin.Context) {
	id, ok := desktopFeedbackID(c)
	if !ok {
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if !readDesktopFeedbackBody(c, &input) {
		return
	}
	if !model.ValidDesktopFeedbackStatus(input.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "反馈状态无效"})
		return
	}
	row, err := model.UpdateDesktopFeedbackStatus(c.Request.Context(), id, input.Status, c.GetInt("id"))
	if err != nil {
		desktopFeedbackFailure(c, err)
		return
	}
	detail, err := service.DesktopFeedbackDetail(row)
	if err != nil {
		desktopFeedbackFailure(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}
