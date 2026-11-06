package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPublicDescriptionPricingAndGroupBoundaries(t *testing.T) {
	isolateOfficialAttributeRequests(t)
	db := setupModelSyncTestDB(t)
	oldRedis, oldMemory := common.RedisEnabled, common.MemoryCacheEnabled
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	t.Cleanup(func() { common.RedisEnabled, common.MemoryCacheEnabled = oldRedis, oldMemory })
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Channel{}, &model.ModelPermission{}, &model.ModelDescriptionTranslation{}, &model.User{}))
	oldGroups, oldRatios := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(oldGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
		_ = db.Where("1 = 1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
	})
	groups := map[string]string{"default": "概率降智", "特殊": "0注入可蒸；禁止破，蒸"}
	raw, _ := common.Marshal(groups)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(raw)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"特殊":0.6}`))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "fixture", Group: "default", Status: common.UserStatusEnabled}).Error)
	fixture, err := os.ReadFile("../model/data/public_model_descriptions.json")
	require.NoError(t, err)
	var reviews []struct {
		Model           string                  `json:"model"`
		Source          string                  `json:"source"`
		OldSource       string                  `json:"old_source"`
		OldTranslations model.ModelDescriptions `json:"old_translations"`
		Translations    model.ModelDescriptions `json:"translations"`
	}
	require.NoError(t, common.Unmarshal(fixture, &reviews))
	for i, review := range reviews {
		require.NoError(t, db.Create(&model.Model{ModelName: review.Model, Description: review.OldSource, DescriptionOverrides: review.OldTranslations, VendorID: 1, Status: 1}).Error)
		require.NoError(t, db.Create(&model.Channel{Id: i + 1, Name: "fixture", Type: 1, Status: 1, Key: "fixture", Group: "default", Models: review.Model}).Error)
		require.NoError(t, db.Create(&model.Ability{ChannelId: i + 1, Group: "default", Model: review.Model, Enabled: true}).Error)
	}
	require.NoError(t, db.Create(&model.Vendor{Id: 1, Name: "Anthropic", Description: "通过 OpenRouter 接入的 Anthropic 模型", Icon: "Claude"}).Error)
	type response struct {
		Success bool                  `json:"success"`
		Data    []model.Pricing       `json:"data"`
		Vendors []model.PricingVendor `json:"vendors"`
		Groups  map[string]string     `json:"usable_group"`
		Ratios  map[string]float64    `json:"group_ratio"`
	}
	read := func(id int) response {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/pricing", nil)
		if id > 0 {
			c.Set("id", id)
		}
		GetPricing(c)
		require.Equal(t, http.StatusOK, w.Code)
		var body response
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
		require.True(t, body.Success)
		return body
	}
	for pass := 0; pass < 2; pass++ {
		if pass == 1 {
			require.NoError(t, model.NormalizePublicDescriptions())
		}
		model.RefreshPricing()
		for _, id := range []int{0, 1} {
			body := read(id)
			require.Equal(t, groups, body.Groups)
			require.Equal(t, map[string]float64{"default": 1, "特殊": 0.6}, body.Ratios)
			require.Len(t, body.Data, 4)
			require.Len(t, body.Vendors, 1)
			require.NotContains(t, body.Vendors[0].Description, "OpenRouter")
			for _, m := range body.Data {
				for _, review := range reviews {
					if review.Model == m.ModelName {
						require.Equal(t, review.Source, m.Description)
						require.Equal(t, review.Translations, m.DescriptionTranslations)
					}
				}
			}
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/user/self/groups", nil)
		c.Set("id", 1)
		GetUserGroups(c)
		var body struct {
			Data map[string]struct {
				Desc  string  `json:"desc"`
				Ratio float64 `json:"ratio"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
		for name, desc := range groups {
			require.Equal(t, desc, body.Data[name].Desc)
		}
	}
}

func TestPublicDescriptionCatalogImportKeepsProvenance(t *testing.T) {
	entries, err := parseModelCapabilities([]byte(`{"reseller":{"id":"reseller","models":{"claude-sonnet-4-6":{"id":"claude-sonnet-4-6","description":"Claude Sonnet 4.6 is an AI model provided by venice."},"custom":{"id":"custom","description":"An OpenRouter protocol guide."}}}}`))
	require.NoError(t, err)
	require.Equal(t, "reseller", entries["claude-sonnet-4-6"].Provider)
	require.NotContains(t, entries["claude-sonnet-4-6"].Description, "venice")
	require.Equal(t, "An OpenRouter protocol guide.", entries["custom"].Description)
}
