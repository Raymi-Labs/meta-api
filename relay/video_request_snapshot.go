package relay

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
)

// PrepareVideoRequestSnapshot never contacts a provider. It also serves local
// quotes, so byte conversion and the billed parameter view cannot diverge.
func PrepareVideoRequestSnapshot(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.TaskAdaptor) ([]byte, error) {
	body, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if info.BillingRequestInput != nil {
		return data, nil
	} // H3 already prepared.
	var wire map[string]any
	if common.Unmarshal(data, &wire) != nil {
		if _, expr := billing_setting.GetTieredExpr(info.OriginModelName); expr {
			return nil, fmt.Errorf("expression pricing requires a normalized JSON video request; multipart expression pricing is not supported")
		}
		return data, nil
	}
	view := make(map[string]any)
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	original, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	_ = common.Unmarshal(original, &view)
	if view == nil {
		view = make(map[string]any)
	}
	for key, value := range wire {
		view[key] = value
	}
	params := wire
	if nested, ok := wire["parameters"].(map[string]any); ok {
		params = nested
	}
	// Remove raw aliases first: absent provider parameters are unknown, not the
	// original caller's values that an adapter may have discarded.
	for _, key := range []string{"duration", "durationSeconds", "seconds", "resolution", "size", "sampleCount", "mode", "audio", "generate_audio", "generateAudio"} {
		delete(view, key)
	}
	// Kling's std/pro mode is its billing-quality tier, not a pixel resolution.
	// Offer this explicit tier to the price table without inventing pixel sizes.
	if info.ChannelType == constant.ChannelTypeKling {
		if mode, ok := params["mode"]; ok {
			view["resolution"] = mode
		}
	}
	for _, key := range []string{"resolution", "size", "sampleCount", "mode", "audio", "generate_audio", "generateAudio"} {
		if value, ok := params[key]; ok {
			view[key] = value
		}
	}
	if _, ok := view["resolution"]; !ok {
		if size, exists := view["size"]; exists {
			view["resolution"] = size
		}
	}
	if _, ok := view["sampleCount"]; !ok {
		view["sampleCount"] = float64(1)
	}
	for _, key := range []string{"duration", "durationSeconds", "seconds"} {
		if value, ok := params[key]; ok {
			n, err := videoNumber(value)
			if err != nil {
				// Seedance may request automatic duration (-1). Keep its legacy
				// usage-based billing, but never invent a fixed positive seconds quote.
				if (info.ChannelType == constant.ChannelTypeDoubaoVideo || info.ChannelType == constant.ChannelTypeVolcEngine) && fmt.Sprint(value) == "-1" {
					break
				}
				return nil, fmt.Errorf("invalid outgoing video duration")
			}
			view["duration"] = n
			view["seconds"] = n
			view["durationSeconds"] = n
			break
		}
	}
	if info.ChannelType == constant.ChannelTypeJimeng {
		// frames are not seconds; leave duration absent rather than guess FPS.
		delete(view, "duration")
		delete(view, "seconds")
		delete(view, "durationSeconds")
	}
	if request, err := relaycommon.GetTaskRequest(c); err == nil && info.ChannelType != constant.ChannelTypeJimeng && info.Action != constant.TaskActionRemix {
		requested := float64(request.Duration)
		if request.Seconds != "" {
			n, err := videoNumber(request.Seconds)
			if err != nil {
				return nil, err
			}
			if requested > 0 && requested != n {
				return nil, fmt.Errorf("conflicting video duration aliases")
			}
			requested = n
		}
		if requested > 0 {
			actual, ok := view["duration"].(float64)
			if !ok || actual != requested {
				return nil, fmt.Errorf("outgoing video duration differs from requested duration")
			}
		}
	}
	if expr, ok := billing_setting.GetTieredExpr(info.OriginModelName); ok && strings.Contains(expr, "video_config_preview") {
		if text, ok := view["resolution"].(string); ok {
			view["resolution"] = strings.ToUpper(strings.TrimSpace(text))
		}
		if _, exists := view["duration"]; !exists {
			return nil, fmt.Errorf("video pricing requires explicit effective output duration; adapter did not produce one")
		}
		if resolution, ok := view["resolution"].(string); !ok || strings.TrimSpace(resolution) == "" {
			return nil, fmt.Errorf("video pricing requires an effective resolution/size")
		}
		if n, err := videoNumber(view["sampleCount"]); err != nil || n > 100 {
			return nil, fmt.Errorf("invalid video sample count")
		}
	}
	// Legacy expressions may read metadata.duration/parameters.duration. Keep
	// those paths only with effective billable values, not discarded overrides.
	if metadata, ok := view["metadata"].(map[string]any); ok {
		copyMeta := make(map[string]any, len(metadata))
		for k, v := range metadata {
			copyMeta[k] = v
		}
		for _, key := range []string{"model", "model_name", "duration", "durationSeconds", "seconds", "resolution", "size", "sampleCount", "mode", "audio", "generate_audio", "generateAudio"} {
			delete(copyMeta, key)
			if value, exists := view[key]; exists {
				copyMeta[key] = value
			}
		}
		delete(copyMeta, "parameters")
		if wire["parameters"] != nil {
			copyMeta["parameters"] = params
		}
		view["metadata"] = copyMeta
	}
	if wire["parameters"] == nil {
		delete(view, "parameters")
	}
	encoded, err := common.Marshal(view)
	if err != nil {
		return nil, err
	}
	info.BillingRequestInput = &billingexpr.RequestInput{Body: encoded, Headers: info.RequestHeaders, EvaluationAt: info.StartTime}
	return data, nil
}

func videoNumber(value any) (float64, error) {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case string:
		var err error
		n, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("invalid numeric parameter")
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n != math.Trunc(n) {
		return 0, fmt.Errorf("duration must be a positive integer")
	}
	return n, nil
}
