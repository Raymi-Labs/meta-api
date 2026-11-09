package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type cancelResponseTransport struct {
	cancel context.CancelFunc
	calls  int
}

func (t *cancelResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls++
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: t, Request: req}, nil
}

func (t *cancelResponseTransport) Read([]byte) (int, error) {
	t.cancel()
	return 0, context.Canceled
}

func (t *cancelResponseTransport) Close() error { return nil }

func TestRelayDeepSeekCanceledResponseDoesNotFailover(t *testing.T) {
	db := useControllerCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}, &model.ContentSafetyViolation{}))
	oldLog, oldRedis, oldCount, oldRetry := constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes
	constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes = true, false, false, 3
	t.Cleanup(func() {
		constant.ErrorLogEnabled, common.RedisEnabled, constant.CountToken, common.RetryTimes = oldLog, oldRedis, oldCount, oldRetry
	})
	oldRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-client-cancel":0}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
	quotaSetting := operation_setting.GetQuotaSetting()
	oldFree := quotaSetting.EnableFreeModelPreConsume
	quotaSetting.EnableFreeModelPreConsume = false
	t.Cleanup(func() { quotaSetting.EnableFreeModelPreConsume = oldFree })
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	client := service.GetHttpClient()
	oldTransport := client.Transport
	transport := &cancelResponseTransport{cancel: cancel}
	client.Transport = transport
	t.Cleanup(func() { client.Transport = oldTransport })
	channel := &model.Channel{Id: 993499, Name: "cancellation-test", Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)
	r := gin.New()
	r.Use(middleware.RequestFailureLog(), middleware.RouteTag("relay"))
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("id", 7312)
		c.Set(common.RequestIdKey, "req-deepseek-canceled")
		require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "test-client-cancel"))
		Relay(c, types.RelayFormatOpenAI)
		require.Equal(t, []string{"993499"}, c.GetStringSlice("use_channel"))
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-client-cancel","messages":[{"role":"user","content":"test"}],"stream":false}`)).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, 1, transport.calls)
	require.Equal(t, types.StatusClientClosedRequest, w.Code, w.Body.String())
	var row model.Log
	require.NoError(t, db.Where("request_id = ?", "req-deepseek-canceled").First(&row).Error)
	require.Equal(t, "客户端连接中断", row.Content)
	require.Zero(t, row.Quota)
	details, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	require.Equal(t, "client_canceled", details["failure_class"])
	diagnostics := details["admin_info"].(map[string]interface{})["cancellation"].(map[string]interface{})
	require.Equal(t, "waiting_body", diagnostics["phase"])
	require.Equal(t, false, diagnostics["upstream_response_started"])
	require.GreaterOrEqual(t, diagnostics["duration_ms"].(float64), float64(0))
	attempts := details["admin_info"].(map[string]interface{})["attempts"].([]interface{})
	require.Len(t, attempts, 1)
	require.Equal(t, "client_canceled", attempts[0].(map[string]interface{})["error_code"])
}
