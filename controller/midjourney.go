package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

func UpdateMidjourneyTaskBulk() {
	ctx := context.TODO()
	for {
		time.Sleep(15 * time.Second)
		tasks := model.GetAllUnFinishTasks()
		if len(tasks) == 0 {
			continue
		}
		// Missing IDs are only legacy personal failures; incomplete enterprise sends
		// remain unknown and must never be converted into refunds by this poller.
		var nullPersonalIDs []int
		for _, task := range tasks {
			if task.MjId == "" && task.EnterpriseId == 0 {
				nullPersonalIDs = append(nullPersonalIDs, task.Id)
			}
		}
		if len(nullPersonalIDs) > 0 {
			if err := model.MjBulkUpdateByTaskIds(nullPersonalIDs, map[string]any{"status": "FAILURE", "progress": "100%"}); err != nil {
				logger.LogError(ctx, "mark incomplete personal MJ tasks: "+err.Error())
			}
		}
		batches, err := model.BuildMJPollingBatches(tasks)
		if err != nil {
			logger.LogError(ctx, err.Error())
		}
		for _, batch := range batches {
			channel, err := model.CacheGetChannel(batch.ChannelID)
			if err != nil {
				// Apply only to this batch's personal rows, never all records sharing an
				// upstream ID (which may belong to a different owner/key/payer).
				if batch.Subject.EnterpriseId == 0 {
					var ids []int
					for _, rows := range batch.TasksByID {
						for _, task := range rows {
							ids = append(ids, task.Id)
						}
					}
					if updateErr := model.MjBulkUpdateByTaskIds(ids, map[string]any{"fail_reason": "获取渠道信息失败，请联系管理员", "status": "FAILURE", "progress": "100%"}); updateErr != nil {
						logger.LogError(ctx, "update personal MJ tasks: "+updateErr.Error())
					}
				}
				continue
			}
			items, attempted, err := relay.FetchMidjourneyPollingBatch(ctx, channel, batch)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("MJ polling channel %d failed: %v", batch.ChannelID, err))
				continue
			}
			if !attempted {
				continue
			}
			for _, item := range items {
				for _, task := range batch.TasksByID[item.MjId] {
					updateMidjourneyPolledTask(ctx, task, item)
				}
			}
		}
	}
}

// Each local row gets its own response value: a personal timeout cannot rewrite
// the response subsequently applied to another row or an enterprise task.
func updateMidjourneyPolledTask(ctx context.Context, task *model.Midjourney, responseItem dto.MidjourneyDto) {
	if task == nil {
		return
	}
	useTime := (time.Now().UnixNano() / int64(time.Millisecond)) - task.SubmitTime
	// 如果时间超过一小时，且进度不是100%，则认为任务失败
	if task.EnterpriseId == 0 && useTime > 3600000 && task.Progress != "100%" {
		responseItem.FailReason = "上游任务超时（超过1小时）"
		responseItem.Status = "FAILURE"
	}
	if !checkMjTaskNeedUpdate(task, responseItem) {
		return
	}
	preStatus := task.Status
	task.Code = 1
	task.Progress = responseItem.Progress
	task.PromptEn = responseItem.PromptEn
	task.State = responseItem.State
	task.SubmitTime = responseItem.SubmitTime
	task.StartTime = responseItem.StartTime
	task.FinishTime = responseItem.FinishTime
	task.ImageUrl = responseItem.ImageUrl
	task.Status = responseItem.Status
	task.FailReason = responseItem.FailReason
	if responseItem.Properties != nil {
		propertiesStr, _ := common.Marshal(responseItem.Properties)
		task.Properties = string(propertiesStr)
	}
	if responseItem.Buttons != nil {
		buttonStr, _ := common.Marshal(responseItem.Buttons)
		task.Buttons = string(buttonStr)
	}
	// 映射 VideoUrl
	task.VideoUrl = responseItem.VideoUrl

	// 映射 VideoUrls - 将数组序列化为 JSON 字符串
	if responseItem.VideoUrls != nil && len(responseItem.VideoUrls) > 0 {
		videoUrlsStr, err := common.Marshal(responseItem.VideoUrls)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("序列化 VideoUrls 失败: %v", err))
			task.VideoUrls = "[]" // 失败时设置为空数组
		} else {
			task.VideoUrls = string(videoUrlsStr)
		}
	} else {
		task.VideoUrls = "" // 空值时清空字段
	}

	if task.EnterpriseId > 0 {
		if task.Status == "SUCCESS" || task.Status == "FAILURE" {
			task.Progress = "100%"
		}
		if _, err := task.UpdateWithStatus(preStatus); err != nil {
			logger.LogError(ctx, "Update enterprise Midjourney task: "+err.Error())
			return
		}
		// Persist the provider-confirmed outcome first. Settlement or
		// correction failures remain recoverable by maintenance.
		if err := model.SettleEnterpriseMJSubmission(task.Id, task.EnterpriseInitialQuota); err != nil {
			logger.LogError(ctx, "Settle enterprise Midjourney task: "+err.Error())
			return
		}
		if task.Status == "SUCCESS" || task.Status == "FAILURE" {
			if _, err := model.FinalizeEnterpriseMJ(task.Id); err != nil {
				logger.LogError(ctx, "Finalize enterprise Midjourney task: "+err.Error())
			}
		}
		return
	}
	shouldReturnQuota := false
	if (task.Progress != "100%" && responseItem.FailReason != "") || (task.Progress == "100%" && task.Status == "FAILURE") {
		logger.LogInfo(ctx, task.MjId+" 构建失败，"+task.FailReason)
		task.Progress = "100%"
		if task.Quota != 0 {
			shouldReturnQuota = true
		}
	}
	won, err := task.UpdateWithStatus(preStatus)
	if err != nil {
		logger.LogError(ctx, "UpdateMidjourneyTask task error: "+err.Error())
	} else if won && shouldReturnQuota {
		err = model.IncreaseUserQuota(task.UserId, task.Quota, false)
		if err != nil {
			logger.LogError(ctx, "fail to increase user quota: "+err.Error())
		}
		model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
			UserId:    task.UserId,
			LogType:   model.LogTypeRefund,
			Content:   "",
			ChannelId: task.ChannelId,
			ModelName: service.CovertMjpActionToModelName(task.Action),
			Quota:     task.Quota,
			Other: map[string]interface{}{
				"task_id": task.MjId,
				"reason":  "构图失败",
			},
		})
	}
}

func checkMjTaskNeedUpdate(oldTask *model.Midjourney, newTask dto.MidjourneyDto) bool {
	if oldTask.Code != 1 {
		return true
	}
	if oldTask.Progress != newTask.Progress {
		return true
	}
	if oldTask.PromptEn != newTask.PromptEn {
		return true
	}
	if oldTask.State != newTask.State {
		return true
	}
	if oldTask.SubmitTime != newTask.SubmitTime {
		return true
	}
	if oldTask.StartTime != newTask.StartTime {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.ImageUrl != newTask.ImageUrl {
		return true
	}
	if oldTask.Status != newTask.Status {
		return true
	}
	if oldTask.FailReason != newTask.FailReason {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.Progress != "100%" && newTask.FailReason != "" {
		return true
	}
	// 检查 VideoUrl 是否需要更新
	if oldTask.VideoUrl != newTask.VideoUrl {
		return true
	}
	// 检查 VideoUrls 是否需要更新
	if newTask.VideoUrls != nil && len(newTask.VideoUrls) > 0 {
		newVideoUrlsStr, _ := common.Marshal(newTask.VideoUrls)
		if oldTask.VideoUrls != string(newVideoUrlsStr) {
			return true
		}
	} else if oldTask.VideoUrls != "" {
		// 如果新数据没有 VideoUrls 但旧数据有，需要更新（清空）
		return true
	}

	return false
}

func GetAllMidjourney(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	// 解析其他查询参数
	queryParams := model.TaskQueryParams{
		ChannelID:      c.Query("channel_id"),
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	items := model.GetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllTasks(queryParams)

	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func GetUserMidjourney(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	queryParams := model.TaskQueryParams{
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	items := model.GetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllUserTask(userId, queryParams)

	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}
