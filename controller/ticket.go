package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// 限流配置
const (
	// 工单回复：每小时10条
	ticketReplyRateLimit    = 10
	ticketReplyRateDuration = time.Hour
)

// CreateTicketRequest 创建工单请求
type CreateTicketRequest struct {
	Subject   string `json:"subject" binding:"required"`
	Category  string `json:"category" binding:"required"`
	Body      string `json:"body" binding:"required"`
	RequestID string `json:"request_id"` // 可选：关联的请求ID
}

// ReplyTicketRequest 回复工单请求
type ReplyTicketRequest struct {
	Body       string `json:"body" binding:"required"`
	IsInternal bool   `json:"is_internal"` // 仅管理员可设置
}

// UpdateTicketStatusRequest 更新工单状态请求
type UpdateTicketStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// checkTicketReplyRateLimit 检查工单回复限流
func checkTicketReplyRateLimit(ctx context.Context, userID int) error {
	key := fmt.Sprintf("ticket_reply:%d", userID)
	return common.CheckRateLimit(ctx, key, ticketReplyRateLimit, int64(ticketReplyRateDuration.Seconds()))
}

// CreateTicket 创建工单（用户）
func CreateTicket(c *gin.Context) {
	var req CreateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	userID := c.GetInt("id")
	ctx := c.Request.Context()

	if req.Category == model.TicketCategoryAppeal {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请通过申诉入口提交"})
		return
	}
	// 创建工单
	ticket, err := model.CreateTicket(ctx, userID, req.Subject, req.Category, req.RequestID, req.Body)
	if err != nil {
		if errors.Is(err, model.ErrTicketMaxOpen) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "您有过多未关闭的工单，请先关闭一些后再创建",
			})
			return
		}
		if errors.Is(err, model.ErrTicketMaxAppeal) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "您已有一个未关闭的申诉工单",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("创建工单失败：%v", err),
		})
		return
	}

	// 异步通知
	service.NotifyTicketCreated(ticket.ID, req.Subject, req.Category)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "工单创建成功",
		"data": gin.H{
			"id": ticket.ID,
		},
	})
}

// CreateAppealTicket 创建申诉工单（申诉会话）
func CreateAppealTicket(c *gin.Context) {
	var req struct {
		Subject   string `json:"subject" binding:"required"`
		Body      string `json:"body" binding:"required"`
		Category  string `json:"category"`
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	// 强制分类为申诉
	req.Category = model.TicketCategoryAppeal

	userID := c.GetInt("appeal_user_id")
	ctx := c.Request.Context()

	// 创建工单
	ticket, err := model.CreateTicket(ctx, userID, req.Subject, req.Category, req.RequestID, req.Body)
	if err != nil {
		if errors.Is(err, model.ErrTicketMaxAppeal) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "您已有一个未关闭的申诉工单",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("创建申诉工单失败：%v", err),
		})
		return
	}

	// 异步通知
	service.NotifyTicketCreated(ticket.ID, req.Subject, req.Category)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "申诉工单创建成功",
		"data": gin.H{
			"id": ticket.ID,
		},
	})
}

// GetTicket 获取单个工单详情
func GetTicket(c *gin.Context) {
	ticketID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的工单ID",
		})
		return
	}

	userID := c.GetInt("id")
	ctx := c.Request.Context()

	ticket, err := model.GetTicketByID(ctx, ticketID)
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
			"message": fmt.Sprintf("获取工单失败：%v", err),
		})
		return
	}

	// 检查权限
	if ticket.UserID != userID {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此工单",
		})
		return
	}

	// 获取消息
	messages, err := model.ReadTicketMessages(ctx, ticket, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取工单消息失败：%v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"ticket":   ticket,
			"messages": messages,
		},
	})
}

// GetMyTickets 获取我的工单列表
func GetMyTickets(c *gin.Context) {
	userID := c.GetInt("id")
	ctx := c.Request.Context()

	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	tickets, total, err := model.GetUserTickets(ctx, userID, offset, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取工单列表失败：%v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"tickets": tickets,
			"total":   total,
			"page":    page,
		},
	})
}

// ReplyTicket 回复工单（用户）
func ReplyTicket(c *gin.Context) {
	ticketID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的工单ID",
		})
		return
	}

	var req ReplyTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	userID := c.GetInt("id")
	ctx := c.Request.Context()

	// 限流检查
	if err := checkTicketReplyRateLimit(ctx, userID); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 获取工单检查权限
	ticket, err := model.GetTicketByID(ctx, ticketID)
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
			"message": fmt.Sprintf("获取工单失败：%v", err),
		})
		return
	}

	if ticket.UserID != userID {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此工单",
		})
		return
	}

	// 检查工单状态
	if ticket.Status == model.TicketStatusClosed {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "工单已关闭，无法回复",
		})
		return
	}

	// 用户不能设置内部备注
	req.IsInternal = false

	// 回复并转换状态
	var newStatus string
	if ticket.Status == model.TicketStatusWaitingUser {
		newStatus = model.TicketStatusOpen
	}

	err = model.TicketReplyAndTransition(ctx, ticketID, userID, model.SenderRoleUser, req.Body, req.IsInternal, newStatus, ticket.Status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("回复工单失败：%v", err),
		})
		return
	}

	// 异步通知
	service.NotifyTicketUserReply(ticket)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "回复成功",
	})
}

// CloseTicket 关闭工单（用户）
func CloseTicket(c *gin.Context) {
	ticketID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的工单ID",
		})
		return
	}

	userID := c.GetInt("id")
	ctx := c.Request.Context()

	// 获取工单检查权限
	ticket, err := model.GetTicketByID(ctx, ticketID)
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
			"message": fmt.Sprintf("获取工单失败：%v", err),
		})
		return
	}

	if ticket.UserID != userID {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此工单",
		})
		return
	}

	// 关闭工单
	err = model.CloseTicket(ctx, ticketID, userID, ticket.Status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("关闭工单失败：%v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "工单已关闭",
	})
}

// AdminGetTickets 管理员获取工单列表
func AdminGetTickets(c *gin.Context) {
	ctx := c.Request.Context()

	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filter := model.TicketFilter{Status: c.Query("status"), Category: c.Query("category"), Keyword: c.Query("keyword"), Unassigned: c.Query("unassigned") == "true", StaffUnreadOnly: c.Query("unread") == "true"}
	if c.Query("assigned_to_me") == "true" {
		filter.AssignedToMe = c.GetInt("id")
	}
	if (filter.Status != "" && !model.ValidTicketStatus(filter.Status)) || (filter.Category != "" && !model.ValidTicketCategory(filter.Category)) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "筛选参数无效"})
		return
	}
	offset := (page - 1) * pageSize

	tickets, total, err := model.ListTickets(ctx, filter, offset, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取工单列表失败：%v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"tickets": tickets,
			"total":   total,
			"page":    page,
		},
	})
}

// AdminGetTicket 管理员获取工单详情
func AdminGetTicket(c *gin.Context) {
	ticketID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的工单ID",
		})
		return
	}

	ctx := c.Request.Context()

	ticket, err := model.GetTicketByID(ctx, ticketID)
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
			"message": fmt.Sprintf("获取工单失败：%v", err),
		})
		return
	}

	// 获取消息（包括内部备注）
	messages, err := model.ReadTicketMessages(ctx, ticket, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取工单消息失败：%v", err),
		})
		return
	}

	// 获取关联日志（如果有request_id）
	var logs []model.Log
	if ticket.RequestID != "" {
		logs, _ = model.GetTicketRelatedLogs(ctx, ticket.UserID, ticket.RequestID, 50)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"ticket":   ticket,
			"messages": messages,
			"logs":     logs,
		},
	})
}

// AdminReplyTicket 管理员回复工单
func AdminReplyTicket(c *gin.Context) {
	ticketID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的工单ID",
		})
		return
	}

	var req ReplyTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	staffID := c.GetInt("id")
	ctx := c.Request.Context()

	// 获取工单
	ticket, err := model.GetTicketByID(ctx, ticketID)
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
			"message": fmt.Sprintf("获取工单失败：%v", err),
		})
		return
	}

	// 检查工单状态
	if ticket.Status == model.TicketStatusClosed {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "工单已关闭，无法回复",
		})
		return
	}

	// 回复并转换状态（非内部备注时转为waiting_user）
	var newStatus string
	if !req.IsInternal {
		if ticket.Status == model.TicketStatusOpen {
			newStatus = model.TicketStatusWaitingUser
		} else if ticket.Status == model.TicketStatusInProgress {
			newStatus = model.TicketStatusWaitingUser
		}
	}

	err = model.TicketReplyAndTransition(ctx, ticketID, staffID, model.SenderRoleStaff, req.Body, req.IsInternal, newStatus, ticket.Status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("回复工单失败：%v", err),
		})
		return
	}

	// 异步通知（仅非内部备注）
	if !req.IsInternal {
		service.NotifyTicketStaffReply(ticket)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "回复成功",
	})
}

// AdminUpdateTicketStatus 管理员更新工单状态
func AdminUpdateTicketStatus(c *gin.Context) {
	ticketID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的工单ID",
		})
		return
	}

	var req UpdateTicketStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	// 验证状态值
	validStatuses := []string{
		model.TicketStatusOpen,
		model.TicketStatusInProgress,
		model.TicketStatusWaitingUser,
		model.TicketStatusResolved,
		model.TicketStatusClosed,
	}
	valid := false
	for _, s := range validStatuses {
		if req.Status == s {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的状态值",
		})
		return
	}

	ctx := c.Request.Context()

	// 获取当前工单状态
	ticket, err := model.GetTicketByID(ctx, ticketID)
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
			"message": fmt.Sprintf("获取工单失败：%v", err),
		})
		return
	}

	// 更新状态
	err = model.UpdateTicketStatus(ctx, ticketID, req.Status, ticket.Status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("更新工单状态失败：%v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "工单状态已更新",
	})
}

// AdminAssignTicket 管理员分配工单
func AdminAssignTicket(c *gin.Context) {
	ticketID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的工单ID",
		})
		return
	}

	var req struct {
		AssigneeID *int `json:"assignee_id"` // nil 表示取消分配
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	ctx := c.Request.Context()

	// 验证assignee是否存在且有权限
	if req.AssigneeID != nil {
		assignee, err := model.GetUserById(*req.AssigneeID, false)
		if err != nil || assignee == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "分配的用户不存在",
			})
			return
		}
		// 只能分配给管理员
		if assignee.Role < common.RoleAdminUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "只能将工单分配给管理员",
			})
			return
		}
	}

	assigneeID := 0
	if req.AssigneeID != nil {
		assigneeID = *req.AssigneeID
	}
	err = model.AssignTicket(ctx, ticketID, assigneeID)
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
			"message": fmt.Sprintf("分配工单失败：%v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "工单分配成功",
	})
}

// GetUnreadTicketCount 获取未读工单数量
func GetUnreadTicketCount(c *gin.Context) {
	userID := c.GetInt("id")
	ctx := c.Request.Context()

	count, err := model.GetUserUnreadTicketCount(ctx, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取未读数量失败：%v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"count": count,
		},
	})
}

// AdminGetUnreadTicketCount 管理员获取未读工单数量
func AdminGetUnreadTicketCount(c *gin.Context) {
	ctx := c.Request.Context()

	count, err := model.GetStaffUnreadTicketCount(ctx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取未读数量失败：%v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"count": count,
		},
	})
}

// GetAppealSession 获取申诉会话信息
func GetAppealSession(c *gin.Context) {
	userID := c.GetInt("appeal_user_id")

	// 获取用户信息
	user, err := model.GetUserById(userID, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取用户信息失败",
		})
		return
	}

	// 从会话中获取过期时间
	session := sessions.Default(c)
	expiresAt := session.Get("appeal_expires_at")
	if expiresAt == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "会话已失效",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user_id":    userID,
			"username":   user.Username,
			"expires_at": expiresAt,
		},
	})
}

// GetAppealTickets 获取申诉工单列表
func GetAppealTickets(c *gin.Context) {
	userID := c.GetInt("appeal_user_id")
	ctx := c.Request.Context()

	// 只返回该用户的申诉工单
	tickets, _, err := model.ListTickets(ctx, model.TicketFilter{UserID: userID, Category: model.TicketCategoryAppeal}, 0, 100)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取工单列表失败：%v", err),
		})
		return
	}

	// 过滤出申诉工单
	appealTickets := make([]model.Ticket, 0)
	for _, ticket := range tickets {
		if ticket.Category == model.TicketCategoryAppeal {
			appealTickets = append(appealTickets, ticket)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"tickets": appealTickets,
			"total":   len(appealTickets),
		},
	})
}

// GetAppealTicket 获取申诉工单详情
func GetAppealTicket(c *gin.Context) {
	ticketID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的工单ID",
		})
		return
	}

	userID := c.GetInt("appeal_user_id")
	ctx := c.Request.Context()

	ticket, err := model.GetTicketByID(ctx, ticketID)
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
			"message": fmt.Sprintf("获取工单失败：%v", err),
		})
		return
	}

	// 检查权限：必须是本人且分类为申诉
	if ticket.UserID != userID || ticket.Category != model.TicketCategoryAppeal {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此工单",
		})
		return
	}

	// 获取消息（不包括内部备注）
	messages, err := model.ReadTicketMessages(ctx, ticket, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取工单消息失败：%v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"ticket":   ticket,
			"messages": messages,
		},
	})
}

// ReplyAppealTicket 回复申诉工单
func ReplyAppealTicket(c *gin.Context) {
	ticketID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的工单ID",
		})
		return
	}

	var req ReplyTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	userID := c.GetInt("appeal_user_id")
	ctx := c.Request.Context()

	// 限流检查
	if err := checkTicketReplyRateLimit(ctx, userID); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 获取工单检查权限
	ticket, err := model.GetTicketByID(ctx, ticketID)
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
			"message": fmt.Sprintf("获取工单失败：%v", err),
		})
		return
	}

	// 检查权限：必须是本人且分类为申诉
	if ticket.UserID != userID || ticket.Category != model.TicketCategoryAppeal {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此工单",
		})
		return
	}

	// 检查工单状态
	if ticket.Status == model.TicketStatusClosed {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "工单已关闭，无法回复",
		})
		return
	}

	// 用户不能设置内部备注
	req.IsInternal = false

	// 回复并转换状态
	var newStatus string
	if ticket.Status == model.TicketStatusWaitingUser {
		newStatus = model.TicketStatusOpen
	}

	err = model.TicketReplyAndTransition(ctx, ticketID, userID, model.SenderRoleUser, req.Body, req.IsInternal, newStatus, ticket.Status)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("回复工单失败：%v", err),
		})
		return
	}

	// 异步通知
	service.NotifyTicketUserReply(ticket)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "回复成功",
	})
}
