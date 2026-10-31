package model

import (
	"github.com/bytedance/gopkg/util/gopool"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestTokenQuotaFailureDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			quotaNoticeTestDB(t, dialect)
			oldRedis := common.RedisEnabled
			common.RedisEnabled = false
			t.Cleanup(func() { common.RedisEnabled = oldRedis })
			require.NoError(t, DB.AutoMigrate(&Token{}))
			require.NoError(t, DB.Unscoped().Where("id IN ?", []int{900081, 900082}).Delete(&Token{}).Error)
			token := Token{Id: 900081, UserId: 900081, Key: "auto-disable-fixture", ExpiredTime: -1, RemainQuota: 100}
			require.NoError(t, token.Insert())
			other := Token{Id: 900082, UserId: 900081, Key: "auto-disable-other", ExpiredTime: -1, RemainQuota: 100}
			require.NoError(t, other.Insert())
			read := func() Token { var got Token; require.NoError(t, DB.First(&got, token.Id).Error); return got }
			record := func(fail bool) { require.NoError(t, RecordTokenQuotaOutcome(token.Id, 0, fail)) }
			record(true)
			record(true)
			require.Equal(t, common.TokenStatusEnabled, read().Status)
			require.Equal(t, 2, read().QuotaFailureCount)
			record(false)
			require.Zero(t, read().QuotaFailureCount)
			record(true)
			record(true)
			record(true)
			require.Equal(t, common.TokenStatusDisabled, read().Status)
			record(false)
			require.Equal(t, 3, read().QuotaFailureCount, "in-flight success cannot re-enable")
			require.NoError(t, DB.First(&other, other.Id).Error)
			require.Equal(t, common.TokenStatusEnabled, other.Status)
			require.Zero(t, other.QuotaFailureCount)
			fresh := read()
			fresh.Status = common.TokenStatusEnabled
			require.NoError(t, fresh.Update())
			require.Zero(t, read().QuotaFailureCount)
			require.EqualValues(t, 1, read().QuotaFailureVersion)
			record(true) // late result from before manual enable
			require.Zero(t, read().QuotaFailureCount)
			var wg sync.WaitGroup
			errs := make(chan error, 24)
			for i := 0; i < 24; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- RecordTokenQuotaOutcome(token.Id, 1, true) }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				require.NoError(t, err)
			}
			require.Equal(t, 3, read().QuotaFailureCount)
			require.Equal(t, common.TokenStatusDisabled, read().Status)
			// A delayed exhaustion/expiry write must not overwrite automatic disable.
			fresh.Status = common.TokenStatusExhausted
			require.NoError(t, fresh.SelectUpdate())
			require.Equal(t, common.TokenStatusDisabled, read().Status)
			require.NoError(t, DB.Model(&Token{}).Where("id = ?", other.Id).Update("status", common.TokenStatusExpired).Error)
			require.NoError(t, RecordTokenQuotaOutcome(other.Id, 0, true))
			require.NoError(t, DB.First(&other, other.Id).Error)
			require.Zero(t, other.QuotaFailureCount)
		})
	}
}

func TestTokenQuotaFailureStaleRedisCannotReenable(t *testing.T) {
	quotaNoticeTestDB(t, "sqlite")
	require.NoError(t, DB.AutoMigrate(&Token{}))
	oldRedis, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	server := miniredis.RunT(t)
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, 5*time.Second, 10*time.Millisecond)
		common.RDB.Close()
		common.RedisEnabled, common.RDB = oldRedis, oldRDB
	})
	token := Token{Id: 900081, UserId: 900081, Key: "stale-quota-fixture", Status: common.TokenStatusEnabled, RemainQuota: 100, ExpiredTime: -1}
	require.NoError(t, token.Insert())
	require.NoError(t, cacheSetToken(token))
	for i := 0; i < 3; i++ {
		require.NoError(t, RecordTokenQuotaOutcome(token.Id, 0, true))
	}
	require.NoError(t, cacheSetToken(token)) // delayed old cache writer
	got, err := ValidateUserToken(token.Key)
	require.ErrorContains(t, err, "连续三次")
	require.Equal(t, common.TokenStatusDisabled, got.Status)
	// Cache outage cannot restore access; use DB and preserve failure reason.
	server.Close()
	_, err = ValidateUserToken(token.Key)
	require.ErrorContains(t, err, "连续三次")
}
