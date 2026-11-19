package router

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
)

func TestInviteTierAPIIsolationAndPermissions(t *testing.T) {
	require.NoError(t, i18n.Init())
	api := setupEnterpriseAPI(t, false)
	require.NoError(t, model.DB.AutoMigrate(&model.InviteCommissionStatement{}, &model.InviteCommissionLedger{}, &model.PromoterProfile{}, &model.InviteeQualification{}, &model.PromoterIdentity{}, &model.InviteCommissionDebt{}))
	owner := api.user("invite-owner")
	other := api.user("invite-other")
	admin := api.user("invite-admin")
	root := api.user("invite-root")
	require.NoError(t, model.DB.Model(root).Update("role", common.RoleRootUser).Error)
	denied := api.call(admin.Id, common.RoleAdminUser, "POST", "/api/admin/invite/promoters/"+strconv.Itoa(root.Id)+"/freeze", map[string]any{"frozen": true})
	require.False(t, denied.Success)
	st := model.InviteCommissionStatement{InviterUserId: owner.Id, PeriodType: 2, PeriodKey: "2026-11", Status: 2}
	require.NoError(t, model.DB.Create(&st).Error)
	l := model.InviteCommissionLedger{InviterUserId: owner.Id, InviteeUserId: other.Id, DirectInviteeUserId: other.Id, TopupTradeNo: "private-trade-123456789", SourceTradeNo: "private-source", StatementId: st.Id, AmountVersion: 2, CommissionQuota: 500, Status: model.InviteCommissionStatusPending}
	require.NoError(t, model.DB.Create(&l).Error)
	anonymous := api.call(0, 0, "GET", "/api/invite/tier/status", nil)
	require.NotEqual(t, 200, anonymous.Code)
	forbidden := api.call(other.Id, common.RoleCommonUser, "PUT", "/api/admin/invite/tier/settings", map[string]any{"enabled": true})
	require.False(t, forbidden.Success)
	response := api.call(other.Id, common.RoleCommonUser, "GET", "/api/invite/commission/ledgers?user_id=1&statement_id=1", nil)
	require.True(t, response.Success)
	require.NotContains(t, fmt.Sprint(response.Data), "private-trade")
	own := api.call(owner.Id, common.RoleCommonUser, "GET", "/api/invite/commission/ledgers", nil)
	require.True(t, own.Success)
	require.NotContains(t, fmt.Sprint(own.Data), "private-source")
	require.NotContains(t, fmt.Sprint(own.Data), "private-trade-123456789")
	decrypt := api.call(admin.Id, common.RoleAdminUser, "GET", "/api/admin/invite/identities/1/plaintext?reason=testing", nil)
	require.False(t, decrypt.Success)
	invalid := api.call(owner.Id, common.RoleCommonUser, "GET", "/api/invite/commission/ledgers?statement_id=NaN", nil)
	require.False(t, invalid.Success)
}
