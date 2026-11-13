package router

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// Full first-batch handoff through real API auth, distributor and billing, with only the upstream synthetic.
func TestEnterpriseHandoffSpendApproveResume(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	calls := api.relayTo(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"synthetic","object":"chat.completion","model":"model-a","choices":[{"index":0,"message":{"role":"assistant","content":"synthetic result"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":2000,"total_tokens":3000}}`))
	})
	root, owner, staff := api.user("root"), api.user("owner"), api.user("staff")
	const personal = 123456
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", staff.Id).Update("quota", personal).Error)
	ent := api.org(root, "本地闭环企业", owner)
	base := "/api/enterprises/" + strconv.Itoa(ent)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit", map[string]any{"amount": 50000000, "voucher": "CLOSEOUT-ONE"})
	memberID := api.join(ent, owner, staff, model.EnterpriseRoleMember, 20000000)
	token := api.token(staff, "闭环企业令牌", model.TokenPayerEnterprise, nil)
	const body = `{"model":"model-a","messages":[{"role":"user","content":"synthetic"}]}`
	first := api.relayCall(token, http.MethodPost, "/v1/chat/completions", body)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var member model.EnterpriseMember
	require.NoError(t, model.DB.First(&member, memberID).Error)
	require.Positive(t, member.Consumed)
	require.Zero(t, member.Reserved)
	// Reclaim the remaining budget via owner API, then exercise exhaustion.
	api.ok(owner.Id, common.RoleCommonUser, http.MethodPost, base+"/members/"+strconv.Itoa(memberID)+"/reclaim", map[string]any{"amount": member.Available})
	for range 3 {
		denied := api.relayCall(token, http.MethodPost, "/v1/chat/completions", body)
		require.NotEqual(t, http.StatusOK, denied.Code)
		require.Contains(t, denied.Body.String(), "企业额度不足")
	}
	require.EqualValues(t, 1, calls.Load(), "exhausted calls never reach upstream")
	var stored model.Token
	require.NoError(t, model.DB.First(&stored, token.Id).Error)
	require.Equal(t, common.TokenStatusDisabled, stored.Status)
	request := api.ok(staff.Id, common.RoleCommonUser, http.MethodPost, "/api/enterprise/self/requests", map[string]any{"amount": 10000000, "reason": "本地验收"}).object()
	rid := strconv.Itoa(int(request["id"].(float64)))
	api.ok(owner.Id, common.RoleCommonUser, http.MethodPost, base+"/quota-requests/"+rid+"/approve", map[string]any{"amount": 0})
	require.NoError(t, model.DB.First(&stored, token.Id).Error)
	require.Equal(t, common.TokenStatusDisabled, stored.Status, "approval must not undo disabled status")
	blocked := api.relayCall(token, http.MethodPost, "/v1/chat/completions", body)
	require.NotEqual(t, http.StatusOK, blocked.Code)
	api.ok(staff.Id, common.RoleCommonUser, http.MethodPut, "/api/token/?status_only=true", map[string]any{"id": token.Id, "status": common.TokenStatusEnabled})
	second := api.relayCall(token, http.MethodPost, "/v1/chat/completions", body)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.EqualValues(t, 2, calls.Load())
	bal, err := model.GetEnterpriseBalance(ent)
	require.NoError(t, err)
	require.True(t, bal.Balanced)
	require.Zero(t, bal.MemberReserved)
	require.Equal(t, personal, api.personalQuota(staff))
	overview := api.ok(owner.Id, common.RoleCommonUser, http.MethodGet, base+"/overview", nil).object()
	require.Equal(t, true, overview["balance"].(map[string]any)["balanced"])
}
