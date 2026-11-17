package router

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseOperationHTTPRecovery(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	root, owner := api.user("root"), api.user("owner")
	ent := api.org(root, "operation-http", owner)
	base := "/api/enterprise/admin/enterprises/" + strconv.Itoa(ent)
	// Missing keys fail before money moves, including stale clients.
	body := map[string]any{"amount": 1000, "voucher": "OP-CREDIT"}
	r := api.callWithKey(root.Id, common.RoleRootUser, http.MethodPost, base+"/credit", body, "")
	require.False(t, r.Success)
	key := common.GetUUID()
	r = api.callWithKey(root.Id, common.RoleRootUser, http.MethodPost, base+"/credit", body, key)
	require.True(t, r.Success, r.Message)
	opID := r.object()["operation_id"]
	// Simulate losing the response: same POST and subsequent GET return one receipt.
	r = api.callWithKey(root.Id, common.RoleRootUser, http.MethodPost, base+"/credit", body, key)
	require.True(t, r.Success, r.Message)
	require.Equal(t, opID, r.object()["operation_id"])
	require.Equal(t, true, r.object()["replayed"])
	got := api.ok(root.Id, common.RoleRootUser, http.MethodGet, base+"/operations/"+key, nil)
	require.Equal(t, opID, got.object()["receipt"].(map[string]any)["operation_id"])
	bal, err := model.GetEnterpriseBalance(ent)
	require.NoError(t, err)
	require.EqualValues(t, 1000, bal.NetCredited)
	// Session remains root, but the main database no longer grants it.
	require.NoError(t, model.DB.Model(root).Update("role", common.RoleCommonUser).Error)
	r = api.callWithKey(root.Id, common.RoleRootUser, http.MethodPost, base+"/credit", body, key)
	require.False(t, r.Success)
	r = api.call(root.Id, common.RoleRootUser, http.MethodGet, base+"/operations/"+key, nil)
	require.False(t, r.Success)
}
