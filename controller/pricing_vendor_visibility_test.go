package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPricingVendorsProjectionDoesNotMutateCache(t *testing.T) {
	vendors := []model.PricingVendor{{ID: 7, Name: "shared", Description: "keep"}, {ID: 63, Name: "Venice"}, {ID: 8, Name: "private"}, {ID: 0, Name: "invalid"}}
	original := append([]model.PricingVendor(nil), vendors...)
	items := []model.Pricing{{VendorID: 7}, {VendorID: 7}, {VendorID: 999}, {VendorID: 0}}
	result := pricingVendorsForModels(items, vendors)
	require.Equal(t, []model.PricingVendor{vendors[0]}, result)
	result[0].Name = "changed"
	require.Equal(t, original, vendors)
	require.Equal(t, []model.PricingVendor{vendors[1]}, pricingVendorsForModels([]model.Pricing{{VendorID: 63}}, vendors), "Venice is not a name blacklist")
	require.NotNil(t, pricingVendorsForModels(nil, vendors))
	require.Empty(t, pricingVendorsForModels(nil, vendors))
	require.Empty(t, pricingVendorsForModels(items, nil))
}

func TestPricingVendorVisibilityFollowsResponseModels(t *testing.T) {
	isolateOfficialAttributeRequests(t)
	db := setupModelSyncTestDB(t)
	conn, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Channel{}, &model.ModelPermission{}))
	oldGroups, oldRatios := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"概率降智；0注入可蒸；禁止破，蒸"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":0.6}`))
	model.InvalidateModelPermissionCache()
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(oldGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
		_ = db.Where("1 = 1").Delete(&model.Ability{}).Error
		model.InvalidateModelPermissionCache()
		model.RefreshPricing()
	})
	for _, v := range []model.Vendor{{Id: 1, Name: "Shared"}, {Id: 2, Name: "AdminVendor"}, {Id: 3, Name: "UserVendor"}, {Id: 4, Name: "HiddenVendor"}, {Id: 63, Name: "Venice"}} {
		require.NoError(t, db.Create(&v).Error)
	}
	definitions := []struct {
		name          string
		vendor, scope int
	}{
		{"public-fixture", 1, model.ModelPermissionScopeAll},
		{"admin-shared", 1, model.ModelPermissionScopeAdminOnly},
		{"admin-fixture", 2, model.ModelPermissionScopeAdminOnly},
		{"user-fixture", 3, model.ModelPermissionScopeCommonOnly},
		{"hidden-fixture", 4, model.ModelPermissionScopeNone},
	}
	for i, d := range definitions {
		require.NoError(t, db.Create(&model.Model{ModelName: d.name, VendorID: d.vendor, Status: 1}).Error)
		require.NoError(t, db.Create(&model.Channel{Id: i + 1, Name: "fixture", Type: 1, Key: "fixture", Status: 1, Group: "default", Models: d.name}).Error)
		require.NoError(t, db.Create(&model.Ability{ChannelId: i + 1, Model: d.name, Group: "default", Enabled: true}).Error)
		permission := model.ModelPermission{ModelName: d.name, NameRule: model.NameRuleExact, VisibilityScope: d.scope}
		require.NoError(t, permission.Insert())
	}
	model.RefreshPricing()
	cachedVendors := append([]model.PricingVendor(nil), model.GetVendors()...)
	for _, role := range []int{common.RoleGuestUser, common.RoleCommonUser, common.RoleAdminUser, common.RoleGuestUser} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/pricing", nil)
		c.Set("role", role)
		GetPricing(c)
		var body struct {
			Success bool                  `json:"success"`
			Data    []model.Pricing       `json:"data"`
			Vendors []model.PricingVendor `json:"vendors"`
			Groups  map[string]string     `json:"usable_group"`
			Ratios  map[string]float64    `json:"group_ratio"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
		require.True(t, body.Success)
		expectedJSON, err := common.Marshal(model.FilterPricingByVisibility(model.GetPricing(), role))
		require.NoError(t, err)
		actualJSON, err := common.Marshal(body.Data)
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), string(actualJSON), "model payload and prices unchanged")
		require.Equal(t, map[string]string{"default": "概率降智；0注入可蒸；禁止破，蒸"}, body.Groups)
		require.Equal(t, map[string]float64{"default": 0.6}, body.Ratios)
		wanted := map[int]bool{}
		for _, m := range body.Data {
			if m.VendorID > 0 {
				wanted[m.VendorID] = true
			}
		}
		returned := map[int]bool{}
		for _, v := range body.Vendors {
			returned[v.ID] = true
		}
		require.Equal(t, wanted, returned)
		require.NotContains(t, returned, 63)
		require.NotContains(t, returned, 4)
		if role >= common.RoleAdminUser {
			require.True(t, returned[2])
			require.NotContains(t, returned, 3)
		} else {
			require.True(t, returned[3])
			require.NotContains(t, returned, 2)
		}
		require.True(t, returned[1])
		require.Equal(t, cachedVendors, model.GetVendors(), "role projection must not mutate shared cache")
	}
	// Management and storage still contain the unused record; no model is reassigned.
	all, err := model.GetAllVendors(0, 100)
	require.NoError(t, err)
	require.Len(t, all, 5)
	venice, err := model.GetVendorByID(63)
	require.NoError(t, err)
	require.Equal(t, "Venice", venice.Name)
	// Revoke visibility without rebuilding pricing. Cached models must not expose old vendors.
	require.NoError(t, db.Model(&model.ModelPermission{}).Where("model_name IN ?", []string{"public-fixture", "user-fixture"}).Update("visibility_scope", model.ModelPermissionScopeNone).Error)
	model.InvalidateModelPermissionCache()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/pricing", nil)
	GetPricing(c)
	var empty struct {
		Data    []model.Pricing       `json:"data"`
		Vendors []model.PricingVendor `json:"vendors"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &empty))
	require.Empty(t, empty.Data)
	require.NotNil(t, empty.Vendors)
	require.Empty(t, empty.Vendors)
}
