package router

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetupTicketRouter(router *gin.RouterGroup) {
	jsonBodyLimit := middleware.TicketRequestBodyLimit()
	// 用户工单路由
	ticketGroup := router.Group("/ticket")
	ticketGroup.Use(middleware.UserAuth(), middleware.TicketAccountAuth(common.RoleCommonUser))
	{
		ticketGroup.POST("", jsonBodyLimit, controller.CreateTicket)
		ticketGroup.GET("", controller.GetMyTickets)
		ticketGroup.GET("/unread", controller.GetUnreadTicketCount)
		ticketGroup.GET("/:id", controller.GetTicket)
		ticketGroup.POST("/:id/reply", jsonBodyLimit, controller.ReplyTicket)
		ticketGroup.POST("/:id/close", jsonBodyLimit, controller.CloseTicket)
	}

	// 申诉用户工单路由
	appealGroup := router.Group("/appeal")
	appealGroup.Use(middleware.AppealAuth())
	{
		appealGroup.GET("/tickets/:id/attachments", controller.ListTicketAttachments)
		appealGroup.POST("/attachment/upload", controller.UploadTicketAttachment)
		appealGroup.GET("/attachment/:id/download", controller.DownloadTicketAttachment)
		appealGroup.DELETE("/attachment/:id", controller.DeleteTicketAttachment)
		appealGroup.GET("/session", controller.GetAppealSession)
		appealGroup.POST("/tickets", jsonBodyLimit, controller.CreateAppealTicket)
		appealGroup.GET("/tickets", controller.GetAppealTickets)
		appealGroup.GET("/tickets/:id", controller.GetAppealTicket)
		appealGroup.POST("/tickets/:id/reply", jsonBodyLimit, controller.ReplyAppealTicket)
	}

	// 管理员工单路由
	adminGroup := router.Group("/admin/ticket")
	adminGroup.Use(middleware.AdminAuth(), middleware.TicketAccountAuth(common.RoleAdminUser))
	{
		adminGroup.GET("", controller.AdminGetTickets)
		adminGroup.GET("/unread", controller.AdminGetUnreadTicketCount)
		adminGroup.GET("/:id", controller.AdminGetTicket)
		adminGroup.POST("/:id/reply", jsonBodyLimit, controller.AdminReplyTicket)
		adminGroup.PUT("/:id/status", jsonBodyLimit, controller.AdminUpdateTicketStatus)
		adminGroup.PUT("/:id/assign", jsonBodyLimit, controller.AdminAssignTicket)
	}

	// 附件路由
	attachmentGroup := router.Group("/ticket")
	attachmentGroup.Use(middleware.UserAuth(), middleware.TicketAccountAuth(common.RoleCommonUser))
	{
		attachmentGroup.GET("/:id/attachments", controller.ListTicketAttachments)
		attachmentGroup.POST("/attachment/upload", controller.UploadTicketAttachment)
		attachmentGroup.GET("/attachment/:id/download", controller.DownloadTicketAttachment)
		attachmentGroup.DELETE("/attachment/:id", controller.DeleteTicketAttachment)
	}

	// 管理员附件路由
	adminAttachmentGroup := router.Group("/admin/ticket")
	adminAttachmentGroup.Use(middleware.AdminAuth(), middleware.TicketAccountAuth(common.RoleAdminUser))
	{
		adminAttachmentGroup.GET("/:id/attachments", controller.ListTicketAttachments)
		adminAttachmentGroup.GET("/attachment/:id/download", controller.DownloadTicketAttachment)
		adminAttachmentGroup.DELETE("/attachment/:id", controller.DeleteTicketAttachment)
	}
}
