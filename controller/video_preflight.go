package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

const videoPreflightKind = "video_preflight"

type videoCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

type videoPreflight struct {
	Kind               string               `json:"kind"`
	Model              string               `json:"model"`
	Checks             []videoCheck         `json:"checks"`
	Passed             bool                 `json:"passed"`
	GenerationVerified bool                 `json:"generation_verified"`
	UpstreamRequests   int                  `json:"upstream_requests"`
	History            []videoHistoryItem   `json:"history,omitempty"`
	HistoryUnavailable bool                 `json:"history_unavailable,omitempty"`
	Quote              *videoPriceReference `json:"quote,omitempty"`
	QuoteUnavailable   string               `json:"quote_unavailable,omitempty"`
}

type videoHistoryItem struct {
	Status     model.TaskStatus `json:"status"`
	SubmitTime int64            `json:"submit_time"`
	FinishTime int64            `json:"finish_time"`
}

type videoPriceReference struct {
	Quota        int     `json:"quota"`
	AmountUSD    float64 `json:"amount_usd"`
	Group        string  `json:"group"`
	GroupRatio   float64 `json:"group_ratio"`
	TimeRatio    float64 `json:"time_ratio"`
	CalculatedAt int64   `json:"calculated_at"`
}

func channelTestModel(ch *model.Channel, requested string) string {
	if name := strings.TrimSpace(requested); name != "" {
		return name
	}
	if ch.TestModel != nil && strings.TrimSpace(*ch.TestModel) != "" {
		return strings.TrimSpace(*ch.TestModel)
	}
	for _, name := range ch.GetModels() {
		if strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return "gpt-4o-mini"
}

func channelVideoTest(ch *model.Channel, name, endpoint string) bool {
	if endpoint == string(constant.EndpointTypeOpenAIVideo) {
		return true
	}
	if ch == nil {
		return common.IsVideoGenerationTest(0, name)
	}
	if common.IsVideoGenerationTest(ch.Type, name) {
		return true
	}
	for _, ep := range model.GetModelSupportEndpointTypes(name) {
		if ep == constant.EndpointTypeOpenAIVideo {
			return true
		}
	}
	if ch.ParamOverride != nil {
		var overrides map[string]any
		if common.UnmarshalJsonStr(*ch.ParamOverride, &overrides) == nil {
			if mapped, ok := overrides["model"].(string); ok && common.IsVideoGenerationTest(ch.Type, mapped) {
				return true
			}
		}
	}
	var mapping map[string]string
	if common.UnmarshalJsonStr(ch.GetModelMapping(), &mapping) == nil {
		// Follow mapping chains just as the relay does; stop cycles locally.
		seen := map[string]bool{}
		for !seen[name] {
			seen[name] = true
			mapped := strings.TrimSpace(mapping[name])
			if mapped == "" {
				break
			}
			if common.IsVideoGenerationTest(ch.Type, mapped) {
				return true
			}
			name = mapped
		}
	}
	return false
}

// This path must stay local: no adapter requests, key rotation, billing or
// channel health mutation. A configured key/adapter is not proof of access.
func inspectVideoConfiguration(ch *model.Channel, name string) *videoPreflight {
	r := &videoPreflight{Kind: videoPreflightKind, Model: name, Passed: true}
	check := func(key string, ok bool) { r.Checks = append(r.Checks, videoCheck{key, ok}); r.Passed = r.Passed && ok }
	declared := false
	for _, m := range ch.GetModels() {
		declared = declared || strings.TrimSpace(m) == name
	}
	check("model_declared", declared)
	var mapping map[string]string
	raw := strings.TrimSpace(ch.GetModelMapping())
	mappingOK := raw == "" || common.UnmarshalJsonStr(raw, &mapping) == nil
	if mappingOK {
		seen := map[string]bool{}
		for current := name; ; {
			if seen[current] {
				mappingOK = false
				break
			}
			seen[current] = true
			next, exists := mapping[current]
			if !exists || next == current {
				break
			}
			if strings.TrimSpace(next) == "" {
				mappingOK = false
				break
			}
			current = next
		}
	}
	check("model_mapping", mappingOK)
	check("credential_configured", strings.TrimSpace(ch.Key) != "")
	u, err := url.Parse(ch.GetBaseURL())
	check("base_url", err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https"))
	check("task_adapter", relay.GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(ch.Type))) != nil)
	_, expr := billing_setting.GetTieredExpr(name)
	_, price := ratio_setting.GetModelPrice(name, false)
	check("explicit_task_price", expr || price)
	check("channel_group", strings.TrimSpace(ch.Group) != "")
	return r
}

func videoPreflightResponse(r *videoPreflight) gin.H {
	return gin.H{
		// Legacy clients only inspect success and may enable a channel on true.
		// A local preflight must never satisfy that live-test success contract.
		"success": false, "test_kind": videoPreflightKind, "time": 0,
		"message": "视频仅进行本地配置预检，未调用上游生成；预检通过不代表生成可用。",
		"data":    r,
	}
}

// Read bounded summaries only, never another user's prompts, assets, provider
// IDs, raw errors or result URLs. Old tasks have no configuration fingerprint:
// all entries are historical references, never current verification.
func loadVideoHistory(chID int, name string, userID int) ([]videoHistoryItem, error) {
	var tasks []model.Task
	q := model.DB.Select("status", "submit_time", "finish_time", "properties").Where("channel_id = ?", chID)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Order("id DESC").Limit(200).Find(&tasks).Error; err != nil {
		return nil, err
	}
	result := make([]videoHistoryItem, 0)
	for _, task := range tasks {
		if task.Properties.OriginModelName != name {
			continue
		}
		result = append(result, videoHistoryItem{task.Status, task.SubmitTime, task.FinishTime})
		if len(result) == 10 {
			break
		}
	}
	return result, nil
}

// Price reference uses the exact existing task expression calculator. It is
// not an executable quote or provider capability validation. Legacy adapter
// multipliers/default fallback prices are intentionally not guessed here.
func calculateVideoPriceReference(c *gin.Context, ch *model.Channel, name, group, userGroup string, params map[string]any) (*videoPriceReference, error) {
	if _, ok := billing_setting.GetTieredExpr(name); !ok {
		return nil, fmt.Errorf("当前模型没有可本地计算的任务定价表达式")
	}
	if !common.StringsContains(strings.Split(ch.Group, ","), group) || !ratio_setting.ContainsGroupRatio(group) {
		return nil, fmt.Errorf("请选择渠道已配置的有效分组")
	}
	params["model"] = name
	body, err := common.Marshal(params)
	if err != nil {
		return nil, err
	}
	ctx, _ := createInternalJSONContext(c, http.MethodPost, "/v1/videos", body, "application/json")
	defer common.CleanupBodyStorage(ctx)
	info := &relaycommon.RelayInfo{OriginModelName: name, UsingGroup: group, UserGroup: userGroup, StartTime: time.Now()}
	price, err := helper.ModelPriceHelperTask(ctx, info)
	if err != nil {
		return nil, fmt.Errorf("当前参数不能完成本地估价，请检查定价表达式和请求参数")
	}
	return &videoPriceReference{Quota: price.Quota, AmountUSD: float64(price.Quota) / common.QuotaPerUnit, Group: group, GroupRatio: price.GroupRatioInfo.GroupRatio, TimeRatio: price.TimeRatioInfo.EffectiveRatio(), CalculatedAt: time.Now().Unix()}, nil
}

func VideoChannelPreflight(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, fmt.Errorf("渠道 ID 无效"))
		return
	}
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var request struct {
		Model      string         `json:"model"`
		Group      string         `json:"group"`
		Parameters map[string]any `json:"parameters"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, fmt.Errorf("检查参数无效或超过 16KB"))
		return
	}
	name := strings.TrimSpace(request.Model)
	if name == "" {
		common.ApiError(c, fmt.Errorf("请选择模型"))
		return
	}
	r := inspectVideoConfiguration(ch, name)
	r.History, err = loadVideoHistory(ch.Id, name, 0)
	r.HistoryUnavailable = err != nil
	if request.Parameters != nil {
		user, userErr := model.GetUserCache(c.GetInt("id"))
		if userErr != nil {
			common.ApiError(c, userErr)
			return
		}
		r.Quote, err = calculateVideoPriceReference(c, ch, name, request.Group, user.Group, request.Parameters)
		if err != nil {
			r.QuoteUnavailable = err.Error()
		}
	}
	c.JSON(http.StatusOK, videoPreflightResponse(r))
}
