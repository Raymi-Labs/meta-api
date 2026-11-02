package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestBuildImagePlaygroundOriginPrefersRequestHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://nan.meta-api.vip"
	defer func() {
		system_setting.ServerAddress = originalServerAddress
	}()

	req := httptest.NewRequest(http.MethodPost, "http://internal/api/image-playground/session", nil)
	req.Header.Set("X-Forwarded-Host", "cn.meta-api.vip")
	req.Header.Set("X-Forwarded-Proto", "https")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if got := buildImagePlaygroundOrigin(c); got != "https://cn.meta-api.vip" {
		t.Fatalf("buildImagePlaygroundOrigin() = %q, want %q", got, "https://cn.meta-api.vip")
	}
}

func TestBuildImagePlaygroundOriginPrefersTrustedOriginHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://nan.meta-api.vip"
	defer func() {
		system_setting.ServerAddress = originalServerAddress
	}()

	req := httptest.NewRequest(http.MethodPost, "http://nan.meta-api.vip/api/image-playground/session", nil)
	req.Header.Set("Origin", "https://cn.meta-api.vip")
	req.Header.Set("X-Forwarded-Proto", "https")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if got := buildImagePlaygroundOrigin(c); got != "https://cn.meta-api.vip" {
		t.Fatalf("buildImagePlaygroundOrigin() = %q, want %q", got, "https://cn.meta-api.vip")
	}
}

func TestBuildImagePlaygroundOriginRejectsUntrustedOriginHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://nan.meta-api.vip"
	defer func() {
		system_setting.ServerAddress = originalServerAddress
	}()

	req := httptest.NewRequest(http.MethodPost, "http://nan.meta-api.vip/api/image-playground/session", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if got := buildImagePlaygroundOrigin(c); got != "https://nan.meta-api.vip" {
		t.Fatalf("buildImagePlaygroundOrigin() = %q, want %q", got, "https://nan.meta-api.vip")
	}
}

func TestBuildImagePlaygroundOriginFallsBackToServerAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://nan.meta-api.vip/"
	defer func() {
		system_setting.ServerAddress = originalServerAddress
	}()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = ""
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	if got := buildImagePlaygroundOrigin(c); got != "https://nan.meta-api.vip" {
		t.Fatalf("buildImagePlaygroundOrigin() = %q, want %q", got, "https://nan.meta-api.vip")
	}
}

func TestBuildImagePlaygroundLaunchURLForcesGalleryImagesMode(t *testing.T) {
	launchURL := buildImagePlaygroundLaunchURL("https://cn.meta-api.vip", "test-key", true)
	parsed, err := url.Parse(launchURL)
	if err != nil {
		t.Fatalf("parse launch url: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "cn.meta-api.vip" || parsed.Path != "/image-playground/" {
		t.Fatalf("unexpected launch url: %s", launchURL)
	}
	query := parsed.Query()
	if query.Get("appMode") != "gallery" {
		t.Fatalf("appMode = %q, want gallery", query.Get("appMode"))
	}
	var settings imagePlaygroundLaunchSettings
	if err := common.Unmarshal([]byte(query.Get("settings")), &settings); err != nil {
		t.Fatalf("unmarshal launch settings: %v", err)
	}
	if settings.ActiveProfileID != "newapi-image-playground" {
		t.Fatalf("activeProfileId = %q, want newapi-image-playground", settings.ActiveProfileID)
	}
	if settings.DefaultImageModel != imagePlaygroundDefaultModel {
		t.Fatalf("defaultImageModel = %q, want %s", settings.DefaultImageModel, imagePlaygroundDefaultModel)
	}
	if settings.DefaultPlanModel != imagePlaygroundAgentModel {
		t.Fatalf("defaultPlanModel = %q, want %s", settings.DefaultPlanModel, imagePlaygroundAgentModel)
	}
	if !settings.SupportsEcommerce {
		t.Fatal("supportsEcommerce = false, want true")
	}
	if len(settings.Profiles) != 1 {
		t.Fatalf("profiles length = %d, want 1", len(settings.Profiles))
	}
	profile := settings.Profiles[0]
	if profile.APIMode != "images" {
		t.Fatalf("apiMode = %q, want images", profile.APIMode)
	}
	if profile.Model != imagePlaygroundDefaultModel {
		t.Fatalf("model = %q, want %s", profile.Model, imagePlaygroundDefaultModel)
	}
	if profile.StreamImages {
		t.Fatal("streamImages = true, want false")
	}
	if !profile.ResponseFormatB64Json {
		t.Fatal("responseFormatB64Json = false, want true")
	}
	if profile.BaseURL != "https://cn.meta-api.vip/v1" {
		t.Fatalf("baseUrl = %q, want https://cn.meta-api.vip/v1", profile.BaseURL)
	}
	if profile.APIKey != "sk-test-key" {
		t.Fatalf("apiKey = %q, want sk-test-key", profile.APIKey)
	}
}

func setupImagePlaygroundGroupTestDB(t *testing.T) {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalUserUsableGroups := setting.UserUsableGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		_ = setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups)
		_ = setting.UpdateAutoGroupsByJsonString(originalAutoGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}, &model.Channel{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"VIP","team":"团队"}`); err != nil {
		t.Fatalf("update user usable groups: %v", err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`["default","vip"]`); err != nil {
		t.Fatalf("update auto groups: %v", err)
	}
	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"team":1}`); err != nil {
		t.Fatalf("update group ratio: %v", err)
	}
}

func TestBuildImagePlaygroundGroupCandidatesExcludesRemovedDefaultGroup(t *testing.T) {
	setupImagePlaygroundGroupTestDB(t)

	if err := setting.UpdateUserUsableGroupsByJSONString(`{"vip":"VIP","team":"团队"}`); err != nil {
		t.Fatalf("update user usable groups: %v", err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`["default","team","vip"]`); err != nil {
		t.Fatalf("update auto groups: %v", err)
	}
	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"vip":1,"team":1}`); err != nil {
		t.Fatalf("update group ratio: %v", err)
	}

	got := buildImagePlaygroundGroupCandidates("default")
	want := []string{"team", "vip"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildImagePlaygroundGroupCandidates() = %v, want %v", got, want)
	}
}

func TestResolveImagePlaygroundTokenGroupPrefersModelEnabledGroup(t *testing.T) {
	setupImagePlaygroundGroupTestDB(t)

	if err := model.DB.Create(&model.User{
		Id:       1,
		Username: "image-user",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		AffCode:  "aff-image-user",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := model.DB.Create(&model.Channel{
		Id:     10,
		Name:   "vip-image",
		Key:    "sk-image",
		Group:  "vip",
		Models: imagePlaygroundDefaultModel,
		Status: common.ChannelStatusEnabled,
	}).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if err := model.DB.Create(&model.Ability{
		Group:     "vip",
		Model:     imagePlaygroundDefaultModel,
		ChannelId: 10,
		Enabled:   true,
	}).Error; err != nil {
		t.Fatalf("seed ability: %v", err)
	}

	group, supportsEcommerce, err := resolveImagePlaygroundTokenGroup(1)
	if err != nil {
		t.Fatalf("resolve image playground group: %v", err)
	}
	if group != "vip" {
		t.Fatalf("resolveImagePlaygroundTokenGroup() = %q, want vip", group)
	}
	if supportsEcommerce {
		t.Fatal("supportsEcommerce = true, want false (only image model enabled)")
	}
}

func TestResolveImagePlaygroundTokenGroupKeepsImageGroupWithPlanningFallback(t *testing.T) {
	setupImagePlaygroundGroupTestDB(t)

	if err := model.DB.Create(&model.User{
		Id:       1,
		Username: "image-user",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		AffCode:  "aff-image-user",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	channels := []model.Channel{
		{Id: 10, Name: "default-image", Key: "sk-image", Group: "default", Models: imagePlaygroundDefaultModel, Status: common.ChannelStatusEnabled},
		{Id: 11, Name: "vip-image", Key: "sk-image", Group: "vip", Models: imagePlaygroundDefaultModel + "," + imagePlaygroundAgentModel, Status: common.ChannelStatusEnabled},
	}
	if err := model.DB.Create(&channels).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	abilities := []model.Ability{
		{Group: "default", Model: imagePlaygroundDefaultModel, ChannelId: 10, Enabled: true},
		{Group: "vip", Model: imagePlaygroundDefaultModel, ChannelId: 11, Enabled: true},
		{Group: "vip", Model: imagePlaygroundAgentModel, ChannelId: 11, Enabled: true},
	}
	if err := model.DB.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}

	group, supportsEcommerce, err := resolveImagePlaygroundTokenGroup(1)
	if err != nil {
		t.Fatalf("resolve image playground group: %v", err)
	}
	if group != "default" {
		t.Fatalf("resolveImagePlaygroundTokenGroup() = %q, want default", group)
	}
	if !supportsEcommerce {
		t.Fatal("supportsEcommerce = false, want true (vip is reachable through fallback)")
	}
}

func TestResolveImagePlaygroundTokenGroupReturnsErrorWithoutCapableGroup(t *testing.T) {
	setupImagePlaygroundGroupTestDB(t)

	if err := model.DB.Create(&model.User{
		Id:       1,
		Username: "image-user",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		AffCode:  "aff-image-user",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	group, supportsEcommerce, err := resolveImagePlaygroundTokenGroup(1)
	if err == nil {
		t.Fatal("resolveImagePlaygroundTokenGroup() error = nil, want no capable group error")
	}
	if group != "" {
		t.Fatalf("resolveImagePlaygroundTokenGroup() group = %q, want empty", group)
	}
	if supportsEcommerce {
		t.Fatal("supportsEcommerce = true, want false")
	}
	if !strings.Contains(err.Error(), imagePlaygroundDefaultModel) {
		t.Fatalf("error = %q, want model name %q", err, imagePlaygroundDefaultModel)
	}
}

func TestCreateImagePlaygroundTokenUsesCapableGroupWithoutModelLimits(t *testing.T) {
	setupImagePlaygroundGroupTestDB(t)

	if err := model.DB.Create(&model.User{
		Id:       1,
		Username: "image-user",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		AffCode:  "aff-image-user",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := model.DB.Create(&model.Channel{
		Id:     10,
		Name:   "vip-image",
		Key:    "sk-image",
		Group:  "vip",
		Models: imagePlaygroundDefaultModel,
		Status: common.ChannelStatusEnabled,
	}).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if err := model.DB.Create(&model.Ability{
		Group:     "vip",
		Model:     imagePlaygroundDefaultModel,
		ChannelId: 10,
		Enabled:   true,
	}).Error; err != nil {
		t.Fatalf("seed ability: %v", err)
	}

	token, supportsEcommerce, err := createImagePlaygroundToken(1, 100)
	if err != nil {
		t.Fatalf("create image playground token: %v", err)
	}
	if token.Group != "vip" {
		t.Fatalf("image playground token group = %q, want vip", token.Group)
	}
	if token.ModelLimitsEnabled {
		t.Fatal("image playground token should not enable model limits")
	}
	if token.ModelLimits != "" {
		t.Fatalf("image playground token model limits = %q, want empty", token.ModelLimits)
	}
	if supportsEcommerce {
		t.Fatal("supportsEcommerce = true, want false (only image model enabled)")
	}
}

func TestClearImagePlaygroundTokenModelLimits(t *testing.T) {
	token := &model.Token{
		ModelLimitsEnabled: true,
		ModelLimits:        "gpt-image-2",
	}

	if !clearImagePlaygroundTokenModelLimits(token) {
		t.Fatal("clearImagePlaygroundTokenModelLimits() should report change")
	}
	if token.ModelLimitsEnabled {
		t.Fatal("model limits should be disabled")
	}
	if token.ModelLimits != "" {
		t.Fatalf("model limits = %q, want empty", token.ModelLimits)
	}
	if clearImagePlaygroundTokenModelLimits(token) {
		t.Fatal("second clear should not report change")
	}
}

func TestRefreshImagePlaygroundTokenClearsLimitsAndUpdatesGroup(t *testing.T) {
	setupImagePlaygroundGroupTestDB(t)

	// 模拟 default 已从当前分组配置中移除，但历史用户和旧令牌仍保留 default。
	if err := setting.UpdateUserUsableGroupsByJSONString(`{"vip":"VIP","team":"团队"}`); err != nil {
		t.Fatalf("update user usable groups: %v", err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`["vip"]`); err != nil {
		t.Fatalf("update auto groups: %v", err)
	}
	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"vip":1,"team":1}`); err != nil {
		t.Fatalf("update group ratio: %v", err)
	}

	if err := model.DB.Create(&model.User{
		Id:       1,
		Username: "image-user",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		AffCode:  "aff-image-user",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := model.DB.Create(&model.Channel{
		Id:     10,
		Name:   "vip-image",
		Key:    "sk-image",
		Group:  "vip",
		Models: imagePlaygroundDefaultModel + "," + imagePlaygroundAgentModel,
		Status: common.ChannelStatusEnabled,
	}).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	abilities := []model.Ability{
		{Group: "vip", Model: imagePlaygroundDefaultModel, ChannelId: 10, Enabled: true},
		{Group: "vip", Model: imagePlaygroundAgentModel, ChannelId: 10, Enabled: true},
	}
	if err := model.DB.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}

	token := &model.Token{
		Group:              "default",
		ModelLimitsEnabled: true,
		ModelLimits:        "gpt-image-2",
	}
	changed, supportsEcommerce, err := refreshImagePlaygroundToken(token, 1)
	if err != nil {
		t.Fatalf("refresh image playground token: %v", err)
	}
	if !changed {
		t.Fatal("refreshImagePlaygroundToken() should report change")
	}
	if !supportsEcommerce {
		t.Fatal("supportsEcommerce = false, want true (vip supports both models)")
	}
	if token.Group != "vip" {
		t.Fatalf("token group = %q, want vip", token.Group)
	}
	if token.ModelLimitsEnabled || token.ModelLimits != "" {
		t.Fatalf("model limits not cleared: enabled=%v limits=%q", token.ModelLimitsEnabled, token.ModelLimits)
	}
}

func TestImagePlaygroundWelfareSessionRouting(t *testing.T) {
	setupImagePlaygroundGroupTestDB(t)
	if err := setting.UpdateUserUsableGroupsByJSONString(`{"福利":"福利","vip":"VIP","team":"团队"}`); err != nil {
		t.Fatal(err)
	}
	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"福利":0.6,"vip":1,"team":1}`); err != nil {
		t.Fatal(err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`["removed","vip","team"]`); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.User{Id: 1, Username: "image-user", Group: "vip", Status: common.UserStatusEnabled, AffCode: "image"}).Error; err != nil {
		t.Fatal(err)
	}
	for id, group := range []string{"福利", "vip", "team"} {
		if err := model.DB.Create(&model.Channel{Id: id + 1, Name: group, Key: "test", Group: group, Models: "gpt-image-2,gpt-6-sol", Status: common.ChannelStatusEnabled}).Error; err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"gpt-image-2", "gpt-6-sol"} {
			if err := model.DB.Create(&model.Ability{Group: group, Model: name, ChannelId: id + 1, Enabled: true}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	token, ecommerce, err := createImagePlaygroundToken(1, common.GetTimestamp())
	if err != nil {
		t.Fatal(err)
	}
	if token.Group != "福利" || !ecommerce || !token.AutoGroupFallback {
		t.Fatalf("unexpected session routing: group=%s ecommerce=%v fallback=%v", token.Group, ecommerce, token.AutoGroupFallback)
	}
	groups, err := service.TokenConfiguredGroups("vip", token, "gpt-image-2")
	if err != nil || !reflect.DeepEqual(groups, []string{"福利", "vip", "team"}) {
		t.Fatalf("fallback groups=%v err=%v", groups, err)
	}
	for _, name := range []string{"gpt-image-2", "gpt-6-sol"} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "vip")
		common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroupFallback, true)
		param := &service.RetryParam{Ctx: ctx, TokenGroup: token.Group, ModelName: name, ExcludeChannels: []int{1}}
		channel, actualGroup, routeErr := service.CacheGetRandomSatisfiedChannel(param)
		if routeErr != nil || channel == nil || actualGroup != "vip" {
			t.Fatalf("%s fallback: channel=%v group=%s err=%v", name, channel, actualGroup, routeErr)
		}
	}
	id, key, expires := token.Id, token.Key, token.ExpiredTime
	token.Group, token.AutoGroupFallback, token.CrossGroupRetry = "vip", false, true
	token.RoutingConfig = `{"groups":["team"]}`
	if err := token.Update(); err != nil {
		t.Fatal(err)
	}
	changed, ecommerce, err := refreshImagePlaygroundToken(token, 1)
	if err != nil || !changed || !ecommerce {
		t.Fatalf("refresh changed=%v ecommerce=%v err=%v", changed, ecommerce, err)
	}
	if err := token.Update(); err != nil {
		t.Fatal(err)
	}
	var saved model.Token
	if err := model.DB.First(&saved, id).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Group != "福利" || !saved.AutoGroupFallback || saved.CrossGroupRetry || saved.RoutingConfig != "" || saved.Key != key || saved.ExpiredTime != expires {
		t.Fatal("refresh must persist default fallback while preserving the session key and expiry")
	}
	changed, _, err = refreshImagePlaygroundToken(&saved, 1)
	if err != nil || changed {
		t.Fatalf("second refresh changed=%v err=%v", changed, err)
	}

	for _, tc := range []struct {
		name, usable, ratios string
		disable              bool
	}{
		{"not permitted", `{"vip":"VIP","team":"team"}`, `{"福利":0.6,"vip":1,"team":1}`, false},
		{"removed ratio", `{"福利":"福利","vip":"VIP","team":"team"}`, `{"vip":1,"team":1}`, false},
		{"disabled channel", `{"福利":"福利","vip":"VIP","team":"team"}`, `{"福利":0.6,"vip":1,"team":1}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := setting.UpdateUserUsableGroupsByJSONString(tc.usable); err != nil {
				t.Fatal(err)
			}
			if err := ratio_setting.UpdateGroupRatioByJSONString(tc.ratios); err != nil {
				t.Fatal(err)
			}
			if tc.disable {
				if err := model.DB.Model(&model.Channel{}).Where("id = ?", 1).Update("status", common.ChannelStatusManuallyDisabled).Error; err != nil {
					t.Fatal(err)
				}
			}
			group, ok, err := resolveImagePlaygroundTokenGroup(1)
			if err != nil || group != "vip" || !ok {
				t.Fatalf("group=%s ecommerce=%v err=%v", group, ok, err)
			}
		})
	}
}

func TestImagePlaygroundMediaGroupIndependentOfPlanning(t *testing.T) {
	for _, tc := range []struct {
		name                                             string
		usable                                           string
		ratios                                           string
		auto                                             string
		disabled, disabledAbility, noImage, queryFailure bool
		wantGroup                                        string
		wantPlanning                                     bool
	}{
		{"separate planning fallback", `{"生图和视频":"","福利":"","vip":"","team":""}`, `{"生图和视频":0.2,"福利":0.6,"vip":1,"team":1}`, `["vip"]`, false, false, false, false, "生图和视频", true},
		{"planning outside fallback", `{"生图和视频":"","福利":"","vip":"","team":""}`, `{"生图和视频":0.2,"福利":0.6,"vip":1,"team":1}`, `["team"]`, false, false, false, false, "生图和视频", false},
		{"unauthorized media", `{"福利":"","vip":"","team":""}`, `{"生图和视频":0.2,"福利":0.6,"vip":1,"team":1}`, `["vip"]`, false, false, false, false, "福利", true},
		{"removed media ratio", `{"生图和视频":"","福利":"","vip":"","team":""}`, `{"福利":0.6,"vip":1,"team":1}`, `["vip"]`, false, false, false, false, "福利", true},
		{"disabled media channel", `{"生图和视频":"","福利":"","vip":"","team":""}`, `{"生图和视频":0.2,"福利":0.6,"vip":1,"team":1}`, `["vip"]`, true, false, false, false, "福利", true},
		{"disabled media ability", `{"生图和视频":"","福利":"","vip":"","team":""}`, `{"生图和视频":0.2,"福利":0.6,"vip":1,"team":1}`, `["vip"]`, false, true, false, false, "福利", true},
		{"no images", `{"生图和视频":"","福利":"","vip":"","team":""}`, `{"生图和视频":0.2,"福利":0.6,"vip":1,"team":1}`, `["vip"]`, false, false, true, false, "", false},
		{"query error preserves old group", `{"生图和视频":"","福利":"","vip":"","team":""}`, `{"生图和视频":0.2,"福利":0.6,"vip":1,"team":1}`, `["vip"]`, false, false, false, true, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupImagePlaygroundGroupTestDB(t)
			for _, err := range []error{setting.UpdateUserUsableGroupsByJSONString(tc.usable), ratio_setting.UpdateGroupRatioByJSONString(tc.ratios), setting.UpdateAutoGroupsByJsonString(tc.auto)} {
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := model.DB.Create(&model.User{Id: 1, Username: "gallery", Group: "team", Status: common.UserStatusEnabled, AffCode: "gallery"}).Error; err != nil {
				t.Fatal(err)
			}
			for id, group := range []string{"生图和视频", "福利", "vip"} {
				status := common.ChannelStatusEnabled
				if id == 0 && tc.disabled {
					status = common.ChannelStatusManuallyDisabled
				}
				if err := model.DB.Create(&model.Channel{Id: id + 1, Name: group, Key: "fixture", Group: group, Models: imagePlaygroundDefaultModel + "," + imagePlaygroundAgentModel, Status: status}).Error; err != nil {
					t.Fatal(err)
				}
				if err := model.DB.Create(&model.Ability{Group: group, Model: imagePlaygroundDefaultModel, ChannelId: id + 1, Enabled: !tc.noImage && !(id == 0 && tc.disabledAbility)}).Error; err != nil {
					t.Fatal(err)
				}
				if id > 0 {
					if err := model.DB.Create(&model.Ability{Group: group, Model: imagePlaygroundAgentModel, ChannelId: id + 1, Enabled: true}).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			if tc.queryFailure {
				if err := model.DB.Migrator().DropTable(&model.Ability{}); err != nil {
					t.Fatal(err)
				}
				old := &model.Token{Group: "vip", AutoGroupFallback: false, RoutingConfig: `{"groups":["vip"]}`}
				changed, _, err := refreshImagePlaygroundToken(old, 1)
				if err == nil || changed || old.Group != "vip" || old.RoutingConfig == "" {
					t.Fatal("query failure must not change stored routing")
				}
				return
			}
			token, planning, err := createImagePlaygroundToken(1, common.GetTimestamp())
			if tc.wantGroup == "" {
				if err == nil || token != nil {
					t.Fatal("no image capability must reject session")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if token.Group != tc.wantGroup || planning != tc.wantPlanning {
				t.Fatalf("group=%s planning=%v", token.Group, planning)
			}
			// Exercise actual channel selection for each request model, not only the candidate list.
			for _, name := range []string{imagePlaygroundDefaultModel, imagePlaygroundAgentModel} {
				if name == imagePlaygroundAgentModel && !planning {
					continue
				}
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				common.SetContextKey(ctx, constant.ContextKeyUserGroup, "team")
				common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroupFallback, token.AutoGroupFallback)
				channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: ctx, TokenGroup: token.Group, ModelName: name})
				want := tc.wantGroup
				if name == imagePlaygroundAgentModel && want == "生图和视频" {
					want = "vip"
				}
				if err != nil || channel == nil || group != want {
					t.Fatalf("%s routed to %s, want %s: %v", name, group, want, err)
				}
			}
			key, expiry := token.Key, token.ExpiredTime
			token.Group, token.AutoGroupFallback, token.CrossGroupRetry = "vip", false, true
			token.RoutingConfig = `{"groups":["vip"]}`
			token.ModelLimitsEnabled, token.ModelLimits = true, "obsolete"
			if err := token.Update(); err != nil {
				t.Fatal(err)
			}
			// Use the real HTTP session entry to cover reuse and persistence.
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodPost, "https://gallery.example/api/image-playground/session", nil)
			c.Set("id", 1)
			CreateImagePlaygroundSession(c)
			var payload struct {
				Success bool                           `json:"success"`
				Data    imagePlaygroundSessionResponse `json:"data"`
			}
			if err := common.Unmarshal(response.Body.Bytes(), &payload); err != nil || !payload.Success {
				t.Fatalf("session failed: %s", response.Body.String())
			}
			var saved model.Token
			if err := model.DB.First(&saved, token.Id).Error; err != nil {
				t.Fatal(err)
			}
			if saved.Group != tc.wantGroup || saved.Key != key || saved.ExpiredTime != expiry || !saved.AutoGroupFallback || saved.CrossGroupRetry || saved.RoutingConfig != "" || saved.ModelLimitsEnabled {
				t.Fatal("session refresh lost routing or identity invariants")
			}
			launch, err := url.Parse(payload.Data.URL)
			if err != nil {
				t.Fatal(err)
			}
			var config imagePlaygroundLaunchSettings
			if err := common.UnmarshalJsonStr(launch.Query().Get("settings"), &config); err != nil {
				t.Fatal(err)
			}
			if launch.Query().Get("appMode") != "gallery" || config.SupportsEcommerce != planning || config.Profiles[0].APIMode != "images" {
				t.Fatal("gallery launch must remain usable independently of planning")
			}
			changed, _, err := refreshImagePlaygroundToken(&saved, 1)
			if changed || err != nil {
				t.Fatal("refresh must be idempotent")
			}
		})
	}
}
