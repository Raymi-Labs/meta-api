package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTokenGroupFallbackCatalogAndPersistence(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)
	for _, on := range []bool{false, true} {
		t.Run(fmt.Sprint(on), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/token/", strings.NewReader(fmt.Sprintf(`{"name":"fallback-%v","group":"default","auto_group_fallback":%v,"unlimited_quota":true,"expired_time":-1}`, on, on)))
			c.Set("id", 1)
			c.Set("role", common.RoleCommonUser)
			AddToken(c)
			require.Contains(t, w.Body.String(), `"success":true`)
			var token model.Token
			require.NoError(t, model.DB.Where("name = ?", fmt.Sprintf("fallback-%v", on)).First(&token).Error)
			require.Equal(t, on, token.AutoGroupFallback)
			models, err := resolveTokenAllowedModels(1, &token)
			require.NoError(t, err)
			require.Equal(t, on, common.StringsContains(models, "claude-sonnet-4-6"))
			require.NotContains(t, models, "gemini-2.5-pro", "non-auto groups must not be added")
			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/token/models?group=default&auto_group_fallback=%v&detail=true", on), nil)
			c.Set("id", 1)
			GetTokenModels(c)
			require.Equal(t, on, strings.Contains(w.Body.String(), "claude-sonnet-4-6"))
			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			require.NoError(t, middleware.SetupContextForToken(c, &token))
			ListModels(c, constant.ChannelTypeOpenAI)
			require.Equal(t, on, strings.Contains(w.Body.String(), "claude-sonnet-4-6"), w.Body.String())
			require.NotContains(t, w.Body.String(), "gemini-2.5-pro")
			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/token/channels?group=default&auto_group_fallback=%v", on), nil)
			c.Set("id", 1)
			c.Set("role", common.RoleAdminUser)
			GetTokenChannels(c)
			require.Equal(t, on, strings.Contains(w.Body.String(), "default-anthropic"), w.Body.String())
			options, err := buildTokenChannelOptions(1, "default", "", false, on)
			require.NoError(t, err)
			require.Len(t, options, map[bool]int{false: 1, true: 2}[on])
			_, _, err = normalizeTokenChannelLimitsForSave(1, common.RoleAdminUser, "default", "claude-sonnet-4-6", true, "2", true, "", on)
			require.Equal(t, !on, err != nil)
			require.NoError(t, middleware.SetupContextForToken(c, &token))
			require.Equal(t, on, common.GetContextKeyBool(c, constant.ContextKeyTokenAutoGroupFallback))
			token.SourceType = model.TokenSourceTypeSellableToken
			require.NoError(t, middleware.SetupContextForToken(c, &token))
			require.False(t, common.GetContextKeyBool(c, constant.ContextKeyTokenAutoGroupFallback))
			models, err = resolveTokenAllowedModels(1, &token)
			require.NoError(t, err)
			require.NotContains(t, models, "claude-sonnet-4-6")
			// Saving false must persist rather than be skipped as a zero-value update.
			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/token/", strings.NewReader(fmt.Sprintf(`{"id":%d,"name":"updated","group":"default","auto_group_fallback":false,"unlimited_quota":true,"expired_time":-1}`, token.Id)))
			c.Set("id", 1)
			c.Set("role", common.RoleCommonUser)
			UpdateToken(c)
			require.Contains(t, w.Body.String(), `"success":true`)
			require.NoError(t, model.DB.First(&token, token.Id).Error)
			require.False(t, token.AutoGroupFallback)
		})
	}
	_, err := resolveRequestedTokenModels(1, "private", true)
	require.Error(t, err)
}
