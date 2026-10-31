package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NormalizeImagePricing reads exactly the converted, overridden payload to be
// sent. User-provided private billing fields are deliberately never read.
func NormalizeImagePricing(info *relaycommon.RelayInfo, body []byte, count int) error {
	info.ImagePricingInput, info.ImagePricingResult = nil, nil
	if info.TieredBillingSnapshot == nil || !common.IsImagePriceExpression(info.TieredBillingSnapshot.ExprString) {
		return nil
	}
	if info.ChannelType != constant.ChannelTypeAli || !common.IsQwen3ImageModel(gjson.GetBytes(body, "model").String()) {
		return fmt.Errorf("image input/output pricing requires an Ali Qwen Image 3.0 channel")
	}
	if count < 1 || count > 6 {
		return fmt.Errorf("Qwen Image 3.0 n must be between 1 and 6")
	}
	messages := gjson.GetBytes(body, "input.messages")
	if !messages.IsArray() || len(messages.Array()) != 1 {
		return fmt.Errorf("image pricing requires one input message")
	}
	content := messages.Array()[0].Get("content")
	if !content.IsArray() {
		return fmt.Errorf("invalid image message content")
	}
	inputs := 0
	for _, item := range content.Array() {
		if value := item.Get("image"); value.Exists() {
			if value.Type != gjson.String || strings.TrimSpace(value.String()) == "" {
				return fmt.Errorf("invalid input image")
			}
			inputs++
		}
	}
	if inputs > 3 {
		return fmt.Errorf("Qwen Image 3.0 supports at most 3 input images")
	}
	size := gjson.GetBytes(body, "parameters.size")
	resolution := "auto"
	if size.Exists() && size.String() != "" && size.String() != "auto" {
		parts := strings.Split(strings.ReplaceAll(size.String(), "x", "*"), "*")
		if len(parts) != 2 {
			return fmt.Errorf("image size must be auto or WIDTH*HEIGHT")
		}
		width, errW := strconv.Atoi(parts[0])
		height, errH := strconv.Atoi(parts[1])
		if errW != nil || errH != nil || width < 1 || height < 1 || width > 8192 || height > 8192 || width*height < 512*512 || width*height > 2048*2048 {
			return fmt.Errorf("invalid Qwen Image 3.0 size")
		}
		resolution = imageResolution(width, height)
	}
	info.ImagePricingInput = &relaycommon.ImagePricingDimensions{InputCount: inputs, OutputCount: count, Resolution: resolution}
	return nil
}

func imageResolution(width, height int) string {
	if int64(width)*int64(height) > 2250000 {
		return "2K"
	}
	return "1K"
}

// SetImagePricingResult requires upstream metering for this opt-in template.
// Counts must agree with real payloads, so empty/partial responses cannot be
// reported as a successful full generation or billed using requested n.
func SetImagePricingResult(info *relaycommon.RelayInfo, inputs, outputs *int, width, height int, tier string, payloads int) error {
	if info.ImagePricingInput == nil {
		return nil
	}
	if inputs == nil || outputs == nil || *inputs < 0 || *inputs > info.ImagePricingInput.InputCount || *outputs < 1 || *outputs != payloads || *outputs > info.ImagePricingInput.OutputCount {
		return fmt.Errorf("upstream image usage is missing or inconsistent")
	}
	resolution := ""
	switch tier {
	case "qima_output_1k":
		resolution = "1K"
	case "qima_output_2k":
		resolution = "2K"
	case "":
	default:
		return fmt.Errorf("unknown image usage tier")
	}
	if width > 0 && height > 0 && width <= 8192 && height <= 8192 {
		measured := imageResolution(width, height)
		if resolution != "" && resolution != measured {
			return fmt.Errorf("inconsistent image usage resolution")
		}
		resolution = measured
	}
	if resolution == "" {
		return fmt.Errorf("upstream image resolution usage is missing")
	}
	info.ImagePricingResult = &relaycommon.ImagePricingDimensions{InputCount: *inputs, OutputCount: *outputs, Resolution: resolution}
	return nil
}

func withImagePricing(body []byte, dims *relaycommon.ImagePricingDimensions) ([]byte, error) {
	if len(body) == 0 {
		body = []byte(`{}`)
	}
	return sjson.SetBytes(body, common.ImagePricingField, dims)
}
