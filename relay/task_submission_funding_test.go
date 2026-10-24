package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskSubmissionInsufficientFundsNeverReachesProvider(t *testing.T) {
	db := useRelayCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.PulseFundingLedger{}, &model.UserSubscription{}))
	seedRelayUser(t, 7520, 1)
	service.InitHttpClient()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"sora-2","prompt":"test"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeSora)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-provider-key")
	info := &relaycommon.RelayInfo{UserId: 7520, IsPlayground: true, UsingGroup: "default", UserGroup: "default", OriginModelName: "sora-2", TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	result, taskErr := RelayTaskSubmit(c, info)
	require.Nil(t, result)
	require.NotNil(t, taskErr)
	require.True(t, taskErr.LocalError)
	require.Equal(t, http.StatusForbidden, taskErr.StatusCode)
	require.Equal(t, "insufficient_user_quota", taskErr.Code)
	require.Zero(t, calls.Load())
	var user model.User
	require.NoError(t, db.First(&user, 7520).Error)
	require.Equal(t, 1, user.Quota)
}
