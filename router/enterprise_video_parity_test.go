package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseVideoUsesContractPriceAndOriginalPayer(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	calls := api.relayTo(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-fixture","object":"video","model":"sora-2","status":"queued","progress":0,"seconds":"4","size":"720x1280"}`))
	})
	prices, special := ratio_setting.ModelPrice2JSONString(), ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(prices))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(special))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"sora-2":0.001}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"enterprise-contract":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"enterprise-contract":{"default":0.5}}`))
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 1).Updates(map[string]any{"type": constant.ChannelTypeSora, "models": "sora-2"}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "sora-2", ChannelId: 1, Enabled: true}).Error)
	root, owner, staff := api.user("root"), api.user("owner"), api.user("staff")
	require.NoError(t, model.DB.Model(staff).Update("quota", 100000).Error)
	ent := api.org(root, "视频付款验收", owner)
	require.NoError(t, model.DB.Model(&model.Enterprise{}).Where("id = ?", ent).Update("billing_group", "enterprise-contract").Error)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit", map[string]any{"amount": 100000, "voucher": "VIDEO-FIXTURE"})
	member := api.join(ent, owner, staff, model.EnterpriseRoleMember, 50000)
	token := api.token(staff, "企业视频", model.TokenPayerEnterprise, nil)
	personal := api.token(staff, "个人", model.TokenPayerPersonal, nil)
	w := api.relayCall(token, http.MethodPost, "/v1/videos", `{"model":"sora-2","prompt":"fixture","seconds":"4","size":"720x1280"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.EqualValues(t, 1, calls.Load())
	var task model.Task
	require.NoError(t, model.DB.Where("user_id = ?", staff.Id).First(&task).Error)
	require.Equal(t, ent, task.EnterpriseId)
	require.Equal(t, member, task.EnterpriseMemberId)
	require.Equal(t, service.BillingSourceEnterprise, task.PrivateData.BillingSource)
	require.Equal(t, 0.5, task.PrivateData.BillingContext.GroupRatio)
	require.Equal(t, 1000, task.Quota)
	require.Equal(t, 100000, api.personalQuota(staff))
	require.NotEqual(t, http.StatusOK, api.relayCall(personal, http.MethodGet, "/v1/videos/"+task.TaskID, "").Code)
	require.Equal(t, http.StatusOK, api.relayCall(token, http.MethodGet, "/v1/videos/"+task.TaskID, "").Code)
	// A terminal failure is refunded to the persisted enterprise even after the
	// member is disabled, the feature switched off, and a polling process restarts.
	require.NoError(t, model.DisableEnterpriseMember(model.EnterpriseActor{UserId: owner.Id}, ent, member, "fixture"))
	common.EnterpriseEnabled = false
	require.NoError(t, service.ApplyVideoTaskResult(context.Background(), nil, &task, &relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: "fixture failed"}, nil))
	require.NoError(t, service.ApplyVideoTaskResult(context.Background(), nil, &task, &relaycommon.TaskInfo{Status: model.TaskStatusFailure}, nil))
	balance, err := model.GetEnterpriseBalance(ent)
	require.NoError(t, err)
	require.True(t, balance.Balanced)
	require.Zero(t, balance.Consumed)
	require.Equal(t, 100000, api.personalQuota(staff))
}

func TestEnterprisePlaygroundPayerOverActualRelay(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	api.relayTo(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fixture","object":"chat.completion","model":"model-a","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":2000,"total_tokens":3000}}`))
	})
	require.NoError(t, model.DB.AutoMigrate(&model.PulseWalletReservation{}, &model.PulseFundingLedger{}))
	root, owner, staff := api.user("root"), api.user("owner"), api.user("staff")
	require.NoError(t, model.DB.Model(staff).Update("quota", 100000).Error)
	ent := api.org(root, "体验付款验收", owner)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit", map[string]any{"amount": 100000, "voucher": "PG-FIXTURE"})
	api.join(ent, owner, staff, model.EnterpriseRoleMember, 50000)
	for _, payer := range []string{"enterprise", "personal"} {
		before := api.personalQuota(staff)
		request := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(`{"model":"model-a","group":"default","payer":"`+payer+`","messages":[{"role":"user","content":"hi"}]}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-User", strconv.Itoa(staff.Id))
		request.Header.Set("X-Test-Role", strconv.Itoa(common.RoleCommonUser))
		request.Header.Set("New-Api-User", strconv.Itoa(staff.Id))
		response := httptest.NewRecorder()
		api.engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		if payer == "enterprise" {
			require.Equal(t, before, api.personalQuota(staff))
		} else {
			require.Less(t, api.personalQuota(staff), before)
		}
	}
	var holds []model.EnterpriseReservation
	require.NoError(t, model.DB.Where("enterprise_id = ? AND kind = ?", ent, model.EnterpriseChargeRequest).Find(&holds).Error)
	require.Len(t, holds, 1)
	require.Equal(t, model.EnterpriseReservationSettled, holds[0].Status)
	require.Positive(t, holds[0].Quota)
}
