package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserGroupControllerTest(t *testing.T, userGroups string, channelGroups string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	originDB := model.DB
	originLogDB := model.LOG_DB
	originRedisEnabled := common.RedisEnabled
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originUserGroups := setting.UserGroups2JSONString()
	originGroupRatio := ratio_setting.GroupRatio2JSONString()

	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, setting.UpdateUserGroupsByJSONString(userGroups))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(channelGroups))

	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.RedisEnabled = originRedisEnabled
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		require.NoError(t, setting.UpdateUserGroupsByJSONString(originUserGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originGroupRatio))
	})

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.SubscriptionPlan{}, &model.Log{}))
	return db
}

func newUserGroupTestContext(t *testing.T, method string, target string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	payload := ""
	if body != nil {
		data, err := common.Marshal(body)
		require.NoError(t, err)
		payload = string(data)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(method, target, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("id", 1)
	ctx.Set("role", common.RoleRootUser)
	ctx.Set("username", "root")
	return ctx, recorder
}

func seedUserGroupTestUser(t *testing.T, db *gorm.DB, id int, group string) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id:       id,
		Username: fmt.Sprintf("user_%d", id),
		Password: "password123",
		Role:     common.RoleCommonUser,
		Group:    group,
		Status:   common.UserStatusEnabled,
		AffCode:  fmt.Sprintf("aff_%d", id),
	}).Error)
}

func decodeUserGroupResponse(t *testing.T, recorder *httptest.ResponseRecorder, data any) (bool, string) {
	t.Helper()
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	}
	if data != nil {
		resp.Data = data
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	return resp.Success, resp.Message
}

func TestValidateUserGroupAssignmentKeepsLegacyValues(t *testing.T) {
	setupUserGroupControllerTest(t, `{"default":"","vip":""}`, `{"福利":0.6}`)

	require.NoError(t, validateUserGroupAssignment("default", "vip"))
	// An unregistered legacy value may stay, but cannot be newly assigned.
	require.NoError(t, validateUserGroupAssignment("OpenAI · 蒸馏", "OpenAI · 蒸馏"))
	require.EqualError(t, validateUserGroupAssignment("default", "福利"),
		"用户分组「福利」尚未登记，请先在「分组相关设置 → 用户分组」中添加")
	require.Error(t, validateUserGroupAssignment("default", ""))
}

func TestValidatePlanUpgradeGroupUsesUserGroups(t *testing.T) {
	setupUserGroupControllerTest(t, `{"default":"","vip":""}`, `{"福利":0.6}`)

	require.NoError(t, validatePlanUpgradeGroup("", ""))
	require.NoError(t, validatePlanUpgradeGroup("", "vip"))
	require.NoError(t, validatePlanUpgradeGroup("分销商", "分销商"))
	require.Error(t, validatePlanUpgradeGroup("", "福利"))
	require.Error(t, validatePlanUpgradeGroup("vip", "svip"))
}

func TestUpdateUserRejectsUnregisteredGroup(t *testing.T) {
	db := setupUserGroupControllerTest(t, `{"default":"","vip":""}`, `{"福利":0.6}`)
	seedUserGroupTestUser(t, db, 2, "default")

	ctx, recorder := newUserGroupTestContext(t, http.MethodPut, "/api/user/", gin.H{
		"id": 2, "username": "user_2", "display_name": "user_2", "role": common.RoleCommonUser, "group": "福利",
	})
	UpdateUser(ctx)

	success, message := decodeUserGroupResponse(t, recorder, nil)
	require.False(t, success)
	require.Contains(t, message, "尚未登记")
	var user model.User
	require.NoError(t, db.First(&user, 2).Error)
	require.Equal(t, "default", user.Group)
}

func TestUpdateOptionRejectsNameUsedByBothGroupLists(t *testing.T) {
	setupUserGroupControllerTest(t, `{"default":"","vip":""}`, `{"default":1,"福利":0.6}`)

	ctx, recorder := newUserGroupTestContext(t, http.MethodPut, "/api/option/", gin.H{
		"key": "UserGroups", "value": `{"default":"","福利":"同名"}`,
	})
	UpdateOption(ctx)
	success, message := decodeUserGroupResponse(t, recorder, nil)
	require.False(t, success)
	require.Equal(t, "用户分组与渠道分组不能重名：福利", message)

	ctx, recorder = newUserGroupTestContext(t, http.MethodPut, "/api/option/", gin.H{
		"key": "GroupRatio", "value": `{"default":1,"vip":1}`,
	})
	UpdateOption(ctx)
	success, message = decodeUserGroupResponse(t, recorder, nil)
	require.False(t, success)
	require.Equal(t, "用户分组与渠道分组不能重名：vip", message)

	ctx, recorder = newUserGroupTestContext(t, http.MethodPut, "/api/option/", gin.H{
		"key": "UserGroups", "value": `{"vip":""}`,
	})
	UpdateOption(ctx)
	success, message = decodeUserGroupResponse(t, recorder, nil)
	require.False(t, success)
	require.Contains(t, message, "default")
}

func TestGetUserGroupOverviewListsUnregisteredGroupsInUse(t *testing.T) {
	db := setupUserGroupControllerTest(t, `{"default":"默认","vip":"VIP"}`, `{"福利":0.6,"Kimi · 福利":0.35}`)
	seedUserGroupTestUser(t, db, 1, "default")
	seedUserGroupTestUser(t, db, 2, "default")
	seedUserGroupTestUser(t, db, 3, "Kimi · 福利")
	require.NoError(t, db.Create(&model.SubscriptionPlan{Id: 1, Title: "svip", UpgradeGroup: "svip"}).Error)

	ctx, recorder := newUserGroupTestContext(t, http.MethodGet, "/api/group/user_groups", nil)
	GetUserGroupOverview(ctx)

	var items []userGroupOverviewItem
	success, _ := decodeUserGroupResponse(t, recorder, &items)
	require.True(t, success)
	byName := map[string]userGroupOverviewItem{}
	for _, item := range items {
		byName[item.Name] = item
	}
	require.Equal(t, userGroupOverviewItem{Name: "default", Desc: "默认", Registered: true, UserCount: 2}, byName["default"])
	require.True(t, byName["vip"].Registered)
	require.Zero(t, byName["vip"].UserCount)
	require.Equal(t, userGroupOverviewItem{Name: "svip", PlanCount: 1}, byName["svip"])
	require.Equal(t, userGroupOverviewItem{Name: "Kimi · 福利", ChannelGroup: true, UserCount: 1}, byName["Kimi · 福利"])
	// Registered groups come first.
	require.True(t, items[0].Registered)
	require.False(t, items[len(items)-1].Registered)
}

func TestGetStaleTokenGroupsListsMissingChannelGroups(t *testing.T) {
	db := setupUserGroupControllerTest(t, `{"default":""}`, `{"福利":0.6}`)
	for i, group := range []string{"福利", "svip", "svip", "土豪组", "auto", ""} {
		require.NoError(t, db.Create(&model.Token{Id: i + 1, Key: fmt.Sprintf("k%d", i), Group: group, Status: common.TokenStatusEnabled}).Error)
	}

	ctx, recorder := newUserGroupTestContext(t, http.MethodGet, "/api/group/stale_token_groups", nil)
	GetStaleTokenGroups(ctx)

	var items []staleTokenGroupItem
	success, _ := decodeUserGroupResponse(t, recorder, &items)
	require.True(t, success)
	require.Equal(t, []staleTokenGroupItem{{Name: "svip", TokenCount: 2}, {Name: "土豪组", TokenCount: 1}}, items)
}

func TestReassignUnregisteredUserGroup(t *testing.T) {
	db := setupUserGroupControllerTest(t, `{"default":"","vip":""}`, `{"福利":0.6}`)
	seedUserGroupTestUser(t, db, 2, "OpenAI · 蒸馏")
	seedUserGroupTestUser(t, db, 3, "OpenAI · 蒸馏")
	seedUserGroupTestUser(t, db, 4, "vip")

	for _, body := range []gin.H{
		{"from": "vip", "to": "default"},             // registered groups are edited per user
		{"from": "OpenAI · 蒸馏", "to": "svip"},        // target must be registered
		{"from": "OpenAI · 蒸馏", "to": "OpenAI · 蒸馏"}, // no-op
	} {
		ctx, recorder := newUserGroupTestContext(t, http.MethodPost, "/api/group/user_groups/reassign", body)
		ReassignUnregisteredUserGroup(ctx)
		success, _ := decodeUserGroupResponse(t, recorder, nil)
		require.False(t, success, body)
	}

	ctx, recorder := newUserGroupTestContext(t, http.MethodPost, "/api/group/user_groups/reassign", gin.H{"from": "OpenAI · 蒸馏", "to": "default"})
	ReassignUnregisteredUserGroup(ctx)
	var data struct {
		Updated int `json:"updated"`
	}
	success, _ := decodeUserGroupResponse(t, recorder, &data)
	require.True(t, success)
	require.Equal(t, 2, data.Updated)

	counts, err := model.CountUsersByGroup()
	require.NoError(t, err)
	require.Equal(t, map[string]int64{"default": 2, "vip": 1}, counts)

	var log model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).First(&log).Error)
	require.Equal(t, "管理员将 2 个用户的用户分组从「OpenAI · 蒸馏」改为「default」", log.Content)
}

func TestUpdateUserChannelGroupsOption(t *testing.T) {
	db := setupUserGroupControllerTest(t, `{"default":"","vip":""}`, `{"default":1,"premium":2}`)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Enterprise{}))
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = oldOptions })
	raw, err := common.Marshal(model.UserChannelGroupOptions{UserGroups: `{"default":"","premium":""}`, GroupRatio: `{"default":1,"vip":2}`})
	require.NoError(t, err)
	ctx, recorder := newUserGroupTestContext(t, http.MethodPut, "/api/option/", gin.H{"key": model.UserChannelGroupsOptionKey, "value": string(raw)})
	UpdateOption(ctx)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.True(t, setting.IsRegisteredUserGroup("premium"))
	require.True(t, ratio_setting.ContainsGroupRatio("vip"))
}
