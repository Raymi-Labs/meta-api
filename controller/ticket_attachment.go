package controller

import (
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	MaxTicketAttachmentSize       = 20 * 1024 * 1024 // 20MB
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

func init() {
	if err := os.MkdirAll(TicketAttachmentStorageDir, 0755); err != nil {
		common.FatalLog("无法创建工单附件存储目录: " + err.Error())
	}
}

// UploadTicketAttachment 上传工单附件
func UploadTicketAttachment(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetInt("id")

	ticketIDStr := c.PostForm("ticket_id")
	ticketID, err := strconv.Atoi(ticketIDStr)
	if err != nil || ticketID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的工单ID"})
		return
	}

	// 验证工单权限
	ticket, err := model.GetTicketByID(ctx, ticketID)
	if err != nil {
		if err == model.ErrTicketNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "工单不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询工单失败"})
		}
		return
	}
	if ticket.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权访问此工单"})
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

	// 检查 MIME 类型
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if !allowedAttachmentMimeTypes[mimeType] {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "不支持的文件类型"})
		return
	}

	// 生成存储路径
	filename := header.Filename
	if len(filename) > 200 {
		ext := filepath.Ext(filename)
		filename = filename[:200-len(ext)] + ext
	}
	storagePath := filepath.Join(TicketAttachmentStorageDir, fmt.Sprintf("%d_%d_%s", ticketID, time.Now().UnixNano(), sanitizeFilename(filename)))

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
	userID := c.GetInt("id")

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

	// 验证权限
	ticket, err := model.GetTicketByID(ctx, attachment.TicketID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询工单失败"})
		return
	}
	if ticket.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权访问此附件"})
		return
	}

	// 读取文件
	file, err := os.Open(attachment.StoragePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "读取文件失败"})
		return
	}
	defer file.Close()

	// 设置响应头
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, attachment.Filename))
	c.Header("Content-Type", attachment.MimeType)
	c.Header("Content-Length", strconv.Itoa(attachment.Size))

	// 发送文件
	io.Copy(c.Writer, file)
}

// DeleteTicketAttachment 删除工单附件
func DeleteTicketAttachment(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetInt("id")

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

	// 验证权限
	ticket, err := model.GetTicketByID(ctx, attachment.TicketID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询工单失败"})
		return
	}
	if ticket.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权删除此附件"})
		return
	}

	// 删除物理文件
	os.Remove(attachment.StoragePath)

	// 删除数据库记录
	if err := model.DeleteTicketAttachment(ctx, attachmentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "删除附件失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "附件已删除"})
}

func sanitizeFilename(filename string) string {
	filename = strings.ReplaceAll(filename, "..", "")
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")
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

	// 检查权限
	userID := c.GetInt("id")
	role := c.GetInt("role")
	isAdmin := role >= common.RoleAdminUser

	ticket, err := model.GetTicket(ctx, ticketID)
	if err != nil {
		if errors.Is(err, model.ErrTicketNotFound) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "工单不存在",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取工单失败",
		})
		return
	}

	// 检查权限
	if !isAdmin && ticket.UserID != userID {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此工单",
		})
		return
	}

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
