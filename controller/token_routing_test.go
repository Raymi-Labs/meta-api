package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func routingRequest(t *testing.T, method, path string, body any, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	data, err := common.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(string(data)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("role", common.RoleAdminUser)
	handler(c)
	return w
}

func TestTokenRoutingPersistenceCatalogAndPermissions(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)
	config := `{"groups":["default","vip","team"],"model_groups":{"gpt-5.2":["default"],"claude-sonnet-4-6":["team"],"gemini-2.5-pro":["team"]}}`
	payload := map[string]any{"name": "multi", "group": "default", "routing_config": config, "unlimited_quota": true, "expired_time": -1}
	w := routingRequest(t, http.MethodPost, "/api/token/", payload, AddToken)
	require.Contains(t, w.Body.String(), `"success":true`)
	var token model.Token
	require.NoError(t, model.DB.Where("name = ?", "multi").First(&token).Error)
	require.JSONEq(t, config, token.RoutingConfig)
	listItems := buildTokenListItems(1, []*model.Token{&token})
	require.Equal(t, "auto", listItems[0].GroupRatioStatus)
	require.Nil(t, listItems[0].GroupRatio)
	router := gin.New()
	router.GET("/v1/models", middleware.TokenAuth(), func(c *gin.Context) { ListModels(c, constant.ChannelTypeOpenAI) })
	authRequest := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	authRequest.Header.Set("Authorization", "Bearer sk-"+token.Key)
	authResult := httptest.NewRecorder()
	router.ServeHTTP(authResult, authRequest)
	require.Equal(t, http.StatusOK, authResult.Code, authResult.Body.String())
	require.Contains(t, authResult.Body.String(), "gemini-2.5-pro")
	require.NotContains(t, authResult.Body.String(), "claude-sonnet")

	models, err := resolveTokenAllowedModels(1, &token)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"gpt-5.2", "gpt-5.2-codex", "gemini-2.5-pro"}, models, "Claude exists in vip, but is assigned only to team")
	options, err := buildTokenModelOptions(1, &token, "")
	require.NoError(t, err)
	for _, option := range options {
		if option.Name == "gpt-5.2" {
			require.Equal(t, []string{"default"}, option.Groups)
		}
	}
	w = routingRequest(t, http.MethodGet, "/v1/models", nil, func(c *gin.Context) {
		require.NoError(t, middleware.SetupContextForToken(c, &token))
		ListModels(c, constant.ChannelTypeOpenAI)
	})
	require.NotContains(t, w.Body.String(), "claude-sonnet")
	require.Contains(t, w.Body.String(), "gemini-2.5-pro")
	channels, err := buildTokenChannelOptionsForToken(1, &token)
	require.NoError(t, err)
	require.Len(t, channels, 2)
	for _, channel := range channels {
		require.NotEqual(t, 2, channel.ID)
	}

	// Omission by an older client preserves the routing boundary; explicit empty clears it.
	payload["id"] = token.Id
	delete(payload, "routing_config")
	w = routingRequest(t, http.MethodPut, "/api/token/", payload, UpdateToken)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	require.Equal(t, config, token.RoutingConfig)
	payload["routing_config"] = ""
	w = routingRequest(t, http.MethodPut, "/api/token/", payload, UpdateToken)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	require.Empty(t, token.RoutingConfig)

	for _, invalid := range []string{`null`, `{`, `{}`, `{"groups":["auto"]}`, `{"groups":["default","default"]}`, `{"groups":["default","private"]}`, `{"groups":["default"],"model_groups":{"gpt-5.2":["vip"]}}`, `{"groups":["default"],"model_groups":{"gpt-5.2":[]}}`} {
		payload["routing_config"] = invalid
		w = routingRequest(t, http.MethodPut, "/api/token/", payload, UpdateToken)
		require.Contains(t, w.Body.String(), `"success":false`, invalid)
	}
	// Revoking a secondary group removes its models even from an existing token.
	token.RoutingConfig = config
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"","vip":""}`))
	models, err = resolveTokenAllowedModels(1, &token)
	require.NoError(t, err)
	require.NotContains(t, models, "gemini-2.5-pro")
}

func TestTokenRoutingTestScopeAndChannelWhitelist(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)
	token := &model.Token{UserId: 1, Key: "test-scope-key", Name: "scope", Group: "default", AutoGroupFallback: true, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, token.Insert())
	query := "/api/token/models?token_id=" + strconv.Itoa(token.Id) + "&detail=true"
	w := routingRequest(t, http.MethodGet, query, nil, GetTokenModels)
	require.Contains(t, w.Body.String(), "gpt-5.2")
	require.NotContains(t, w.Body.String(), "claude-sonnet", "fixed preferred group remains the default test scope")
	w = routingRequest(t, http.MethodGet, query+"&group=vip", nil, GetTokenModels)
	require.Contains(t, w.Body.String(), "claude-sonnet")
	w = routingRequest(t, http.MethodGet, query+"&group=team", nil, GetTokenModels)
	require.Contains(t, w.Body.String(), `"success":false`)
	token.RoutingConfig = `{"groups":["default","vip"],"model_groups":{"gpt-5.2":["default"]}}`
	token.ChannelLimitsEnabled, token.ChannelLimits = true, "1"
	require.NoError(t, token.Update())
	w = routingRequest(t, http.MethodGet, query+"&group=vip", nil, GetTokenModels)
	require.Contains(t, w.Body.String(), `"data":[]`, "neither other-model channel nor wrong-model group can leak")
	scoped, err := scopedTokenForTest(1, token, "vip")
	require.NoError(t, err)
	require.Equal(t, token.Id, scoped.Id)
	require.Equal(t, token.Key, scoped.Key)
	require.True(t, scoped.ModelLimitsEnabled)
	require.Empty(t, scoped.ModelLimits)
	// An unsaved draft can preview multiple groups, while token ownership is enforced.
	w = routingRequest(t, http.MethodGet, "/api/token/models?group=default&routing_config="+url.QueryEscape(token.RoutingConfig), nil, GetTokenModels)
	require.Contains(t, w.Body.String(), "claude-sonnet")
	w = routingRequest(t, http.MethodGet, query, nil, func(c *gin.Context) { c.Set("id", 2); GetTokenModels(c) })
	require.Contains(t, w.Body.String(), `"success":false`)
}

func TestTokenRoutingTestRejectsExactAliasScopeBypass(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)
	token := &model.Token{UserId: 1, Key: "alias-test-key", Name: "alias", Group: "default", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, RoutingConfig: `{"groups":["default","vip"],"model_groups":{"gpt-4-gizmo-private":["vip"]}}`}
	require.NoError(t, token.Insert())
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-4-gizmo-*", ChannelId: 1, Enabled: true}).Error)
	w := routingRequest(t, http.MethodPost, "/api/token/test/"+strconv.Itoa(token.Id), map[string]string{"model": "gpt-4-gizmo-private", "group": "default"}, func(c *gin.Context) { c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}; TestToken(c) })
	require.Contains(t, w.Body.String(), `"success":false`)
	require.Contains(t, w.Body.String(), "该模型不允许使用所选测试分组")
}
