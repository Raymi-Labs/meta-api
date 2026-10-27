package hailuo

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func isH3Family(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "minimax-h3" || strings.HasPrefix(name, "minimax-h3-")
}

func validH3TaskID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// Only the documented text-to-video subset is enabled. Reference inputs have
// separate costs and must not be silently accepted with output-seconds pricing.
type h3Request struct {
	Model      string   `json:"model"`
	Content    []h3Text `json:"content"`
	Duration   int      `json:"duration"`
	Resolution string   `json:"resolution"`
	Ratio      string   `json:"ratio"`
}
type h3Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func h3Prompt(raw map[string]any) (string, error) {
	prompt, ok := raw["prompt"].(string)
	if _, exists := raw["prompt"]; exists && !ok {
		return "", fmt.Errorf("prompt must be a string")
	}
	if content, exists := raw["content"]; exists {
		items, ok := content.([]any)
		if !ok || len(items) != 1 {
			return "", fmt.Errorf("H3 currently supports text-to-video only: content must contain one text item")
		}
		item, ok := items[0].(map[string]any)
		if !ok || len(item) != 2 || item["type"] != "text" {
			return "", fmt.Errorf("H3 reference inputs are not supported by this billing path")
		}
		text, ok := item["text"].(string)
		if !ok || (prompt != "" && prompt != text) {
			return "", fmt.Errorf("content text is invalid or conflicts with prompt")
		}
		prompt = text
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("H3 requires a non-empty prompt or text content")
	}
	return prompt, nil
}

func normalizeH3(raw map[string]any, upstreamModel string) (*h3Request, error) {
	minDuration := 4
	var resolutions []string
	switch upstreamModel {
	case "MiniMax-H3":
		resolutions = []string{"768P", "2K"}
	case "MiniMax-H3-Highspeed":
		resolutions = []string{"768P"}
	case "MiniMax-H3-Lite":
		resolutions = []string{"480P", "768P"}
	case "MiniMax-H3-Max":
		resolutions = []string{"480P", "768P"}
		minDuration = 5
	default:
		return nil, fmt.Errorf("unsupported H3 model")
	}
	allowed := map[string]bool{"model": true, "prompt": true, "content": true, "duration": true, "seconds": true, "resolution": true, "size": true, "ratio": true, "aspect_ratio": true, "metadata": true}
	for key := range raw {
		if !allowed[key] {
			return nil, fmt.Errorf("H3 parameter %q is not supported by the text-to-video path", key)
		}
	}
	// Merge only scalar aliases. A conflicting value is an error, never an
	// override of a priced field. Do not mutate the incoming body across retries.
	fields := make(map[string]any)
	for key, value := range raw {
		if key != "metadata" {
			fields[key] = value
		}
	}
	if value, exists := raw["metadata"]; exists {
		metadata, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("H3 metadata must be an object")
		}
		for key, value := range metadata {
			if !contains([]string{"duration", "seconds", "resolution", "size", "ratio", "aspect_ratio"}, key) {
				return nil, fmt.Errorf("H3 metadata field %q is not supported", key)
			}
			if old, exists := fields[key]; exists && fmt.Sprint(old) != fmt.Sprint(value) {
				return nil, fmt.Errorf("H3 metadata conflicts with %s", key)
			}
			fields[key] = value
		}
	}
	prompt, err := h3Prompt(raw)
	if err != nil {
		return nil, err
	}
	duration := 0
	for _, key := range []string{"duration", "seconds"} {
		value, exists := fields[key]
		if !exists {
			continue
		}
		var n int
		switch v := value.(type) {
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v < float64(minDuration) || v > 15 {
				return nil, fmt.Errorf("H3 duration must be an integer between %d and 15 seconds", minDuration)
			}
			n = int(v)
		case string:
			if key != "seconds" {
				return nil, fmt.Errorf("H3 duration must be a JSON integer")
			}
			n, err = strconv.Atoi(v)
			if err != nil || n < minDuration || n > 15 {
				return nil, fmt.Errorf("H3 seconds must be an integer between %d and 15", minDuration)
			}
		default:
			return nil, fmt.Errorf("invalid H3 duration type")
		}
		if duration != 0 && duration != n {
			return nil, fmt.Errorf("H3 seconds conflicts with duration")
		}
		duration = n
	}
	if duration == 0 {
		return nil, fmt.Errorf("H3 duration is required; no implicit billable default")
	}
	resolveString := func(keys []string, uppercase bool) (string, error) {
		result := ""
		for _, key := range keys {
			value, exists := fields[key]
			if !exists {
				continue
			}
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return "", fmt.Errorf("H3 %s must be a non-empty string", key)
			}
			text = strings.TrimSpace(text)
			if uppercase {
				text = strings.ToUpper(text)
			}
			if result != "" && result != text {
				return "", fmt.Errorf("conflicting H3 %s aliases", keys[0])
			}
			result = text
		}
		return result, nil
	}
	resolution, err := resolveString([]string{"resolution", "size"}, true)
	if err != nil {
		return nil, err
	}
	if !contains(resolutions, resolution) {
		return nil, fmt.Errorf("unsupported H3 resolution for model; allowed: %s", strings.Join(resolutions, ", "))
	}
	ratio, err := resolveString([]string{"ratio", "aspect_ratio"}, false)
	if err != nil {
		return nil, err
	}
	if !contains([]string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}, ratio) {
		return nil, fmt.Errorf("H3 text-to-video requires an explicit supported ratio (adaptive is not supported)")
	}
	return &h3Request{Model: upstreamModel, Content: []h3Text{{Type: "text", Text: prompt}}, Duration: duration, Resolution: resolution, Ratio: ratio}, nil
}

// PrepareTaskRequest runs after model mapping, before billing/any paid request.
func (a *TaskAdaptor) PrepareTaskRequest(c *gin.Context, info *relaycommon.RelayInfo) error {
	a.h3Body = nil
	if !isH3Family(info.UpstreamModelName) {
		if strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
			var raw map[string]any
			if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
				return err
			}
			if _, exists := raw["content"]; exists {
				return fmt.Errorf("content-array video requests require an H3 upstream model")
			}
		}
		return nil
	}
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		return fmt.Errorf("H3 requires application/json")
	}
	var raw map[string]any
	if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
		return fmt.Errorf("invalid H3 JSON request")
	}
	payload, err := normalizeH3(raw, info.UpstreamModelName)
	if err != nil {
		return err
	}
	if _, expr := billing_setting.GetTieredExpr(info.OriginModelName); !expr {
		price, configured := ratio_setting.GetModelPrice(info.OriginModelName, false)
		if !configured {
			return fmt.Errorf("H3 requires an explicit task price or billing expression for the requested model")
		}
		if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 || price > 1000000 {
			return fmt.Errorf("H3 fixed task price must be finite and between 0 and 1000000 USD")
		}
	}
	a.h3Body, err = common.Marshal(payload)
	if err != nil {
		return err
	}
	info.BillingRequestInput = &billingexpr.RequestInput{Body: append([]byte(nil), a.h3Body...), Headers: info.RequestHeaders, EvaluationAt: info.StartTime}
	return nil
}

type h3TaskResponse struct {
	Task *struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Modality string `json:"modality"`
		Content  struct {
			URL string `json:"url"`
		} `json:"content"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"task"`
	Error any `json:"error"`
}

func (a *TaskAdaptor) parseH3Result(body []byte) (*relaycommon.TaskInfo, error) {
	var response h3TaskResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("invalid H3 query JSON")
	}
	task := response.Task
	if response.Error != nil || task == nil || task.ID == "" || (a.h3TaskID != "" && task.ID != a.h3TaskID) {
		return nil, fmt.Errorf("invalid H3 task response or mismatched task ID")
	}
	result := &relaycommon.TaskInfo{TaskID: task.ID}
	switch task.Status {
	case "queued":
		result.Status = model.TaskStatusQueued
		result.Progress = "20%"
	case "running":
		result.Status = model.TaskStatusInProgress
		result.Progress = "50%"
	case "succeeded":
		if task.Error != nil || (task.Modality != "" && task.Modality != "video") {
			return nil, fmt.Errorf("H3 success response is not a video result")
		}
		u, err := url.Parse(task.Content.URL)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, fmt.Errorf("H3 success response missing valid video URL")
		}
		result.Status = model.TaskStatusSuccess
		result.Progress = "100%"
		result.Url = task.Content.URL
	case "failed", "cancelled":
		result.Status = model.TaskStatusFailure
		result.Progress = "100%"
		result.Reason = "H3 task " + task.Status
		if task.Error != nil && task.Error.Code != "" {
			result.Reason += " (" + task.Error.Code + ")"
		}
	default:
		return nil, fmt.Errorf("unknown H3 task status")
	}
	// H3 usage tokens are not language tokens. Do not trigger generic token
	// settlement on top of the configured output-duration/fixed task charge.
	return result, nil
}
