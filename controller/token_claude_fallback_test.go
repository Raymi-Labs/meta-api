package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestTokenClaudeFallbackPersistence(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)
	payload := map[string]any{"name": "native-fallback", "group": "default", "unlimited_quota": true, "expired_time": -1}
	read := func() model.Token {
		t.Helper()
		var token model.Token
		require.NoError(t, model.DB.Where("name = ?", payload["name"]).First(&token).Error)
		return token
	}
	save := func(method string, handler gin.HandlerFunc) {
		t.Helper()
		w := routingRequest(t, method, "/api/token/", payload, handler)
		require.Contains(t, w.Body.String(), `"success":true`)
	}
	save(http.MethodPost, AddToken)
	require.False(t, read().ClaudeRefusalFallback)
	payload["name"], payload["claude_refusal_fallback"] = "opted-in", true
	save(http.MethodPost, AddToken)
	token := read()
	require.True(t, token.ClaudeRefusalFallback)
	payload["id"] = token.Id
	delete(payload, "claude_refusal_fallback")
	save(http.MethodPut, UpdateToken)
	require.True(t, read().ClaudeRefusalFallback)
	routingRequest(t, http.MethodGet, "/v1/models", nil, func(c *gin.Context) {
		require.NoError(t, middleware.SetupContextForToken(c, &token))
		require.True(t, common.GetContextKeyBool(c, constant.ContextKeyTokenClaudeRefusalFallback))
		token.SourceType = model.TokenSourceTypeSellableToken
		require.NoError(t, middleware.SetupContextForToken(c, &token))
		require.False(t, common.GetContextKeyBool(c, constant.ContextKeyTokenClaudeRefusalFallback))
	})
	payload["claude_refusal_fallback"] = false
	save(http.MethodPut, UpdateToken)
	require.False(t, read().ClaudeRefusalFallback)
}
