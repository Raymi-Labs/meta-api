package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type routePlanAdminRow struct {
	model.RoutePlan
	TokenCount int64 `json:"token_count"`
}

func GetRoutePlans(c *gin.Context) {
	plans, err := model.ListRoutePlans()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	counts, err := model.CountRoutePlanTokens()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows := make([]routePlanAdminRow, 0, len(plans))
	for _, plan := range plans {
		rows = append(rows, routePlanAdminRow{RoutePlan: plan, TokenCount: counts[plan.Id]})
	}
	common.ApiSuccess(c, rows)
}

func bindRoutePlan(c *gin.Context) (*model.RoutePlan, bool) {
	plan := &model.RoutePlan{}
	if err := c.ShouldBindJSON(plan); err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if err := plan.Normalize(ratio_setting.ContainsGroupRatio); err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return plan, true
}

func CreateRoutePlan(c *gin.Context) {
	plan, ok := bindRoutePlan(c)
	if !ok {
		return
	}
	if err := model.CreateRoutePlan(plan); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plan)
}

func UpdateRoutePlan(c *gin.Context) {
	plan, ok := bindRoutePlan(c)
	if !ok {
		return
	}
	if plan.Id <= 0 {
		common.ApiErrorMsg(c, "缺少方案 ID")
		return
	}
	if err := model.UpdateRoutePlan(plan); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plan)
}

func DeleteRoutePlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的方案 ID")
		return
	}
	if err := model.DeleteRoutePlan(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// routePlanUserView is what a customer may see: the contract price, never the
// line pool, purchase cost or other customers' grants.
type routePlanUserView struct {
	Id          int                `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	SaleRatio   float64            `json:"sale_ratio"`
	ModelRatios map[string]float64 `json:"model_ratios"`
	Version     int64              `json:"version"`
}

func GetSelfRoutePlans(c *gin.Context) {
	userID := c.GetInt("id")
	userGroup, err := model.GetUserGroup(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	plans, err := model.ListRoutePlans()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views := make([]routePlanUserView, 0)
	for _, plan := range plans {
		if !plan.AuthorizedFor(userID, userGroup) {
			continue
		}
		fields, fieldsErr := plan.Fields()
		if fieldsErr != nil {
			continue
		}
		views = append(views, routePlanUserView{Id: plan.Id, Name: plan.Name, Description: plan.Description,
			SaleRatio: plan.SaleRatio, ModelRatios: fields.ModelRatios, Version: plan.Version})
	}
	common.ApiSuccess(c, views)
}
