package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	userScopedCircuitPrefix     = "channel:user_scoped_circuit"
	userScopedCircuitOpenPrefix = userScopedCircuitPrefix + ":open"
	userScopedCircuitFailPrefix = userScopedCircuitPrefix + ":fails"
)

type userScopedCircuitLocalEntry struct {
	count int
	until time.Time
}

var (
	userScopedCircuitMu    sync.Mutex
	userScopedCircuitOpen  = map[string]time.Time{}
	userScopedCircuitFails = map[string]userScopedCircuitLocalEntry{}
)

func IsChannelUnavailableForRequestContext(c *gin.Context, channel *model.Channel) bool {
	return IsChannelUnavailableForRequest(channel) || IsUserScopedCircuitOpen(c, channel)
}

func IsUserScopedCircuitOpen(c *gin.Context, channel *model.Channel) bool {
	scopeKey := userScopedCircuitScopeKey(c, channel)
	if scopeKey == "" {
		return false
	}
	key := userScopedCircuitRedisKey(userScopedCircuitOpenPrefix, scopeKey)
	if userScopedCircuitRedisEnabled() {
		_, err := common.RedisGet(key)
		return err == nil
	}
	return isLocalUserScopedCircuitOpen(key)
}

func RecordUserScopedCircuitFailure(c *gin.Context, channel *model.Channel, err *types.NewAPIError) bool {
	if !shouldRecordUserScopedCircuitFailure(c, channel, err) {
		return false
	}
	scopeKey := userScopedCircuitScopeKey(c, channel)
	if scopeKey == "" {
		return false
	}
	ttl := userScopedCircuitTTL()
	if ttl <= 0 {
		return false
	}
	threshold := userScopedCircuitFailureThreshold()
	if userScopedCircuitRedisEnabled() {
		return recordRedisUserScopedCircuitFailure(scopeKey, ttl, threshold)
	}
	return recordLocalUserScopedCircuitFailure(scopeKey, ttl, threshold)
}

func ClearUserScopedCircuit(c *gin.Context, channel *model.Channel) {
	scopeKey := userScopedCircuitScopeKey(c, channel)
	if scopeKey == "" {
		return
	}
	openKey := userScopedCircuitRedisKey(userScopedCircuitOpenPrefix, scopeKey)
	failKey := userScopedCircuitRedisKey(userScopedCircuitFailPrefix, scopeKey)
	if userScopedCircuitRedisEnabled() {
		_ = common.RedisDel(openKey)
		_ = common.RedisDel(failKey)
		return
	}
	userScopedCircuitMu.Lock()
	delete(userScopedCircuitOpen, openKey)
	delete(userScopedCircuitFails, failKey)
	userScopedCircuitMu.Unlock()
}

func shouldRecordUserScopedCircuitFailure(c *gin.Context, channel *model.Channel, err *types.NewAPIError) bool {
	if c == nil || channel == nil || err == nil || types.IsSkipRetryError(err) || RelayErrorSource(err) == "local" {
		return false
	}
	if IsRetryableUpstreamOverloadError(err) {
		return false
	}
	if !operation_setting.ShouldUseUserScopedCircuitBreakerByStatusCode(err.StatusCode) {
		return false
	}
	return userScopedCircuitScopeKey(c, channel) != ""
}

func userScopedCircuitScopeKey(c *gin.Context, channel *model.Channel) string {
	if c == nil || channel == nil {
		return ""
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if userID <= 0 {
		return ""
	}
	tag := strings.TrimSpace(channel.GetTag())
	if tag == "" {
		return ""
	}
	return fmt.Sprintf("user:%d|tag:%s", userID, tag)
}

func userScopedCircuitTTL() time.Duration {
	seconds := operation_setting.NormalizeUserScopedCircuitBreakerTTLSeconds(
		operation_setting.UserScopedCircuitBreakerTTLSeconds,
	)
	return time.Duration(seconds) * time.Second
}

func userScopedCircuitFailureThreshold() int {
	return operation_setting.NormalizeUserScopedCircuitBreakerFailureThreshold(
		operation_setting.UserScopedCircuitBreakerFailureThreshold,
	)
}

func userScopedCircuitRedisEnabled() bool {
	return common.RedisEnabled && common.RDB != nil
}

func userScopedCircuitRedisKey(prefix string, scopeKey string) string {
	return fmt.Sprintf("%s:%s", prefix, common.GenerateHMAC(scopeKey))
}

func recordRedisUserScopedCircuitFailure(scopeKey string, ttl time.Duration, threshold int) bool {
	ctx := context.Background()
	failKey := userScopedCircuitRedisKey(userScopedCircuitFailPrefix, scopeKey)
	txn := common.RDB.TxPipeline()
	incrCmd := txn.Incr(ctx, failKey)
	txn.Expire(ctx, failKey, ttl)
	if _, err := txn.Exec(ctx); err != nil {
		return false
	}
	if int(incrCmd.Val()) < threshold {
		return false
	}
	openKey := userScopedCircuitRedisKey(userScopedCircuitOpenPrefix, scopeKey)
	_ = common.RedisDel(failKey)
	return common.RedisSet(openKey, "1", ttl) == nil
}

func recordLocalUserScopedCircuitFailure(scopeKey string, ttl time.Duration, threshold int) bool {
	now := time.Now()
	openKey := userScopedCircuitRedisKey(userScopedCircuitOpenPrefix, scopeKey)
	failKey := userScopedCircuitRedisKey(userScopedCircuitFailPrefix, scopeKey)
	userScopedCircuitMu.Lock()
	defer userScopedCircuitMu.Unlock()
	if until, ok := userScopedCircuitOpen[openKey]; ok && now.Before(until) {
		return true
	}
	entry := userScopedCircuitFails[failKey]
	if now.After(entry.until) {
		entry.count = 0
	}
	entry.count++
	entry.until = now.Add(ttl)
	if entry.count < threshold {
		userScopedCircuitFails[failKey] = entry
		return false
	}
	delete(userScopedCircuitFails, failKey)
	userScopedCircuitOpen[openKey] = now.Add(ttl)
	return true
}

func isLocalUserScopedCircuitOpen(key string) bool {
	now := time.Now()
	userScopedCircuitMu.Lock()
	defer userScopedCircuitMu.Unlock()
	until, ok := userScopedCircuitOpen[key]
	if !ok {
		return false
	}
	if now.Before(until) {
		return true
	}
	delete(userScopedCircuitOpen, key)
	return false
}

func resetUserScopedCircuitForTest() {
	userScopedCircuitMu.Lock()
	defer userScopedCircuitMu.Unlock()
	userScopedCircuitOpen = map[string]time.Time{}
	userScopedCircuitFails = map[string]userScopedCircuitLocalEntry{}
}
