package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRouteOutageServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() { model.DB, common.RedisEnabled = oldDB, oldRedis; _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.RouteOutage{}, &model.Channel{}, &model.Ability{}))
	return db
}

func sendRouteOutageNoticesAt(now int64, deliver func(*model.RouteOutage) string) error {
	return sendRouteOutageNoticesWithClock(now, deliver, func() int64 { return now })
}

func TestRouteOutageBufferBoundedConcurrent(t *testing.T) {
	var b routeOutageBuffer
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for range 100 {
				b.observe([]string{"g", "g", "auto", ""}, "m", i+1, 10)
			}
		}(i)
	}
	wg.Wait()
	rows := b.drain()
	require.Len(t, rows, 1)
	require.EqualValues(t, 6400, rows[0].Count)
	require.Len(t, rows[0].TokenIDs, 8)
	for i := range routeOutageBufferLimit + 10 {
		b.observe([]string{fmt.Sprintf("g-%d", i)}, "m", 1, 11)
	}
	require.Len(t, b.drain(), routeOutageBufferLimit)
	require.EqualValues(t, 10, b.dropped.Load())
	require.Empty(t, b.drain())
}

func TestRouteOutageAggregateDeliveryRecovery(t *testing.T) {
	db := setupRouteOutageServiceDB(t)
	now := time.Now().UnixMilli()
	channel := model.Channel{Id: 970001, Key: "secret-fixture", Status: common.ChannelStatusAutoDisabled, Name: "private upstream"}
	require.NoError(t, db.Create(&channel).Error)
	ability := model.Ability{Group: "enterprise", Model: "grok-fixture", ChannelId: channel.Id, Enabled: false}
	require.NoError(t, db.Create(&ability).Error)
	batch := model.RouteOutageBatch{Group: ability.Group, Model: ability.Model, Count: 748, FirstAt: now - 3600000, LastAt: now, TokenIDs: []int{25439}}
	processRouteOutageSignals([]model.RouteOutageBatch{batch}, now)
	var row model.RouteOutage
	require.NoError(t, db.First(&row).Error)
	require.True(t, row.Active)
	sent := 0
	require.NoError(t, sendRouteOutageNoticesAt(now, func(r *model.RouteOutage) string {
		sent++
		msg := routeOutageMessage(r, dto.NotifyTypeWebhook)
		require.Contains(t, msg.Content, "748")
		require.Contains(t, msg.Content, "25439")
		require.NotContains(t, msg.Content, "secret-fixture")
		require.NotContains(t, msg.Content, "private upstream")
		return model.QuotaNoticeFailed
	}))
	require.Equal(t, 1, sent)
	require.NoError(t, sendRouteOutageNoticesAt(now+60000, func(*model.RouteOutage) string { sent++; return model.QuotaNoticeSubmitted }))
	require.Equal(t, 1, sent)
	require.NoError(t, db.First(&row).Error)
	require.NoError(t, sendRouteOutageNoticesAt(row.NextNotifyAt+1, func(*model.RouteOutage) string { sent++; return model.QuotaNoticeSubmitted }))
	require.Equal(t, 2, sent)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusEnabled).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", channel.Id).Update("enabled", true).Error)
	require.NoError(t, checkRouteOutageRecovery(now+10*time.Minute.Milliseconds()))
	require.NoError(t, sendRouteOutageNoticesAt(now+10*time.Minute.Milliseconds(), func(r *model.RouteOutage) string {
		sent++
		require.False(t, r.Active)
		msg := routeOutageMessage(r, dto.NotifyTypeEmail)
		require.Contains(t, msg.Title, "路由恢复可选")
		require.Contains(t, msg.Content, "不是上游模型成功调用证明")
		return model.QuotaNoticeSubmitted
	}))
	require.Equal(t, 3, sent)
	require.NoError(t, db.First(&row).Error)
	require.False(t, row.Active)
	require.False(t, row.Notified)
}

func TestRouteOutageRequiresWholeConfiguredScope(t *testing.T) {
	db := setupRouteOutageServiceDB(t)
	now := time.Now().UnixMilli()
	for _, tc := range []struct {
		name     string
		channels []model.Channel
		enabled  []bool
		outage   bool
	}{
		{"some_healthy", []model.Channel{{Id: 970011, Key: "k", Status: 1}, {Id: 970012, Key: "k", Status: 3}}, []bool{true, false}, false},
		{"all_disabled", []model.Channel{{Id: 970013, Key: "k", Status: 3}, {Id: 970014, Key: "k", Status: 2}}, []bool{false, false}, true},
		{"pending", []model.Channel{{Id: 970015, Key: "k", Status: 1, OtherInfo: `{"pre_disable_wait_until":9999999999}`}}, []bool{true}, true},
		{"multi_key_disabled", []model.Channel{{Id: 970016, Key: "k1\nk2", Status: 1, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyStatusList: map[int]int{0: 2, 1: 3}}}}, []bool{true}, true},
		{"unknown", nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i, ch := range tc.channels {
				require.NoError(t, db.Create(&ch).Error)
				require.NoError(t, db.Create(&model.Ability{Group: tc.name, Model: "m", ChannelId: ch.Id, Enabled: tc.enabled[i]}).Error)
			}
			processRouteOutageSignals([]model.RouteOutageBatch{{Group: tc.name, Model: "m", Count: 1, FirstAt: now, LastAt: now}}, now)
			var count int64
			require.NoError(t, db.Model(&model.RouteOutage{}).Where("group_name = ?", tc.name).Count(&count).Error)
			if tc.outage {
				require.EqualValues(t, 1, count)
			} else {
				require.Zero(t, count)
			}
		})
	}
}

func TestRouteOutageMessageEscapesUntrustedLabels(t *testing.T) {
	row := &model.RouteOutage{Active: true, GroupName: "<script>\nforged", ModelName: "<img src=x onerror=1>", TokenSamples: "[1,2]"}
	msg := routeOutageMessage(row, dto.NotifyTypeEmail)
	require.NotContains(t, msg.Content, "<script>")
	require.NotContains(t, msg.Content, "<img")
	require.Contains(t, msg.Content, "&lt;script&gt; forged")
}

func TestRouteOutageIgnoresNodeLocalCooldown(t *testing.T) {
	db := setupRouteOutageServiceDB(t)
	ch := model.Channel{Id: 970021, Key: "k", Status: 1}
	require.NoError(t, db.Create(&ch).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "local-only", Model: "m", ChannelId: ch.Id, Enabled: true}).Error)
	extendChannelRateLimitLocalCooldown(ch.Id, time.Minute, time.Now())
	t.Cleanup(func() {
		channelRateLimitCooldownMu.Lock()
		delete(channelRateLimitLocalCooldown, ch.Id)
		channelRateLimitCooldownMu.Unlock()
	})
	require.True(t, IsChannelRateLimitCoolingDown(&ch))
	state, err := inspectRouteAvailability("local-only", "m")
	require.NoError(t, err)
	require.True(t, state.available, "local cooldown cannot prove a shared outage")
}

func TestRouteOutageStaleRecoveryCannotStarveQueue(t *testing.T) {
	db := setupRouteOutageServiceDB(t)
	now := time.Now().UnixMilli()
	// Ten due recovery messages without selectable routes used to occupy every scan.
	for i := range 10 {
		g := fmt.Sprintf("stale-%d", i)
		require.NoError(t, model.RecordRouteOutage(model.RouteOutageBatch{Group: g, Model: "m", Count: 1, FirstAt: now, LastAt: now}, 1, now))
		id := model.RouteOutageID(g, "m")
		claim, err := model.ClaimRouteOutageNotice(id, now, "outage")
		require.NoError(t, err)
		require.NoError(t, model.FinishRouteOutageNotice(claim, now, model.QuotaNoticeSubmitted))
		require.NoError(t, model.ResolveRouteOutage(id, now, true))
		if i%2 == 0 {
			ch := model.Channel{Id: 970100 + i, Key: "k", Status: 3}
			require.NoError(t, db.Create(&ch).Error)
			require.NoError(t, db.Create(&model.Ability{Group: g, Model: "m", ChannelId: ch.Id, Enabled: false}).Error)
		}
	}
	ch := model.Channel{Id: 970200, Key: "k", Status: 3}
	require.NoError(t, db.Create(&ch).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "legitimate", Model: "m", ChannelId: ch.Id, Enabled: false}).Error)
	require.NoError(t, model.RecordRouteOutage(model.RouteOutageBatch{Group: "legitimate", Model: "m", Count: 1, FirstAt: now, LastAt: now}, 1, now+1))
	delivered := 0
	send := func(r *model.RouteOutage) string {
		require.Equal(t, "legitimate", r.GroupName)
		delivered++
		return model.QuotaNoticeSubmitted
	}
	require.NoError(t, sendRouteOutageNoticesAt(now+1, send))
	require.NoError(t, sendRouteOutageNoticesAt(now+2, send))
	require.Equal(t, 1, delivered)
}

func TestRouteOutageAliasesShareConfiguredModelState(t *testing.T) {
	db := setupRouteOutageServiceDB(t)
	now := time.Now().UnixMilli()
	ch := model.Channel{Id: 970300, Key: "k", Status: 3}
	require.NoError(t, db.Create(&ch).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "aliases", Model: "gpt-4-gizmo-*", ChannelId: ch.Id, Enabled: false}).Error)
	batches := []model.RouteOutageBatch{}
	for i := range 20 {
		batches = append(batches, model.RouteOutageBatch{Group: "aliases", Model: fmt.Sprintf("gpt-4-gizmo-%d", i), Count: 1, FirstAt: now, LastAt: now})
	}
	processRouteOutageSignals(batches, now)
	var rows []model.RouteOutage
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "gpt-4-gizmo-*", rows[0].ModelName)
	require.EqualValues(t, 20, rows[0].FailureCount)
}

func TestRouteOutageBatchClaimsUseCurrentClock(t *testing.T) {
	db := setupRouteOutageServiceDB(t)
	scanAt := time.Now().UnixMilli()
	for i := range 2 {
		group := fmt.Sprintf("slow-batch-%d", i)
		channel := model.Channel{Id: 970400 + i, Key: "k", Status: common.ChannelStatusAutoDisabled}
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, db.Create(&model.Ability{Group: group, Model: "m", ChannelId: channel.Id, Enabled: false}).Error)
		require.NoError(t, model.RecordRouteOutage(model.RouteOutageBatch{Group: group, Model: "m", Count: 1, FirstAt: scanAt, LastAt: scanAt}, 1, scanAt))
	}
	currentTime, delivered := scanAt, 0
	require.NoError(t, sendRouteOutageNoticesWithClock(scanAt, func(claim *model.RouteOutage) string {
		delivered++
		require.Equal(t, currentTime+2*time.Minute.Milliseconds(), claim.LeaseUntil, "each claim needs a full lease at acquisition")
		duplicate, err := model.ClaimRouteOutageNotice(claim.ID, currentTime, "another-instance")
		require.NoError(t, err)
		require.Nil(t, duplicate, "another instance must not reclaim an in-flight notification")
		if delivered == 1 {
			currentTime += 3 * time.Minute.Milliseconds()
		}
		return model.QuotaNoticeSubmitted
	}, func() int64 { return currentTime }))
	require.Equal(t, 2, delivered)
	var rows []model.RouteOutage
	require.NoError(t, db.Find(&rows).Error)
	for _, row := range rows {
		require.True(t, row.Notified)
		require.Equal(t, currentTime+30*time.Minute.Milliseconds(), row.NextNotifyAt, "cadence starts after delivery, not at the batch tick")
	}
}
