package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func getGeminiVideoURL(c *gin.Context, channel *model.Channel, task *model.Task, apiKey string) (string, error) {
	if channel == nil || task == nil {
		return "", fmt.Errorf("invalid channel or task")
	}

	if url := extractGeminiVideoURLFromTaskData(task); url != "" {
		return url, nil
	}

	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	adaptor := relay.GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channel.Type)))
	if adaptor == nil {
		return "", fmt.Errorf("gemini task adaptor not found")
	}

	if apiKey == "" {
		return "", fmt.Errorf("api key not available for task")
	}

	releaseSlot, capacityErr := acquireFixedChannelCapacityWithWait(c, channel)
	if capacityErr != nil {
		return "", capacityErr
	}
	if releaseSlot != nil {
		defer releaseSlot()
	}

	proxy := channel.GetSetting().Proxy
	adaptor.Init(service.TaskPollingRelayInfo(channel, task.UserId, apiKey))
	resp, err := adaptor.FetchTask(baseURL, apiKey, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, proxy)
	if err != nil {
		return "", fmt.Errorf("fetch task failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			apiErr := types.NewOpenAIError(
				fmt.Errorf("gemini video lookup upstream rate limited"),
				types.ErrorCodeBadResponseStatusCode,
				resp.StatusCode,
			)
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
	if parseErr == nil && taskInfo != nil && taskInfo.RemoteUrl != "" {
		return taskInfo.RemoteUrl, nil
	}

	if url := extractGeminiVideoURLFromPayload(body); url != "" {
		return url, nil
	}

	if parseErr != nil {
		return "", fmt.Errorf("parse task result failed: %w", parseErr)
	}

	return "", fmt.Errorf("gemini video url not found")
}

func extractGeminiVideoURLFromTaskData(task *model.Task) string {
	if task == nil || len(task.Data) == 0 {
		return ""
	}
	var payload map[string]any
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		return ""
	}
	return extractGeminiVideoURLFromMap(payload)
}

func extractGeminiVideoURLFromPayload(body []byte) string {
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return extractGeminiVideoURLFromMap(payload)
}

func extractGeminiVideoURLFromMap(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if uri, ok := payload["uri"].(string); ok && uri != "" {
		return uri
	}
	if resp, ok := payload["response"].(map[string]any); ok {
		if uri := extractGeminiVideoURLFromResponse(resp); uri != "" {
			return uri
		}
	}
	return ""
}

func extractGeminiVideoURLFromResponse(resp map[string]any) string {
	if resp == nil {
		return ""
	}
	if gvr, ok := resp["generateVideoResponse"].(map[string]any); ok {
		if uri := extractGeminiVideoURLFromGeneratedSamples(gvr); uri != "" {
			return uri
		}
	}
	if videos, ok := resp["videos"].([]any); ok {
		for _, video := range videos {
			if vm, ok := video.(map[string]any); ok {
				if uri, ok := vm["uri"].(string); ok && uri != "" {
					return uri
				}
			}
		}
	}
	if uri, ok := resp["video"].(string); ok && uri != "" {
		return uri
	}
	if uri, ok := resp["uri"].(string); ok && uri != "" {
		return uri
	}
	return ""
}

func extractGeminiVideoURLFromGeneratedSamples(gvr map[string]any) string {
	if gvr == nil {
		return ""
	}
	if samples, ok := gvr["generatedSamples"].([]any); ok {
		for _, sample := range samples {
			if sm, ok := sample.(map[string]any); ok {
				if video, ok := sm["video"].(map[string]any); ok {
					if uri, ok := video["uri"].(string); ok && uri != "" {
						return uri
					}
				}
			}
		}
	}
	return ""
}

// Called only after the content origin is verified. Preserve signed query
// bytes/order and any provider-supplied key; append only our missing credential.
func addGeminiContentAPIKey(uri *url.URL, key string) error {
	query, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		return fmt.Errorf("invalid Gemini content query")
	}
	if key == "" || query.Has("key") {
		return nil
	}
	if uri.RawQuery != "" {
		uri.RawQuery += "&"
	}
	uri.RawQuery += "key=" + url.QueryEscape(key)
	return nil
}
