package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIdentityVendorCRUDAndSync(t *testing.T) {
	db := setupModelSyncTestDB(t)
	call := func(handler gin.HandlerFunc, body string) bool {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/vendors", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		handler(c)
		var result struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
		return result.Success
	}
	require.True(t, call(CreateVendorMeta, `{"name":" MoonshotAI ","icon":"Moonshot","description":"manual"}`))
	var canonical model.Vendor
	require.NoError(t, db.Where("name = ?", "Moonshot AI").First(&canonical).Error)
	require.False(t, call(CreateVendorMeta, `{"name":"moonshot"}`))
	other := model.Vendor{Name: "Custom", Status: 1}
	require.NoError(t, other.Insert())
	body, err := common.Marshal(map[string]interface{}{"id": other.Id, "name": "Moonshot"})
	require.NoError(t, err)
	require.False(t, call(UpdateVendorMeta, string(body)))
	require.NoError(t, db.First(&other, other.Id).Error)
	require.Equal(t, "Custom", other.Name)
	require.False(t, call(UpdateVendorMeta, `{"id":1,"name":"   "}`))
	count := 0
	cache := map[string]int{}
	for _, name := range []string{"Moonshot", "MoonshotAI", "Moonshot AI"} {
		require.Equal(t, canonical.Id, ensureVendorID(name, nil, cache, &count))
	}
	require.Zero(t, count)
	require.True(t, call(CreateVendorMeta, `{"name":"Z.AI"}`))
	require.False(t, call(CreateVendorMeta, `{"name":"ChatGLM"}`))
	require.False(t, call(CreateVendorMeta, `{"name":"智谱"}`))
	require.True(t, call(CreateVendorMeta, `{"name":"Moondream"}`))
	require.True(t, call(CreateVendorMeta, `{"name":"Tencent TokenHub"}`))
}

func TestIdentityVendorPricingKeepsModelInventory(t *testing.T) {
	isolateOfficialAttributeRequests(t)
	db := setupModelSyncTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Channel{}, &model.ModelPermission{}))
	t.Cleanup(func() { _ = db.Where("1 = 1").Delete(&model.Ability{}).Error; model.RefreshPricing() })
	for _, v := range []model.Vendor{{Id: 19, Name: "Moonshot"}, {Id: 23, Name: "Moonshot AI"}, {Id: 59, Name: "MoonshotAI"}, {Id: 10, Name: "ChatGLM"}, {Id: 22, Name: "Z.AI"}, {Id: 18, Name: "智谱"}} {
		require.NoError(t, db.Create(&v).Error)
	}
	for i, row := range []struct {
		name string
		id   int
	}{{"kimi-k2.8", 19}, {"k3", 19}, {"kimi-k3", 23}, {"kimi-k2.7-code", 23}, {"moonshotai/kimi-k2", 59}, {"glm-5", 10}, {"glm-5.3", 22}} {
		require.NoError(t, db.Create(&model.Model{ModelName: row.name, VendorID: row.id, Status: 1, NameRule: model.NameRuleExact}).Error)
		require.NoError(t, db.Create(&model.Channel{Id: i + 1, Name: "fixture", Status: common.ChannelStatusEnabled, Type: 1, Models: row.name, Group: "default", Key: "fixture"}).Error)
		require.NoError(t, db.Create(&model.Ability{Model: row.name, Group: "default", ChannelId: i + 1, Enabled: true}).Error)
	}
	read := func() (map[string]model.Pricing, map[int]string) {
		model.RefreshPricing()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/pricing", nil)
		GetPricing(c)
		var result struct {
			Success bool                  `json:"success"`
			Data    []model.Pricing       `json:"data"`
			Vendors []model.PricingVendor `json:"vendors"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
		require.True(t, result.Success)
		items := map[string]model.Pricing{}
		vendors := map[int]string{}
		for _, v := range result.Vendors {
			vendors[v.ID] = v.Name
		}
		for _, m := range result.Data {
			items[m.ModelName] = m
		}
		return items, vendors
	}
	before, _ := read()
	require.NoError(t, model.NormalizeIdentityVendorMetadata())
	after, vendors := read()
	require.Len(t, after, len(before))
	counts := map[string]int{}
	for name, item := range after {
		prior := before[name]
		prior.VendorID = item.VendorID
		prior.PricingVersion = item.PricingVersion // derived price cache digest may be populated on refresh
		require.Equal(t, prior, item, "prices, groups, models and metadata must remain unchanged")
		counts[vendors[item.VendorID]]++
	}
	require.Equal(t, map[string]int{"Moonshot AI": 5, "智谱": 2}, counts)
	require.Contains(t, after, "k3")
	require.Contains(t, after, "kimi-k3")
	require.Len(t, vendors, 2)
}
