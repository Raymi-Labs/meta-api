package router

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseOwnerSelfApprovalOverAPI(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	root, owner, admin := api.user("root"), api.user("owner"), api.user("admin")
	ent := api.org(root, "Owner审批测试", owner)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit", map[string]any{"amount": 1000, "voucher": "OWNER-API"})
	api.join(ent, owner, admin, model.EnterpriseRoleAdmin, 0)
	const role = common.RoleCommonUser
	base := "/api/enterprises/" + strconv.Itoa(ent)
	for _, action := range []string{"approve", "reject"} {
		created := api.ok(owner.Id, role, http.MethodPost, "/api/enterprise/self/requests", map[string]any{"amount": 100}).object()
		path := base + "/quota-requests/" + strconv.Itoa(int(created["id"].(float64))) + "/" + action
		api.denied(admin.Id, role, http.MethodPost, path, nil, model.ErrEnterpriseForbidden)
		api.ok(owner.Id, role, http.MethodPost, path, nil)
		api.denied(owner.Id, role, http.MethodPost, path, nil, model.ErrEnterpriseRequestState)
		require.EqualValues(t, 0, api.ok(owner.Id, role, http.MethodGet, "/api/enterprise/self", nil).object()["pending_requests"])
	}
	created := api.ok(admin.Id, role, http.MethodPost, "/api/enterprise/self/requests", map[string]any{"amount": 100}).object()
	path := base + "/quota-requests/" + strconv.Itoa(int(created["id"].(float64))) + "/approve"
	api.denied(admin.Id, role, http.MethodPost, path, nil, model.ErrEnterpriseSelfOperation)
	api.ok(owner.Id, role, http.MethodPost, path, nil)
	balance, err := model.GetEnterpriseBalance(ent)
	require.NoError(t, err)
	require.True(t, balance.Balanced)
	require.EqualValues(t, 800, balance.Unallocated)
	require.EqualValues(t, 200, balance.MemberAvailable)
	require.Zero(t, api.personalQuota(owner))
}
