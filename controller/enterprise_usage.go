package controller

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// 企业用量：列表、汇总、导出共用同一套筛选条件，企业 ID 只取鉴权中间件写入的值。
// 返回内容限于金额、模型、时间、令牌名称；密钥与请求内容不在这些接口的数据源里。

func enterpriseUsageFilter(c *gin.Context, enterpriseID int) (model.EnterpriseUsageFilter, bool) {
	filter := model.EnterpriseUsageFilter{
		EnterpriseId: enterpriseID, ModelName: c.Query("model_name"), Kind: c.Query("kind"), Status: c.Query("status"),
	}
	var err error
	number := func(name string) int64 {
		raw := c.Query(name)
		if raw == "" || err != nil {
			return 0
		}
		var value int64
		if value, err = strconv.ParseInt(raw, 10, 64); err == nil && value < 0 {
			err = model.ErrEnterpriseInvalidInput
		}
		return value
	}
	filter.MemberId, filter.TokenId = int(number("member_id")), int(number("token_id"))
	filter.Start, filter.End = number("start_timestamp"), number("end_timestamp")
	if err != nil || len(filter.ModelName) > 191 {
		common.ApiError(c, model.ErrEnterpriseInvalidInput)
		return filter, false
	}
	return filter, true
}

func ListEnterpriseUsage(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	filter, ok := enterpriseUsageFilter(c, enterpriseID)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseUsage(filter, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

func GetEnterpriseUsageSummary(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	filter, ok := enterpriseUsageFilter(c, enterpriseID)
	if !ok {
		return
	}
	ent, err := model.GetEnterpriseByID(enterpriseID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	summary, err := model.GetEnterpriseUsageSummary(filter)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	summary.Timezone = ent.Timezone
	common.ApiSuccess(c, summary)
}

var enterpriseUsageKindText = map[string]string{
	model.EnterpriseChargeRequest:      "请求",
	model.EnterpriseChargeViolationFee: "违规费",
	model.EnterpriseChargeTaskAdjust:   "任务调整",
	model.EnterpriseChargeManual:       "人工结算",
}

var enterpriseUsageStatusText = map[string]string{
	model.EnterpriseReservationReserved: "进行中",
	model.EnterpriseReservationSettled:  "已结算",
	model.EnterpriseReservationRefunded: "已退回",
	model.EnterpriseReservationReview:   "待对账",
}

func ExportEnterpriseUsage(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	filter, ok := enterpriseUsageFilter(c, enterpriseID)
	if !ok {
		return
	}
	if _, _, ok = csvExportRange(c); !ok {
		return
	}
	ent, err := model.GetEnterpriseByID(enterpriseID)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	// 时间按企业配置的时区展示，表头写明时区，每个值带偏移量，离开页面后仍不会读错。
	zone, err := time.LoadLocation(ent.Timezone)
	if err != nil {
		zone = time.UTC
	}
	headers := []string{"时间（" + zone.String() + "）", "成员", "令牌", "模型", "类型", "状态", "额度（quota）", "请求ID"}
	writeCSVExport(c, c.GetInt("id"), "enterprise-usage.csv", headers, func(ctx context.Context, emit func([]string) error) error {
		return model.WalkEnterpriseUsage(ctx, filter, func(row *model.EnterpriseReservation) error {
			return emit([]string{
				time.Unix(row.CreatedAt, 0).In(zone).Format(time.RFC3339), logCSVText(row.MemberName), logCSVText(row.TokenName),
				logCSVText(row.ModelName), enterpriseUsageKindText[row.Kind], enterpriseUsageStatusText[row.Status],
				strconv.FormatInt(row.Quota, 10), logCSVText(row.RequestId),
			})
		})
	})
}

func ListEnterpriseTokens(c *gin.Context) {
	enterpriseID, _, ok := enterpriseScope(c)
	if !ok {
		return
	}
	memberID, _ := strconv.Atoi(c.Query("member_id"))
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseTokens(enterpriseID, memberID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	enterprisePage(c, pageInfo, items, total, err)
}

func RevokeEnterpriseToken(c *gin.Context) {
	enterpriseID, actor, ok := enterpriseScope(c)
	if !ok {
		return
	}
	tokenID, ok := enterprisePathInt(c, "token_id")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if c.Request.ContentLength != 0 && !enterpriseBody(c, &req) {
		return
	}
	if err := model.RevokeEnterpriseToken(actor, enterpriseID, tokenID, req.Reason); err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
