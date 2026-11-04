package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func testVMCapacityContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	return c
}

func TestVMCapacityAdmissionTwentyCallersWithRedis(t *testing.T) {
	enableControllerCapacityForTest(t)
	oldDB, oldRDB := model.DB, common.RDB
	model.DB = nil
	r := miniredis.RunT(t)
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: r.Addr()})
	t.Cleanup(func() { common.RDB.Close(); model.DB = oldDB; common.RDB = oldRDB })
	service.InitHttpClient()
	var queries, active, peak, completed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		fmt.Fprintf(w, `{"schema_version":1,"model":"claude-haiku-4-5","observed_at":%q,"max_age_ms":1000,"state_quality":"known","effective_admission_limit":4,"available_now":4,"key_inflight":0}`, time.Now().UTC().Format(time.RFC3339Nano))
	}))
	defer server.Close()
	base := server.URL
	settings := `{"max_concurrency":10,"upstream_capacity":{"enabled":true,"max_wait_ms":5000}}`
	ch := &model.Channel{Id: 778901, Type: constant.ChannelTypeAnthropic, Key: "capacity-fixture", BaseURL: &base, Setting: &settings}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := testVMCapacityContext()
			deadline := capacityDeadline(c, ch, time.Second)
			admission := waitVMChannelCapacity(c, ch, "claude-haiku-4-5", "", deadline)
			if !admission.Acquired {
				t.Errorf("request not acquired: %s", admission.Reason)
				return
			}
			n := active.Add(1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			admission.Release()
			admission.Release()
			completed.Add(1)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 20, completed.Load())
	require.LessOrEqual(t, peak.Load(), int32(4))
	require.LessOrEqual(t, queries.Load(), int32(2))
	used, err := middleware.QueryChannelConcurrency(ch.Id)
	require.NoError(t, err)
	require.Zero(t, used)
}

func TestVMCapacityZeroRecoveryCancellationAndDeadline(t *testing.T) {
	enableControllerCapacityForTest(t)
	oldDB := model.DB
	model.DB = nil
	t.Cleanup(func() { model.DB = oldDB })
	service.InitHttpClient()
	var limit atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := limit.Load()
		fmt.Fprintf(w, `{"schema_version":1,"model":"claude-haiku-4-5","observed_at":%q,"max_age_ms":1000,"state_quality":"known","effective_admission_limit":%d,"available_now":%d,"key_inflight":0}`, time.Now().UTC().Format(time.RFC3339Nano), n, n)
	}))
	defer server.Close()
	base := server.URL
	settings := `{"max_concurrency":10,"upstream_capacity":{"enabled":true,"max_wait_ms":2000}}`
	ch := &model.Channel{Id: 778902, Type: constant.ChannelTypeAnthropic, Key: "capacity-zero-fixture", BaseURL: &base, Setting: &settings}
	c := testVMCapacityContext()
	deadline := capacityDeadline(c, ch, time.Second)
	require.False(t, acquireRelayChannelCapacity(c, ch, "claude-haiku-4-5").Acquired, "zero is not unlimited")
	limit.Store(2)
	a := waitVMChannelCapacity(c, ch, "claude-haiku-4-5", "", deadline)
	require.True(t, a.Acquired)
	a.Release()
	require.Equal(t, deadline, capacityDeadline(c, ch, time.Second), "retry must not reset deadline")
	ctx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(ctx)
	require.False(t, waitVMChannelCapacity(c, ch, "claude-haiku-4-5", "", deadline).Acquired)
	require.Zero(t, middleware.GetChannelConcurrencyMetrics().CurrentWaiting)
}

func TestVMCapacityErrorParsingAndReplayBoundary(t *testing.T) {
	setting := `{"upstream_capacity":{"enabled":true}}`
	ch := &model.Channel{Setting: &setting}
	for _, code := range []string{"channel_capacity_busy", "account_pool_exhausted", "channel_capacity_unavailable", "api_key_concurrency_limit"} {
		resp := &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": []string{"1"}}, Body: http.NoBody}
		resp.Body = io.NopCloser(strings.NewReader(fmt.Sprintf(`{"type":"error","error":{"type":"api_error","code":%q,"message":"capacity unavailable"}}`, code)))
		err := service.RelayErrorHandler(context.Background(), resp, false)
		require.True(t, service.IsVMChannelCapacityError(ch, err))
		require.Equal(t, time.Second, err.RetryAfter)
		c := testVMCapacityContext()
		require.True(t, vmCapacityRetrySafe(c, err))
		c.Writer.Write([]byte("business output"))
		require.False(t, vmCapacityRetrySafe(c, err))
	}
	err := types.WithOpenAIError(types.OpenAIError{Code: "unrelated", Message: "too many requests"}, 429)
	require.False(t, service.IsVMChannelCapacityError(ch, err))
	require.False(t, service.IsVMChannelCapacityError(&model.Channel{}, err))
}
