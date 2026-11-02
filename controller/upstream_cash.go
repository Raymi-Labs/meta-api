package controller

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func cashAccountID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账户")
		return 0, false
	}
	return id, true
}

func EnableCashAccounting(c *gin.Context) {
	data, err := model.EnableCashAccounting(c.GetInt("id"))
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func GetUpstreamCashReport(c *gin.Context) {
	id, ok := cashAccountID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	data, err := model.GetCashReport(ctx, id, c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func SyncUpstreamCash(c *gin.Context) {
	id, ok := cashAccountID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Minute)
	defer cancel()
	data, err := model.SyncCashAccount(ctx, id)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func GetCashBindings(c *gin.Context) {
	id, ok := cashAccountID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	data, err := model.GetCashBindingCandidates(ctx, id)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func SaveCashBinding(c *gin.Context) {
	id, ok := cashAccountID(c)
	if !ok {
		return
	}
	var input struct {
		KeyHash  string `json:"key_hash"`
		SiteOnly bool   `json:"site_only"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	data, err := model.SaveCashBinding(ctx, id, c.GetInt("id"), input.KeyHash, input.SiteOnly)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func cashPagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	size = min(max(size, 1), 100)
	page = min(max(page, 1), 100000)
	return (page - 1) * size, size
}

func GetCashFundingLots(c *gin.Context) {
	offset, limit := cashPagination(c)
	userID, _ := strconv.Atoi(c.Query("user_id"))
	rows, total, err := model.ListCashFundingLots(offset, limit, userID, c.Query("pending") == "true")
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows, "total": total, "quota_per_unit": common.QuotaPerUnit})
}

func SaveCashFundingReceipt(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("lot_id"), 10, 64)
	var input model.CashReceiptInput
	if err != nil || id <= 0 || c.ShouldBindJSON(&input) != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	data, err := model.SaveCashFundingReceipt(id, c.GetInt("id"), input)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func GetCashFundingReceipts(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("lot_id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	offset, limit := cashPagination(c)
	data, total, err := model.ListCashFundingReceipts(id, offset, limit)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": data, "total": total})
}

func GetCashUpstreamEntries(c *gin.Context) {
	id, ok := cashAccountID(c)
	if !ok {
		return
	}
	offset, limit := cashPagination(c)
	data, total, err := model.ListCashUpstreamEntries(id, offset, limit)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": data, "total": total})
}

func SaveCashUpstreamEntry(c *gin.Context) {
	id, ok := cashAccountID(c)
	if !ok {
		return
	}
	var input model.CashEntryInput
	if c.ShouldBindJSON(&input) != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	data, err := model.SaveCashUpstreamEntry(id, c.GetInt("id"), input)
	if err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func VoidCashUpstreamEntry(c *gin.Context) {
	id, ok := cashAccountID(c)
	if !ok {
		return
	}
	entryID, err := strconv.ParseInt(c.Param("entry_id"), 10, 64)
	var input struct {
		Reason string `json:"reason"`
	}
	if err != nil || entryID <= 0 || c.ShouldBindJSON(&input) != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err = model.VoidCashUpstreamEntry(id, entryID, c.GetInt("id"), input.Reason); err != nil {
		upstreamAccountApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"voided": true})
}
