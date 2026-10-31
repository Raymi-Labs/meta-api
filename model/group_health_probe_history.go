package model

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

var groupHealthProbeHistoryOnce sync.Once

// StartGroupHealthProbeHistoryTask fills existing evidence without holding up
// startup or adding log scans to the health API. Only the master performs work.
func StartGroupHealthProbeHistoryTask() {
	if !common.IsMasterNode {
		return
	}
	groupHealthProbeHistoryOnce.Do(func() {
		now := time.Now()
		go func() {
			for attempt := 0; attempt < 3; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				count, err := BackfillGroupHealthProbeHistory(ctx, now)
				cancel()
				if err == nil {
					common.SysLog(fmt.Sprintf("group health probe history completed: added_evidence=%d", count))
					return
				}
				common.SysError(fmt.Sprintf("group health probe history failed (attempt %d): %v", attempt+1, err))
				if attempt < 2 {
					time.Sleep(time.Minute)
				}
			}
		}()
	})
}

// BackfillGroupHealthProbeHistory adds at most one successful observation per
// configured group/model/hour. Legacy failure logs are incomplete; their absence
// is never interpreted as success. Current abilities define historical routing
// scope because the log's group belongs to the administrator, not the channel.
func BackfillGroupHealthProbeHistory(ctx context.Context, now time.Time) (int64, error) {
	var abilities []Ability
	if err := DB.WithContext(ctx).Model(&Ability{}).Select(commonGroupCol + ", model, channel_id").Find(&abilities).Error; err != nil {
		return 0, err
	}
	targets := make(map[int][]Ability)
	for _, ability := range abilities {
		if ability.ChannelId > 0 && ability.Group != "" && ability.Group != "auto" && len(ability.Group) <= 64 && ability.Model != "" && len(ability.Model) <= 255 {
			targets[ability.ChannelId] = append(targets[ability.ChannelId], ability)
		}
	}
	if len(targets) == 0 {
		return 0, nil
	}
	var inserted int64
	end := now.Unix()
	currentHour := end / 3600 * 3600
	for hour := currentHour; hour >= currentHour-71*3600; hour -= 3600 {
		var channelIDs []int
		// A narrow time range uses the existing created_at/type index, including
		// when LogDB is separate from DB. Ordinary billed calls cannot match.
		err := LOG_DB.WithContext(ctx).Model(&Log{}).
			Where("created_at >= ? AND created_at < ? AND type = ? AND token_id = ? AND token_name = ? AND content = ? AND channel_id > 0",
				hour, end, LogTypeConsume, 0, "模型测试", "模型测试").
			Distinct("channel_id").Pluck("channel_id", &channelIDs).Error
		if err != nil {
			return inserted, err
		}
		end = hour
		if len(channelIDs) == 0 {
			continue
		}
		var successful []GroupHealthMetric
		if err = DB.WithContext(ctx).Model(&GroupHealthMetric{}).
			Select("group_name, model_name").Where("bucket_ts = ? AND probe_success_count > 0", hour).Find(&successful).Error; err != nil {
			return inserted, err
		}
		type targetKey struct{ group, model string }
		seen := make(map[targetKey]bool)
		for _, row := range successful {
			seen[targetKey{row.GroupName, row.ModelName}] = true
		}
		rows := make([]GroupHealthMetric, 0)
		for _, channelID := range channelIDs {
			for _, target := range targets[channelID] {
				key := targetKey{target.Group, target.Model}
				if seen[key] {
					continue
				}
				seen[key] = true
				// Stable identity makes partial retries and concurrent masters safe.
				id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("meta-api/probe-history/v1\x00%s\x00%s\x00%d", target.Group, target.Model, hour))).String()
				rows = append(rows, GroupHealthMetric{ID: id, GroupName: target.Group, ModelName: target.Model, BucketTs: hour,
					ProbeCount: 1, ProbeSuccessCount: 1, UpdatedAt: now.Unix()})
			}
		}
		if len(rows) > 0 {
			result := DB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).CreateInBatches(&rows, 100)
			if result.Error != nil {
				return inserted, result.Error
			}
			inserted += result.RowsAffected
		}
	}
	return inserted, nil
}
