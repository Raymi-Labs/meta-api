package router

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/require"
)

// raw 发起请求并返回原始响应，用于 CSV 导出和检查响应体里有没有不该出现的内容。
func (a *enterpriseAPI) raw(userID, role int, path string) *httptest.ResponseRecorder {
	a.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Test-User", strconv.Itoa(userID))
	req.Header.Set("X-Test-Role", strconv.Itoa(role))
	req.Header.Set("New-Api-User", strconv.Itoa(userID))
	w := httptest.NewRecorder()
	a.engine.ServeHTTP(w, req)
	return w
}

func (a *enterpriseAPI) csvRows(userID, role int, path string) [][]string {
	a.t.Helper()
	w := a.raw(userID, role, path)
	require.Equal(a.t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(a.t, w.Header().Get("Content-Type"), "text/csv")
	require.Equal(a.t, "no-store", w.Header().Get("Cache-Control"))
	require.True(a.t, strings.HasPrefix(w.Body.String(), "\xef\xbb\xbf"))
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(a.t, err)
	return rows
}

func enterprisePageItems(t *testing.T, resp enterpriseAPIResponse) (items []map[string]any, total int) {
	t.Helper()
	page := resp.object()
	require.NotNil(t, page)
	for _, item := range page["items"].([]any) {
		items = append(items, item.(map[string]any))
	}
	return items, int(page["total"].(float64))
}

func TestEnterpriseUsageOverAPI(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	require.NoError(t, model.DB.AutoMigrate(&model.Log{}))
	const user = common.RoleCommonUser
	root, platformAdmin := api.user("root"), api.user("platform-admin")
	ownerA, adminA, staffA, staff2A := api.user("owner-a"), api.user("admin-a"), api.user("staff-a"), api.user("staff2-a")
	ownerB, outsider := api.user("owner-b"), api.user("outsider")
	entA, entB := api.org(root, "甲公司", ownerA), api.org(root, "乙公司", ownerB)
	api.join(entA, ownerA, adminA, model.EnterpriseRoleAdmin, 0)
	staffMember := api.join(entA, ownerA, staffA, model.EnterpriseRoleMember, 0)
	staff2Member := api.join(entA, ownerA, staff2A, model.EnterpriseRoleMember, 0)
	ownerBMember, err := model.GetActiveEnterpriseMemberByUser(ownerB.Id)
	require.NoError(t, err)
	for _, ent := range []int{entA, entB} {
		_, err := model.CreditEnterprise(root.Id, ent, 100000, "V-1", "")
		require.NoError(t, err)
	}
	require.NoError(t, model.AllocateEnterpriseQuota(model.EnterpriseActor{UserId: ownerA.Id}, entA, staffMember, 20000, ""))
	require.NoError(t, model.AllocateEnterpriseQuota(model.EnterpriseActor{UserId: ownerA.Id}, entA, staff2Member, 20000, ""))
	require.NoError(t, model.AllocateEnterpriseQuota(model.EnterpriseActor{Platform: true}, entB, ownerBMember.Id, 20000, ""))

	staffToken := api.token(staffA, "员工企业令牌", model.TokenPayerEnterprise, nil)
	staffPersonal := api.token(staffA, "员工个人令牌", model.TokenPayerPersonal, nil)
	foreignToken := api.token(ownerB, "乙公司令牌", model.TokenPayerEnterprise, nil)

	settle := func(sub model.EnterpriseSubject, id string, quota int) {
		_, err := model.AdjustEnterpriseReservation(sub, id, quota, model.EnterpriseReservationSettled, false)
		require.NoError(t, err)
	}
	settle(model.EnterpriseSubject{EnterpriseId: entA, MemberId: staffMember, UserId: staffA.Id, TokenId: staffToken.Id,
		ModelName: "gpt-a", TokenName: staffToken.Name}, "usage-a1", 300)
	settle(model.EnterpriseSubject{EnterpriseId: entA, MemberId: staff2Member, UserId: staff2A.Id,
		ModelName: "gpt-b", TokenName: "另一个令牌"}, "usage-a2", 1000)
	settle(model.EnterpriseSubject{EnterpriseId: entB, MemberId: ownerBMember.Id, UserId: ownerB.Id, TokenId: foreignToken.Id,
		ModelName: "gpt-b", TokenName: foreignToken.Name}, "usage-b1", 4000)

	baseA, baseB := "/api/enterprises/"+strconv.Itoa(entA), "/api/enterprises/"+strconv.Itoa(entB)
	platformA := "/api/enterprise/admin/enterprises/" + strconv.Itoa(entA)
	now := time.Now().Unix()
	window := fmt.Sprintf("start_timestamp=%d&end_timestamp=%d", now-3600, now+3600)

	// 所有者与管理员看到本企业账单：金额、模型、时间、令牌名称；看不到别的企业。
	for _, who := range []*model.User{ownerA, adminA} {
		items, total := enterprisePageItems(t, api.ok(who.Id, user, http.MethodGet, baseA+"/usage", nil))
		require.Equal(t, 2, total)
		require.Equal(t, "gpt-b", items[0]["model_name"])
		require.EqualValues(t, 1000, items[0]["quota"])
		require.Equal(t, staffToken.Name, items[1]["token_name"])
		require.Equal(t, "staff-a", items[1]["member_name"])
		for _, item := range items {
			require.EqualValues(t, entA, item["enterprise_id"])
		}
	}
	items, total := enterprisePageItems(t, api.ok(ownerA.Id, user, http.MethodGet, baseA+"/usage?member_id="+strconv.Itoa(staffMember), nil))
	require.Equal(t, 1, total)
	require.EqualValues(t, 300, items[0]["quota"])
	// 把别的企业的成员、令牌塞进筛选条件：查不到任何东西，而不是查到对方的账单。
	_, total = enterprisePageItems(t, api.ok(ownerA.Id, user, http.MethodGet, baseA+"/usage?member_id="+strconv.Itoa(ownerBMember.Id), nil))
	require.Zero(t, total)
	_, total = enterprisePageItems(t, api.ok(ownerA.Id, user, http.MethodGet, baseA+"/usage?token_id="+strconv.Itoa(foreignToken.Id), nil))
	require.Zero(t, total)
	for _, bad := range []string{"member_id=abc", "member_id=-1", "kind=x", "status=x", "start_timestamp=9&end_timestamp=1"} {
		api.denied(ownerA.Id, user, http.MethodGet, baseA+"/usage?"+bad, nil, model.ErrEnterpriseInvalidInput)
		api.denied(ownerA.Id, user, http.MethodGet, baseA+"/usage/summary?"+bad, nil, model.ErrEnterpriseInvalidInput)
	}

	summary := api.ok(ownerA.Id, user, http.MethodGet, baseA+"/usage/summary", nil).object()
	require.EqualValues(t, 1300, summary["net_consumed"])
	require.EqualValues(t, 2, summary["requests"])
	require.Equal(t, "Asia/Shanghai", summary["timezone"])
	require.Len(t, summary["by_member"], 2)
	require.Len(t, summary["by_model"], 2)
	filtered := api.ok(ownerA.Id, user, http.MethodGet, baseA+"/usage/summary?model_name=gpt-a", nil).object()
	require.EqualValues(t, 300, filtered["net_consumed"])

	// 导出与列表同条件、同行数；时间按企业时区。
	rows := api.csvRows(ownerA.Id, user, baseA+"/usage/export?"+window)
	require.Len(t, rows, 3)
	require.Equal(t, "时间（Asia/Shanghai）", rows[0][0])
	require.True(t, strings.HasSuffix(rows[1][0], "+08:00"), rows[1][0])
	require.Equal(t, []string{"staff2-a", "另一个令牌", "gpt-b", "请求", "已结算", "1000", "usage-a2"}, rows[1][1:])
	rows = api.csvRows(ownerA.Id, user, baseA+"/usage/export?"+window+"&member_id="+strconv.Itoa(staffMember))
	require.Len(t, rows, 2)
	// 导出必须给时间范围；被拒绝时返回 JSON 而不是空文件。
	api.denied(ownerA.Id, user, http.MethodGet, baseA+"/usage/export", nil, nil)
	api.denied(ownerA.Id, user, http.MethodGet, baseB+"/usage/export?"+window, nil, model.ErrEnterpriseNotFound)
	api.denied(staffA.Id, user, http.MethodGet, baseA+"/usage/export?"+window, nil, model.ErrEnterpriseForbidden)

	// 平台管理员走平台接口看同一份账单；企业后台接口不认平台身份。
	_, total = enterprisePageItems(t, api.ok(platformAdmin.Id, common.RoleAdminUser, http.MethodGet, platformA+"/usage", nil))
	require.Equal(t, 2, total)
	require.EqualValues(t, 1300, api.ok(platformAdmin.Id, common.RoleAdminUser, http.MethodGet, platformA+"/usage/summary", nil).object()["net_consumed"])
	require.Len(t, api.csvRows(platformAdmin.Id, common.RoleAdminUser, platformA+"/usage/export?"+window), 3)
	api.denied(ownerA.Id, user, http.MethodGet, platformA+"/usage", nil, nil)

	// 总览带成员消费排行。
	spenders := api.ok(ownerA.Id, user, http.MethodGet, baseA+"/overview", nil).object()["top_spenders"].([]any)
	require.Len(t, spenders, 2)
	require.Equal(t, "staff2-a", spenders[0].(map[string]any)["name"])
	require.EqualValues(t, 1000, spenders[0].(map[string]any)["consumed"])

	// 任何企业后台响应都不含密钥明文。
	for _, path := range []string{baseA + "/usage", baseA + "/usage/summary", baseA + "/usage/export?" + window, baseA + "/tokens", baseA + "/overview"} {
		body := api.raw(ownerA.Id, user, path).Body.String()
		for _, token := range []*model.Token{staffToken, staffPersonal, foreignToken} {
			require.NotContains(t, body, token.Key, path)
		}
	}

	// 成员令牌列表：只有本企业由企业付款的令牌。
	tokens, total := enterprisePageItems(t, api.ok(adminA.Id, user, http.MethodGet, baseA+"/tokens", nil))
	require.Equal(t, 1, total)
	require.EqualValues(t, staffToken.Id, tokens[0]["id"])
	require.Equal(t, "staff-a", tokens[0]["member_name"])
	require.NotEmpty(t, tokens[0]["masked_key"])
	_, total = enterprisePageItems(t, api.ok(platformAdmin.Id, common.RoleAdminUser, http.MethodGet, platformA+"/tokens", nil))
	require.Equal(t, 1, total)

	// 本人使用日志按付款方区分。
	sessionName := func(u *model.User) string { return "local-test-user-" + strconv.Itoa(u.Id) }
	logs := []*model.Log{
		{UserId: staffA.Id, Username: sessionName(staffA), CreatedAt: now, Type: model.LogTypeConsume, TokenId: staffToken.Id, TokenName: staffToken.Name, ModelName: "gpt-a", Quota: 300, RequestId: "usage-a1"},
		{UserId: staffA.Id, Username: sessionName(staffA), CreatedAt: now, Type: model.LogTypeConsume, TokenId: staffPersonal.Id, TokenName: staffPersonal.Name, ModelName: "gpt-a", Quota: 70},
		{UserId: outsider.Id, Username: sessionName(outsider), CreatedAt: now, Type: model.LogTypeConsume, TokenId: 987654, TokenName: "外人令牌", ModelName: "gpt-a", Quota: 5},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)
	selfLogs := func(u *model.User, query string) ([]map[string]any, int) {
		return enterprisePageItems(t, api.ok(u.Id, user, http.MethodGet, "/api/log/self?"+query, nil))
	}
	selfQuota := func(u *model.User, query string) float64 {
		return api.ok(u.Id, user, http.MethodGet, "/api/log/self/stat?"+query, nil).object()["quota"].(float64)
	}
	checkStaffLogs := func() {
		t.Helper()
		items, total := selfLogs(staffA, "")
		require.Equal(t, 2, total)
		payers := map[any]any{}
		for _, item := range items {
			payers[item["token_name"]] = item["payer"]
		}
		require.Equal(t, map[any]any{staffToken.Name: model.TokenPayerEnterprise, staffPersonal.Name: model.TokenPayerPersonal}, payers)

		items, total = selfLogs(staffA, "payer=enterprise")
		require.Equal(t, 1, total)
		require.Equal(t, staffToken.Name, items[0]["token_name"])
		require.Equal(t, "甲公司", items[0]["enterprise_name"])
		items, total = selfLogs(staffA, "payer=personal")
		require.Equal(t, 1, total)
		require.Equal(t, staffPersonal.Name, items[0]["token_name"])
		require.Nil(t, items[0]["enterprise_name"])

		require.EqualValues(t, 370, selfQuota(staffA, ""))
		require.EqualValues(t, 300, selfQuota(staffA, "payer=enterprise"))
		require.EqualValues(t, 70, selfQuota(staffA, "payer=personal"))
		require.Len(t, api.csvRows(staffA.Id, user, "/api/log/self/export?"+window), 3)
		require.Len(t, api.csvRows(staffA.Id, user, "/api/log/self/export?"+window+"&payer=enterprise"), 2)
		require.Len(t, api.csvRows(staffA.Id, user, "/api/log/self/export?"+window+"&payer=personal"), 2)
	}
	checkStaffLogs()
	api.denied(staffA.Id, user, http.MethodGet, "/api/log/self?payer=other", nil, nil)
	api.denied(staffA.Id, user, http.MethodGet, "/api/log/self/stat?payer=other", nil, nil)
	api.denied(staffA.Id, user, http.MethodGet, "/api/log/self/export?"+window+"&payer=other", nil, nil)
	// 没有企业令牌的账号筛"企业"：结果为空，不能退化成全部日志。
	_, total = selfLogs(outsider, "payer=enterprise")
	require.Zero(t, total)
	require.Zero(t, selfQuota(outsider, "payer=enterprise"))
	_, total = selfLogs(outsider, "payer=personal")
	require.Equal(t, 1, total)

	// 撤销：管理员只能撤销本企业普通成员的企业令牌。
	revoke := func(base string, token *model.Token) string {
		return base + "/tokens/" + strconv.Itoa(token.Id) + "/revoke"
	}
	api.denied(adminA.Id, user, http.MethodPost, revoke(baseA, foreignToken), nil, model.ErrEnterpriseNotFound)
	api.denied(adminA.Id, user, http.MethodPost, revoke(baseA, staffPersonal), nil, model.ErrEnterpriseNotFound)
	api.denied(ownerB.Id, user, http.MethodPost, revoke(baseA, staffToken), nil, model.ErrEnterpriseNotFound)
	api.denied(staff2A.Id, user, http.MethodPost, revoke(baseA, staffToken), nil, model.ErrEnterpriseForbidden)
	api.relay()
	require.Equal(t, http.StatusOK, api.relayCall(staffToken, http.MethodGet, "/v1/models", "").Code)
	api.ok(adminA.Id, user, http.MethodPost, revoke(baseA, staffToken), map[string]any{"reason": "交接"})
	// 撤销立即生效：同一把密钥不能再调用；同一成员的个人令牌不受影响。
	require.Equal(t, http.StatusUnauthorized, api.relayCall(staffToken, http.MethodGet, "/v1/models", "").Code)
	require.Equal(t, http.StatusOK, api.relayCall(staffPersonal, http.MethodGet, "/v1/models", "").Code)
	api.denied(adminA.Id, user, http.MethodPost, revoke(baseA, staffToken), nil, model.ErrEnterpriseNotFound)
	var remaining int64
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id IN ?", []int{staffToken.Id, staffPersonal.Id, foreignToken.Id}).Count(&remaining).Error)
	require.EqualValues(t, 2, remaining)
	_, total = enterprisePageItems(t, api.ok(adminA.Id, user, http.MethodGet, baseA+"/tokens", nil))
	require.Zero(t, total)
	// 令牌撤销后，历史账单与本人日志的付款方归属不变。
	_, total = enterprisePageItems(t, api.ok(ownerA.Id, user, http.MethodGet, baseA+"/usage", nil))
	require.Equal(t, 2, total)
	checkStaffLogs()
}
