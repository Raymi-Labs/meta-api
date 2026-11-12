package middleware

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func abortEnterprise(c *gin.Context, err error) {
	c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
	c.Abort()
}

// EnterpriseEnabled 在功能开关关闭时拒绝全部企业接口。
func EnterpriseEnabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.EnterpriseEnabled {
			abortEnterprise(c, model.ErrEnterpriseDisabled)
			return
		}
		c.Next()
	}
}

// EnterpriseMemberAuth 必须排在 UserAuth 之后。它确认登录账号是路径中企业的有效成员且角色不低于 minRole，
// 并把企业与成员写入上下文。非成员、跨企业与企业不存在返回同一个响应，避免探测其他企业。
// 这里只做入口拦截；写操作在数据层事务内重新读取角色判定权限。
func EnterpriseMemberAuth(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		enterpriseID, err := strconv.Atoi(c.Param("id"))
		if err != nil || enterpriseID <= 0 {
			abortEnterprise(c, model.ErrEnterpriseNotFound)
			return
		}
		member, err := model.GetActiveEnterpriseMemberByUser(c.GetInt("id"))
		if err != nil {
			logger.LogError(c.Request.Context(), "enterprise member lookup failed: "+err.Error())
			abortEnterprise(c, model.ErrEnterpriseNotFound)
			return
		}
		if member == nil || member.EnterpriseId != enterpriseID {
			abortEnterprise(c, model.ErrEnterpriseNotFound)
			return
		}
		if !model.EnterpriseRoleAtLeast(member.Role, minRole) {
			abortEnterprise(c, model.ErrEnterpriseForbidden)
			return
		}
		common.SetContextKey(c, constant.ContextKeyEnterpriseId, enterpriseID)
		common.SetContextKey(c, constant.ContextKeyEnterpriseMemberId, member.Id)
		common.SetContextKey(c, constant.ContextKeyEnterpriseRole, member.Role)
		c.Next()
	}
}

// EnterprisePlatformScope 必须排在 AdminAuth 之后，把路径中的企业写入上下文并标记为平台操作。
// 自行再核对一次平台角色，避免路由漏挂 AdminAuth 时把普通账号当成平台操作者。
func EnterprisePlatformScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		enterpriseID, err := strconv.Atoi(c.Param("id"))
		if err != nil || enterpriseID <= 0 || c.GetInt("role") < common.RoleAdminUser {
			abortEnterprise(c, model.ErrEnterpriseNotFound)
			return
		}
		common.SetContextKey(c, constant.ContextKeyEnterpriseId, enterpriseID)
		common.SetContextKey(c, constant.ContextKeyEnterprisePlatform, true)
		c.Next()
	}
}
