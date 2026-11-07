package model

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm/clause"
)

// RouteAttemptMetric counts every upstream attempt per channel/group/model
// minute. Unlike GroupHealthMetric, retries are never folded into the final
// request result, so a failed first attempt stays visible after a retry
// succeeds. Each process writes its own UUID rows; queries sum replicas.
type RouteAttemptMetric struct {
	ID                    string `gorm:"type:varchar(36);primaryKey" json:"-"`
	BucketTs              int64  `gorm:"index:idx_route_attempt_window,priority:1" json:"bucket_ts"`
	ChannelID             int    `gorm:"index:idx_route_attempt_window,priority:2" json:"channel_id"`
	GroupName             string `gorm:"size:64" json:"group"`
	ModelName             string `gorm:"size:255" json:"model"`
	AttemptCount          int64  `gorm:"not null;default:0" json:"attempts"`
	SuccessCount          int64  `gorm:"not null;default:0" json:"success"`
	ServiceFailureCount   int64  `gorm:"not null;default:0" json:"service_failure"`
	RateLimitedCount      int64  `gorm:"not null;default:0" json:"rate_limited"`
	CapacityRejectedCount int64  `gorm:"not null;default:0" json:"capacity_rejected"`
	UnknownFailureCount   int64  `gorm:"not null;default:0" json:"unknown_failure"`
	UserErrorCount        int64  `gorm:"not null;default:0" json:"user_error"`
	ClientCanceledCount   int64  `gorm:"not null;default:0" json:"client_canceled"`
	LocalDeliveryCount    int64  `gorm:"not null;default:0" json:"local_delivery"`
	// Histograms are JSON integer arrays over fixed millisecond bounds so P95
	// can be estimated; a mean cannot produce a tail percentile.
	TTFTHistogram     string `gorm:"type:text" json:"-"`
	DurationHistogram string `gorm:"type:text" json:"-"`
	UpdatedAt         int64  `gorm:"autoUpdateTime:false" json:"updated_at"`
}

// RouteShadowMetric compares the shadow recommendation with the channel the
// production scheduler actually used. It never changes scheduling. Former
// cheaper_count/cost_unknown_count columns may remain in existing databases;
// they are no longer written (shadow decisions no longer rank by cost).
type RouteShadowMetric struct {
	ID        string `gorm:"type:varchar(36);primaryKey" json:"-"`
	BucketTs  int64  `gorm:"index:idx_route_shadow_window,priority:1" json:"bucket_ts"`
	ModelName string `gorm:"size:255;index:idx_route_shadow_window,priority:2" json:"model"`
	// Logical requests (one per client request, regardless of retries).
	RequestCount      int64 `gorm:"not null;default:0" json:"requests"`
	DeliveredCount    int64 `gorm:"not null;default:0" json:"delivered"`
	RetriedCount      int64 `gorm:"not null;default:0" json:"retried"`
	TotalAttemptCount int64 `gorm:"not null;default:0" json:"total_attempts"`
	// Per-attempt decisions.
	DecisionCount        int64 `gorm:"not null;default:0" json:"decisions"`
	AgreeCount           int64 `gorm:"not null;default:0" json:"agree"`
	DisagreeCount        int64 `gorm:"not null;default:0" json:"disagree"`
	CrossGroupCount      int64 `gorm:"not null;default:0" json:"cross_group"`
	NoDataCount          int64 `gorm:"not null;default:0" json:"no_data"`
	AvoidUnhealthyCount  int64 `gorm:"not null;default:0" json:"avoid_unhealthy"`
	AgreeSuccessCount    int64 `gorm:"not null;default:0" json:"agree_success"`
	AgreeFailureCount    int64 `gorm:"not null;default:0" json:"agree_failure"`
	DisagreeSuccessCount int64 `gorm:"not null;default:0" json:"disagree_success"`
	DisagreeFailureCount int64 `gorm:"not null;default:0" json:"disagree_failure"`
	UpdatedAt            int64 `gorm:"autoUpdateTime:false" json:"updated_at"`
}

const (
	routeAttemptRetention = 3 * 24 * time.Hour
	routeShadowRetention  = 30 * 24 * time.Hour
)

func SaveRouteAttemptSnapshots(ctx context.Context, rows []RouteAttemptMetric) error {
	if len(rows) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"attempt_count", "success_count", "service_failure_count", "rate_limited_count", "capacity_rejected_count",
			"unknown_failure_count", "user_error_count", "client_canceled_count", "local_delivery_count",
			"ttft_histogram", "duration_histogram", "updated_at",
		}),
	}).CreateInBatches(&rows, 100).Error
}

func SaveRouteShadowSnapshots(ctx context.Context, rows []RouteShadowMetric) error {
	if len(rows) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"request_count", "delivered_count", "retried_count", "total_attempt_count",
			"decision_count", "agree_count", "disagree_count", "cross_group_count", "no_data_count",
			"avoid_unhealthy_count",
			"agree_success_count", "agree_failure_count", "disagree_success_count", "disagree_failure_count", "updated_at",
		}),
	}).CreateInBatches(&rows, 100).Error
}

// QueryRouteAttemptMetrics returns raw replica rows; histogram merging happens
// in Go because the JSON arrays cannot be summed portably in SQL.
func QueryRouteAttemptMetrics(ctx context.Context, start, end int64, modelName string) ([]RouteAttemptMetric, error) {
	rows := make([]RouteAttemptMetric, 0)
	query := DB.WithContext(ctx).Where("bucket_ts >= ? AND bucket_ts <= ?", start, end)
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	err := query.Find(&rows).Error
	return rows, err
}

func QueryRouteShadowMetrics(ctx context.Context, start, end int64, modelName string) ([]RouteShadowMetric, error) {
	rows := make([]RouteShadowMetric, 0)
	query := DB.WithContext(ctx).Where("bucket_ts >= ? AND bucket_ts <= ?", start, end)
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	err := query.Find(&rows).Error
	return rows, err
}

func DeleteExpiredRouteInsightMetrics(ctx context.Context, now time.Time) error {
	if err := DB.WithContext(ctx).Where("bucket_ts < ?", now.Add(-routeAttemptRetention).Unix()).Delete(&RouteAttemptMetric{}).Error; err != nil {
		return err
	}
	return DB.WithContext(ctx).Where("bucket_ts < ?", now.Add(-routeShadowRetention).Unix()).Delete(&RouteShadowMetric{}).Error
}

// GetAllSatisfiedChannels lists every eligible channel of a group/model across
// all priority levels. It reads only the memory cache; callers on the request
// path must not fall back to per-request database scans.
func GetAllSatisfiedChannels(group, modelName string, allowedChannels, excludeChannels []int, filters ...ChannelFilter) ([]*Channel, bool) {
	if !common.MemoryCacheEnabled {
		return nil, false
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	ids := group2model2channels[group][modelName]
	if len(ids) == 0 {
		ids = group2model2channels[group][ratio_setting.FormatMatchingModelName(modelName)]
	}
	allowed := make(map[int]bool, len(allowedChannels))
	for _, id := range allowedChannels {
		allowed[id] = true
	}
	excluded := make(map[int]bool, len(excludeChannels))
	for _, id := range excludeChannels {
		excluded[id] = true
	}
	result := make([]*Channel, 0, len(ids))
	for _, id := range ids {
		if (len(allowed) > 0 && !allowed[id]) || excluded[id] {
			continue
		}
		channel, ok := channelsIDM[id]
		if !ok || channel == nil {
			continue
		}
		pass := true
		for _, filter := range filters {
			if !filter(channel) {
				pass = false
				break
			}
		}
		if pass {
			result = append(result, channel)
		}
	}
	return result, true
}
