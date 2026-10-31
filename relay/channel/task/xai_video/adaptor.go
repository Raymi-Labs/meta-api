package xai_video

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// TaskAdaptor implements xAI's asynchronous Imagine video API:
// POST /v1/videos/generations -> request_id, then GET /v1/videos/{request_id}.
// It is separate from the regular xAI chat/image adaptor so existing
// OpenAI-compatible request handling is unchanged.
type TaskAdaptor struct {
	taskcommon.BaseBilling
	pollingInfo *relaycommon.RelayInfo
	apiKey      string
	baseURL     string
}

type submitResponse struct {
	RequestID string `json:"request_id"`
}

type videoResponse struct {
	RequestID string `json:"request_id"`
	Model     string `json:"model"`
	Status    string `json:"status"`
	Video     *struct {
		URL      string  `json:"url"`
		Duration float64 `json:"duration"`
	} `json:"video,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.pollingInfo = info
	if info == nil {
		return
	}
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var payload map[string]any
	if err := common.UnmarshalBodyReusable(c, &payload); err != nil {
		return referenceRequestError(err)
	}
	reference, err := normalizeReferenceImage(payload)
	if err != nil {
		return referenceRequestError(err)
	}
	// Decode a string only for shared validation; the original request body
	// remains intact for the xAI outbound representation and billing snapshot.
	delete(payload, "image")
	data, err := common.Marshal(payload)
	if err != nil {
		return referenceRequestError(err)
	}
	var req relaycommon.TaskSubmitReq
	if err := common.Unmarshal(data, &req); err != nil {
		return referenceRequestError(err)
	}
	req.Image = reference
	action := constant.TaskActionTextGenerate
	if reference != "" {
		action = constant.TaskActionGenerate
	}
	return relaycommon.ValidateAndStoreTaskRequest(c, info, action, req)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("xAI video relay info is nil")
	}
	return xAIVideoURL(info.ChannelBaseUrl, "/videos/generations"), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_request_body_failed")
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("xAI video request body is empty")
	}

	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, errors.Wrap(err, "unmarshal_xai_video_request_failed")
	}
	if _, err := normalizeReferenceImage(payload); err != nil {
		return nil, err
	}
	if raw, ok := payload["duration"]; ok {
		n, valid := numericInt(raw)
		if !valid {
			return nil, errors.New("invalid video duration")
		}
		payload["duration"] = n
		if alias, exists := payload["seconds"]; exists {
			m, valid := numericInt(alias)
			if !valid || m != n {
				return nil, errors.New("conflicting duration and seconds")
			}
		}
	}
	if info != nil && info.ChannelMeta != nil && strings.TrimSpace(info.UpstreamModelName) != "" {
		payload["model"] = info.UpstreamModelName
	}
	if _, ok := payload["duration"]; !ok {
		if raw, exists := payload["seconds"]; exists {
			if duration, ok := numericInt(raw); ok {
				payload["duration"] = duration
			} else {
				return nil, errors.New("invalid seconds")
			}
		}
	}
	if _, ok := payload["resolution"]; !ok {
		if size, exists := payload["size"].(string); exists && strings.TrimSpace(size) != "" {
			payload["resolution"] = strings.TrimSpace(size)
		}
	}
	if _, ok := payload["resolution"]; !ok {
		if height, ok := numericInt(payload["height"]); ok {
			payload["resolution"] = fmt.Sprintf("%dp", height)
		}
	}
	if _, ok := payload["resolution"]; !ok {
		// xAI defaults to 480p; make the default explicit so the request and
		// any parameter-based billing expression describe the same variant.
		payload["resolution"] = "480p"
	}
	if _, ok := payload["aspect_ratio"]; !ok {
		if aspect, exists := payload["aspectRatio"]; exists {
			payload["aspect_ratio"] = aspect
		}
	}
	// These are New API aliases, not xAI fields. Do not forward them upstream.
	delete(payload, "seconds")
	delete(payload, "size")
	delete(payload, "aspectRatio")
	delete(payload, "height")
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_xai_video_request_failed")
	}
	return bytes.NewReader(data), nil
}

func numericInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed > 0
	case int64:
		return int(typed), typed > 0
	case float64:
		return int(typed), typed > 0 && typed < float64(math.MaxInt) && typed == math.Trunc(typed)
	case string:
		v, err := strconv.Atoi(strings.TrimSpace(typed))
		return v, err == nil && v > 0
	default:
		return 0, false
	}
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	if resp == nil {
		return "", nil, service.TaskErrorWrapperLocal(errors.New("xAI video response is nil"), "invalid_response", http.StatusBadGateway)
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var response submitResponse
	if err := common.Unmarshal(responseBody, &response); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_xai_video_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(response.RequestID) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("xAI video response missing request_id"), "invalid_response", http.StatusInternalServerError)
	}

	result := dto.NewOpenAIVideo()
	result.ID = info.PublicTaskID
	result.TaskID = info.PublicTaskID
	result.Model = info.OriginModelName
	result.Status = dto.VideoStatusQueued
	result.CreatedAt = common.GetTimestamp()
	c.JSON(http.StatusOK, result)
	return response.RequestID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, errors.New("invalid xAI video request_id")
	}
	req, err := http.NewRequest(http.MethodGet, xAIVideoURL(baseURL, "/videos/"+taskID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, errors.Wrap(err, "new proxy http client failed")
	}
	return channel.DoTaskPollingRequest(a.pollingInfo, key, req, client)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var response videoResponse
	if err := common.Unmarshal(respBody, &response); err != nil {
		return nil, errors.Wrap(err, "unmarshal xAI video status failed")
	}
	result := &relaycommon.TaskInfo{}
	switch strings.ToLower(strings.TrimSpace(response.Status)) {
	case "pending", "queued", "processing", "in_progress", "running":
		result.Status = model.TaskStatusInProgress
		result.Progress = taskcommon.ProgressInProgress
	case "done", "completed", "success":
		result.Status = model.TaskStatusSuccess
		result.Progress = taskcommon.ProgressComplete
		if response.Video != nil {
			result.Url = strings.TrimSpace(response.Video.URL)
		}
		if result.Url == "" {
			return nil, errors.New("xAI video completed response missing video.url")
		}
	case "failed", "expired", "cancelled", "canceled":
		result.Status = model.TaskStatusFailure
		result.Progress = taskcommon.ProgressComplete
		if response.Error != nil {
			result.Reason = response.Error.Message
		}
		if result.Reason == "" {
			result.Reason = "xAI video generation failed"
		}
	default:
		return nil, fmt.Errorf("xAI video returned unknown status %q", response.Status)
	}
	return result, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"grok-imagine-video", "grok-imagine-video-1.5"}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "xai-video"
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	if task == nil {
		return nil, errors.New("task is nil")
	}
	video := task.ToOpenAIVideo()
	video.Model = task.Properties.OriginModelName
	video.Status = task.Status.ToVideoStatus()
	video.SetMetadata("url", task.GetResultURL())
	return common.Marshal(video)
}

func xAIVideoURL(baseURL, suffix string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return baseURL + "/v1" + suffix
	}
	path := strings.TrimRight(parsed.Path, "/")
	path = strings.TrimSuffix(path, "/v1")
	parsed.Path = path + "/v1" + suffix
	parsed.RawPath = ""
	return parsed.String()
}
