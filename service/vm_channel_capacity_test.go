package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func vmCapacityTestState(t *testing.T) {
	t.Helper()
	oldDB, oldRedis, oldRDB, oldClient := model.DB, common.RedisEnabled, common.RDB, httpClient
	setting := operation_setting.GetChannelConcurrencySetting()
	oldSetting := *setting
	t.Setenv("CHANNEL_CONCURRENCY_ENABLED", "")
	model.DB = nil
	common.RedisEnabled = false
	common.RDB = nil
	httpClient = &http.Client{}
	setting.Enabled = true
	vmCapacityLocal.Lock()
	vmCapacityLocal.entries = map[string]vmCapacityEntry{}
	vmCapacityLocal.nextQuery = map[string]time.Time{}
	vmCapacityLocal.refreshing = map[string]bool{}
	vmCapacityLocal.Unlock()
	t.Cleanup(func() {
		model.DB = oldDB
		common.RedisEnabled = oldRedis
		common.RDB = oldRDB
		httpClient = oldClient
		*setting = oldSetting
	})
}

func vmTestChannel(base string) *model.Channel {
	setting := `{"upstream_capacity":{"enabled":true}}`
	return &model.Channel{Id: 77001, Type: constant.ChannelTypeAnthropic, Key: "fixture-only-not-a-secret", BaseURL: &base, Setting: &setting}
}

func writeVMCapacity(w http.ResponseWriter, limit, available int) {
	s := VMCapacitySnapshot{SchemaVersion: 1, Model: "claude-haiku-4-5", ObservedAt: time.Now().UTC(), MaxAgeMs: 1000, StateQuality: "known", EffectiveAdmissionLimit: limit, AvailableNow: available}
	body, _ := common.Marshal(s)
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func TestVMCapacityClientCacheMappingAndConfigIsolation(t *testing.T) {
	vmCapacityTestState(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "/v1/channel/capacity", r.URL.Path)
		require.Equal(t, "claude-haiku-4-5", r.URL.Query().Get("model"))
		require.Equal(t, "Bearer fixture-only-not-a-secret", r.Header.Get("Authorization"))
		writeVMCapacity(w, 4, 3)
	}))
	defer server.Close()
	ch := vmTestChannel(server.URL + "/v1/")
	mapping := `{"my-model":"claude-haiku-4-5"}`
	ch.ModelMapping = &mapping
	s, err := GetVMChannelCapacity(context.Background(), ch, "my-model")
	require.NoError(t, err)
	require.Equal(t, 4, s.EffectiveAdmissionLimit)
	require.Equal(t, 3, s.AvailableNow)
	_, err = GetVMChannelCapacity(context.Background(), ch, "my-model")
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	_, before := vmCapacityKeys(ch, "claude-haiku-4-5")
	ch.Key = "changed-fixture"
	_, after := vmCapacityKeys(ch, "claude-haiku-4-5")
	require.NotEqual(t, before, after)
}

func TestVMCapacityClientRejectsInvalidAndStaleResponses(t *testing.T) {
	vmCapacityTestState(t)
	for _, kind := range []string{"missing", "stale", "negative", "schema", "redirect", "status", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "missing":
					w.Write([]byte(`{"schema_version":1}`))
					return
				case "redirect":
					w.Header().Set("Location", "https://example.invalid")
					w.WriteHeader(302)
					return
				case "status":
					w.WriteHeader(503)
					return
				case "oversized":
					w.Write(make([]byte, 32769))
					return
				}
				s := VMCapacitySnapshot{SchemaVersion: 1, Model: "claude-haiku-4-5", ObservedAt: time.Now(), MaxAgeMs: 1000, StateQuality: "known", EffectiveAdmissionLimit: 4, AvailableNow: 4}
				if kind == "stale" {
					s.ObservedAt = time.Now().Add(-time.Minute)
				}
				if kind == "negative" {
					s.AvailableNow = -1
				}
				if kind == "schema" {
					s.SchemaVersion = 2
				}
				b, _ := common.Marshal(s)
				w.Write(b)
			}))
			defer server.Close()
			_, err := GetVMChannelCapacity(context.Background(), vmTestChannel(server.URL), "claude-haiku-4-5")
			require.ErrorIs(t, err, ErrVMCapacityUnavailable)
		})
	}
}

func TestVMCapacityRedisCoalescesRefreshAndFailsClosed(t *testing.T) {
	vmCapacityTestState(t)
	r := miniredis.RunT(t)
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: r.Addr(), MaxRetries: -1})
	t.Cleanup(func() { common.RDB.Close() })
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); writeVMCapacity(w, 0, 0) }))
	defer server.Close()
	ch := vmTestChannel(server.URL)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); GetVMChannelCapacity(context.Background(), ch, "claude-haiku-4-5") }()
	}
	wg.Wait()
	s, err := GetVMChannelCapacity(context.Background(), ch, "claude-haiku-4-5")
	require.NoError(t, err)
	require.Zero(t, s.EffectiveAdmissionLimit)
	require.EqualValues(t, 1, calls.Load())
	HoldVMChannelCapacity(ch, time.Second)
	_, err = GetVMChannelCapacity(context.Background(), ch, "claude-haiku-4-5")
	require.Error(t, err)
	r.Close()
	_, err = GetVMChannelCapacity(context.Background(), ch, "claude-haiku-4-5")
	require.Error(t, err)
}

func TestVMCapacityChannelValidationAndURL(t *testing.T) {
	vmCapacityTestState(t)
	ch := vmTestChannel("https://vm.example/v1")
	require.NoError(t, ValidateVMCapacityChannel(ch))
	u, err := vmCapacityURL(ch, "claude test")
	require.NoError(t, err)
	require.Equal(t, "https://vm.example/v1/channel/capacity?model=claude+test", u)
	ch.ChannelInfo.IsMultiKey = true
	require.Error(t, ValidateVMCapacityChannel(ch))
	ch.ChannelInfo.IsMultiKey = false
	ch.Type = constant.ChannelTypeOpenAI
	require.Error(t, ValidateVMCapacityChannel(ch))
	ch.Type = constant.ChannelTypeAnthropic
	bad := "https://user:pass@vm.example"
	ch.BaseURL = &bad
	require.Error(t, ValidateVMCapacityChannel(ch))
}
