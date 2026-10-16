package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestLogExportLock(t *testing.T) {
	for _, useRedis := range []bool{false, true} {
		oldEnabled, oldDB := common.RedisEnabled, common.RDB
		srv := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
		common.RedisEnabled, common.RDB = useRedis, client
		ctx := context.Background()
		release, ok, err := AcquireLogExportLock(ctx, 1)
		require.NoError(t, err)
		require.True(t, ok)
		_, ok, err = AcquireLogExportLock(ctx, 1)
		require.NoError(t, err)
		require.False(t, ok)
		releaseOther, ok, err := AcquireLogExportLock(ctx, 2)
		require.NoError(t, err)
		require.True(t, ok)
		releaseOther()
		if useRedis {
			srv.FastForward(time.Minute)
			later, ok, err := AcquireLogExportLock(ctx, 1)
			require.NoError(t, err)
			require.True(t, ok)
			release()
			_, ok, err = AcquireLogExportLock(ctx, 1)
			require.NoError(t, err)
			require.False(t, ok)
			later()
		} else {
			release()
		}
		release, ok, err = AcquireLogExportLock(ctx, 1)
		require.NoError(t, err)
		require.True(t, ok)
		release()
		common.RedisEnabled, common.RDB = oldEnabled, oldDB
		client.Close()
	}
}
