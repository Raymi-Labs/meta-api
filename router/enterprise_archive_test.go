package router

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEnterpriseArchiveHTTPFlow(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	calls := api.relay()
	root, admin, owner := api.user("archive-root"), api.user("archive-admin"), api.user("archive-owner")
	ent := api.org(root, "归档验收", owner)
	enterpriseToken := api.token(owner, "待归档令牌", model.TokenPayerEnterprise, nil)
	personalToken := api.token(owner, "个人令牌", model.TokenPayerPersonal, nil)
	base := "/api/enterprise/admin/enterprises/" + strconv.Itoa(ent)
	body := map[string]any{"name": "归档验收", "reason": "验收结束"}
	for _, actor := range []struct{ id, role int }{{admin.Id, common.RoleAdminUser}, {owner.Id, common.RoleCommonUser}, {0, 0}} {
		api.denied(actor.id, actor.role, http.MethodGet, base+"/archive-preview", nil, nil)
		api.denied(actor.id, actor.role, http.MethodPost, base+"/archive", body, nil)
	}
	api.denied(root.Id, common.RoleRootUser, http.MethodPost, base+"/archive", body, model.ErrEnterpriseArchiveBlocked)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, base+"/status", map[string]any{"status": "frozen"})
	preview := api.ok(root.Id, common.RoleRootUser, http.MethodGet, base+"/archive-preview", nil)
	require.Equal(t, true, preview.object()["can_archive"])
	// A positive unit arriving after preview must be rechecked by the commit path.
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, base+"/adjust", map[string]any{"delta": 1, "reason": "fixture"})
	api.denied(root.Id, common.RoleRootUser, http.MethodPost, base+"/archive", body, model.ErrEnterpriseArchiveBlocked)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, base+"/adjust", map[string]any{"delta": -1, "reason": "fixture"})
	api.denied(root.Id, common.RoleRootUser, http.MethodPost, base+"/archive", map[string]any{"name": "错误企业", "reason": "fixture"}, model.ErrEnterpriseArchiveName)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, base+"/archive", body)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, base+"/archive", body)
	denied := api.relayCall(enterpriseToken, http.MethodGet, "/v1/models", "")
	require.NotEqual(t, http.StatusOK, denied.Code, denied.Body.String())
	require.Equal(t, []string{"model-a", "model-b", "model-c"}, api.modelIDs(personalToken))
	require.Zero(t, calls.Load(), "archive checks never send an upstream inference")
	list := api.ok(root.Id, common.RoleRootUser, http.MethodGet, "/api/enterprise/admin/enterprises", nil)
	require.EqualValues(t, 0, list.object()["total"])
	list = api.ok(root.Id, common.RoleRootUser, http.MethodGet, "/api/enterprise/admin/enterprises?status=archived&page_size=1", nil)
	require.EqualValues(t, 1, list.object()["total"])
	api.denied(root.Id, common.RoleRootUser, http.MethodGet, "/api/enterprise/admin/enterprises?status=invalid", nil, model.ErrEnterpriseInvalidInput)
	for _, endpoint := range []string{"overview", "members?status=all", "usage?status=all", "ledger", "audit"} {
		api.ok(admin.Id, common.RoleAdminUser, http.MethodGet, base+"/"+endpoint, nil)
	}
	api.denied(owner.Id, common.RoleCommonUser, http.MethodGet, "/api/enterprises/"+strconv.Itoa(ent)+"/overview", nil, model.ErrEnterpriseNotFound)
	api.denied(root.Id, common.RoleRootUser, http.MethodPost, base+"/status", map[string]any{"status": "active"}, model.ErrEnterpriseArchived)
	api.denied(root.Id, common.RoleRootUser, http.MethodPost, base+"/adjust", map[string]any{"delta": 1, "reason": "fixture"}, model.ErrEnterpriseArchived)
	api.denied(root.Id, common.RoleRootUser, http.MethodPost, base+"/credit", map[string]any{"amount": 1, "voucher": "fixture"}, model.ErrEnterpriseArchived)
}

func TestEnterpriseArchiveWaitsForViolationFee(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	calls := api.relayTo(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Failed check: SAFETY_CHECK_TYPE_CSAM","type":"invalid_request_error","code":"content_policy_violation"}}`))
	})
	settings := model_setting.GetGrokSettings()
	oldEnabled, oldAmount := settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount
	settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount = true, 0.05
	t.Cleanup(func() { settings.ViolationDeductionEnabled, settings.ViolationDeductionAmount = oldEnabled, oldAmount })
	root, owner, staff := api.user("fee-root"), api.user("fee-owner"), api.user("fee-staff")
	ent := api.org(root, "费用收尾验收", owner)
	_, err := model.CreditEnterprise(root.Id, ent, 50000000, "fixture-fee", "")
	require.NoError(t, err)
	memberID := api.join(ent, owner, staff, model.EnterpriseRoleMember, 20000000)
	token := api.token(staff, "fee-enterprise", model.TokenPayerEnterprise, nil)
	var reservedDuringFee bool
	callback := "fee_requires_open_hold"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if charge, ok := tx.Statement.Dest.(*model.EnterpriseReservation); ok && charge.Kind == model.EnterpriseChargeViolationFee {
			var count int64
			if err := tx.Session(&gorm.Session{NewDB: true}).Model(&model.EnterpriseReservation{}).Where("request_id = ? AND status IN ?", strings.TrimPrefix(charge.RequestId, "vf:"), []string{model.EnterpriseReservationReserved, model.EnterpriseReservationReview}).Count(&count).Error; err != nil {
				tx.AddError(err)
				return
			}
			reservedDuringFee = count == 1
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Create().Remove(callback)) })
	w := api.relayCall(token, http.MethodPost, "/v1/chat/completions", `{"model":"model-a","messages":[{"role":"user","content":"synthetic"}]}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.True(t, reservedDuringFee, "archive must observe the original open hold until the fee is durable")
	require.EqualValues(t, 1, calls.Load())
	require.Eventually(t, func() bool {
		var member model.EnterpriseMember
		if err := model.DB.First(&member, memberID).Error; err != nil {
			return false
		}
		return member.Reserved == 0 && member.Consumed == int64(0.05*common.QuotaPerUnit)
	}, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, 0, api.personalQuota(staff))
	balance, err := model.GetEnterpriseBalance(ent)
	require.NoError(t, err)
	require.True(t, balance.Balanced)
}
