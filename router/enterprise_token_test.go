package router

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const enterpriseUnsupported = "暂不支持企业额度"

// relay 在企业接口之外再挂上全部调用入口，并准备一个可用渠道与三个模型。
// 上游恒返回 500，返回它被调用的次数：用来区分"请求被网关拦下"和"请求已受理并发往上游"。
func (a *enterpriseAPI) relay() *atomic.Int32 {
	a.t.Helper()
	return a.relayTo(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
}

// relayTo 与 relay 相同，但由调用方决定上游怎么回答。
func (a *enterpriseAPI) relayTo(answer http.HandlerFunc) *atomic.Int32 {
	a.t.Helper()
	require.NoError(a.t, i18n.Init())
	service.InitHttpClient()
	oldSelfUse, oldMemory := operation_setting.SelfUseModeEnabled, common.MemoryCacheEnabled
	operation_setting.SelfUseModeEnabled, common.MemoryCacheEnabled = true, false
	model.InvalidateModelPermissionCache()
	a.t.Cleanup(func() {
		operation_setting.SelfUseModeEnabled, common.MemoryCacheEnabled = oldSelfUse, oldMemory
		model.InvalidateModelPermissionCache()
	})
	// 请求到达上游后会记录线路健康度；不让它的后台循环启动，否则会在测试结束后继续读已关闭的数据库。
	a.t.Cleanup(routeinsight.SetHealthForTesting())
	require.NoError(a.t, model.DB.AutoMigrate(&model.Ability{}, &model.Channel{}, &model.Model{}, &model.ModelPermission{},
		&model.Vendor{}, &model.Log{}, &model.Task{}, &model.Midjourney{}, &model.UserSubscription{}, &model.ContentSafetyViolation{},
		&model.ContentSafetyReviewCase{}, &model.ContentSafetyEvidence{}, &model.ContentSafetyNotification{}))

	calls := &atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		answer(w, r)
	}))
	a.t.Cleanup(upstream.Close)
	channel := model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "local-test-channel",
		Key: "local-test-upstream-key", BaseURL: &upstream.URL, Group: "default", Models: "model-a,model-b,model-c"}
	require.NoError(a.t, model.DB.Create(&channel).Error)
	for _, name := range []string{"model-a", "model-b", "model-c"} {
		require.NoError(a.t, model.DB.Create(&model.Ability{Group: "default", Model: name, ChannelId: channel.Id, Enabled: true}).Error)
	}
	SetDashboardRouter(a.engine)
	SetRelayRouter(a.engine)
	SetVideoRouter(a.engine)
	return calls
}

// token 通过控制台接口创建令牌并返回落库后的记录。
func (a *enterpriseAPI) token(owner *model.User, name, payer string, extra map[string]any) *model.Token {
	a.t.Helper()
	body := map[string]any{"name": name, "group": "default", "expired_time": -1, "unlimited_quota": true, "payer": payer}
	for key, value := range extra {
		body[key] = value
	}
	a.ok(owner.Id, common.RoleCommonUser, http.MethodPost, "/api/token/", body)
	var token model.Token
	require.NoError(a.t, model.DB.Where("user_id = ? AND name = ?", owner.Id, name).First(&token).Error)
	return &token
}

// relayCall 以令牌身份发起一次调用。
func (a *enterpriseAPI) relayCall(token *model.Token, method, path, body string) *httptest.ResponseRecorder {
	a.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-"+token.Key)
	w := httptest.NewRecorder()
	a.engine.ServeHTTP(w, req)
	return w
}

func (a *enterpriseAPI) modelIDs(token *model.Token) []string {
	a.t.Helper()
	w := a.relayCall(token, http.MethodGet, "/v1/models", "")
	require.Equal(a.t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		Data []struct {
			Id string `json:"id"`
		} `json:"data"`
	}
	require.NoError(a.t, common.Unmarshal(w.Body.Bytes(), &out))
	ids := make([]string, 0, len(out.Data))
	for _, item := range out.Data {
		ids = append(ids, item.Id)
	}
	sort.Strings(ids)
	return ids
}

func (a *enterpriseAPI) personalQuota(user *model.User) int {
	a.t.Helper()
	var fresh model.User
	require.NoError(a.t, model.DB.First(&fresh, user.Id).Error)
	return fresh.Quota
}

// 第一批对企业令牌开放的调用入口。新增或删除必须同时改这里，避免入口被无意放开。
var enterpriseOpenRoutes = []string{
	"GET /v1/models", "GET /v1/models/:model", "GET /v1beta/models", "GET /v1beta/openai/models",
	"POST /v1/messages", "POST /v1/completions", "POST /v1/chat/completions", "POST /v1/responses", "POST /v1/responses/compact",
	"POST /v1/edits", "POST /v1/images/generations", "POST /v1/images/edits",
	"POST /v1/embeddings", "POST /v1/audio/transcriptions", "POST /v1/audio/translations", "POST /v1/audio/speech", "POST /v1/rerank",
	"POST /v1/engines/:model/embeddings", "POST /v1/models/*path", "POST /v1/moderations", "POST /v1beta/models/*path",
}

// 不按令牌鉴权的调用入口：网页体验区走登录态，媒体桥接凭一次性地址访问。
var enterpriseNonTokenRoutes = map[string]bool{"POST /pg/chat/completions": true, "GET /v1/bridge/media/:id": true}

func TestEnterpriseTokenRouteEnumeration(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	calls := api.relay()
	root, owner, staff := api.user("root"), api.user("owner"), api.user("staff")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", staff.Id).Update("quota", 100000).Error)
	ent := api.org(root, "甲公司", owner)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit",
		map[string]any{"amount": 100000, "voucher": "BANK-ROUTES"})
	api.join(ent, owner, staff, model.EnterpriseRoleMember, 50000)
	enterpriseToken := api.token(staff, "企业令牌", model.TokenPayerEnterprise, nil)
	personalToken := api.token(staff, "个人令牌", model.TokenPayerPersonal, nil)
	require.Positive(t, enterpriseToken.EnterpriseId)
	require.Zero(t, personalToken.EnterpriseId)

	fill := strings.NewReplacer(":model", "model-a", "*path", "model-a:generateContent", ":mode", "fast", ":task_id", "task_1",
		":video_id", "video_1", ":id", "1")
	seen := map[string]bool{}
	checked := 0
	for _, route := range api.engine.Routes() {
		key := route.Method + " " + route.Path
		if strings.HasPrefix(route.Path, "/api/") || enterpriseNonTokenRoutes[key] {
			continue
		}
		seen[key] = true
		checked++
		path := fill.Replace(route.Path)
		if strings.HasPrefix(route.Path, "/jimeng") {
			path += "?Action=CVSync2AsyncSubmitTask"
		}

		w := api.relayCall(enterpriseToken, route.Method, path, "{}")
		require.NotEqual(t, http.StatusUnauthorized, w.Code, "%s: %s", key, w.Body.String())
		require.NotContains(t, w.Body.String(), enterpriseUnsupported, key)
		// 同一路由的个人令牌也不能受企业付款约束影响。
		w = api.relayCall(personalToken, route.Method, path, "{}")
		require.NotEqual(t, http.StatusUnauthorized, w.Code, "%s: %s", key, w.Body.String())
		require.NotContains(t, w.Body.String(), enterpriseUnsupported, key)
	}
	require.GreaterOrEqual(t, checked, 80, "relay routes must be registered")
	for _, route := range enterpriseOpenRoutes {
		require.True(t, seen[route], "既有同步入口 %s 必须保持注册", route)
	}
	require.Zero(t, calls.Load())
	require.Equal(t, 100000, api.personalQuota(staff))
}

func TestEnterpriseTokenTakesEffectImmediately(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	calls := api.relay()
	root, owner, staff, other := api.user("root"), api.user("owner"), api.user("staff"), api.user("other")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", staff.Id).Update("quota", 100000).Error)
	ent := api.org(root, "甲公司", owner)
	base := "/api/enterprises/" + strconv.Itoa(ent)
	admin := "/api/enterprise/admin/enterprises/" + strconv.Itoa(ent)
	const user = common.RoleCommonUser
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, admin+"/credit", map[string]any{"amount": 100000, "voucher": "BANK-LIVE"})
	staffMember := api.join(ent, owner, staff, model.EnterpriseRoleMember, 50000)
	api.join(ent, owner, other, model.EnterpriseRoleMember, 1000)
	staffToken := api.token(staff, "企业令牌", model.TokenPayerEnterprise, nil)
	otherToken := api.token(other, "企业令牌", model.TokenPayerEnterprise, nil)
	personalToken := api.token(staff, "个人令牌", model.TokenPayerPersonal, nil)
	chat := func(token *model.Token, name string) *httptest.ResponseRecorder {
		return api.relayCall(token, http.MethodPost, "/v1/chat/completions",
			`{"model":"`+name+`","messages":[{"role":"user","content":"hi"}]}`)
	}

	// 默认继承平台开放的全部模型。
	require.Equal(t, []string{"model-a", "model-b", "model-c"}, api.modelIDs(staffToken))

	// 企业与成员两级"指定模型"取交集；令牌自己的模型限制再收窄一层。
	settings := map[string]any{"name": "甲公司", "timezone": "Asia/Shanghai", "model_mode": model.EnterpriseModelModeCustom,
		"models": []string{"model-a", "model-b"}}
	api.ok(owner.Id, user, http.MethodPut, base+"/settings", settings)
	require.Equal(t, []string{"model-a", "model-b"}, api.modelIDs(staffToken))
	memberModels := base + "/members/" + strconv.Itoa(staffMember) + "/models"
	api.ok(owner.Id, user, http.MethodPut, memberModels, map[string]any{"model_mode": model.EnterpriseModelModeCustom, "models": []string{"model-b", "model-c"}})
	require.Equal(t, []string{"model-b"}, api.modelIDs(staffToken))
	require.Equal(t, []string{"model-a", "model-b"}, api.modelIDs(otherToken), "成员级收窄只影响本人")
	require.Equal(t, []string{"model-a", "model-b", "model-c"}, api.modelIDs(personalToken), "个人令牌不受企业模型范围影响")
	narrowed := api.token(staff, "收窄令牌", model.TokenPayerEnterprise, map[string]any{"model_limits_enabled": true, "model_limits": "model-a,model-c"})
	require.Empty(t, api.modelIDs(narrowed))
	w := chat(staffToken, "model-a")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), enterpriseUnsupported)

	// 允许的模型由企业成员预算付款并发往上游；上游失败后预留全额释放，不动个人钱包。
	require.Zero(t, calls.Load(), "被模型范围拦下的请求不应到达上游")
	w = chat(staffToken, "model-b")
	require.NotEqual(t, http.StatusForbidden, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), enterpriseUnsupported)
	require.Positive(t, calls.Load(), "企业请求应到达上游")
	reached := calls.Load()
	var holds []model.EnterpriseReservation
	require.Eventually(t, func() bool {
		holds = nil
		require.NoError(t, model.DB.Find(&holds).Error)
		return len(holds) == 1 && holds[0].Status == model.EnterpriseReservationRefunded
	}, 2*time.Second, 5*time.Millisecond, "失败的企业请求应留下一条已释放的预留")
	require.Equal(t, staffMember, holds[0].MemberId)
	require.Equal(t, staffToken.Id, holds[0].TokenId)
	require.Zero(t, holds[0].Quota)

	// 显式空集合 = 全部禁止。
	api.ok(owner.Id, user, http.MethodPut, memberModels, map[string]any{"model_mode": model.EnterpriseModelModeCustom, "models": []string{}})
	require.Empty(t, api.modelIDs(staffToken))
	w = chat(staffToken, "model-b")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), enterpriseUnsupported)
	api.ok(owner.Id, user, http.MethodPut, memberModels, map[string]any{"model_mode": model.EnterpriseModelModeInherit})

	// 冻结企业：全部成员的企业令牌立即被拒，解冻后恢复；个人令牌不受影响。
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, admin+"/status", map[string]any{"status": model.EnterpriseStatusFrozen, "reason": "欠费"})
	for _, token := range []*model.Token{staffToken, otherToken} {
		w = api.relayCall(token, http.MethodGet, "/v1/models", "")
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), model.ErrEnterpriseFrozen.Error())
		w = api.relayCall(token, http.MethodGet, "/api/usage/token/", "")
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	}
	api.denied(staff.Id, user, http.MethodPost, "/api/token/", map[string]any{"name": "冻结期间", "expired_time": -1, "payer": model.TokenPayerEnterprise}, model.ErrEnterpriseFrozen)
	require.Len(t, api.modelIDs(personalToken), 3)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, admin+"/status", map[string]any{"status": model.EnterpriseStatusActive})
	require.Len(t, api.modelIDs(staffToken), 2)

	// 关闭功能开关：企业令牌立即被拒。
	common.EnterpriseEnabled = false
	w = api.relayCall(staffToken, http.MethodGet, "/v1/models", "")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), model.ErrEnterpriseDisabled.Error())
	require.Len(t, api.modelIDs(personalToken), 3)
	common.EnterpriseEnabled = true

	// 停用成员：本人的企业令牌立即失效，且不能重新启用；同事与本人的个人令牌不受影响。
	api.ok(owner.Id, user, http.MethodPost, base+"/members/"+strconv.Itoa(staffMember)+"/disable", map[string]any{"reason": "离职"})
	w = api.relayCall(staffToken, http.MethodGet, "/v1/models", "")
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	api.denied(staff.Id, user, http.MethodPut, "/api/token/?status_only=true",
		map[string]any{"id": staffToken.Id, "status": common.TokenStatusEnabled}, model.ErrEnterpriseMemberInactive)
	api.call(staff.Id, user, http.MethodPost, "/api/token/manage/batch", map[string]any{"ids": []int{staffToken.Id}, "action": "enable"})
	var stored model.Token
	require.NoError(t, model.DB.First(&stored, staffToken.Id).Error)
	require.Equal(t, common.TokenStatusDisabled, stored.Status)
	// 即便令牌状态被改回启用，成员关系校验仍然拒绝。
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", staffToken.Id).Update("status", common.TokenStatusEnabled).Error)
	w = api.relayCall(staffToken, http.MethodGet, "/v1/models", "")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), model.ErrEnterpriseMemberInactive.Error())
	api.denied(staff.Id, user, http.MethodPost, "/api/token/", map[string]any{"name": "停用之后", "expired_time": -1, "payer": model.TokenPayerEnterprise}, model.ErrEnterpriseNotFound)
	require.Len(t, api.modelIDs(otherToken), 2)
	require.Len(t, api.modelIDs(personalToken), 3)

	require.Equal(t, reached, calls.Load(), "被拦下的请求都不应到达上游")
	require.Equal(t, 100000, api.personalQuota(staff), "个人钱包零变动")
	balance, err := model.GetEnterpriseBalance(ent)
	require.NoError(t, err)
	require.Zero(t, balance.MemberReserved)
	require.Zero(t, balance.Consumed)
}

// 一次成功的企业请求：按真实用量从成员预算结算，日志标明付款方，个人钱包与个人已用额度不动。
func TestEnterpriseRequestSettlesAgainstMemberBudget(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	calls := api.relayTo(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"model-a",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":1000,"completion_tokens":2000,"total_tokens":3000}}`))
	})
	oldLog := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = oldLog })
	root, owner, staff := api.user("root"), api.user("owner"), api.user("staff")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", staff.Id).Update("quota", 100000).Error)
	ent := api.org(root, "甲公司", owner)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit",
		map[string]any{"amount": 50_000_000, "voucher": "BANK-SETTLE"})
	member := api.join(ent, owner, staff, model.EnterpriseRoleMember, 20_000_000)
	token := api.token(staff, "企业令牌", model.TokenPayerEnterprise, nil)

	w := api.relayCall(token, http.MethodPost, "/v1/chat/completions", `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.EqualValues(t, 1, calls.Load())

	var hold model.EnterpriseReservation
	require.NoError(t, model.DB.First(&hold).Error)
	require.Equal(t, model.EnterpriseReservationSettled, hold.Status)
	require.Positive(t, hold.Quota)
	require.Equal(t, member, hold.MemberId)
	require.Equal(t, token.Id, hold.TokenId)

	var row model.EnterpriseMember
	require.NoError(t, model.DB.First(&row, member).Error)
	require.Equal(t, [3]int64{20_000_000 - hold.Quota, 0, hold.Quota}, [3]int64{row.Available, row.Reserved, row.Consumed})
	balance, err := model.GetEnterpriseBalance(ent)
	require.NoError(t, err)
	require.True(t, balance.Balanced, "%+v", balance)
	require.Equal(t, hold.Quota, balance.Consumed)

	var entry model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).First(&entry).Error)
	require.EqualValues(t, hold.Quota, entry.Quota)
	require.Equal(t, staff.Id, entry.UserId)
	require.Equal(t, token.Id, entry.TokenId)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(entry.Other, &other))
	require.Equal(t, service.BillingSourceEnterprise, other["billing_source"])
	require.EqualValues(t, ent, other["enterprise_id"])
	require.EqualValues(t, member, other["enterprise_member_id"])

	var user model.User
	require.NoError(t, model.DB.First(&user, staff.Id).Error)
	require.Equal(t, 100000, user.Quota, "个人钱包零变动")
	require.Zero(t, user.UsedQuota, "企业消费不计入个人已用额度")
	require.Equal(t, 1, user.RequestCount)
	var stored model.Token
	require.NoError(t, model.DB.First(&stored, token.Id).Error)
	require.EqualValues(t, hold.Quota, stored.UsedQuota)
	require.Zero(t, stored.RemainQuota)
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, 1).Error)
	require.EqualValues(t, hold.Quota, channel.UsedQuota)
}

func TestEnterpriseTokenPayerOverAPI(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	root, owner, staff, outsider := api.user("root"), api.user("owner"), api.user("staff"), api.user("outsider")
	ent := api.org(root, "甲公司", owner)
	const user = common.RoleCommonUser
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit",
		map[string]any{"amount": 100000, "voucher": "BANK-PAYER"})
	staffMember := api.join(ent, owner, staff, model.EnterpriseRoleMember, 50000)

	// 不是企业成员不能选企业付款；付款方取值只有两种。
	api.denied(outsider.Id, user, http.MethodPost, "/api/token/", map[string]any{"name": "x", "expired_time": -1, "payer": model.TokenPayerEnterprise}, model.ErrEnterpriseNotFound)
	api.denied(staff.Id, user, http.MethodPost, "/api/token/", map[string]any{"name": "x", "expired_time": -1, "payer": "company"}, nil)
	api.denied(outsider.Id, user, http.MethodGet, "/api/user/self/groups?payer=enterprise", nil, model.ErrEnterpriseNotFound)
	api.ok(staff.Id, user, http.MethodGet, "/api/user/self/groups?payer=enterprise", nil)

	// 请求体里伪造的企业归属不被采信：不选企业付款就是个人令牌。
	forged := api.token(staff, "伪造归属", "", map[string]any{"enterprise_id": ent, "membership_id": staffMember})
	require.Zero(t, forged.EnterpriseId)
	require.Zero(t, forged.MembershipId)
	forgedOutsider := api.token(outsider, "伪造归属", "", map[string]any{"enterprise_id": ent, "membership_id": staffMember})
	require.Zero(t, forgedOutsider.EnterpriseId)

	// 企业令牌不使用令牌表的剩余额度，上限记在企业账本里。
	token := api.token(staff, "企业令牌", model.TokenPayerEnterprise,
		map[string]any{"unlimited_quota": false, "remain_quota": 777, "enterprise_limit_quota": 300})
	require.Equal(t, ent, token.EnterpriseId)
	require.Equal(t, staffMember, token.MembershipId)
	require.True(t, token.UnlimitedQuota)
	require.Zero(t, token.RemainQuota)
	budgets, err := model.GetEnterpriseTokenBudgets([]int{token.Id})
	require.NoError(t, err)
	require.EqualValues(t, 300, budgets[token.Id].LimitQuota)
	api.denied(staff.Id, user, http.MethodPost, "/api/token/", map[string]any{"name": "套餐", "expired_time": -1,
		"payer": model.TokenPayerEnterprise, "package_enabled": true, "package_limit_quota": 100, "package_period": "daily"}, nil)
	api.denied(staff.Id, user, http.MethodPost, "/api/token/", map[string]any{"name": "负数", "expired_time": -1,
		"payer": model.TokenPayerEnterprise, "enterprise_limit_quota": -1}, nil)

	// 列表标出付款方并可筛选。
	list := func(query string) []any {
		resp := api.ok(staff.Id, user, http.MethodGet, "/api/token/"+query, nil)
		items, _ := resp.object()["items"].([]any)
		return items
	}
	require.Len(t, list(""), 2)
	enterpriseItems := list("?payer=enterprise")
	require.Len(t, enterpriseItems, 1)
	item := enterpriseItems[0].(map[string]any)
	require.Equal(t, model.TokenPayerEnterprise, item["payer"])
	require.Equal(t, "甲公司", item["enterprise_name"])
	require.EqualValues(t, 300, item["enterprise_limit_quota"])
	personalItems := list("?payer=personal")
	require.Len(t, personalItems, 1)
	require.Equal(t, model.TokenPayerPersonal, personalItems[0].(map[string]any)["payer"])
	api.denied(staff.Id, user, http.MethodGet, "/api/token/?payer=company", nil, nil)
	require.Len(t, api.ok(staff.Id, user, http.MethodGet, "/api/token/search?keyword=&payer=enterprise", nil).object()["items"], 1)

	// 付款方创建后不可改：编辑只能调上限，改不了归属，也改不回令牌表额度。
	update := api.ok(staff.Id, user, http.MethodPut, "/api/token/", map[string]any{"id": token.Id, "name": "企业令牌", "group": "default", "expired_time": -1,
		"enterprise_id": 0, "membership_id": 0, "unlimited_quota": false, "remain_quota": 999, "enterprise_limit_quota": 120})
	require.Equal(t, model.TokenPayerEnterprise, update.object()["payer"])
	require.EqualValues(t, 120, update.object()["enterprise_limit_quota"])
	var stored model.Token
	require.NoError(t, model.DB.First(&stored, token.Id).Error)
	require.Equal(t, ent, stored.EnterpriseId)
	require.Equal(t, staffMember, stored.MembershipId)
	require.True(t, stored.UnlimitedQuota)
	require.Zero(t, stored.RemainQuota)
	api.ok(staff.Id, user, http.MethodPut, "/api/token/", map[string]any{"id": forged.Id, "name": "伪造归属", "group": "default", "expired_time": -1,
		"unlimited_quota": true, "enterprise_id": ent, "membership_id": staffMember, "enterprise_limit_quota": 50})
	var personal model.Token
	require.NoError(t, model.DB.First(&personal, forged.Id).Error)
	require.Zero(t, personal.EnterpriseId)
	require.Zero(t, personal.MembershipId)
	budgets, err = model.GetEnterpriseTokenBudgets([]int{forged.Id})
	require.NoError(t, err)
	require.Empty(t, budgets, "个人令牌不会写入企业账本")
	// 别人的企业令牌改不了。
	api.denied(outsider.Id, user, http.MethodPut, "/api/token/", map[string]any{"id": token.Id, "name": "x", "group": "default", "expired_time": -1, "enterprise_limit_quota": 1}, nil)

	// 用量查询展示企业账本里的令牌上限，不暴露个人钱包。
	req := httptest.NewRequest(http.MethodGet, "/api/usage/token/", nil)
	req.Header.Set("Authorization", "Bearer sk-"+stored.Key)
	w := httptest.NewRecorder()
	api.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var usage struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &usage))
	require.Equal(t, model.TokenPayerEnterprise, usage.Data["payer"])
	require.EqualValues(t, 120, usage.Data["total_granted"])
	require.EqualValues(t, 120, usage.Data["total_available"])
	require.Equal(t, false, usage.Data["unlimited_quota"])
}

// 没有请求号（无法建立可对账预留）或落到个人钱包结算路径的企业请求都必须报错，而不是扣个人的钱。
func TestEnterpriseRequestNeverFallsBackToPersonalWallet(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	staff := api.user("staff")
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", staff.Id).Update("quota", 100000).Error)
	token := api.token(staff, "个人令牌", model.TokenPayerPersonal, map[string]any{"unlimited_quota": false, "remain_quota": 5000})
	info := &relaycommon.RelayInfo{UserId: staff.Id, TokenId: token.Id, TokenKey: token.Key, EnterpriseId: 1, EnterpriseMemberId: 1}

	require.ErrorIs(t, service.PostConsumeQuota(info, 100, 0, false), model.ErrEnterpriseUnsupported)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	session, apiErr := service.NewBillingSession(c, info, 100)
	require.Nil(t, session)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.Contains(t, apiErr.Error(), enterpriseUnsupported)

	require.Equal(t, 100000, api.personalQuota(staff))
	var stored model.Token
	require.NoError(t, model.DB.First(&stored, token.Id).Error)
	require.Equal(t, 5000, stored.RemainQuota)
	require.Zero(t, stored.UsedQuota)
}
