package model

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
)

// GroupHealthMetric is an absolute snapshot of one process-local hourly bucket.
// Its random ID makes snapshots idempotent, including a retry after an ambiguous
// database timeout; other replicas have separate IDs and are summed at query time.
// No channel, account, token, user, prompt, or request identifier is stored.
type GroupHealthMetric struct {
	ID                     string `gorm:"type:varchar(36);primaryKey"`
	GroupName              string `gorm:"size:64;index:idx_group_health_window,priority:1"`
	ModelName              string `gorm:"size:255;index:idx_group_health_model,priority:1"`
	BucketTs               int64  `gorm:"index:idx_group_health_window,priority:2;index:idx_group_health_model,priority:2;index"`
	RequestCount           int64
	SuccessCount           int64
	ServiceFailureCount    int64 `gorm:"not null;default:0"`
	ExcludedUserErrorCount int64 `gorm:"not null;default:0"`
	ClientCanceledCount    int64 `gorm:"not null;default:0"`
	TTFTSumMs              int64
	TTFTCount              int64
	CompletionSumMs        int64
	CompletionCount        int64
	CacheReadTokens        int64 `gorm:"default:0"`
	CacheInputTokens       int64 `gorm:"default:0"`
	CacheSampleCount       int64 `gorm:"default:0"`
	UpdatedAt              int64 `gorm:"autoUpdateTime:false"`
}

func SaveGroupHealthSnapshots(ctx context.Context, rows []GroupHealthMetric) error {
	if len(rows) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"request_count", "success_count", "service_failure_count", "excluded_user_error_count", "client_canceled_count", "ttft_sum_ms", "ttft_count",
			"completion_sum_ms", "completion_count", "cache_read_tokens", "cache_input_tokens", "cache_sample_count", "updated_at",
		}),
	}).CreateInBatches(&rows, 100).Error
}

func QueryGroupHealthMetrics(ctx context.Context, groups []string, modelName string, start, end int64) ([]GroupHealthMetric, error) {
	rows := make([]GroupHealthMetric, 0)
	if len(groups) == 0 {
		return rows, nil
	}
	query := DB.WithContext(ctx).Model(&GroupHealthMetric{}).
		Select("group_name, model_name, bucket_ts, SUM(request_count) AS request_count, SUM(success_count) AS success_count, SUM(service_failure_count) AS service_failure_count, SUM(excluded_user_error_count) AS excluded_user_error_count, SUM(client_canceled_count) AS client_canceled_count, SUM(ttft_sum_ms) AS ttft_sum_ms, SUM(ttft_count) AS ttft_count, SUM(completion_sum_ms) AS completion_sum_ms, SUM(completion_count) AS completion_count, SUM(cache_read_tokens) AS cache_read_tokens, SUM(cache_input_tokens) AS cache_input_tokens, SUM(cache_sample_count) AS cache_sample_count, MAX(updated_at) AS updated_at").
		Where("group_name IN ? AND bucket_ts >= ? AND bucket_ts <= ?", groups, start, end)
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	err := query.Group("group_name, model_name, bucket_ts").Scan(&rows).Error
	return rows, err
}

func DeleteExpiredGroupHealthMetrics(ctx context.Context, now time.Time) error {
	return DB.WithContext(ctx).Where("bucket_ts < ?", now.Add(-7*24*time.Hour).Unix()).Delete(&GroupHealthMetric{}).Error
}

// HasConfiguredGroupModel includes disabled abilities: a configured model whose
// channels are all down is an availability failure, while a typo is not a sample.
func HasConfiguredGroupModel(group, modelName string, allowedChannels []int) bool {
	if DB == nil || group == "" || group == "auto" || modelName == "" {
		return false
	}
	if allowedChannels != nil && len(allowedChannels) == 0 {
		return false
	}
	var count int64
	query := DB.Model(&Ability{}).Where(commonGroupCol+" = ? AND model = ?", group, modelName)
	if len(allowedChannels) > 0 {
		query = query.Where("channel_id IN ?", allowedChannels)
	}
	return query.Limit(1).Count(&count).Error == nil && count > 0
}
