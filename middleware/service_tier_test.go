package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPaidServiceTierRequestAndExplicitChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, false
	t.Cleanup(func() { model.DB, common.MemoryCacheEnabled = oldDB, oldCache; conn, _ := db.DB(); conn.Close() })
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	supported := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Models: "tier-test", OtherSettings: `{"allow_service_tier":true,"paid_service_tiers":{"tier-test":["priority","ultrafast"]}}`}
	require.NoError(t, db.Create(supported).Error)
	for _, tc := range []struct {
		path, tier string
		valid      bool
	}{{"/v1/responses", "priority", true}, {"/v1/chat/completions", "ultrafast", true}, {"/pg/chat/completions", "priority", true}, {"/v1/responses", "", true}, {"/v1/responses", "fast", false}, {"/v1/messages", "priority", false}, {"/v1/responses/compact", "ultrafast", false}} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", tc.path, nil)
		err := prepareServiceTier(c, &ModelRequest{Model: "tier-test", ServiceTier: tc.tier})
		if !tc.valid {
			require.NotNil(t, err)
			require.Equal(t, 400, err.StatusCode)
			continue
		}
		require.Nil(t, err)
		require.True(t, c.GetBool(service.ManagedServiceTierKey))
		if tc.tier != "" {
			require.NotNil(t, SetupContextForSelectedChannelKey(c, &model.Channel{Id: 2}, "tier-test", "unused-fixture", 0))
		}
		require.True(t, service.ChannelAllowsRequestedServiceTier(c, supported, "tier-test"))
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	require.Nil(t, prepareServiceTier(c, &ModelRequest{Model: "legacy", ServiceTier: "flex"}))
	require.False(t, c.GetBool(service.ManagedServiceTierKey))
}
