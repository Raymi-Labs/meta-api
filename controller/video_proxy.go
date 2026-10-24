package controller

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// videoProxyError returns a standardized OpenAI-style error response.
func videoProxyError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
		},
	})
}

func VideoProxy(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "task_id is required")
		return
	}

	task, exists, err := model.GetByOnlyTaskId(taskID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to query task %s: %s", taskID, err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}

	if task.Status != model.TaskStatusSuccess {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("Task is not completed yet, current status: %s", task.Status))
		return
	}

	channel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to get channel for task %s: %s", taskID, err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to retrieve channel information")
		return
	}
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	var videoURL string
	credentialedContent := false
	contentKey := channel.Key
	if task.PrivateData.Key != "" {
		contentKey = task.PrivateData.Key
	}
	proxy := channel.GetSetting().Proxy
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create proxy client for task %s: %s", taskID, err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to create proxy client")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "", nil)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create request: %s", err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to create proxy request")
		return
	}

	switch channel.Type {
	case constant.ChannelTypeGemini:
		apiKey := task.PrivateData.Key
		if apiKey == "" {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Missing stored API key for Gemini task %s", taskID))
			videoProxyError(c, http.StatusInternalServerError, "server_error", "API key not stored for task")
			return
		}
		videoURL, err = getGeminiVideoURL(c, channel, task, apiKey)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to resolve Gemini video URL for task %s: %s", taskID, err.Error()))
			var apiErr *types.NewAPIError
			if errors.As(err, &apiErr) {
				if apiErr.RetryAfter > 0 {
					c.Header("Retry-After", fmt.Sprintf("%d", int((apiErr.RetryAfter+time.Second-1)/time.Second)))
				}
				videoProxyError(c, apiErr.StatusCode, "server_error", apiErr.Error())
			} else {
				videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to resolve Gemini video URL")
			}
			return
		}
	case constant.ChannelTypeVertexAi:
		adaptor := relay.GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channel.Type)))
		videoURL, err = getVertexVideoURL(c, channel, task, adaptor)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to resolve Vertex video URL for task %s: %s", taskID, err.Error()))
			var apiErr *types.NewAPIError
			if errors.As(err, &apiErr) {
				if apiErr.RetryAfter > 0 {
					c.Header("Retry-After", fmt.Sprintf("%d", int((apiErr.RetryAfter+time.Second-1)/time.Second)))
				}
				videoProxyError(c, apiErr.StatusCode, "server_error", apiErr.Error())
			} else {
				videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to resolve Vertex video URL")
			}
			return
		}
	case constant.ChannelTypeOpenAI, constant.ChannelTypeSora:
		videoURL = fmt.Sprintf("%s/v1/videos/%s/content", baseURL, task.GetUpstreamTaskID())
		req.Header.Set("Authorization", "Bearer "+contentKey)
		credentialedContent = true
	default:
		// Video URL is stored in PrivateData.ResultURL (fallback to FailReason for old data)
		videoURL = task.GetResultURL()
	}

	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch video content")
		return
	}
	if strings.HasPrefix(videoURL, "data:") {
		mimeType, videoBytes, decodeErr := decodeVideoDataURL(videoURL)
		if decodeErr != nil {
			videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to decode video content")
			return
		}
		c.Header("Cache-Control", "public, max-age=86400")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, mimeType, videoBytes)
		return
	}

	req.URL, err = url.Parse(videoURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to parse video URL for task %s (%T)", taskID, err))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to create proxy request")
		return
	}
	if channel.Type == constant.ChannelTypeGemini {
		// Provider results may point at an external signed CDN URL. Only the
		// configured origin receives our credentials or channel-private overrides.
		origin := channel.GetBaseURL()
		if origin == "" {
			origin = constant.ChannelBaseURLs[channel.Type]
		}
		u, parseErr := url.Parse(origin)
		credentialedContent = parseErr == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.Scheme == req.URL.Scheme && u.Host == req.URL.Host
		if credentialedContent {
			if err := addGeminiContentAPIKey(req.URL, contentKey); err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("Invalid Gemini content query for task %s", taskID))
				videoProxyError(c, http.StatusBadGateway, "server_error", "Invalid video content URL")
				return
			}
			req.Header.Set("x-goog-api-key", contentKey)
		}
	}

	var releaseSlot func()
	if channel.Type == constant.ChannelTypeGemini || channel.Type == constant.ChannelTypeVertexAi || channel.Type == constant.ChannelTypeOpenAI || channel.Type == constant.ChannelTypeSora {
		var capacityErr *types.NewAPIError
		releaseSlot, capacityErr = acquireFixedChannelCapacityWithWait(c, channel)
		if capacityErr != nil {
			videoProxyError(c, capacityErr.StatusCode, "server_error", capacityErr.Error())
			return
		}
		if releaseSlot != nil {
			defer releaseSlot()
		}
	}

	var resp *http.Response
	if credentialedContent {
		resp, err = relaychannel.DoTaskContentRequest(service.TaskPollingRelayInfo(channel, task.UserId, contentKey), contentKey, req, client)
	} else {
		resp, err = client.Do(req)
	}
	if err != nil {
		// Transport errors can embed the full URL, including an injected key or
		// upstream signature. Log the task and error class, never that URL.
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to fetch video content for task %s (%T)", taskID, err))
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch video content")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Video content upstream returned status %d for task %s", resp.StatusCode, taskID))
		if resp.StatusCode == http.StatusTooManyRequests {
			apiErr := types.NewOpenAIError(
				fmt.Errorf("video content upstream rate limited"),
				types.ErrorCodeBadResponseStatusCode,
				resp.StatusCode,
			)
			apiErr.UpstreamStatusCode = resp.StatusCode
			apiErr.RetryAfter = service.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			service.RecordChannelRateLimitCooldown(c, channel, apiErr)
			if apiErr.RetryAfter > 0 {
				c.Header("Retry-After", fmt.Sprintf("%d", int((apiErr.RetryAfter+time.Second-1)/time.Second)))
			}
			videoProxyError(c, http.StatusTooManyRequests, "server_error", apiErr.Error())
			return
		}
		videoProxyError(c, http.StatusBadGateway, "server_error",
			fmt.Sprintf("Upstream service returned status %d", resp.StatusCode))
		return
	}

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}

	c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to stream video content: %s", err.Error()))
	}
}

// Decode before writing headers so malformed payloads produce an error response.
func decodeVideoDataURL(dataURL string) (string, []byte, error) {
	header, payload, ok := strings.Cut(dataURL, ",")
	if !ok || !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") {
		return "", nil, fmt.Errorf("unsupported video data url")
	}
	mimeType := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	if mimeType == "" {
		mimeType = "video/mp4"
	}
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil || !strings.HasPrefix(mediaType, "video/") {
		return "", nil, fmt.Errorf("invalid video media type")
	}
	videoBytes, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		videoBytes, err = base64.RawStdEncoding.DecodeString(payload)
	}
	if err != nil || len(videoBytes) == 0 {
		return "", nil, fmt.Errorf("invalid video base64 payload")
	}
	return mimeType, videoBytes, nil
}
