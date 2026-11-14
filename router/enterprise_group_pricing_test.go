package router

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseSelectExistingGroupPricesAllNewRequests(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	calls := api.relayTo(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fixture","object":"chat.completion","model":"model-a","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":0,"total_tokens":1000}}`))
	})
	oldModel, oldSpecial := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModel))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldSpecial))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"优选":0.8,"free":0}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"default":0.6},"优选":{"default":9}}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"model-a":1}`))
	require.NoError(t, model.DB.AutoMigrate(&model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
	root, owner, staff := api.user("root"), api.user("owner"), api.user("staff")
	require.NoError(t, model.DB.Model(staff).Update("quota", 100000).Error)
	ent := api.org(root, "企业选组验收", owner)
	path := "/api/enterprise/admin/enterprises/" + strconv.Itoa(ent)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, path+"/credit", map[string]any{"amount": 100000, "voucher": "GROUP-FIXTURE"})
	api.join(ent, owner, staff, model.EnterpriseRoleMember, 50000)
	enterprise := api.token(staff, "企业付款", model.TokenPayerEnterprise, nil)
	personal := api.token(staff, "个人付款", model.TokenPayerPersonal, nil)
	var expected int64
	for _, tc := range []struct {
		group string
		quota int64
	}{{"优选", 800}, {"default", 1000}, {"free", 0}} {
		api.ok(root.Id, common.RoleRootUser, http.MethodPut, path, map[string]any{"name": "企业选组验收", "billing_group": tc.group})
		w := api.relayCall(enterprise, http.MethodPost, "/v1/chat/completions", `{"model":"model-a","messages":[{"role":"user","content":"test"}]}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		expected += tc.quota
		balance, err := model.GetEnterpriseBalance(ent)
		require.NoError(t, err)
		require.Equal(t, expected, balance.Consumed)
		require.True(t, balance.Balanced)
		require.Equal(t, 100000, api.personalQuota(staff))
		var account model.User
		require.NoError(t, model.DB.First(&account, staff.Id).Error)
		require.Equal(t, "default", account.Group)
	}
	// A retired billing group rejects a new call before any upstream work or hold.
	api.ok(root.Id, common.RoleRootUser, http.MethodPut, path, map[string]any{"name": "企业选组验收", "billing_group": "优选"})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"free":0}`))
	before := calls.Load()
	w := api.relayCall(enterprise, http.MethodPost, "/v1/chat/completions", `{"model":"model-a","messages":[{"role":"user","content":"test"}]}`)
	require.NotEqual(t, http.StatusOK, w.Code)
	require.Equal(t, before, calls.Load())
	balance, err := model.GetEnterpriseBalance(ent)
	require.NoError(t, err)
	require.Equal(t, expected, balance.Consumed)
	require.Zero(t, balance.MemberReserved)
	// Joining an enterprise and changing its group never changes personal pricing.
	w = api.relayCall(personal, http.MethodPost, "/v1/chat/completions", `{"model":"model-a","messages":[{"role":"user","content":"test"}]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 99400, api.personalQuota(staff))
	require.Equal(t, before+1, calls.Load())

	// Midjourney submit uses the same local validation before capacity/upstream.
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 1).Updates(map[string]any{"type": constant.ChannelTypeMidjourney, "models": "mj_imagine"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "mj_imagine", ChannelId: 1, Enabled: true}).Error)
	before = calls.Load()
	w = api.relayCall(enterprise, http.MethodPost, "/mj/submit/imagine", `{"prompt":"fixture"}`)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "计费分组")
	require.Equal(t, before, calls.Load())
}
