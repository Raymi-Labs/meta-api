package controller

import (
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	routeinsight "github.com/QuantumNous/new-api/pkg/route_insight"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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
	ChannelName string `json:"channel_name"`
}

// GetRouteInsightChannels is admin-only: it exposes per-channel attempt
// evidence, never to end users.
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
			row.ChannelName = channel.Name
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

type routeInsightUnhealthyChannel struct {
	ChannelID   int      `json:"channel_id"`
	ChannelName string   `json:"channel_name"`
	Models      []string `json:"models"`
	Reasons     []string `json:"reasons"`
}

type routeInsightGroupRow struct {
	grouphealth.GroupTotal
	// Configured is false for a group that still has samples but no longer
	// exists in the group ratio settings.
	Configured        bool                           `json:"configured"`
	UnhealthyChannels []routeInsightUnhealthyChannel `json:"unhealthy_channels"`
}

// GetRouteInsightGroups is admin-only: per-group request outcomes use the
// group-health definition (same-group retries folded, user errors excluded),
// joined with the live route-health snapshot's unhealthy channels.
func GetRouteInsightGroups(c *gin.Context) {
	hours := boundedQueryInt(c, "hours", 24, grouphealth.MeasurementLookbackHours)
	now := time.Now()
	totals, err := grouphealth.AdminGroupTotals(c.Request.Context(), now, hours)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	configured := ratio_setting.GetGroupRatioCopy()
	rows := make([]routeInsightGroupRow, 0, len(totals.Groups)+len(configured))
	index := make(map[string]int, len(totals.Groups)+len(configured))
	for _, total := range totals.Groups {
		_, ok := configured[total.Group]
		index[total.Group] = len(rows)
		rows = append(rows, routeInsightGroupRow{GroupTotal: total, Configured: ok})
	}
	names := make([]string, 0, len(configured))
	for name := range configured {
		if _, seen := index[name]; !seen && name != "auto" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		index[name] = len(rows)
		rows = append(rows, routeInsightGroupRow{GroupTotal: grouphealth.GroupTotal{Group: name}, Configured: true})
	}

	routes, fresh := routeinsight.UnhealthyRoutes()
	byGroup := make(map[string]map[int]*routeInsightUnhealthyChannel)
	for _, route := range routes {
		channel, cacheErr := model.CacheGetChannel(route.ChannelID)
		if cacheErr != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		// Abilities cover every group x model of a channel, and health is per
		// channel/model, so an unhealthy route is skipped in each of its groups.
		for _, group := range channel.GetGroups() {
			if _, ok := index[group]; !ok {
				continue
			}
			channels := byGroup[group]
			if channels == nil {
				channels = make(map[int]*routeInsightUnhealthyChannel)
				byGroup[group] = channels
			}
			entry := channels[route.ChannelID]
			if entry == nil {
				entry = &routeInsightUnhealthyChannel{ChannelID: route.ChannelID, ChannelName: channel.Name}
				channels[route.ChannelID] = entry
			}
			entry.Models = append(entry.Models, route.Model)
			entry.Reasons = append(entry.Reasons, route.Reason)
		}
	}
	for group, channels := range byGroup {
		list := make([]routeInsightUnhealthyChannel, 0, len(channels))
		for _, entry := range channels {
			list = append(list, *entry)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].ChannelID < list[j].ChannelID })
		rows[index[group]].UnhealthyChannels = list
	}
	for i := range rows {
		if rows[i].UnhealthyChannels == nil {
			rows[i].UnhealthyChannels = []routeInsightUnhealthyChannel{}
		}
	}

	snapshotAt, snapshotFailing := routeinsight.SnapshotState()
	common.ApiSuccess(c, gin.H{
		"hours":                 totals.WindowHours,
		"start_time":            totals.StartTime,
		"end_time":              totals.EndTime,
		"bucket_seconds":        totals.BucketSeconds,
		"collection_incomplete": totals.CollectionIncomplete,
		"snapshot_fresh":        fresh,
		"snapshot_at":           snapshotAt,
		"snapshot_failing":      snapshotFailing,
		"groups":                rows,
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
