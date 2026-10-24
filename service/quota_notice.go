package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type quotaNoticeInput struct {
	UserID   int
	Resource model.QuotaNoticeResource
	Success  bool
}

var quotaNoticeQueue = make(chan quotaNoticeInput, 2048)
var quotaNoticeOnce sync.Once
var quotaNoticeRunning atomic.Bool
var quotaNoticeDropped atomic.Int64

func quotaNoticeScope(source string, id int, modelName string) string {
	if source == BillingSourceSubscription && id == 0 {
		return fmt.Sprintf("subscription:eligible:%x", sha256.Sum256([]byte(modelName)))
	}
	return fmt.Sprintf("%s:%d", source, id)
}

func enqueueQuotaNotice(input quotaNoticeInput) {
	if !quotaNoticeRunning.Load() {
		return
	}
	select {
	case quotaNoticeQueue <- input:
	default:
		quotaNoticeDropped.Add(1)
	}
}

// MarkQuotaNoticeSettlement only records a paid settlement. The final-result
// observer decides whether the enclosing request actually succeeded.
func MarkQuotaNoticeSettlement(c *gin.Context, info *relaycommon.RelayInfo) {
	if c == nil || info == nil {
		return
	}
	source := info.BillingSource
	if source == "" {
		source = BillingSourceWallet
	}
	items := []model.QuotaNoticeResource{{Source: source, ResourceID: 0}}
	if info.BillingSource == BillingSourceSubscription {
		items = append(items, model.QuotaNoticeResource{Source: BillingSourceSubscription, ResourceID: info.SubscriptionId})
	}
	if info.TokenId > 0 {
		items = append(items, model.QuotaNoticeResource{Source: "token", ResourceID: info.TokenId}, model.QuotaNoticeResource{Source: "token_package", ResourceID: info.TokenId})
	}
	for i := range items {
		items[i].ModelName = info.OriginModelName
		items[i].TokenID = info.TokenId
		items[i].Scope = quotaNoticeScope(items[i].Source, items[i].ResourceID, info.OriginModelName)
	}
	c.Set("quota_notice_settled", items)
}

// Exhausted tokens fail before normal authorization completes. Check identity,
// expiry, IP, group and model restrictions before permitting a notification.
// Decode only a bounded model envelope, never retain any prompt or credential.
func MarkQuotaNoticeAuthRejection(c *gin.Context, token *model.Token, err error) {
	var rejected *types.QuotaRejection
	if token == nil || !errors.As(err, &rejected) || c.Request.Method != http.MethodPost {
		return
	}
	if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
		return
	}
	if token.Status != common.TokenStatusEnabled && token.Status != common.TokenStatusExhausted {
		return
	}
	if ips := token.GetIpLimits(); len(ips) > 0 && !common.IsIpInCIDRList(net.ParseIP(c.ClientIP()), ips) {
		return
	}
	u, e := model.GetUserCache(token.UserId)
	if e != nil || u.Status != common.UserStatusEnabled || !u.GetSetting().QuotaBlockedNotificationsEnabled() {
		return
	}
	if token.Group != "" {
		if _, ok := GetUserUsableGroups(u.Group)[token.Group]; !ok {
			return
		}
	}
	var envelope struct {
		Model string `json:"model"`
	}
	if c.Request.Body != nil {
		_ = common.DecodeJson(io.LimitReader(c.Request.Body, 1<<20), &envelope)
	}
	if token.ModelLimitsEnabled && !token.GetModelLimitsMap()[envelope.Model] {
		return
	}
	// Channel overrides require the same admin privilege as SetupContextForToken.
	if c.GetString("quota_notice_specific_channel") != "" && !model.IsAdmin(token.UserId) {
		return
	}
	c.Set("quota_notice_auth_rejection", rejected)
	c.Set("quota_notice_model", envelope.Model)
}

func ObserveQuotaNoticeOutcome(c *gin.Context) {
	if c == nil || c.GetInt("id") <= 0 || c.Request == nil || c.Request.Method != http.MethodPost {
		return
	}
	if done, _ := c.Get("quota_notice_observed"); done == true {
		return
	}
	c.Set("quota_notice_observed", true)
	now := time.Now().UnixMilli()
	requestID := common.GetContextKeyString(c, common.RequestIdKey)
	if requestID == "" {
		return
	}
	if RequestSucceeded(c) {
		value, _ := c.Get("quota_notice_settled")
		items, _ := value.([]model.QuotaNoticeResource)
		for _, r := range items {
			r.LastAt = now
			r.RequestID = requestID
			enqueueQuotaNotice(quotaNoticeInput{c.GetInt("id"), r, true})
		}
		return
	}
	var rejected *types.QuotaRejection
	value, _ := c.Get(requestFailureKey)
	apiErr, _ := value.(*types.NewAPIError)
	if apiErr != nil && apiErr.GetErrorType() == types.ErrorTypeNewAPIError {
		errors.As(apiErr, &rejected)
	}
	if rejected == nil {
		value, _ = c.Get("quota_notice_auth_rejection")
		rejected, _ = value.(*types.QuotaRejection)
	}
	if rejected == nil {
		return
	}
	name := c.GetString("original_model")
	if name == "" {
		name = c.GetString("quota_notice_model")
	}
	r := model.QuotaNoticeResource{Source: rejected.Source, ResourceID: rejected.ResourceID, Available: rejected.Available, Required: rejected.Required, Known: rejected.Known, TokenID: c.GetInt("token_id"), TokenName: quotaNoticeText(c.GetString("token_name"), 80), ModelName: quotaNoticeText(name, 120), RequestID: requestID, LastAt: now}
	r.Scope = quotaNoticeScope(r.Source, r.ResourceID, r.ModelName)
	enqueueQuotaNotice(quotaNoticeInput{c.GetInt("id"), r, false})
}

func quotaNoticeText(s string, n int) string {
	r := []rune(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

func StartQuotaNoticeTask() {
	quotaNoticeOnce.Do(func() {
		quotaNoticeRunning.Store(true)
		for range 2 {
			go func() {
				for input := range quotaNoticeQueue {
					if time.Now().UnixMilli()-input.Resource.LastAt > 5*60*1000 {
						quotaNoticeDropped.Add(1)
						continue
					}
					inserted, total, err := model.RecordQuotaNoticeSignal(input.UserID, input.Resource, input.Success)
					if err != nil {
						common.SysError("quota notice signal persistence failed")
						continue
					}
					if inserted {
						cacheQuotaNoticeSignal(input.UserID, input.Resource, total)
					}
				}
			}()
		}
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if dropped := quotaNoticeDropped.Swap(0); dropped > 0 {
					common.SysError(fmt.Sprintf("quota notice queue dropped %d signals", dropped))
				}
				if err := processQuotaNoticeCandidates(time.Now().UnixMilli()); err != nil {
					common.SysError("quota notice worker failed: " + err.Error())
				}
			}
		}()
		for range 2 {
			go func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for range ticker.C {
					sendPendingQuotaNotices()
				}
			}()
		}
	})
}

func processQuotaNoticeCandidates(now int64) error {
	if err := model.RecoverQuotaNoticeLeases(now); err != nil {
		return err
	}
	var accounts []model.QuotaNoticeAccount
	if err := model.DB.Select("user_id").Where("next_check_at <= ? AND last_activity_at > 0", now).Order("next_check_at asc").Limit(100).Find(&accounts).Error; err != nil {
		return err
	}
	for _, a := range accounts {
		if _, err := model.PrepareQuotaNotice(a.UserID, now, readQuotaNoticeCache); err != nil {
			return err
		}
	}
	return model.CleanupQuotaNotices(now)
}

func sendPendingQuotaNotices() {
	var pending []model.QuotaNotice
	if err := model.DB.Select("id").Where("status = ?", model.QuotaNoticePending).Order("id asc").Limit(20).Find(&pending).Error; err != nil {
		return
	}
	for _, row := range pending {
		n, err := model.ClaimQuotaNotice(row.ID, time.Now().UnixMilli())
		if err != nil || n == nil {
			continue
		}
		status, reason, medium, recipient := deliverQuotaNotice(n)
		if err = model.FinishQuotaNotice(n.ID, time.Now().UnixMilli(), status, reason, medium, recipient); err != nil {
			common.SysError(fmt.Sprintf("quota notice %d outcome persistence failed", n.ID))
		}
	}
}

func deliverQuotaNotice(n *model.QuotaNotice) (string, string, string, string) {
	u, err := model.GetUserById(n.UserID, true)
	if err != nil {
		return model.QuotaNoticeFailed, "user_lookup_failed", "", ""
	}
	settings := u.GetSetting()
	if u.Status != common.UserStatusEnabled || !settings.QuotaBlockedNotificationsEnabled() {
		return model.QuotaNoticeSkipped, "notifications_disabled", "", ""
	}
	var resources []model.QuotaNoticeResource
	if err = common.UnmarshalJsonStr(n.Summary, &resources); err != nil {
		return model.QuotaNoticeFailed, "invalid_summary", "", ""
	}
	medium := settings.NotifyType
	if medium == "" {
		medium = dto.NotifyTypeEmail
	}
	data := quotaNoticeMessage(n.ID, resources, medium)
	result := SendUserNotificationTracked(u.Email, settings, data)
	return result.Status, result.Reason, medium, result.Recipient
}

func quotaNoticeMessage(id int, resources []model.QuotaNoticeResource, medium string) dto.Notify {
	var total int64
	var first, last int64
	tokens := map[int]bool{}
	models := map[string]bool{}
	for _, r := range resources {
		total += r.Count
		if first == 0 || r.FirstAt < first {
			first = r.FirstAt
		}
		last = max(last, r.LastAt)
		for _, id := range r.TokenIDs {
			tokens[id] = true
		}
		if r.TokenID > 0 {
			tokens[r.TokenID] = true
		}
		for _, name := range r.ModelNames {
			models[name] = true
		}
		if r.ModelName != "" {
			models[r.ModelName] = true
		}
	}
	lines := []string{fmt.Sprintf("部分 API 请求因本地额度不足持续失败，已记录 %d 次。请在处理额度前暂停客户端自动重试。", total), fmt.Sprintf("观察区间（UTC）：%s 至 %s；记录涵盖至少 %d 个令牌、%d 个模型。", time.UnixMilli(first).UTC().Format(time.RFC3339), time.UnixMilli(last).UTC().Format(time.RFC3339), len(tokens), len(models))}
	for i, r := range resources {
		if i >= 5 {
			break
		}
		label := "账户余额不足"
		link := "/console/topup"
		switch r.Source {
		case "subscription":
			label = "适用订阅额度不足或没有有效订阅"
		case "token":
			label = "令牌额度不足"
			link = "/console/token"
		case "token_package":
			label = "令牌套餐周期额度不足，请检查重置时间"
			link = "/console/token"
		}
		line := fmt.Sprintf("%s；令牌 %s，模型 %s。", label, r.TokenName, r.ModelName)
		if r.Known {
			line += fmt.Sprintf("该次失败时剩余 %s。", logger.FormatQuota(int(r.Available)))
			if r.Required > 0 {
				line += fmt.Sprintf("需要预扣 %s；预扣额不等于实际费用。", logger.FormatQuota(int(r.Required)))
			}
		}
		line += " Request ID: " + r.RequestID + "；处理入口：" + strings.TrimRight(system_setting.ServerAddress, "/") + link
		lines = append(lines, line)
	}
	lines = append(lines, fmt.Sprintf("通知记录 #%d。此类提醒按用户合并，滚动 24 小时最多 4 次；可在个人通知设置中关闭。", id))
	content := strings.Join(lines, "\n")
	if medium == dto.NotifyTypeEmail {
		content = strings.ReplaceAll(html.EscapeString(content), "\n", "<br/>")
	}
	return dto.NewNotify(dto.NotifyTypeQuotaBlocked, "API 调用因额度不足持续失败，请检查客户端自动重试", content, nil)
}

// Redis is an optional sliding-window accelerator. A missing, incomplete or
// out-of-order cache always falls back to the indexed durable signals.
const quotaNoticeCacheWrite = `
local seq=tonumber(redis.call('HGET',KEYS[2],'seq') or '0')
if seq+1 ~= tonumber(ARGV[3]) then redis.call('HSET',KEYS[2],'since',ARGV[1]) end
redis.call('HSETNX',KEYS[2],'since',ARGV[1])
redis.call('HSET',KEYS[2],'seq',ARGV[3])
redis.call('ZADD',KEYS[1],ARGV[1],ARGV[2])
redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',tonumber(ARGV[1])-600000)
if redis.call('ZCARD',KEYS[1]) > 10000 then
 redis.call('ZREMRANGEBYRANK',KEYS[1],0,-10001)
 redis.call('HSET',KEYS[2],'since',ARGV[1])
end
redis.call('PEXPIRE',KEYS[1],1200000);redis.call('PEXPIRE',KEYS[2],1200000)
return 1`

func quotaNoticeCacheKeys(userID int, scope string) []string {
	key := fmt.Sprintf("quota_notice:{%d}:%x", userID, sha256.Sum256([]byte(scope)))
	return []string{key, key + ":meta"}
}
func cacheQuotaNoticeSignal(userID int, r model.QuotaNoticeResource, total int64) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = common.RDB.Eval(ctx, quotaNoticeCacheWrite, quotaNoticeCacheKeys(userID, r.Scope), r.LastAt, r.RequestID, total).Err()
}

const quotaNoticeCacheRead = `
if tonumber(redis.call('HGET',KEYS[2],'seq') or '-1') ~= tonumber(ARGV[3]) then return {} end
if tonumber(redis.call('HGET',KEYS[2],'since') or ARGV[2]) > tonumber(ARGV[1]) then return {} end
local lo='('..ARGV[1]
local first=redis.call('ZRANGEBYSCORE',KEYS[1],lo,ARGV[2],'WITHSCORES','LIMIT',0,1)
if #first==0 then return {0,0,0} end
local last=redis.call('ZREVRANGEBYSCORE',KEYS[1],ARGV[2],lo,'WITHSCORES','LIMIT',0,1)
return {redis.call('ZCOUNT',KEYS[1],lo,ARGV[2]),tonumber(first[2]),tonumber(last[2])}`

func readQuotaNoticeCache(userID int, r model.QuotaNoticeResource, since, now int64) (model.QuotaNoticeWindowStats, bool) {
	if !common.RedisEnabled || common.RDB == nil {
		return model.QuotaNoticeWindowStats{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	v, err := common.RDB.Eval(ctx, quotaNoticeCacheRead, quotaNoticeCacheKeys(userID, r.Scope), since, now, r.Count).Slice()
	if err != nil || len(v) != 3 {
		return model.QuotaNoticeWindowStats{}, false
	}
	a, ok1 := v[0].(int64)
	b, ok2 := v[1].(int64)
	c, ok3 := v[2].(int64)
	return model.QuotaNoticeWindowStats{Count: a, FirstAt: b, LastAt: c}, ok1 && ok2 && ok3
}
