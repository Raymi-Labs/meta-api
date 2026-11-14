package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupEnterprisePayerContract(t *testing.T) (*model.Enterprise, *model.EnterpriseMember) {
	t.Helper()
	setupImagePlaygroundGroupTestDB(t)
	oldEnabled := common.EnterpriseEnabled
	oldRatios := ratio_setting.GroupGroupRatio2JSONString()
	oldPrices := ratio_setting.ModelRatio2JSONString()
	oldSpecial := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	common.EnterpriseEnabled = true
	t.Cleanup(func() {
		common.EnterpriseEnabled = oldEnabled
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldPrices))
		ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(oldSpecial)
		model.InvalidateModelPermissionCache()
	})
	require.NoError(t, model.DB.AutoMigrate(&model.Enterprise{}, &model.EnterpriseMember{}, &model.EnterpriseTokenBudget{}, &model.ModelPermission{}))
	model.InvalidateModelPermissionCache()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":""}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"private":2,"enterprise-price":0.25}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"default":0.8},"enterprise-price":{"default":9,"private":9}}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-image-2":1,"gpt-6-sol":1,"private-model":1}`))
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(map[string]map[string]string{
		"enterprise-price": {"-:default": "", "+:private": ""},
	})
	for id := 1; id <= 2; id++ {
		require.NoError(t, model.DB.Create(&model.User{Id: id, Username: string(rune('a' + id)), AffCode: string(rune('a' + id)),
			Group: "default", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
	}
	ent := &model.Enterprise{Id: 11, Name: "synthetic-enterprise", BillingGroup: "enterprise-price", Status: model.EnterpriseStatusActive, ModelMode: model.EnterpriseModelModeInherit}
	require.NoError(t, model.DB.Create(ent).Error)
	userID := 1
	member := &model.EnterpriseMember{Id: 21, EnterpriseId: ent.Id, UserId: userID, ActiveUserId: &userID,
		Status: model.EnterpriseMemberStatusActive, Role: model.EnterpriseRoleMember, Available: 100, ModelMode: model.EnterpriseModelModeInherit}
	require.NoError(t, model.DB.Create(member).Error)
	for i, name := range []string{imagePlaygroundDefaultModel, imagePlaygroundAgentModel, "private-model"} {
		group := "default"
		if i == 2 {
			group = "private"
		}
		require.NoError(t, model.DB.Create(&model.Channel{Id: i + 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled,
			Name: "synthetic", Key: "synthetic-only", Group: group, Models: name}).Error)
		require.NoError(t, model.DB.Create(&model.Ability{Group: group, Model: name, ChannelId: i + 1, Enabled: true}).Error)
	}
	return ent, member
}

func TestEnterprisePayerAuthorizationPriceCatalog(t *testing.T) {
	ent, member := setupEnterprisePayerContract(t)
	token := &model.Token{Id: 101, UserId: 1, Group: "default", EnterpriseId: ent.Id, MembershipId: member.Id}
	group, err := tokenBaseGroup(1, token)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	billing, err := tokenBillingGroup(1, token)
	require.NoError(t, err)
	require.Equal(t, "enterprise-price", billing)
	_, err = tokenBaseGroup(2, token)
	require.Error(t, err)
	groups, err := tokenCatalogGroups(1, token)
	require.NoError(t, err)
	require.Equal(t, []string{"default"}, groups)
	models, err := resolveTokenAllowedModels(1, token)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{imagePlaygroundDefaultModel, imagePlaygroundAgentModel}, models)
	require.ElementsMatch(t, models, service.EnterpriseMemberModels(ent, member))
	item := buildTokenListItems(1, []*model.Token{token})[0]
	require.Equal(t, "fixed", item.GroupRatioStatus)
	require.Equal(t, 0.25, *item.GroupRatio)
	token.Group = "private"
	require.Error(t, validateTokenGroupSelection(1, token))
	require.Equal(t, "unavailable", buildTokenListItems(1, []*model.Token{token})[0].GroupRatioStatus)

	for _, tc := range []struct {
		payer string
		ratio float64
	}{{"personal", 0.8}, {"enterprise", 0.25}} {
		c, w := enterprisePayerRequest(1, http.MethodGet, "/api/user/self/groups?payer="+tc.payer, "")
		GetUserGroups(c)
		var response struct {
			Success bool `json:"success"`
			Data    map[string]struct {
				Ratio float64 `json:"ratio"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success, w.Body.String())
		require.Len(t, response.Data, 1)
		require.Equal(t, tc.ratio, response.Data["default"].Ratio)
	}
	c, w := enterprisePayerRequest(1, http.MethodGet, "/api/user/self/groups?payer=company", "")
	GetUserGroups(c)
	require.Contains(t, w.Body.String(), `"success":false`)
}

func enterprisePayerRequest(userID int, method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", userID)
	return c, w
}

func TestImagePlaygroundPayerSessionIsolation(t *testing.T) {
	ent, member := setupEnterprisePayerContract(t)
	t.Setenv("IMAGE_PLAYGROUND_API_ORIGIN", "")
	create := func(userID int, body, payer string) {
		t.Helper()
		c, w := enterprisePayerRequest(userID, http.MethodPost, "/api/image-playground/session", body)
		CreateImagePlaygroundSession(c)
		var response struct {
			Success bool                           `json:"success"`
			Data    imagePlaygroundSessionResponse `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success, w.Body.String())
		require.Equal(t, payer, response.Data.Payer)
	}
	create(1, "", "personal")
	personal, err := model.GetReusableImagePlaygroundToken(1, 0)
	require.NoError(t, err)
	require.Zero(t, personal.EnterpriseId)
	create(1, `{"payer":"enterprise","enterprise_id":999,"membership_id":999}`, "enterprise")
	enterprise, err := model.GetReusableImagePlaygroundTokenForPayer(1, 0, ent.Id, member.Id)
	require.NoError(t, err)
	require.NotEqual(t, personal.Id, enterprise.Id)
	require.Equal(t, "default", enterprise.Group)
	require.NoError(t, model.DB.Model(enterprise).Update("expired_time", enterprise.ExpiredTime+100).Error)
	create(1, `{"payer":"personal"}`, "personal")
	create(1, `{"payer":"enterprise"}`, "enterprise")
	var count int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&count).Error)
	require.EqualValues(t, 2, count)

	for _, body := range []string{`{"payer":"company"}`, `{"payer":"enterprise"}`} {
		c, w := enterprisePayerRequest(2, http.MethodPost, "/api/image-playground/session", body)
		CreateImagePlaygroundSession(c)
		require.Contains(t, w.Body.String(), `"success":false`)
	}
	_, err = model.GetReusableImagePlaygroundTokenForPayer(2, 0, ent.Id, member.Id)
	require.Error(t, err)

	// Rejoining creates a new membership identity, so the old enterprise token cannot be reused.
	require.NoError(t, model.DB.Model(member).Updates(map[string]any{"status": model.EnterpriseMemberStatusLeft, "active_user_id": nil}).Error)
	userID := 1
	next := &model.EnterpriseMember{Id: 22, EnterpriseId: ent.Id, UserId: 1, ActiveUserId: &userID, Status: model.EnterpriseMemberStatusActive,
		Role: model.EnterpriseRoleMember, ModelMode: model.EnterpriseModelModeInherit, Generation: 2}
	require.NoError(t, model.DB.Create(next).Error)
	create(1, `{"payer":"enterprise"}`, "enterprise")
	fresh, err := model.GetReusableImagePlaygroundTokenForPayer(1, 0, ent.Id, next.Id)
	require.NoError(t, err)
	require.NotEqual(t, enterprise.Id, fresh.Id)
	_, _, err = model.ValidateEnterpriseToken(enterprise)
	require.ErrorIs(t, err, model.ErrEnterpriseMemberInactive)
}

func TestPlaygroundRequiresPayerMiddleware(t *testing.T) {
	c, w := enterprisePayerRequest(1, http.MethodPost, "/pg/chat/completions", `{"payer":"enterprise"}`)
	Playground(c)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestImagePlaygroundPayerPolicyAndFreeze(t *testing.T) {
	ent, member := setupEnterprisePayerContract(t)
	// Image access without planner access remains usable, with ecommerce disabled.
	require.NoError(t, model.DB.Model(ent).Updates(map[string]any{"model_mode": model.EnterpriseModelModeCustom, "model_list": `["gpt-image-2"]`}).Error)
	c, _ := enterprisePayerRequest(1, http.MethodPost, "/api/image-playground/session", `{"payer":"enterprise"}`)
	scope, err := middleware.ResolvePlaygroundPayer(c)
	require.NoError(t, err)
	common.CleanupBodyStorage(c)
	group, planning, err := resolveImagePlaygroundPayerGroup(1, scope)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.False(t, planning)
	_, _, err = resolveImagePlaygroundPayerGroup(2, scope)
	require.ErrorIs(t, err, model.ErrEnterpriseNotFound)

	for _, change := range []func(){
		func() {
			require.NoError(t, model.DB.Model(member).Updates(map[string]any{"model_mode": model.EnterpriseModelModeCustom, "model_list": `[]`}).Error)
		},
		func() { require.NoError(t, model.DB.Model(ent).Update("status", model.EnterpriseStatusFrozen).Error) },
	} {
		change()
		c, w := enterprisePayerRequest(1, http.MethodPost, "/api/image-playground/session", `{"payer":"enterprise"}`)
		CreateImagePlaygroundSession(c)
		require.Contains(t, w.Body.String(), `"success":false`)
		var count int64
		require.NoError(t, model.DB.Model(&model.Token{}).Count(&count).Error)
		require.Zero(t, count)
	}
	// A frozen enterprise never silently substitutes a personal token for an enterprise request.
	// Personal requests still work when the enterprise itself is frozen.
	c, w := enterprisePayerRequest(1, http.MethodPost, "/api/image-playground/session", `{}`)
	CreateImagePlaygroundSession(c)
	require.Contains(t, w.Body.String(), `"payer":"personal"`)
}
