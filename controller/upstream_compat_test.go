package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVolcEngineUpstreamModelsEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v3/models", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[{"id":"doubao-test"}]}`))
	}))
	defer server.Close()
	service.InitHttpClient()
	channel := &model.Channel{Type: constant.ChannelTypeVolcEngine, Key: "test-key", BaseURL: common.GetPointer(server.URL + "/")}
	models, err := fetchChannelUpstreamModelIDs(channel)
	require.NoError(t, err)
	require.Equal(t, []string{"doubao-test"}, models)
}

func TestChannelTestUsesModelCapabilities(t *testing.T) {
	for _, name := range []string{"gpt-5.2", "gpt-6-astra", "gpt-6-astra-2026-09-03", "gpt-6-sol", "gpt-6-luna", "gpt-6-sol-2026-09-30", "gpt-6.1-sol", "gpt-6.1-terra", "gpt-6.1-terra-2026-10-28", "o3"} {
		t.Run(name, func(t *testing.T) {
			req := buildTestRequest(name, "", &model.Channel{Type: constant.ChannelTypeOpenAI}, false).(*dto.GeneralOpenAIRequest)
			require.Equal(t, uint(16), req.MaxCompletionTokens)
			require.Zero(t, req.MaxTokens)
		})
	}
}

func TestAdminUserWritesRejectInvalidRoles(t *testing.T) {
	setupTopupCallbackTestDB(t)
	user := createTopupCallbackTestUser(t, "role-validation")
	for _, role := range []int{-1, 5, 99, 101} {
		for _, operator := range []int{common.RoleAdminUser, common.RoleRootUser} {
			for _, handler := range []gin.HandlerFunc{CreateUser, UpdateUser} {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Set("role", operator)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/", strings.NewReader(fmt.Sprintf(`{"id":%d,"username":"injected-user","password":"password123","role":%d}`, user.Id, role)))
				handler(ctx)
				var result struct {
					Success bool `json:"success"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
				require.False(t, result.Success, recorder.Body.String())
			}
		}
	}
	var users []model.User
	require.NoError(t, model.DB.Find(&users).Error)
	require.Len(t, users, 1)
	require.Equal(t, common.RoleCommonUser, users[0].Role)
	require.Equal(t, user.Username, users[0].Username)
}

func TestModelSyncTLSConfigIsIndependent(t *testing.T) {
	old := common.TLSInsecureSkipVerify
	common.TLSInsecureSkipVerify = true
	t.Cleanup(func() { common.TLSInsecureSkipVerify = old })
	first := newHTTPClient().Transport.(*http.Transport)
	second := newHTTPClient().Transport.(*http.Transport)
	require.NotSame(t, common.InsecureTLSConfig, first.TLSClientConfig)
	require.NotSame(t, first.TLSClientConfig, second.TLSClientConfig)
	first.TLSClientConfig.ServerName = "isolated.example"
	require.NotEqual(t, first.TLSClientConfig.ServerName, second.TLSClientConfig.ServerName)
}
