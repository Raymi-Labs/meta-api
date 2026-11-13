package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPlaygroundPayerTest(t *testing.T) (*model.Enterprise, *model.EnterpriseMember) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldRedis, oldEnabled := model.DB, common.RedisEnabled, common.EnterpriseEnabled
	oldUsable := setting.UserUsableGroups2JSONString()
	oldRatio := ratio_setting.GroupRatio2JSONString()
	oldSpecial := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	model.DB, common.RedisEnabled, common.EnterpriseEnabled = db, false, true
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.EnterpriseEnabled = oldDB, oldRedis, oldEnabled
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatio))
		ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(oldSpecial)
		model.InvalidateModelPermissionCache()
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Enterprise{}, &model.EnterpriseMember{}, &model.ModelPermission{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":""}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"private":2}`))
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(map[string]map[string]string{
		"enterprise-price": {"-:default": "", "+:private": ""},
	})
	for id := 1; id <= 2; id++ {
		require.NoError(t, db.Create(&model.User{Id: id, Username: string(rune('a' + id)), AffCode: string(rune('a' + id)),
			Group: "default", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 900}).Error)
	}
	ent := &model.Enterprise{Id: 11, Name: "synthetic-enterprise", BillingGroup: "enterprise-price", Status: model.EnterpriseStatusActive,
		ModelMode: model.EnterpriseModelModeInherit}
	require.NoError(t, db.Create(ent).Error)
	userID := 1
	member := &model.EnterpriseMember{Id: 21, EnterpriseId: ent.Id, UserId: userID, ActiveUserId: &userID,
		Status: model.EnterpriseMemberStatusActive, Role: model.EnterpriseRoleMember, Available: 123, ModelMode: model.EnterpriseModelModeInherit}
	require.NoError(t, db.Create(member).Error)
	model.InvalidateModelPermissionCache()
	return ent, member
}

func TestPlaygroundPayerIdentityAndRequestPreserved(t *testing.T) {
	ent, member := setupPlaygroundPayerTest(t)
	for _, payer := range []string{"", "personal", "enterprise"} {
		t.Run(payer, func(t *testing.T) {
			body := `{"model":"synthetic","payer":"` + payer + `","messages":[{"role":"user","content":"hello"}]}`
			router := gin.New()
			router.POST("/pg/chat/completions", func(c *gin.Context) { c.Set("id", 1) }, PlaygroundPayer(), func(c *gin.Context) {
				require.True(t, common.GetContextKeyBool(c, constant.ContextKeyPlaygroundPayerPrepared))
				info := relaycommon.GenRelayInfoOpenAI(c, nil)
				require.Equal(t, "default", info.UserGroup)
				require.Equal(t, "default", info.UsingGroup)
				if payer == "enterprise" {
					require.Equal(t, ent.Id, info.EnterpriseId)
					require.Equal(t, member.Id, info.EnterpriseMemberId)
					require.Equal(t, "enterprise-price", info.BillingUserGroup())
					require.Equal(t, int(member.Available), info.UserQuota)
				} else {
					require.Zero(t, info.EnterpriseId)
					require.Empty(t, info.EnterpriseBillingGroup)
					require.Equal(t, "default", info.BillingUserGroup())
					require.Equal(t, 900, info.UserQuota)
				}
				read, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				require.Equal(t, body, string(read))
				c.Status(http.StatusNoContent)
			})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(body)))
			require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
		})
	}
}

func TestPlaygroundPayerRejectsBeforeSelection(t *testing.T) {
	ent, member := setupPlaygroundPayerTest(t)
	for _, tc := range []struct {
		name, body string
		userID     int
		change     func()
		status     int
	}{
		{"invalid payer", `{"payer":"company"}`, 1, nil, 400},
		{"malformed", `{"payer":`, 1, nil, 400},
		{"foreign membership", `{"payer":"enterprise","enterprise_id":11,"membership_id":21}`, 2, nil, 403},
		{"price must not grant channels", `{"payer":"enterprise","group":"private"}`, 1, nil, 403},
		{"logged out", `{"payer":"enterprise"}`, 0, nil, 401},
		{"frozen", `{"payer":"enterprise"}`, 1, func() {
			require.NoError(t, model.DB.Model(ent).Update("status", model.EnterpriseStatusFrozen).Error)
		}, 403},
		{"disabled member", `{"payer":"enterprise"}`, 1, func() {
			require.NoError(t, model.DB.Model(ent).Update("status", model.EnterpriseStatusActive).Error)
			require.NoError(t, model.DB.Model(member).Update("status", model.EnterpriseMemberStatusDisabled).Error)
		}, 403},
		{"feature disabled", `{"payer":"enterprise"}`, 1, func() { common.EnterpriseEnabled = false }, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.change != nil {
				tc.change()
			}
			selected := false
			router := gin.New()
			router.POST("/pg/chat/completions", func(c *gin.Context) { c.Set("id", tc.userID) }, PlaygroundPayer(), func(c *gin.Context) {
				selected = true
				c.Status(http.StatusNoContent)
			})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(tc.body)))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.False(t, selected)
		})
	}
}

func TestPlaygroundPayerModelPolicyBeforeDistribute(t *testing.T) {
	ent, _ := setupPlaygroundPayerTest(t)
	require.NoError(t, model.DB.Model(ent).Updates(map[string]any{"model_mode": model.EnterpriseModelModeCustom, "model_list": `[]`}).Error)
	router := gin.New()
	reached := false
	router.POST("/pg/chat/completions", func(c *gin.Context) { c.Set("id", 1) }, PlaygroundPayer(), Distribute(), func(c *gin.Context) { reached = true })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(`{"payer":"enterprise","model":"blocked"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.False(t, reached)
}

func TestEnterpriseTokenAuthKeepsAuthorizationGroup(t *testing.T) {
	ent, member := setupPlaygroundPayerTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	for i, group := range []string{"", "default", "private"} {
		t.Run(group, func(t *testing.T) {
			token := &model.Token{UserId: 1, Key: strings.Repeat(string(rune('a'+i)), 48), Status: common.TokenStatusEnabled,
				ExpiredTime: -1, UnlimitedQuota: true, Group: group, EnterpriseId: ent.Id, MembershipId: member.Id}
			require.NoError(t, model.DB.Create(token).Error)
			reached := false
			router := gin.New()
			router.POST("/v1/chat/completions", TokenAuth(), func(c *gin.Context) {
				reached = true
				info := relaycommon.GenRelayInfoOpenAI(c, nil)
				require.Equal(t, "default", info.UserGroup)
				require.Equal(t, "default", info.UsingGroup)
				require.Equal(t, "default", info.TokenGroup)
				require.Equal(t, "enterprise-price", info.BillingUserGroup())
				c.Status(http.StatusNoContent)
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"synthetic"}`))
			req.Header.Set("Authorization", "Bearer sk-"+token.Key)
			router.ServeHTTP(w, req)
			if group == "private" {
				require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
				require.False(t, reached)
			} else {
				require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
				require.True(t, reached)
			}
		})
	}
}
