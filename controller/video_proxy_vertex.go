package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func getVertexVideoURL(c *gin.Context, channel *model.Channel, task *model.Task, adaptor service.TaskPollingAdaptor) (string, error) {
	if channel == nil || task == nil {
		return "", fmt.Errorf("invalid channel or task")
	}
	if url := strings.TrimSpace(task.GetResultURL()); usableVertexVideoURL(url, task.TaskID) {
		return url, nil
	}
	if url := extractVertexVideoURLFromTaskData(task); usableVertexVideoURL(url, task.TaskID) {
		return url, nil
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	if adaptor == nil {
		return "", fmt.Errorf("vertex task adaptor not found")
	}

	key := getVertexTaskKey(channel, task)
	if key == "" {
		return "", fmt.Errorf("vertex key not available for task")
	}

	releaseSlot, capacityErr := acquireFixedChannelCapacityWithWait(c, channel)
	if capacityErr != nil {
		return "", capacityErr
	}
	if releaseSlot != nil {
		defer releaseSlot()
	}

	adaptor.Init(service.TaskPollingRelayInfo(channel, task.UserId, key))
	resp, err := adaptor.FetchTask(baseURL, key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, channel.GetSetting().Proxy)
	if err != nil {
		return "", fmt.Errorf("fetch task failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			apiErr := types.NewOpenAIError(fmt.Errorf("vertex video lookup upstream rate limited"), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
			apiErr.UpstreamStatusCode = resp.StatusCode
			apiErr.RetryAfter = service.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			service.RecordChannelRateLimitCooldown(c, channel, apiErr)
			return "", apiErr
		}
		return "", fmt.Errorf("fetch task returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read task response failed: %w", err)
	}

	taskInfo, parseErr := adaptor.ParseTaskResult(body)
	if parseErr != nil {
		return "", fmt.Errorf("parse task result failed: %w", parseErr)
	}
	if taskInfo == nil || taskInfo.Status != model.TaskStatusSuccess {
		return "", fmt.Errorf("vertex video result is not successful")
	}
	// Read the original payload: some providers return a URL in response.video,
	// while the task adaptor treats that field as bare base64.
	if videoURL := extractVertexVideoURLFromPayload(body); usableVertexVideoURL(videoURL, task.TaskID) {
		return videoURL, nil
	}
	if usableVertexVideoURL(taskInfo.Url, task.TaskID) {
		return taskInfo.Url, nil
	}
	return "", fmt.Errorf("vertex video url not found")
}

func isTaskProxyContentURL(rawURL string, taskID string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && taskID != "" && strings.TrimRight(u.Path, "/") == "/v1/videos/"+taskID+"/content"
}

func usableVertexVideoURL(videoURL string, taskID string) bool {
	if videoURL == "" || isTaskProxyContentURL(videoURL, taskID) {
		return false
	}
	if strings.HasPrefix(videoURL, "data:") {
		_, _, err := decodeVideoDataURL(videoURL)
		return err == nil
	}
	u, err := url.Parse(videoURL)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func getVertexTaskKey(channel *model.Channel, task *model.Task) string {
	if task != nil {
		if key := strings.TrimSpace(task.PrivateData.Key); key != "" {
			return key
		}
	}
	if channel == nil {
		return ""
	}
	// A single Vertex credential can be pretty-printed JSON. Do not split it
	// into lines when recovering an older task without a stored credential.
	rawKey := strings.TrimSpace(channel.Key)
	if !channel.ChannelInfo.IsMultiKey && !strings.HasPrefix(rawKey, "[") {
		return rawKey
	}
	keys := channel.GetKeys()
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" {
			return key
		}
	}
	return strings.TrimSpace(channel.Key)
}

func extractVertexVideoURLFromTaskData(task *model.Task) string {
	if task == nil || len(task.Data) == 0 {
		return ""
	}
	return extractVertexVideoURLFromPayload(task.Data)
}

func extractVertexVideoURLFromPayload(body []byte) string {
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return ""
	}
	resp, ok := payload["response"].(map[string]any)
	if !ok || resp == nil {
		return ""
	}

	if videos, ok := resp["videos"].([]any); ok && len(videos) > 0 {
		if video, ok := videos[0].(map[string]any); ok && video != nil {
			if b64, _ := video["bytesBase64Encoded"].(string); strings.TrimSpace(b64) != "" {
				mime, _ := video["mimeType"].(string)
				enc, _ := video["encoding"].(string)
				return buildVideoDataURL(mime, enc, b64)
			}
		}
	}
	if b64, _ := resp["bytesBase64Encoded"].(string); strings.TrimSpace(b64) != "" {
		enc, _ := resp["encoding"].(string)
		return buildVideoDataURL("", enc, b64)
	}
	if video, _ := resp["video"].(string); strings.TrimSpace(video) != "" {
		if strings.HasPrefix(video, "data:") || strings.HasPrefix(video, "http://") || strings.HasPrefix(video, "https://") {
			return video
		}
		enc, _ := resp["encoding"].(string)
		return buildVideoDataURL("", enc, video)
	}
	return ""
}

func buildVideoDataURL(mimeType string, encoding string, base64Data string) string {
	mime := strings.TrimSpace(mimeType)
	if mime == "" {
		enc := strings.TrimSpace(encoding)
		if enc == "" {
			enc = "mp4"
		}
		if strings.Contains(enc, "/") {
			mime = enc
		} else {
			mime = "video/" + enc
		}
	}
	return "data:" + mime + ";base64," + base64Data
}
