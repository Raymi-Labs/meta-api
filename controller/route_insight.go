package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func boundedQueryInt(c *gin.Context, name string, fallback, max int) int {
	value, err := strconv.Atoi(c.Query(name))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

type routeInsightChannelRow struct {
	routeinsight.ChannelReport
	ChannelName   string   `json:"channel_name"`
	PurchaseRatio *float64 `json:"purchase_ratio,omitempty"`
	FaultDomain   string   `json:"fault_domain,omitempty"`
}

// GetRouteInsightChannels is admin-only: it exposes per-channel attempt
// evidence and administrator-entered purchase cost, never to end users.
func GetRouteInsightChannels(c *gin.Context) {
	minutes := boundedQueryInt(c, "minutes", 60, 3*24*60)
	modelName := c.Query("model")
	if len(modelName) > 255 {
		common.ApiErrorMsg(c, "model too long")
		return
	}
	reports, err := routeinsight.ChannelReports(c.Request.Context(), time.Now(), minutes, modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows := make([]routeInsightChannelRow, 0, len(reports))
	for _, report := range reports {
		row := routeInsightChannelRow{ChannelReport: report}
		if channel, cacheErr := model.CacheGetChannel(report.ChannelID); cacheErr == nil && channel != nil {
			setting := channel.GetSetting()
			row.ChannelName = channel.Name
			row.FaultDomain = setting.RouteFaultDomain
			if setting.RoutePurchaseRatio > 0 {
				ratio := setting.RoutePurchaseRatio
				row.PurchaseRatio = &ratio
			}
		}
		rows = append(rows, row)
	}
	snapshotAt, snapshotFailing := routeinsight.SnapshotState()
	common.ApiSuccess(c, gin.H{
		"enabled":          routeinsight.Enabled(),
		"minutes":          minutes,
		"snapshot_at":      snapshotAt,
		"snapshot_failing": snapshotFailing,
		"latency_bounds":   routeinsight.LatencyBoundsMs,
		"routing":          operation_setting.GetRouteRoutingSetting(),
		// Shadow and live routing read the memory channel cache only; without
		// it (no Redis and MEMORY_CACHE_ENABLED unset) both silently skip.
		"memory_cache_enabled": common.MemoryCacheEnabled,
		"channels":             rows,
	})
}

// GetRouteInsightShadow compares shadow recommendations with production.
// recent_disagreements is this instance's in-memory sample only.
func GetRouteInsightShadow(c *gin.Context) {
	hours := boundedQueryInt(c, "hours", 24, 30*24)
	modelName := c.Query("model")
	if len(modelName) > 255 {
		common.ApiErrorMsg(c, "model too long")
		return
	}
	reports, err := routeinsight.ShadowReports(c.Request.Context(), time.Now(), hours, modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"enabled":              routeinsight.Enabled(),
		"hours":                hours,
		"models":               reports,
		"recent_disagreements": routeinsight.RecentDisagreements(modelName),
	})
}
