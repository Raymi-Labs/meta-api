package controller

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

const (
	MaxTicketAttachmentSize       = 5 * 1024 * 1024 // 5MB
	MaxTicketAttachmentsPerTicket = 10
	TicketAttachmentStorageDir    = "./data/ticket_attachments"
)

var allowedAttachmentMimeTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
	"text/plain":      true,
}

// ticketAccess is shared by normal, administrator and appeal attachment routes.
func ticketAccess(c *gin.Context, ticketID int) (*model.Ticket, bool) {
	ticket, err := model.GetTicket(c.Request.Context(), ticketID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "工单不存在"})
		return nil, false
	}
	appealID := c.GetInt("appeal_user_id")
	allowed := false
	if appealID > 0 {
		allowed = ticket.UserID == appealID && ticket.Category == model.TicketCategoryAppeal
	} else {
		allowed = ticket.UserID == c.GetInt("id") || c.GetInt("role") >= common.RoleAdminUser
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权访问此工单"})
		return nil, false
	}
	return ticket, true
}

func attachmentAccess(c *gin.Context, attachment *model.TicketAttachment) bool {
	if _, ok := ticketAccess(c, attachment.TicketID); !ok {
		return false
	}
	if attachment.MessageID != nil && (c.GetInt("appeal_user_id") > 0 || c.GetInt("role") < common.RoleAdminUser) {
		var message model.TicketMessage
		err := model.DB.WithContext(c.Request.Context()).Where("id = ? AND ticket_id = ? AND is_internal = ?", *attachment.MessageID, attachment.TicketID, false).First(&message).Error
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "附件不存在"})
			return false
		}
	}
	return true
}

// UploadTicketAttachment 上传工单附件
func UploadTicketAttachment(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetInt("id")
	if c.GetInt("appeal_user_id") > 0 {
		userID = c.GetInt("appeal_user_id")
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxTicketAttachmentSize+64*1024)
	if err := c.Request.ParseMultipartForm(MaxTicketAttachmentSize); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "文件大小不能超过5MB或上传格式无效"})
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	ticketIDStr := c.PostForm("ticket_id")
	ticketID, err := strconv.Atoi(ticketIDStr)
	if err != nil || ticketID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的工单ID"})
		return
	}

	ticket, ok := ticketAccess(c, ticketID)
	if !ok {
		return
	}
	if ticket.Status == model.TicketStatusClosed {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "工单已关闭"})
		return
	}
	if err := common.CheckRateLimit(ctx, fmt.Sprintf("ticket_upload:%d", userID), 20, 3600); err != nil {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": err.Error()})
		return
	}
	// 检查附件数量限制
	count, err := model.GetTicketAttachmentCount(ctx, ticketID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询附件数量失败"})
		return
	}
	if count >= MaxTicketAttachmentsPerTicket {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("每个工单最多%d个附件", MaxTicketAttachmentsPerTicket)})
		return
	}

	// 读取上传文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "读取文件失败"})
		return
	}
	defer file.Close()

	// 检查文件大小
	if header.Size > MaxTicketAttachmentSize {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("文件大小不能超过%dMB", MaxTicketAttachmentSize/(1024*1024))})
		return
	}

	// Sniff the content; browser supplied MIME and suffixes are not trusted.
	prefix := make([]byte, 512)
	n, err := file.Read(prefix)
	if err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "读取文件失败"})
		return
	}
	mimeType := strings.Split(http.DetectContentType(prefix[:n]), ";")[0]
	if !allowedAttachmentMimeTypes[mimeType] {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "不支持的文件类型"})
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "读取文件失败"})
		return
	}
	filename := sanitizeFilename(header.Filename)
	if err := os.MkdirAll(TicketAttachmentStorageDir, 0700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "保存文件失败"})
		return
	}
	storagePath := filepath.Join(TicketAttachmentStorageDir, common.GetUUID())
	// 保存文件
	out, err := os.Create(storagePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "保存文件失败"})
		return
	}
	defer out.Close()

	written, err := io.Copy(out, file)
	if err != nil {
		os.Remove(storagePath)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "写入文件失败"})
		return
	}

	// 创建附件记录
	attachment := &model.TicketAttachment{
		TicketID:     ticketID,
		MessageID:    nil,
		Filename:     filename,
		MimeType:     mimeType,
		Size:         int(written),
		StoragePath:  storagePath,
		UploadedByID: userID,
	}
	attachment, err = model.CreateTicketAttachment(ctx, attachment)
	if err != nil {
		os.Remove(storagePath)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建附件记录失败"})
		return
	}

	if userID == ticket.UserID {
		service.NotifyTicketUserReply(ticket)
	} else {
		service.NotifyTicketStaffReply(ticket)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":         attachment.ID,
			"ticket_id":  attachment.TicketID,
			"filename":   attachment.Filename,
			"mime_type":  attachment.MimeType,
			"size":       attachment.Size,
			"created_at": attachment.CreatedAt,
		},
	})
}

// DownloadTicketAttachment 下载工单附件
func DownloadTicketAttachment(c *gin.Context) {
	ctx := c.Request.Context()

	attachmentIDStr := c.Param("id")
	attachmentID, err := strconv.Atoi(attachmentIDStr)
	if err != nil || attachmentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的附件ID"})
		return
	}

	// 获取附件记录
	attachment, err := model.GetTicketAttachmentByID(ctx, attachmentID)
	if err != nil {
		if err == model.ErrTicketAttachmentNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "附件不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询附件失败"})
		}
		return
	}

	if !attachmentAccess(c, attachment) {
		return
	}
	// 读取文件
	file, err := os.Open(attachment.StoragePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "读取文件失败"})
		return
	}
	defer file.Close()

	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	// 设置响应头
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Filename}))
	c.Header("Content-Type", attachment.MimeType)
	c.Header("Content-Length", strconv.Itoa(attachment.Size))

	// 发送文件
	io.Copy(c.Writer, file)
}

// DeleteTicketAttachment 删除工单附件
func DeleteTicketAttachment(c *gin.Context) {
	ctx := c.Request.Context()

	attachmentIDStr := c.Param("id")
	attachmentID, err := strconv.Atoi(attachmentIDStr)
	if err != nil || attachmentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的附件ID"})
		return
	}

	// 获取附件记录
	attachment, err := model.GetTicketAttachmentByID(ctx, attachmentID)
	if err != nil {
		if err == model.ErrTicketAttachmentNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "附件不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询附件失败"})
		}
		return
	}

	if !attachmentAccess(c, attachment) {
		return
	}
	// Delete metadata first: a database failure must not destroy a still-visible attachment.
	if err := model.DeleteTicketAttachment(ctx, attachmentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "删除附件失败"})
		return
	}
	if err := os.Remove(attachment.StoragePath); err != nil && !os.IsNotExist(err) {
		common.SysLog(fmt.Sprintf("ticket attachment %d metadata deleted; file cleanup failed: %v", attachmentID, err))
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "附件已删除"})
}

func sanitizeFilename(filename string) string {
	filename = filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	filename = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, filename)
	for len(filename) > 200 {
		r := []rune(filename)
		filename = string(r[:len(r)-1])
	}
	if filename == "" || filename == "." || filename == ".." {
		return "attachment"
	}
	return filename
}

// ListTicketAttachments 列出工单的所有附件
func ListTicketAttachments(c *gin.Context) {
	ctx := c.Request.Context()
	ticketIDStr := c.Param("id")
	ticketID, err := strconv.Atoi(ticketIDStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "工单ID无效",
		})
		return
	}

	if _, ok := ticketAccess(c, ticketID); !ok {
		return
	}
	isAdmin := c.GetInt("appeal_user_id") == 0 && c.GetInt("role") >= common.RoleAdminUser
	// 管理员可以看到所有附件，用户只能看到非内部消息的附件
	attachments, err := model.GetTicketAttachments(ctx, ticketID, isAdmin)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取附件列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    attachments,
	})
}
