package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func setInviteTierRouter(api *gin.RouterGroup) {
	user := api.Group("/invite", middleware.UserAuth())
	user.GET("/tier/status", controller.GetInviteTierStatus)
	user.GET("/commission/statements", controller.ListInviteResource("statements", false))
	user.GET("/commission/ledgers", controller.ListInviteResource("ledgers", false))
	user.GET("/department/apply", controller.ListInviteResource("applications", false))
	user.GET("/departments", controller.ListInviteResource("departments", false))
	user.GET("/early-payout", controller.ListInviteResource("early-payout", false))
	user.GET("/identity", controller.ListInviteResource("identities", false))
	user.GET("/rebind", controller.ListInviteResource("rebind", false))
	for path, action := range map[string]string{"/department/apply": "apply", "/department/leave": "leave", "/early-payout": "early", "/identity": "identity", "/device": "device", "/rebind": "rebind", "/rebind/:id/confirm": "confirm-rebind"} {
		user.POST(path, middleware.CriticalRateLimit(), controller.InviteUserMutation(action))
	}
	admin := api.Group("/admin/invite", middleware.AdminAuth())
	admin.GET("/tier/settings", controller.GetInviteTierSettings)
	admin.PUT("/tier/settings", controller.PutInviteTierSettings)
	admin.GET("/promoters/:id", controller.GetInviteTierStatus)
	for _, resource := range []string{"statements", "ledgers", "applications", "early-payout", "identities", "departments", "actions", "signals"} {
		admin.GET("/"+resource, controller.ListInviteResource(resource, true))
	}
	for path, action := range map[string]string{"/applications/:id/review": "application", "/early-payout/:id/review": "early", "/identities/:id/review": "identity", "/statements/:id/freeze": "statement", "/actions/:id/reverse": "reverse", "/promoters/:id/tier": "tier", "/promoters/:id/freeze": "freeze", "/promoters/:id/clear": "clear", "/promoters/:id/clawback": "clawback"} {
		admin.POST(path, middleware.CriticalRateLimit(), controller.InviteAdminMutation(action))
	}
	admin.GET("/identities/:id/plaintext", middleware.RootAuth(), controller.DecryptInviteIdentity)
}
