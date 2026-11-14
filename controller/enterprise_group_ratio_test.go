package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseGroupRatioDisplayForAutoAndMultipleRoutes(t *testing.T) {
	ent, member := setupEnterprisePayerContract(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"","auto":""}`))
	for _, token := range []*model.Token{
		{UserId: 1, EnterpriseId: ent.Id, MembershipId: member.Id, Group: "default"},
		{UserId: 1, EnterpriseId: ent.Id, MembershipId: member.Id, Group: "auto"},
		{UserId: 1, EnterpriseId: ent.Id, MembershipId: member.Id, Group: "default", RoutingConfig: `{"groups":["default"]}`},
	} {
		item := buildTokenListItems(1, []*model.Token{token})[0]
		require.Equal(t, "fixed", item.GroupRatioStatus)
		require.NotNil(t, item.GroupRatio)
		require.Equal(t, 0.25, *item.GroupRatio)
	}
	c, w := enterprisePayerRequest(1, http.MethodGet, "/api/user/self/groups?payer=enterprise", "")
	GetUserGroups(c)
	var response struct {
		Success bool
		Data    map[string]struct{ Ratio float64 }
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, 0.25, response.Data["auto"].Ratio)
	require.Equal(t, 0.25, response.Data["default"].Ratio)
	require.NotContains(t, response.Data, "private", "the pricing group must not grant channel access")
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"private":2}`))
	token := &model.Token{UserId: 1, EnterpriseId: ent.Id, MembershipId: member.Id, Group: "auto"}
	item := buildTokenListItems(1, []*model.Token{token})[0]
	require.Equal(t, "unavailable", item.GroupRatioStatus)
	require.Nil(t, item.GroupRatio)
	c, w = enterprisePayerRequest(1, http.MethodGet, "/api/user/self/groups?payer=enterprise", "")
	GetUserGroups(c)
	require.Contains(t, w.Body.String(), `"success":false`)
	require.NotContains(t, w.Body.String(), `"ratio":1`)
	personal := buildTokenListItems(1, []*model.Token{{UserId: 1, Group: "default"}})[0]
	require.Equal(t, 0.8, *personal.GroupRatio)
}

func TestGroupOptionsPreserveLegacyAndExposeConfiguredRatios(t *testing.T) {
	old := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(old)) })
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"preferred":0.8,"free":0,"auto":9,"invalid":-1}`))
	c, w := enterprisePayerRequest(1, http.MethodGet, "/api/group/", "")
	GetGroups(c)
	var legacy struct{ Data []string }
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &legacy))
	require.Equal(t, []string{"auto", "free", "invalid", "preferred"}, legacy.Data)
	c, w = enterprisePayerRequest(1, http.MethodGet, "/api/group/?with_ratio=true", "")
	GetGroups(c)
	var detailed struct {
		Data []struct {
			Name  string
			Ratio float64
		}
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &detailed))
	require.Len(t, detailed.Data, 2)
	require.Equal(t, "free", detailed.Data[0].Name)
	require.Zero(t, detailed.Data[0].Ratio)
	require.Equal(t, "preferred", detailed.Data[1].Name)
	require.Equal(t, 0.8, detailed.Data[1].Ratio)
}

func TestEnterpriseInvalidPricePrecedesChannelAdmission(t *testing.T) {
	original := *operation_setting.GetChannelConcurrencySetting()
	t.Cleanup(func() { *operation_setting.GetChannelConcurrencySetting() = original })
	t.Setenv("CHANNEL_CONCURRENCY_ENABLED", "")
	for _, enabled := range []bool{true, false} {
		operation_setting.GetChannelConcurrencySetting().Enabled = enabled
		c, _ := enterprisePayerRequest(1, http.MethodPost, "/v1/videos", "")
		info := &relaycommon.RelayInfo{EnterpriseId: 1, EnterpriseBillingGroup: ""}
		// Nil retry state is intentional: no channel lookup or capacity accounting
		// may be reached when the independent enterprise price is invalid.
		ch, release, err, overload := selectChannelWithConcurrency(c, info, nil)
		require.Nil(t, ch)
		require.Nil(t, release)
		require.False(t, overload)
		require.NotNil(t, err)
		require.Equal(t, types.ErrorCodeEnterpriseBillingGroupUnavailable, err.GetErrorCode())
		require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
		taskErr := service.TaskErrorFromAPIError(err)
		require.True(t, taskErr.LocalError)
		require.False(t, shouldRetryTaskRelay(c, 1, taskErr, 3))
		// Recreate the context with its own writer for the public error assertion.
		c, w := enterprisePayerRequest(1, http.MethodPost, "/v1/videos", "")
		respondTaskError(c, taskErr)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		require.Contains(t, w.Body.String(), "enterprise_billing_group_unavailable")
		require.Contains(t, w.Body.String(), "计费分组")
	}
}
