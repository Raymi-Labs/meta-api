package service

import (
	"fmt"
	"html"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const routeOutageBufferLimit = 1024

// A full buffer loses only notification evidence, never the relay request.
// Counts and samples explicitly describe observed signals rather than all traffic.
type routeOutageBuffer struct {
	mu      sync.Mutex
	batches map[string]*model.RouteOutageBatch
	dropped atomic.Int64
}

func (b *routeOutageBuffer) observe(groups []string, name string, tokenID int, now int64) {
	if name == "" || len(name) > 1024 || len([]rune(name)) > 255 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.batches == nil {
		b.batches = make(map[string]*model.RouteOutageBatch)
	}
	seen := map[string]bool{}
	for i, group := range groups {
		if i >= 32 {
			break
		}
		if group == "" || group == "auto" || len(group) > 256 || len([]rune(group)) > 64 || seen[group] {
			continue
		}
		seen[group] = true
		id := model.RouteOutageID(group, name)
		batch := b.batches[id]
		if batch == nil {
			if len(b.batches) >= routeOutageBufferLimit {
				b.dropped.Add(1)
				continue
			}
			batch = &model.RouteOutageBatch{Group: group, Model: name, FirstAt: now}
			b.batches[id] = batch
		}
		batch.Count++
		batch.LastAt = now
		found := false
		for _, id := range batch.TokenIDs {
			if id == tokenID {
				found = true
				break
			}
		}
		if tokenID > 0 && !found && len(batch.TokenIDs) < 8 {
			batch.TokenIDs = append(batch.TokenIDs, tokenID)
		}
	}
}

func (b *routeOutageBuffer) drain() []model.RouteOutageBatch {
	b.mu.Lock()
	defer b.mu.Unlock()
	batches := make([]model.RouteOutageBatch, 0, len(b.batches))
	for _, batch := range b.batches {
		batches = append(batches, *batch)
	}
	b.batches = make(map[string]*model.RouteOutageBatch)
	return batches
}

var routeOutageSignals routeOutageBuffer
var routeOutageStarted atomic.Bool
var routeOutageOnce sync.Once

// ObserveUnavailableRoutes is called only after final selection has exhausted
// the caller's allowed groups. The middleware excludes restricted requests.
func ObserveUnavailableRoutes(groups []string, name string, tokenID int) {
	if routeOutageStarted.Load() {
		routeOutageSignals.observe(groups, name, tokenID, time.Now().UnixMilli())
	}
}

type routeAvailability struct {
	canonical        string
	configured       int
	available, known bool
}

// Local/Redis cooldowns and per-user circuits may differ between replicas.
// Only persistent routing state can prove an outage in this shared table.
func persistedRouteSelectable(channel *model.Channel) bool {
	if channel == nil || channel.Status != common.ChannelStatusEnabled || channel.HasPendingDisable() {
		return false
	}
	if !channel.ChannelInfo.IsMultiKey {
		return true
	}
	size := channel.ChannelInfo.MultiKeySize
	if size <= 0 {
		size = len(channel.GetKeys())
	}
	for index := 0; index < size; index++ {
		status, exists := channel.ChannelInfo.MultiKeyStatusList[index]
		if (!exists || status == common.ChannelStatusEnabled) && !channel.HasPendingDisableKey(index) {
			return true
		}
	}
	return false
}

func inspectRouteAvailability(group, name string) (routeAvailability, error) {
	channels, canonical, known, err := model.ConfiguredRouteChannels(group, name)
	if err != nil || !known {
		return routeAvailability{}, err
	}
	result := routeAvailability{configured: len(channels), canonical: canonical, known: true}
	for _, channel := range channels {
		if persistedRouteSelectable(channel) {
			result.available = true
			break
		}
	}
	return result, nil
}

func processRouteOutageSignals(batches []model.RouteOutageBatch, now int64) {
	for _, batch := range batches {
		state, err := inspectRouteAvailability(batch.Group, batch.Model)
		if err != nil || !state.known {
			continue
		}
		if state.configured == 0 || state.available {
			continue
		}
		batch.Model = state.canonical
		if err := model.RecordRouteOutage(batch, state.configured, now); err != nil {
			common.SysError("route outage signal persistence failed")
		}
	}
}

func checkRouteOutageRecovery(now int64) error {
	rows, err := model.ListRouteOutageChecks(now)
	if err != nil {
		return err
	}
	for _, row := range rows {
		state, checkErr := inspectRouteAvailability(row.GroupName, row.ModelName)
		if checkErr == nil && state.known && (state.available || state.configured == 0) {
			if err := model.ResolveRouteOutage(row.ID, now, state.configured > 0); err != nil {
				return err
			}
		} else if err := model.DeferRouteOutageCheck(row.ID, now); err != nil {
			return err
		}
	}
	return nil
}

func routeOutageMessage(row *model.RouteOutage, medium string) dto.Notify {
	var samples []int
	_ = common.UnmarshalJsonStr(row.TokenSamples, &samples)
	title := "分组模型已无可用渠道"
	detail := fmt.Sprintf("分组：%s；模型：%s。已配置 %d 条渠道，后台核对到持久化路由状态均不可选（停用、待禁用或账号状态）。", quotaNoticeText(row.GroupName, 64), quotaNoticeText(row.ModelName, 255), row.ChannelCount)
	if !row.Active {
		title = "分组模型路由恢复可选"
		detail = fmt.Sprintf("分组：%s；模型：%s。持久化路由配置已恢复可选；各节点临时冷却仍可能生效，这不是上游模型成功调用证明。", quotaNoticeText(row.GroupName, 64), quotaNoticeText(row.ModelName, 255))
	}
	content := strings.Join([]string{
		detail,
		fmt.Sprintf("本次已观察选渠失败 %d 次；首次/末次（UTC）：%s / %s。", row.FailureCount, time.UnixMilli(row.FirstFailureAt).UTC().Format(time.RFC3339), time.UnixMilli(row.LastFailureAt).UTC().Format(time.RFC3339)),
		fmt.Sprintf("已观察到的令牌 ID 样本（最多 8 个）：%v。", samples),
		"统计只包含本功能收集并持久化的失败信号，不代表全部请求；未修改渠道、分组权限、令牌回退或计费。请在渠道管理中核对停用、待禁用、额度和冷却状态。",
	}, "\n")
	if medium == "" || medium == dto.NotifyTypeEmail {
		content = strings.ReplaceAll(html.EscapeString(content), "\n", "<br/>")
	}
	return dto.NewNotify("route_outage", title, content, nil)
}

func deliverRouteOutageNotice(row *model.RouteOutage) string {
	root := model.GetRootUser()
	if root == nil || root.Id <= 0 || root.Status != common.UserStatusEnabled {
		return model.QuotaNoticeSkipped
	}
	settings := root.GetSetting()
	allowed, err := CheckNotificationLimit(root.Id, "route_outage")
	if err != nil || !allowed {
		return model.QuotaNoticeFailed
	}
	return SendUserNotificationTracked(root.Email, settings, routeOutageMessage(row, settings.NotifyType)).Status
}

// The final verification prevents an obsolete queued outage from being sent
// after an operator restores routing. Each sender owns a durable expiring lease.
func sendRouteOutageNotices(scanAt int64, deliver func(*model.RouteOutage) string) error {
	return sendRouteOutageNoticesWithClock(scanAt, deliver, func() int64 { return time.Now().UnixMilli() })
}

func sendRouteOutageNoticesWithClock(scanAt int64, deliver func(*model.RouteOutage) string, clock func() int64) error {
	rows, err := model.ListRouteOutageNotices(scanAt)
	if err != nil {
		return err
	}
	for _, row := range rows {
		state, err := inspectRouteAvailability(row.GroupName, row.ModelName)
		// Earlier deliveries in this batch can be slow. Timestamp this row
		// after its availability check so its lease starts when acquired.
		now := clock()
		if err != nil || !state.known {
			if err := model.DeferRouteOutageNotice(row.ID, now); err != nil {
				return err
			}
			continue
		}
		if state.configured == 0 {
			if err := model.RetireRouteOutage(row.ID); err != nil {
				return err
			}
			continue
		}
		if row.Active && (state.available || state.configured == 0) {
			if err := model.ResolveRouteOutage(row.ID, now, state.configured > 0); err != nil {
				return err
			}
			continue
		}
		// A recovered notification must still reflect a selectable route.
		if !row.Active && !state.available {
			if err := model.DeferRouteOutageNotice(row.ID, now); err != nil {
				return err
			}
			continue
		}
		claim, err := model.ClaimRouteOutageNotice(row.ID, now, common.GetUUID())
		if err != nil {
			return err
		}
		if claim == nil {
			continue
		}
		// Another worker may resolve or reopen the incident between the
		// availability check and lease acquisition. Recheck its new state on
		// the next pass rather than sending a differently scoped message.
		if claim.Active != row.Active || claim.Generation != row.Generation {
			if err := model.FinishRouteOutageNotice(claim, clock(), model.QuotaNoticeSkipped); err != nil {
				return err
			}
			continue
		}
		result := deliver(claim)
		if err := model.FinishRouteOutageNotice(claim, clock(), result); err != nil {
			return err
		}
	}
	return nil
}

func StartRouteOutageTask() {
	routeOutageOnce.Do(func() {
		routeOutageStarted.Store(true)
		go func() {
			tick := time.NewTicker(10 * time.Second)
			defer tick.Stop()
			for now := range tick.C {
				processRouteOutageSignals(routeOutageSignals.drain(), now.UnixMilli())
				if err := checkRouteOutageRecovery(now.UnixMilli()); err != nil {
					common.SysError("route outage recovery check failed")
				}
				if err := model.CleanupRouteOutages(now.UnixMilli()); err != nil {
					common.SysError("route outage cleanup failed")
				}
				if dropped := routeOutageSignals.dropped.Swap(0); dropped > 0 {
					common.SysError(fmt.Sprintf("route outage buffer dropped %d signals", dropped))
				}
			}
		}()
		go func() {
			tick := time.NewTicker(15 * time.Second)
			defer tick.Stop()
			for now := range tick.C {
				if err := sendRouteOutageNotices(now.UnixMilli(), deliverRouteOutageNotice); err != nil {
					common.SysError("route outage notification processing failed")
				}
			}
		}()
	})
}
