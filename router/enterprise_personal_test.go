package router

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseFeatureNeverRedirectsPersonalSettlement(t *testing.T) {
	for _, scenario := range []string{"disabled", "enabled_without_membership", "enterprise_member", "frozen_enterprise"} {
		t.Run(scenario, func(t *testing.T) {
			api := setupEnterpriseAPI(t, scenario != "disabled")
			require.NoError(t, model.DB.AutoMigrate(&model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
			calls := api.relayTo(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"fixture","object":"chat.completion","model":"model-a","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":2000,"total_tokens":3000}}`))
			})
			root, owner, user := api.user("root"), api.user("owner"), api.user("personal")
			const initial = 20_000_000
			require.NoError(t, model.DB.Model(user).Update("quota", initial).Error)
			ent := 0
			if scenario == "enterprise_member" || scenario == "frozen_enterprise" {
				ent = api.org(root, "个人隔离验收", owner)
				api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit", map[string]any{"amount": 1000, "voucher": "FIXTURE"})
				api.join(ent, owner, user, model.EnterpriseRoleMember, 500)
				if scenario == "frozen_enterprise" {
					api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/status", map[string]any{"status": "frozen", "reason": "fixture"})
				}
			}
			// 旧客户端省略 payer 时仍创建个人令牌。
			token := api.token(user, "个人付款", "", nil)
			require.Zero(t, token.EnterpriseId)
			response := api.relayCall(token, http.MethodPost, "/v1/chat/completions", `{"model":"model-a","messages":[{"role":"user","content":"hi"}]}`)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.EqualValues(t, 1, calls.Load())
			var stored model.User
			require.NoError(t, model.DB.First(&stored, user.Id).Error)
			require.Positive(t, stored.UsedQuota)
			require.Equal(t, initial-stored.UsedQuota, stored.Quota)
			var reservations int64
			require.NoError(t, model.DB.Model(&model.EnterpriseReservation{}).Count(&reservations).Error)
			require.Zero(t, reservations)
			if ent > 0 {
				balance, err := model.GetEnterpriseBalance(ent)
				require.NoError(t, err)
				require.True(t, balance.Balanced)
				require.EqualValues(t, 1000, balance.NetCredited)
				require.Zero(t, balance.Consumed)
				require.Zero(t, balance.MemberReserved)
			}
		})
	}
}

func TestEnterpriseFeatureOptionRequiresRoot(t *testing.T) {
	api := setupEnterpriseAPI(t, false)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	root, admin, user := api.user("root"), api.user("admin"), api.user("user")
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{}
	for key, value := range previous {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})

	body := map[string]any{"key": "EnterpriseEnabled", "value": "true"}
	api.denied(user.Id, common.RoleCommonUser, http.MethodPut, "/api/option/", body, nil)
	api.denied(admin.Id, common.RoleAdminUser, http.MethodPut, "/api/option/", body, nil)
	require.False(t, common.EnterpriseEnabled)
	api.ok(root.Id, common.RoleRootUser, http.MethodPut, "/api/option/", body)
	require.True(t, common.EnterpriseEnabled)
	api.ok(root.Id, common.RoleRootUser, http.MethodPut, "/api/option/", map[string]any{"key": "EnterpriseEnabled", "value": "false"})
	require.False(t, common.EnterpriseEnabled)
}

func TestEnterpriseExactModelPolicyDoesNotExpandThinkingFamily(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	calls := api.relay()
	root, owner, user := api.user("root"), api.user("owner"), api.user("staff")
	ent := api.org(root, "精确模型范围", owner)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit", map[string]any{"amount": 10000000, "voucher": "FIXTURE"})
	api.join(ent, owner, user, model.EnterpriseRoleMember, 10000000)
	const allowed = "gemini-2.5-pro-thinking-128"
	const forbidden = "gemini-2.5-pro-thinking-256"
	api.ok(owner.Id, common.RoleCommonUser, http.MethodPut, "/api/enterprises/"+strconv.Itoa(ent)+"/settings", map[string]any{"name": "精确模型范围", "timezone": "UTC", "model_mode": "custom", "models": []string{allowed}})
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 1).Update("models", allowed+","+forbidden).Error)
	for _, name := range []string{allowed, forbidden} {
		require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: name, ChannelId: 1, Enabled: true}).Error)
	}
	token := api.token(user, "精确模型", model.TokenPayerEnterprise, nil)
	require.Contains(t, api.modelIDs(token), allowed)
	w := api.relayCall(token, http.MethodPost, "/v1/chat/completions", `{"model":"`+allowed+`","messages":[{"role":"user","content":"hi"}]}`)
	require.NotEqual(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Positive(t, calls.Load(), w.Body.String())
	before := calls.Load()
	w = api.relayCall(token, http.MethodPost, "/v1/chat/completions", `{"model":"`+forbidden+`","messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Equal(t, before, calls.Load())
}
