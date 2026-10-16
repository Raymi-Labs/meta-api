package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var logExportLocks sync.Map

// The export has a 45-second deadline; the distributed lock outlives it even
// if a process dies. Compare-and-delete cannot release a later owner's lock.
func AcquireLogExportLock(ctx context.Context, userID int) (func(), bool, error) {
	if !common.RedisEnabled {
		if _, loaded := logExportLocks.LoadOrStore(userID, struct{}{}); loaded {
			return nil, false, nil
		}
		return func() { logExportLocks.Delete(userID) }, true, nil
	}
	key, owner := fmt.Sprintf("log_export:v1:%d", userID), common.GetUUID()
	client := common.RDB
	if client == nil {
		return nil, false, fmt.Errorf("redis client unavailable")
	}
	acquired, err := client.SetNX(ctx, key, owner, time.Minute).Result()
	if err != nil || !acquired {
		return nil, acquired, err
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		client.Eval(releaseCtx, `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end return 0`, []string{key}, owner)
	}, true, nil
}
