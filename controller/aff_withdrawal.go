package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type CreateAffWithdrawalRequest struct {
	Quota         int    `json:"quota" binding:"required"`
	AlipayAccount string `json:"alipay_account" binding:"required"`
	AlipayName    string `json:"alipay_name" binding:"required"`
}

type ReviewAffWithdrawalRequest struct {
	AdminRemark string `json:"admin_remark"`
}

func CreateAffWithdrawal(c *gin.Context) {
	var req CreateAffWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	withdrawal, err := model.CreateAffWithdrawal(c.GetInt("id"), req.Quota, req.AlipayAccount, req.AlipayName)
	if err != nil {
		if errors.Is(err, model.ErrInviteMinimum) || errors.Is(err, model.ErrInviteIdentity) || errors.Is(err, model.ErrInviteFrozen) {
			inviteError(c, err)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, withdrawal)
}

func GetUserAffWithdrawals(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	params := model.AffWithdrawalSearchParams{
		Status: c.Query("status"),
	}

	withdrawals, total, err := model.GetUserAffWithdrawalsByParams(c.GetInt("id"), params, pageInfo)
	if err != nil {
		if errors.Is(err, model.ErrInviteMinimum) || errors.Is(err, model.ErrInviteIdentity) || errors.Is(err, model.ErrInviteFrozen) {
			inviteError(c, err)
			return
		}
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(withdrawals)
	common.ApiSuccess(c, pageInfo)
}

func GetAllAffWithdrawals(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	params := model.AffWithdrawalSearchParams{
		Status:   c.Query("status"),
		Username: c.Query("username"),
	}

	withdrawals, total, err := model.GetAllAffWithdrawalsByParams(params, pageInfo)
	if err != nil {
		if errors.Is(err, model.ErrInviteMinimum) || errors.Is(err, model.ErrInviteIdentity) || errors.Is(err, model.ErrInviteFrozen) {
			inviteError(c, err)
			return
		}
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(withdrawals)
	common.ApiSuccess(c, pageInfo)
}

func ApproveAffWithdrawal(c *gin.Context) {
	reviewAffWithdrawal(c, model.AffWithdrawalStatusApproved)
}

func RejectAffWithdrawal(c *gin.Context) {
	reviewAffWithdrawal(c, model.AffWithdrawalStatusRejected)
}

func reviewAffWithdrawal(c *gin.Context, targetStatus string) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	var req ReviewAffWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	var withdrawal *model.AffWithdrawal
	if targetStatus == model.AffWithdrawalStatusApproved {
		withdrawal, err = model.ApproveAffWithdrawal(id, c.GetInt("id"), req.AdminRemark)
	} else {
		withdrawal, err = model.RejectAffWithdrawal(id, c.GetInt("id"), req.AdminRemark)
	}
	if err != nil {
		if errors.Is(err, model.ErrInviteMinimum) || errors.Is(err, model.ErrInviteIdentity) || errors.Is(err, model.ErrInviteFrozen) {
			inviteError(c, err)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, withdrawal)
}
