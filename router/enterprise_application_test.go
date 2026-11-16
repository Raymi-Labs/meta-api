package router

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"net/http"
	"strconv"
	"testing"
)

func TestEnterpriseApplicationAPI(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	user, other, admin := api.user("applicant"), api.user("other"), api.user("platform")
	payload := map[string]any{"kind": "opening", "name": "企业申请", "contact_type": "qq", "contact": "12345", "user_id": other.Id}
	const path = "/api/enterprise/applications"
	require.False(t, api.call(0, 0, http.MethodPost, path, payload).Success)
	app := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, path, payload).object()
	require.EqualValues(t, user.Id, app["user_id"])
	require.EqualValues(t, 0, api.ok(other.Id, common.RoleCommonUser, http.MethodGet, path, nil).object()["total"])
	require.EqualValues(t, 1, api.ok(user.Id, common.RoleCommonUser, http.MethodGet, path, nil).object()["total"])
	review := "/api/enterprise/admin/applications/" + strconv.Itoa(int(app["id"].(float64))) + "/review"
	require.False(t, api.call(user.Id, common.RoleCommonUser, http.MethodGet, "/api/enterprise/admin/applications", nil).Success)
	require.False(t, api.call(user.Id, common.RoleCommonUser, http.MethodPost, review, map[string]any{"action": "approve", "billing_group": "default"}).Success)
	require.EqualValues(t, 1, api.ok(admin.Id, common.RoleAdminUser, http.MethodGet, "/api/enterprise/admin/applications?status=pending&kind=opening&page_size=1", nil).object()["total"])
	api.ok(admin.Id, common.RoleAdminUser, http.MethodPost, review, map[string]any{"action": "approve", "billing_group": "default"})
	self := api.ok(user.Id, common.RoleCommonUser, http.MethodGet, "/api/enterprise/self", nil).object()
	require.NotNil(t, self["enterprise"])
	require.False(t, api.call(admin.Id, common.RoleAdminUser, http.MethodPost, review, map[string]any{"action": "approve", "billing_group": "default"}).Success)
	require.EqualValues(t, 0, api.ok(admin.Id, common.RoleAdminUser, http.MethodGet, "/api/enterprise/admin/applications?status=pending", nil).object()["total"])
}

func TestEnterpriseApplicationDisabled(t *testing.T) {
	api := setupEnterpriseAPI(t, false)
	user := api.user("applicant")
	api.denied(user.Id, common.RoleCommonUser, http.MethodPost, "/api/enterprise/applications", map[string]any{}, model.ErrEnterpriseDisabled)
	api.denied(user.Id, common.RoleAdminUser, http.MethodGet, "/api/enterprise/admin/applications", nil, model.ErrEnterpriseDisabled)
}
