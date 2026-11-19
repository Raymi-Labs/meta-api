package controller

import (
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
)

func inviteError(c *gin.Context, e error) {
	key := "invite.error"
	if e.Error() == "invite.crypto_required" {
		key = e.Error()
	}
	for _, known := range []error{model.ErrInviteEligibility, model.ErrInviteConflict, model.ErrInviteInvalid, model.ErrInviteFrozen, model.ErrInviteIdentity, model.ErrInviteMinimum} {
		if errors.Is(e, known) {
			key = known.Error()
			break
		}
	}
	c.JSON(200, gin.H{"success": false, "message": i18n.T(c, key)})
}
func inviteID(c *gin.Context) (int, bool) {
	id, e := strconv.Atoi(c.Param("id"))
	if e != nil || id <= 0 {
		inviteError(c, model.ErrInviteInvalid)
		return 0, false
	}
	return id, true
}
func inviteBody(c *gin.Context) (dto.InviteTierRequest, bool) {
	var req dto.InviteTierRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if e := common.DecodeJson(c.Request.Body, &req); e != nil || len([]rune(req.Remark)) > 255 {
		inviteError(c, model.ErrInviteInvalid)
		return req, false
	}
	return req, true
}
func inviteAudit(c *gin.Context, subject int, action string) {
	model.RecordLogWithAdminInfo(subject, model.LogTypeManage, action, map[string]interface{}{"admin_id": c.GetInt("id")})
}
func GetInviteTierStatus(c *gin.Context) {
	id := c.GetInt("id")
	if c.Param("id") != "" {
		var ok bool
		id, ok = inviteID(c)
		if !ok {
			return
		}
	}
	status, e := model.GetInviteTierStatus(id)
	if e != nil {
		inviteError(c, e)
		return
	}
	common.ApiSuccess(c, status)
}
func ListInviteResource(resource string, admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := common.GetPageQuery(c)
		user := c.GetInt("id")
		if admin {
			user = 0
			if raw := c.Query("user_id"); raw != "" {
				n, e := strconv.Atoi(raw)
				if e != nil || n <= 0 {
					inviteError(c, model.ErrInviteInvalid)
					return
				}
				user = n
			}
		}
		statement := 0
		if raw := c.Query("statement_id"); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n <= 0 {
				inviteError(c, model.ErrInviteInvalid)
				return
			}
			statement = n
		}
		if len(c.Query("status")) > 24 {
			inviteError(c, model.ErrInviteInvalid)
			return
		}
		if e := model.GetInviteList(user, admin, resource, statement, c.Query("status"), page); e != nil {
			inviteError(c, e)
			return
		}
		common.ApiSuccess(c, page)
	}
}
func GetInviteTierSettings(c *gin.Context) { common.ApiSuccess(c, common.GetInviteTierSettings()) }
func PutInviteTierSettings(c *gin.Context) {
	var value map[string]any
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	if e := common.DecodeJson(c.Request.Body, &value); e != nil {
		inviteError(c, model.ErrInviteInvalid)
		return
	}
	raw, e := common.Marshal(value)
	if e != nil {
		inviteError(c, e)
		return
	}
	if e = model.UpdateOption("InviteTierSettings", string(raw)); e != nil {
		inviteError(c, e)
		return
	}
	inviteAudit(c, c.GetInt("id"), "更新邀请档位配置")
	common.ApiSuccess(c, common.GetInviteTierSettings())
}
func InviteUserMutation(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.GetInviteTierSettings().Enabled {
			inviteError(c, model.ErrInviteEligibility)
			return
		}
		user := c.GetInt("id")
		var data any
		var e error
		req, ok := inviteBody(c)
		if !ok {
			return
		}
		switch action {
		case "apply":
			data, e = model.ApplyPromoter(user, req.Tier, req.DepartmentId)
		case "leave":
			e = model.LeaveInviteDepartment(user)
		case "early":
			data, e = model.RequestInviteEarlyPayout(user)
		case "identity":
			e = model.SavePromoterIdentity(user, req.Name, req.Document)
		case "device":
			e = model.SaveInviteDevice(user, req.Device)
		case "rebind":
			e = model.RequestInviteRebind(user, req.NewInviterId)
		case "confirm-rebind":
			id, ok := inviteID(c)
			if !ok {
				return
			}
			e = model.ConfirmInviteRebind(id, user)
		default:
			e = model.ErrInviteInvalid
		}
		if e != nil {
			inviteError(c, e)
			return
		}
		common.ApiSuccess(c, data)
	}
}
func InviteAdminMutation(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := inviteID(c)
		if !ok {
			return
		}
		req, ok := inviteBody(c)
		if !ok {
			return
		}
		var e error
		var data any
		admin := c.GetInt("id")
		subject, err := model.InviteAdminSubject(action, id)
		if err != nil {
			inviteError(c, err)
			return
		}
		target, err := model.GetUserById(subject, false)
		if err != nil {
			inviteError(c, err)
			return
		}
		if c.GetInt("role") != common.RoleRootUser && c.GetInt("role") <= target.Role {
			inviteError(c, model.ErrInviteEligibility)
			return
		}
		switch action {
		case "application":
			e = model.ReviewPromoterApplication(id, admin, req.Approve, req.Remark)
		case "early":
			e = model.ReviewInviteEarlyPayout(id, admin, req.Approve, req.Remark)
		case "identity":
			e = model.ReviewPromoterIdentity(id, admin, req.Approve, req.Remark)
		case "statement":
			e = model.SetInviteStatementFrozen(id, req.Frozen)
		case "reverse":
			e = model.ReverseInviteAdminAction(id, admin)
		default:
			data, e = model.ChangeInvitePromoter(id, admin, action, req.Tier, req.ExpiresAt, req.Frozen, req.Amount, req.Remark)
		}
		if e != nil {
			inviteError(c, e)
			return
		}
		inviteAudit(c, subject, fmt.Sprintf("邀请管理 %s #%d", action, id))
		model.NotifyInviteCommissionSettlement()
		common.ApiSuccess(c, data)
	}
}
func DecryptInviteIdentity(c *gin.Context) {
	id, ok := inviteID(c)
	if !ok {
		return
	}
	reason := strings.TrimSpace(c.Query("reason"))
	if len([]rune(reason)) < 4 || len([]rune(reason)) > 255 {
		inviteError(c, model.ErrInviteInvalid)
		return
	}
	data, e := model.InviteIdentityPlaintext(id)
	if e != nil {
		inviteError(c, e)
		return
	}
	inviteAudit(c, id, "查看实名原文："+reason)
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, data)
}
