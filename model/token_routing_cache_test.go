package model

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestTokenRoutingRedisCompatibility(t *testing.T) {
	address := os.Getenv("TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("TEST_REDIS_ADDR not configured")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	oldClient, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	token := Token{Id: 1, Key: fmt.Sprintf("token-route-test-%d", time.Now().UnixNano()), Group: "default", RemainQuota: 12345, RoutingConfig: `{"groups":["default","vip"]}`}
	t.Cleanup(func() {
		_ = cacheDeleteToken(token.Key)
		common.RDB, common.RedisEnabled = oldClient, oldEnabled
		_ = client.Close()
	})
	require.NoError(t, cacheSetToken(token))
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	require.Equal(t, token.RoutingConfig, cached.RoutingConfig)
	require.Equal(t, token.RemainQuota, cached.RemainQuota)
	// A hash written by the previous application version has no new field.
	key := "token:" + common.GenerateHMAC(token.Key)
	require.NoError(t, client.HDel(t.Context(), key, "RoutingConfig").Err())
	cached, err = cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	require.Empty(t, cached.RoutingConfig)
	require.Equal(t, token.RemainQuota, cached.RemainQuota)
}
