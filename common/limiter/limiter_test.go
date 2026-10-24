package limiter

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestRedisLimiterRecoversAfterScriptFlush(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	sha, err := client.ScriptLoad(ctx, rateLimitScript).Result()
	require.NoError(t, err)
	limiter := &RedisLimiter{client: client, limitScriptSHA: sha}
	allow := func(key string) bool {
		allowed, err := limiter.Allow(ctx, key, WithCapacity(2), WithRate(0))
		require.NoError(t, err)
		return allowed
	}
	require.True(t, allow("test:flush"))
	require.NoError(t, client.ScriptFlush(ctx).Err())
	require.True(t, allow("test:flush"), "the existing bucket survives a script cache flush")
	require.False(t, allow("test:flush"), "recovery must not reset the bucket")
	exists, err := client.ScriptExists(ctx, sha).Result()
	require.NoError(t, err)
	require.Equal(t, []bool{true}, exists)
	require.NoError(t, client.ScriptFlush(ctx).Err())
	require.False(t, allow("test:flush"), "a denied request must also recover the script")
	require.True(t, allow("test:new"))
}

func TestRedisLimiterKeepsRedisErrors(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	sha, err := client.ScriptLoad(ctx, rateLimitScript).Result()
	require.NoError(t, err)
	limiter := &RedisLimiter{client: client, limitScriptSHA: sha}
	for _, flush := range []bool{false, true} {
		if flush {
			require.NoError(t, client.ScriptFlush(ctx).Err())
		}
		require.NoError(t, client.Set(ctx, "test:invalid", "not a bucket", 0).Err())
		allowed, err := limiter.Allow(ctx, "test:invalid")
		require.False(t, allowed)
		require.ErrorContains(t, err, "WRONGTYPE")
	}
}
