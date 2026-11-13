package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// setEnterpriseRoutes 注册企业工作空间接口。全部接口受功能开关保护；
// 企业后台接口只认企业成员身份，不复用平台管理员接口，平台侧接口单独挂在 /enterprise/admin 下。
func setEnterpriseRoutes(apiRouter *gin.RouterGroup) {
	enabled := middleware.EnterpriseEnabled()

	// 成员本人与邀请。
	selfRoute := apiRouter.Group("/enterprise", enabled)
	{
		selfRoute.GET("/invitation", middleware.CriticalRateLimit(), controller.PreviewEnterpriseInvitation)
		authed := selfRoute.Group("/", middleware.UserAuth())
		authed.POST("/invitation/accept", middleware.CriticalRateLimit(), controller.AcceptEnterpriseInvitation)
		authed.GET("/self", controller.GetEnterpriseSelf)
		authed.POST("/self/leave", controller.LeaveEnterpriseSelf)
		authed.GET("/self/requests", controller.ListEnterpriseSelfRequests)
		authed.POST("/self/requests", controller.CreateEnterpriseSelfRequest)
		authed.POST("/self/requests/:request_id/cancel", controller.CancelEnterpriseSelfRequest)
	}

	// 企业后台：所有者与管理员。更细的权限（任免管理员、改设置、禁止自批等）在数据层事务内判定。
	manageRoute := apiRouter.Group("/enterprises/:id", enabled, middleware.UserAuth(),
		middleware.EnterpriseMemberAuth(model.EnterpriseRoleAdmin))
	{
		manageRoute.GET("/overview", controller.GetEnterpriseOverview)
		manageRoute.GET("/members", controller.ListEnterpriseMembers)
		manageRoute.GET("/members/:member_id", controller.GetEnterpriseMember)
		manageRoute.POST("/members/:member_id/allocate", controller.AllocateEnterpriseQuota)
		manageRoute.POST("/members/:member_id/reclaim", controller.ReclaimEnterpriseQuota)
		manageRoute.PUT("/members/:member_id/models", controller.SetEnterpriseMemberModels)
		manageRoute.PUT("/members/:member_id/role", controller.SetEnterpriseMemberRole)
		manageRoute.POST("/members/:member_id/transfer-ownership", controller.TransferEnterpriseOwnership)
		manageRoute.POST("/members/:member_id/disable", controller.DisableEnterpriseMember)
		manageRoute.GET("/invitations", controller.ListEnterpriseInvitations)
		manageRoute.POST("/invitations", controller.CreateEnterpriseInvitation)
		manageRoute.POST("/invitations/:invitation_id/revoke", controller.RevokeEnterpriseInvitation)
		manageRoute.GET("/quota-requests", controller.ListEnterpriseQuotaRequests)
		manageRoute.POST("/quota-requests/:request_id/approve", controller.ApproveEnterpriseQuotaRequest)
		manageRoute.POST("/quota-requests/:request_id/reject", controller.RejectEnterpriseQuotaRequest)
		manageRoute.GET("/usage", controller.ListEnterpriseUsage)
		manageRoute.GET("/usage/summary", controller.GetEnterpriseUsageSummary)
		manageRoute.GET("/usage/export", controller.ExportEnterpriseUsage)
		manageRoute.GET("/tokens", controller.ListEnterpriseTokens)
		manageRoute.POST("/tokens/:token_id/revoke", controller.RevokeEnterpriseToken)
		manageRoute.GET("/ledger", controller.ListEnterpriseLedger)
		manageRoute.GET("/audit", controller.ListEnterpriseAudit)
		manageRoute.GET("/model-options", controller.GetEnterpriseModelOptions)
		manageRoute.PUT("/settings", controller.UpdateEnterpriseSettings)
	}

	// 平台后台：开通与冻结、入账与调账、待对账预留。动钱的操作要求超级管理员。
	adminRoute := apiRouter.Group("/enterprise/admin", enabled, middleware.AdminAuth())
	{
		adminRoute.GET("/enterprises", controller.AdminListEnterprises)
		adminRoute.POST("/enterprises", controller.AdminCreateEnterprise)
		adminRoute.GET("/reservations", controller.AdminListEnterpriseReservations)
		adminRoute.POST("/reservations/:reservation_id/resolve", middleware.RootAuth(), controller.AdminResolveEnterpriseReservation)

		scoped := adminRoute.Group("/enterprises/:id", middleware.EnterprisePlatformScope())
		scoped.GET("/overview", controller.GetEnterpriseOverview)
		scoped.GET("/members", controller.ListEnterpriseMembers)
		scoped.GET("/usage", controller.ListEnterpriseUsage)
		scoped.GET("/usage/summary", controller.GetEnterpriseUsageSummary)
		scoped.GET("/usage/export", controller.ExportEnterpriseUsage)
		scoped.GET("/tokens", controller.ListEnterpriseTokens)
		scoped.GET("/ledger", controller.ListEnterpriseLedger)
		scoped.GET("/audit", controller.ListEnterpriseAudit)
		scoped.PUT("", controller.AdminUpdateEnterprise)
		scoped.POST("/status", controller.AdminSetEnterpriseStatus)
		scoped.POST("/credit", middleware.RootAuth(), controller.AdminCreditEnterprise)
		scoped.POST("/adjust", middleware.RootAuth(), controller.AdminAdjustEnterprise)
	}
}
