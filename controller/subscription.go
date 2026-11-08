package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ---- 共享类型 ----

type SubscriptionPlanDTO struct {
	Plan model.SubscriptionPlan `json:"plan"`
}

type BillingPreferenceRequest struct {
	BillingPreference string `json:"billing_preference"`
}

func buildActiveQuantityByPlan(activeSubscriptions []model.SubscriptionSummary) map[int]int {
	activeQuantityByPlan := map[int]int{}
	if len(activeSubscriptions) == 0 {
		return activeQuantityByPlan
	}
	nowUnix := common.GetTimestamp()
	planCache := make(map[int]*model.SubscriptionPlan)
	for _, summary := range activeSubscriptions {
		sub := summary.Subscription
		if sub == nil || sub.PlanId <= 0 {
			continue
		}
		plan, ok := planCache[sub.PlanId]
		if !ok {
			loadedPlan, planErr := model.GetSubscriptionPlanById(sub.PlanId)
			if planErr != nil || loadedPlan == nil {
				continue
			}
			planCache[sub.PlanId] = loadedPlan
			plan = loadedPlan
		}
		quantity := model.CountRemainingSubscriptionQuantity(sub, plan, nowUnix)
		if quantity <= 0 {
			continue
		}
		activeQuantityByPlan[sub.PlanId] += quantity
	}
	return activeQuantityByPlan
}

// ---- 用户接口 ----

func GetSubscriptionPlans(c *gin.Context) {
	var plans []model.SubscriptionPlan
	if err := model.DB.Where("enabled = ?", true).Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]SubscriptionPlanDTO, 0, len(plans))
	for _, p := range plans {
		result = append(result, SubscriptionPlanDTO{
			Plan: p,
		})
	}
	common.ApiSuccess(c, result)
}

func GetSubscriptionSelf(c *gin.Context) {
	userId := c.GetInt("id")
	settingMap, _ := model.GetUserSetting(userId, false)
	pref := common.NormalizeBillingPreference(settingMap.BillingPreference)

	// 获取全部订阅（含已过期）
	allSubscriptions, err := model.GetAllUserSubscriptions(userId)
	if err != nil {
		allSubscriptions = []model.SubscriptionSummary{}
	}

	// 为兼容旧前端，单独返回生效订阅列表
	activeSubscriptions, err := model.GetAllActiveUserSubscriptions(userId)
	if err != nil {
		activeSubscriptions = []model.SubscriptionSummary{}
	}
	pendingIssuances, err := model.ListSubscriptionIssuancesByUser(userId, model.SubscriptionIssuanceStatusPending)
	if err != nil {
		pendingIssuances = []*model.SubscriptionIssuance{}
	}
	// active_quantity_by_plan：后端统一计算“每个套餐当前未过期份数”，
	// 供前端直接展示动态可买上限，避免前后端算法漂移。
	activeQuantityByPlan := buildActiveQuantityByPlan(activeSubscriptions)

	common.ApiSuccess(c, gin.H{
		"billing_preference":      pref,
		"subscriptions":           activeSubscriptions, // 全部生效订阅
		"all_subscriptions":       allSubscriptions,    // 全部订阅（含已过期）
		"pending_issuances":       pendingIssuances,
		"active_quantity_by_plan": activeQuantityByPlan,
	})
}

func ListSubscriptionIssuances(c *gin.Context) {
	userId := c.GetInt("id")
	status := strings.TrimSpace(c.Query("status"))
	issuances, err := model.ListSubscriptionIssuancesByUser(userId, status)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, issuance := range issuances {
		if issuance == nil {
			continue
		}
		if err := model.ResolveSubscriptionIssuanceDetails(issuance); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, issuances)
}

func GetSubscriptionIssuance(c *gin.Context) {
	userId := c.GetInt("id")
	issuanceId, _ := strconv.Atoi(c.Param("id"))
	issuance, err := model.GetSubscriptionIssuanceByIdForUser(issuanceId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.ResolveSubscriptionIssuanceDetails(issuance); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"issuance": issuance})
}

type confirmSubscriptionIssuanceRequest struct {
	PurchaseMode              string `json:"purchase_mode"`
	RenewTargetSubscriptionId int    `json:"renew_target_subscription_id"`
}

func ConfirmSubscriptionIssuance(c *gin.Context) {
	userId := c.GetInt("id")
	issuanceId, _ := strconv.Atoi(c.Param("id"))
	var req confirmSubscriptionIssuanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	var (
		issuance *model.SubscriptionIssuance
		summary  string
	)
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var confirmErr error
		issuance, summary, confirmErr = model.ConfirmSubscriptionIssuanceTx(
			tx,
			issuanceId,
			userId,
			req.PurchaseMode,
			req.RenewTargetSubscriptionId,
		)
		return confirmErr
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"issuance": issuance,
		"message":  summary,
	})
}

func UpdateSubscriptionPreference(c *gin.Context) {
	userId := c.GetInt("id")
	var req BillingPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	pref := common.NormalizeBillingPreference(req.BillingPreference)

	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	current := user.GetSetting()
	current.BillingPreference = pref
	user.SetSetting(current)
	if err := user.Update(false); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"billing_preference": pref})
}

// ---- 管理端接口 ----

func AdminListSubscriptionPlans(c *gin.Context) {
	var plans []model.SubscriptionPlan
	if err := model.DB.Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]SubscriptionPlanDTO, 0, len(plans))
	for _, p := range plans {
		result = append(result, SubscriptionPlanDTO{
			Plan: p,
		})
	}
	common.ApiSuccess(c, result)
}

type AdminUpsertSubscriptionPlanRequest struct {
	Plan model.SubscriptionPlan `json:"plan"`
}

// normalizeSubscriptionPlanPurchaseRule 归一化套餐购买数量规则（最小值/最大值）。
func normalizeSubscriptionPlanPurchaseRule(plan *model.SubscriptionPlan) error {
	if plan == nil {
		return nil
	}
	if plan.PurchaseQuantityMin <= 0 {
		plan.PurchaseQuantityMin = 1
	}
	if plan.PurchaseQuantityMax <= 0 {
		plan.PurchaseQuantityMax = 12
	}
	if plan.PurchaseQuantityMax < plan.PurchaseQuantityMin {
		return errors.New("购买数量最大值不能小于最小值")
	}
	return nil
}

func AdminCreateSubscriptionPlan(c *gin.Context) {
	var req AdminUpsertSubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	req.Plan.Id = 0
	if strings.TrimSpace(req.Plan.Title) == "" {
		common.ApiErrorMsg(c, "套餐标题不能为空")
		return
	}
	if req.Plan.PriceAmount < 0 {
		common.ApiErrorMsg(c, "价格不能为负数")
		return
	}
	if req.Plan.PriceAmount > 9999 {
		common.ApiErrorMsg(c, "价格不能超过9999")
		return
	}
	if req.Plan.Currency == "" {
		req.Plan.Currency = "USD"
	}
	req.Plan.Currency = "USD"
	if req.Plan.DurationUnit == "" {
		req.Plan.DurationUnit = model.SubscriptionDurationMonth
	}
	if req.Plan.DurationValue <= 0 && req.Plan.DurationUnit != model.SubscriptionDurationCustom {
		req.Plan.DurationValue = 1
	}
	if req.Plan.MaxPurchasePerUser < 0 {
		common.ApiErrorMsg(c, "购买上限不能为负数")
		return
	}
	if req.Plan.MaxStackPerUser < 0 {
		common.ApiErrorMsg(c, "叠加上限不能为负数")
		return
	}
	if err := normalizeSubscriptionPlanPurchaseRule(&req.Plan); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if req.Plan.TotalAmount < 0 {
		common.ApiErrorMsg(c, "总额度不能为负数")
		return
	}
	req.Plan.UpgradeGroup = strings.TrimSpace(req.Plan.UpgradeGroup)
	if err := validatePlanUpgradeGroup("", req.Plan.UpgradeGroup); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	req.Plan.QuotaResetPeriod = model.NormalizeResetPeriod(req.Plan.QuotaResetPeriod)
	if req.Plan.QuotaResetPeriod == model.SubscriptionResetCustom && req.Plan.QuotaResetCustomSeconds <= 0 {
		common.ApiErrorMsg(c, "自定义重置周期需大于0秒")
		return
	}
	err := model.DB.Create(&req.Plan).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(req.Plan.Id)
	common.ApiSuccess(c, req.Plan)
}

func AdminUpdateSubscriptionPlan(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	var req AdminUpsertSubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if strings.TrimSpace(req.Plan.Title) == "" {
		common.ApiErrorMsg(c, "套餐标题不能为空")
		return
	}
	if req.Plan.PriceAmount < 0 {
		common.ApiErrorMsg(c, "价格不能为负数")
		return
	}
	if req.Plan.PriceAmount > 9999 {
		common.ApiErrorMsg(c, "价格不能超过9999")
		return
	}
	req.Plan.Id = id
	if req.Plan.Currency == "" {
		req.Plan.Currency = "USD"
	}
	req.Plan.Currency = "USD"
	if req.Plan.DurationUnit == "" {
		req.Plan.DurationUnit = model.SubscriptionDurationMonth
	}
	if req.Plan.DurationValue <= 0 && req.Plan.DurationUnit != model.SubscriptionDurationCustom {
		req.Plan.DurationValue = 1
	}
	if req.Plan.MaxPurchasePerUser < 0 {
		common.ApiErrorMsg(c, "购买上限不能为负数")
		return
	}
	if req.Plan.MaxStackPerUser < 0 {
		common.ApiErrorMsg(c, "叠加上限不能为负数")
		return
	}
	if err := normalizeSubscriptionPlanPurchaseRule(&req.Plan); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if req.Plan.TotalAmount < 0 {
		common.ApiErrorMsg(c, "总额度不能为负数")
		return
	}
	req.Plan.UpgradeGroup = strings.TrimSpace(req.Plan.UpgradeGroup)
	currentUpgradeGroup := ""
	if currentPlan, err := model.GetSubscriptionPlanById(id); err == nil && currentPlan != nil {
		currentUpgradeGroup = strings.TrimSpace(currentPlan.UpgradeGroup)
	}
	if err := validatePlanUpgradeGroup(currentUpgradeGroup, req.Plan.UpgradeGroup); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	req.Plan.QuotaResetPeriod = model.NormalizeResetPeriod(req.Plan.QuotaResetPeriod)
	if req.Plan.QuotaResetPeriod == model.SubscriptionResetCustom && req.Plan.QuotaResetCustomSeconds <= 0 {
		common.ApiErrorMsg(c, "自定义重置周期需大于0秒")
		return
	}

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 更新套餐（用 map 支持零值字段更新）
		updateMap := map[string]interface{}{
			"title":                      req.Plan.Title,
			"subtitle":                   req.Plan.Subtitle,
			"price_amount":               req.Plan.PriceAmount,
			"currency":                   req.Plan.Currency,
			"duration_unit":              req.Plan.DurationUnit,
			"duration_value":             req.Plan.DurationValue,
			"custom_seconds":             req.Plan.CustomSeconds,
			"enabled":                    req.Plan.Enabled,
			"sort_order":                 req.Plan.SortOrder,
			"stripe_price_id":            req.Plan.StripePriceId,
			"creem_product_id":           req.Plan.CreemProductId,
			"max_purchase_per_user":      req.Plan.MaxPurchasePerUser,
			"max_stack_per_user":         req.Plan.MaxStackPerUser,
			"purchase_quantity_min":      req.Plan.PurchaseQuantityMin,
			"purchase_quantity_max":      req.Plan.PurchaseQuantityMax,
			"total_amount":               req.Plan.TotalAmount,
			"upgrade_group":              req.Plan.UpgradeGroup,
			"quota_reset_period":         req.Plan.QuotaResetPeriod,
			"quota_reset_custom_seconds": req.Plan.QuotaResetCustomSeconds,
			"updated_at":                 common.GetTimestamp(),
		}
		if err := tx.Model(&model.SubscriptionPlan{}).Where("id = ?", id).Updates(updateMap).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(id)
	common.ApiSuccess(c, nil)
}

type AdminUpdateSubscriptionPlanStatusRequest struct {
	Enabled *bool `json:"enabled"`
}

func AdminUpdateSubscriptionPlanStatus(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	var req AdminUpdateSubscriptionPlanStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", id).Update("enabled", *req.Enabled).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateSubscriptionPlanCache(id)
	common.ApiSuccess(c, nil)
}

type AdminBindSubscriptionRequest struct {
	UserId int `json:"user_id"`
	AdminCreateUserSubscriptionRequest
}

type AdminCreateUserSubscriptionRequest struct {
	PlanId int `json:"plan_id"`
	// PurchaseMode 支持 stack / renew / renew_extend；默认 stack。
	PurchaseMode string `json:"purchase_mode"`
	// PurchaseQuantity 单次新增份数；默认按套餐最小购买份数兜底。
	PurchaseQuantity int `json:"purchase_quantity"`
	// RenewTargetSubscriptionId 仅在 purchase_mode=renew 且同套餐有多条可续费订阅时生效。
	RenewTargetSubscriptionId int `json:"renew_target_subscription_id"`
}

type preparedAdminSubscriptionPurchase struct {
	PlanId                    int
	PurchaseMode              string
	PurchaseQuantity          int
	RenewTargetSubscriptionId int
}

// prepareAdminSubscriptionPurchase 统一执行管理端新增订阅的参数归一化与校验逻辑。
// 该函数被两个入口复用：
// 1) /api/subscription/admin/bind
// 2) /api/subscription/admin/users/:id/subscriptions
func prepareAdminSubscriptionPurchase(userId int, req AdminCreateUserSubscriptionRequest) (*preparedAdminSubscriptionPurchase, error) {
	if userId <= 0 || req.PlanId <= 0 {
		return nil, errors.New("参数错误")
	}
	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		return nil, err
	}

	purchaseQuantity, err := normalizeSubscriptionPurchaseQuantity(userId, req.PurchaseQuantity, plan)
	if err != nil {
		return nil, err
	}

	purchaseMode, renewTargetSubId, err := resolveSubscriptionPurchaseModeAndTarget(
		userId, plan.Id, req.PurchaseMode, req.RenewTargetSubscriptionId,
	)
	if err != nil {
		return nil, err
	}
	if err := checkSubscriptionOrderLimits(userId, plan, purchaseMode, purchaseQuantity); err != nil {
		return nil, err
	}

	return &preparedAdminSubscriptionPurchase{
		PlanId:                    plan.Id,
		PurchaseMode:              purchaseMode,
		PurchaseQuantity:          purchaseQuantity,
		RenewTargetSubscriptionId: renewTargetSubId,
	}, nil
}

func AdminBindSubscription(c *gin.Context) {
	var req AdminBindSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UserId <= 0 || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	prepared, err := prepareAdminSubscriptionPurchase(req.UserId, req.AdminCreateUserSubscriptionRequest)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	msg, err := model.AdminBindSubscriptionWithOptions(
		req.UserId,
		prepared.PlanId,
		prepared.PurchaseMode,
		prepared.PurchaseQuantity,
		prepared.RenewTargetSubscriptionId,
		"",
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if msg != "" {
		common.ApiSuccess(c, gin.H{"message": msg})
		return
	}
	common.ApiSuccess(c, nil)
}

// ---- 管理端：用户订阅管理 ----

func AdminListUserSubscriptions(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	if userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}
	subs, err := model.GetAllUserSubscriptions(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	activeSubs, err := model.GetAllActiveUserSubscriptions(userId)
	if err != nil {
		activeSubs = []model.SubscriptionSummary{}
	}
	issuances, err := model.ListSubscriptionIssuancesByUser(userId, "")
	if err != nil {
		issuances = []*model.SubscriptionIssuance{}
	}
	for _, issuance := range issuances {
		if issuance == nil {
			continue
		}
		if err := model.ResolveSubscriptionIssuanceDetails(issuance); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, gin.H{
		"subscriptions":           subs,
		"issuances":               issuances,
		"active_quantity_by_plan": buildActiveQuantityByPlan(activeSubs),
	})
}

// AdminCreateUserSubscription 管理端按套餐创建用户订阅（无需支付）。
func AdminCreateUserSubscription(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	if userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}
	var req AdminCreateUserSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	prepared, err := prepareAdminSubscriptionPurchase(userId, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	msg, err := model.AdminBindSubscriptionWithOptions(
		userId,
		prepared.PlanId,
		prepared.PurchaseMode,
		prepared.PurchaseQuantity,
		prepared.RenewTargetSubscriptionId,
		"",
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if msg != "" {
		common.ApiSuccess(c, gin.H{"message": msg})
		return
	}
	common.ApiSuccess(c, nil)
}

type AdminManageUserSubscriptionRequest struct {
	Id     int    `json:"id"`
	Action string `json:"action"`
}

type AdminManageUserSubscriptionBatchRequest struct {
	Ids    []int  `json:"ids"`
	Action string `json:"action"`
}

// AdminManageUserSubscription 管理员单条管理用户订阅（启用/禁用/删除）。
func AdminManageUserSubscription(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	if userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}
	var req AdminManageUserSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	msg, err := model.AdminManageUserSubscription(userId, req.Id, action)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"id":      req.Id,
		"action":  action,
		"message": msg,
	})
}

// AdminManageUserSubscriptionBatch 管理员批量管理用户订阅（启用/禁用/删除），支持部分成功。
func AdminManageUserSubscriptionBatch(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Param("id"))
	if userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}
	req := AdminManageUserSubscriptionBatchRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	seen := make(map[int]struct{}, len(req.Ids))
	updated := make([]gin.H, 0, len(req.Ids))
	failed := make([]gin.H, 0)

	for _, id := range req.Ids {
		if id <= 0 {
			failed = append(failed, gin.H{
				"id":      id,
				"message": "订阅 ID 无效",
			})
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		msg, err := model.AdminManageUserSubscription(userId, id, action)
		if err != nil {
			failed = append(failed, gin.H{
				"id":      id,
				"message": err.Error(),
			})
			continue
		}
		updated = append(updated, gin.H{
			"id":      id,
			"action":  action,
			"message": msg,
		})
	}

	common.ApiSuccess(c, gin.H{
		"success_count": len(updated),
		"failed_count":  len(failed),
		"updated":       updated,
		"failed":        failed,
	})
}

// AdminInvalidateUserSubscription 立即取消一条用户订阅。
func AdminInvalidateUserSubscription(c *gin.Context) {
	subId, _ := strconv.Atoi(c.Param("id"))
	if subId <= 0 {
		common.ApiErrorMsg(c, "无效的订阅ID")
		return
	}
	msg, err := model.AdminInvalidateUserSubscription(subId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if msg != "" {
		common.ApiSuccess(c, gin.H{"message": msg})
		return
	}
	common.ApiSuccess(c, nil)
}

// AdminDeleteUserSubscription 硬删除一条用户订阅。
func AdminDeleteUserSubscription(c *gin.Context) {
	subId, _ := strconv.Atoi(c.Param("id"))
	if subId <= 0 {
		common.ApiErrorMsg(c, "无效的订阅ID")
		return
	}
	msg, err := model.AdminDeleteUserSubscription(subId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if msg != "" {
		common.ApiSuccess(c, gin.H{"message": msg})
		return
	}
	common.ApiSuccess(c, nil)
}
