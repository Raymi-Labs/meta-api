package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// channelConcurrencyChannelStat 单个渠道的在途并发快照。
type channelConcurrencyChannelStat struct {
	ChannelId            int                       `json:"channel_id"`
	ChannelName          string                    `json:"channel_name"`
	Inflight             int64                     `json:"inflight"`
	MaxConcurrency       int                       `json:"max_concurrency"`
	StaticMaxConcurrency int                       `json:"static_max_concurrency"`
	VMCapacity           *service.VMCapacityStatus `json:"vm_capacity,omitempty"`
}

// GetChannelConcurrencyStats 只读返回渠道并发控制的运行时统计：
// 全局配置、过载指标（等待/超时/取消/队列拒绝/平均等待时长/当前等待数）、
// 以及在途渠道和已启用 VM 容量渠道明细（包括空闲、零容量和未知状态）。
func GetChannelConcurrencyStats(c *gin.Context) {
	setting := operation_setting.GetChannelConcurrencySetting()
	metrics := middleware.GetChannelConcurrencyMetrics()

	channelStats := make([]channelConcurrencyChannelStat, 0)
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err == nil {
		for _, ch := range channels {
			if ch == nil {
				continue
			}
			inflight, qErr := middleware.QueryChannelConcurrency(ch.Id)
			if (qErr != nil || inflight <= 0) && !service.VMCapacityEnabled(ch) {
				continue
			}
			stat := channelConcurrencyChannelStat{
				ChannelId:            ch.Id,
				ChannelName:          ch.Name,
				Inflight:             inflight,
				MaxConcurrency:       middleware.ResolveChannelMaxConcurrency(ch),
				StaticMaxConcurrency: middleware.ResolveChannelMaxConcurrency(ch),
			}
			if service.VMCapacityEnabled(ch) {
				modelName := c.Query("model")
				if modelName == "" {
					modelName = strings.Split(ch.Models, ",")[0]
				}
				status := service.PeekVMChannelCapacity(c.Request.Context(), ch, modelName)
				if qErr != nil {
					status = service.VMCapacityStatus{Model: status.Model, State: "unknown"}
				}
				stat.VMCapacity = &status
				stat.MaxConcurrency = 0
				if status.Snapshot != nil {
					stat.MaxConcurrency = min(stat.StaticMaxConcurrency, status.Snapshot.EffectiveAdmissionLimit)
				}
			}
			channelStats = append(channelStats, stat)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":  setting.Enabled,
			"config":   setting,
			"metrics":  metrics,
			"channels": channelStats,
		},
	})
}
