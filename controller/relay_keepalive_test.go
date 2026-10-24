package controller

import (
	"bufio"
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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func configureRequestKeepAliveTest(t *testing.T) {
	t.Helper()
	settings := operation_setting.GetGeneralSetting()
	oldEnabled, oldInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	settings.PingIntervalEnabled, settings.PingIntervalSeconds = true, 1
	t.Cleanup(func() { settings.PingIntervalEnabled, settings.PingIntervalSeconds = oldEnabled, oldInterval })
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
}

func readClaudeKeepAlive(t *testing.T, reader *bufio.Reader) {
	t.Helper()
	for _, want := range []string{"event: ping\n", "data: {\"type\":\"ping\"}\n", "\n"} {
		got, err := reader.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestRelayRequestKeepAliveDuringPreparationAndFinalError(t *testing.T) {
	configureRequestKeepAliveTest(t)
	db := useControllerCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ContentSafetyViolation{}))
	oldCount, oldMedia, oldRedis := constant.CountToken, constant.GetMediaToken, common.RedisEnabled
	constant.CountToken, constant.GetMediaToken, common.RedisEnabled = true, true, false
	fetch := system_setting.GetFetchSetting()
	oldFetch := *fetch
	fetch.AllowPrivateIp, fetch.AllowedPorts = true, []string{"1-65535"}
	t.Cleanup(func() {
		constant.CountToken, constant.GetMediaToken, common.RedisEnabled = oldCount, oldMedia, oldRedis
		*fetch = oldFetch
	})
	imageRequested, releaseImage := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(imageRequested)
		select {
		case <-releaseImage:
			w.WriteHeader(http.StatusBadGateway)
		case <-r.Context().Done():
		}
	}))
	defer imageServer.Close()
	result := make(chan bool, 1)
	r := gin.New()
	r.POST("/v1/messages", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "claude-sonnet-4-20250514")
		Relay(c, types.RelayFormatClaude)
		result <- service.RequestSucceeded(c)
	})
	gateway := httptest.NewServer(r)
	defer gateway.Close()
	defer releaseOnce.Do(func() { close(releaseImage) })
	body := `{"model":"claude-sonnet-4-20250514","max_tokens":16,"stream":true,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"` + imageServer.URL + `"}}]}]}`
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Post(gateway.URL+"/v1/messages", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	reader := bufio.NewReader(resp.Body)
	readClaudeKeepAlive(t, reader)
	select {
	case <-imageRequested:
	default:
		t.Fatal("heartbeat did not cover image preparation")
	}
	releaseOnce.Do(func() { close(releaseImage) })
	remainder, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Contains(t, string(remainder), "event: error\ndata: ")
	require.Contains(t, string(remainder), `"type":"error"`)
	require.NotContains(t, string(remainder), "event: message_stop")
	require.False(t, <-result, "a heartbeat is not a completed response")
}

func TestRelayRequestKeepAliveAcrossHTTPAttempts(t *testing.T) {
	configureRequestKeepAliveTest(t)
	var calls atomic.Int32
	retryStarted, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		close(retryStarted)
		select {
		case <-release:
			_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	finished := make(chan error, 1)
	r := gin.New()
	r.POST("/v1/messages", func(c *gin.Context) {
		defer helper.StopStreamKeepAlive(c)
		info := &relaycommon.RelayInfo{IsStream: true, RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{}}
		for range 2 {
			helper.StartStreamKeepAlive(c, 10*time.Millisecond, types.RelayFormatClaude)
			req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstream.URL, strings.NewReader("{}"))
			if err != nil {
				finished <- err
				return
			}
			resp, err := channel.DoRequest(c, req, info)
			if err != nil {
				finished <- err
				return
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				resp.Body.Close()
				// A retry diagnostic header joins the old pinger; next attempt
				// must resume it without relaxing business-output replay rules.
				c.Header("X-Request-Adjusted", "retry")
				continue
			}
			_, err = io.Copy(c.Writer, resp.Body)
			resp.Body.Close()
			finished <- err
		}
	})
	gateway := httptest.NewServer(r)
	defer gateway.Close()
	defer releaseOnce.Do(func() { close(release) })
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Post(gateway.URL+"/v1/messages", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer resp.Body.Close()
	select {
	case <-retryStarted:
	case <-time.After(time.Second):
		t.Fatal("second HTTP attempt did not start")
	}
	reader := bufio.NewReader(resp.Body)
	readClaudeKeepAlive(t, reader)
	readClaudeKeepAlive(t, reader)
	releaseOnce.Do(func() { close(release) })
	remainder, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Contains(t, string(remainder), "event: message_stop")
	require.NoError(t, <-finished)
	require.EqualValues(t, 2, calls.Load())
}

func TestRelayRequestKeepAlivePreservesValidationJSON(t *testing.T) {
	configureRequestKeepAliveTest(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-20250514","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	Relay(c, types.RelayFormatClaude)
	// Preserve the local validation status; this port only changes heartbeat
	// ownership, not the existing invalid-request error mapping.
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
	require.NotContains(t, recorder.Body.String(), "event: ping")
}
