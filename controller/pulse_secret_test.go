package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeneratePulseSecretReturnsUsableUniqueSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	call := func(role int) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/option/pulse/secret/generate", nil)
		c.Set("id", 7)
		c.Set("role", role)
		GeneratePulseSecret(c)
		return response
	}

	require.Equal(t, http.StatusForbidden, call(common.RoleCommonUser).Code)

	require.Equal(t, http.StatusForbidden, call(common.RoleAdminUser).Code)

	seen := make(map[string]bool, 8)
	for i := 0; i < 8; i++ {
		response := call(common.RoleRootUser)
		require.Equal(t, http.StatusOK, response.Code)
		var payload struct {
			Success bool   `json:"success"`
			Data    string `json:"data"`
		}
		require.NoError(t, common.UnmarshalJsonStr(response.Body.String(), &payload))
		require.True(t, payload.Success)
		// The generated value has to pass the same gate the signing path uses,
		// otherwise the console would hand out secrets Pulse refuses.
		require.True(t, middlewarePulseSecretUsable(payload.Data))
		require.False(t, seen[payload.Data], "生成的密钥必须唯一")
		seen[payload.Data] = true
	}
}
