package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	ChannelVendorAll      = "all"
	ChannelVendorAuto     = "auto"
	ChannelVendorProtocol = "protocol"
	ChannelVendorMiMo     = "mimo"

	ChannelCategoryAll          = "all"
	ChannelCategoryTypePrefix   = "type:"
	ChannelCategoryVendorPrefix = "vendor:"
)

// Display vendors never change channel selection or protocol. Opt-in reasoning
// adaptation may use the resolved vendor to select a bounded model template.
var channelVendorSegments = map[string][]string{
	"mimo":      {"mimo"},
	"deepseek":  {"deepseek"},
	"openai":    {"gpt", "chatgpt", "o1", "o3", "o4", "dall", "whisper"},
	"anthropic": {"claude"},
	"google":    {"gemini", "gemma"},
	"qwen":      {"qwen", "qwen2", "qwen3", "qwq", "qvq"},
	"moonshot":  {"kimi", "moonshot"},
	"zhipu":     {"glm", "chatglm"},
	"xai":       {"grok"},
	"minimax":   {"minimax"},
	"mistral":   {"mistral", "mixtral", "codestral", "pixtral", "ministral", "devstral"},
	"inception": {"mercury"},
}

type ChannelCategory struct {
	Key    string
	Type   int
	Vendor string
}

func (category ChannelCategory) IsAll() bool {
	return category.Key == ChannelCategoryAll
}

func NormalizeChannelVendorSetting(vendor string) string {
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	switch vendor {
	case "", ChannelVendorAuto:
		return ChannelVendorAuto
	case ChannelVendorProtocol:
		return ChannelVendorProtocol
	case "claude":
		return "anthropic"
	case "gemini":
		return "google"
	case "kimi":
		return "moonshot"
	case "glm":
		return "zhipu"
	case "grok":
		return "xai"
	case "inception labs", "inceptionlabs", "mercury":
		return "inception"
	case ChannelVendorMiMo, strings.ToLower(defaultMiMoVendorName), "xiaomi", "xiaomi mimo":
		return ChannelVendorMiMo
	default:
		return vendor
	}
}

func IsSupportedChannelVendorSetting(vendor string) bool {
	vendor = NormalizeChannelVendorSetting(vendor)
	if vendor == ChannelVendorAuto || vendor == ChannelVendorProtocol {
		return true
	}
	_, ok := channelVendorSegments[vendor]
	return ok
}

func (channel *Channel) GetChannelVendorSetting() string {
	if channel == nil || channel.ChannelVendor == nil {
		return ChannelVendorAuto
	}
	return NormalizeChannelVendorSetting(*channel.ChannelVendor)
}

// NormalizeChannelVendorFilter converts the public query value into a stable
// internal key. Unknown values are kept so they fail closed instead of
// accidentally returning every channel.
func NormalizeChannelVendorFilter(vendor string) string {
	if strings.TrimSpace(vendor) == "" {
		return ChannelVendorAll
	}
	return NormalizeChannelVendorSetting(vendor)
}

func ParseChannelCategory(value string) (ChannelCategory, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == ChannelCategoryAll {
		return ChannelCategory{Key: ChannelCategoryAll, Type: -1}, nil
	}
	if strings.HasPrefix(value, ChannelCategoryTypePrefix) {
		typeValue := strings.TrimSpace(strings.TrimPrefix(value, ChannelCategoryTypePrefix))
		channelType, err := strconv.Atoi(typeValue)
		if err != nil || channelType < 0 {
			return ChannelCategory{}, fmt.Errorf("invalid channel category: %s", value)
		}
		return ChannelCategory{Key: ChannelCategoryTypePrefix + strconv.Itoa(channelType), Type: channelType}, nil
	}
	if strings.HasPrefix(value, ChannelCategoryVendorPrefix) {
		vendor := NormalizeChannelVendorFilter(strings.TrimPrefix(value, ChannelCategoryVendorPrefix))
		if vendor == ChannelVendorAll || !IsSupportedChannelVendorSetting(vendor) || vendor == ChannelVendorAuto || vendor == ChannelVendorProtocol {
			return ChannelCategory{}, fmt.Errorf("invalid channel category: %s", value)
		}
		return ChannelCategory{Key: ChannelCategoryVendorPrefix + vendor, Type: -1, Vendor: vendor}, nil
	}
	return ChannelCategory{}, fmt.Errorf("invalid channel category: %s", value)
}

func resolveMappedChannelModel(channel *Channel, modelName string) string {
	if channel == nil || channel.ModelMapping == nil || strings.TrimSpace(*channel.ModelMapping) == "" {
		return modelName
	}
	mapping := make(map[string]string)
	if err := common.Unmarshal([]byte(*channel.ModelMapping), &mapping); err != nil {
		return modelName
	}
	if mapped := strings.TrimSpace(mapping[strings.TrimSpace(modelName)]); mapped != "" {
		return mapped
	}
	return modelName
}

func inferModelDisplayVendor(modelName string) string {
	// This known alias has no GPT segment; keep other Codex names unclassified.
	if strings.EqualFold(strings.TrimSpace(modelName), "codex-auto-review") {
		return "openai"
	}
	vendor := ""
	for candidate, segments := range channelVendorSegments {
		if hasModelNameSegment(modelName, segments...) {
			// Distilled or namespaced models can name more than one vendor.
			if vendor != "" {
				return ""
			}
			vendor = candidate
		}
	}
	return vendor
}

func InferChannelVendor(channel *Channel) string {
	if channel == nil {
		return ""
	}
	vendor := ""
	for _, modelName := range channel.GetModels() {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		candidate := inferModelDisplayVendor(resolveMappedChannelModel(channel, modelName))
		if candidate == "" || (vendor != "" && vendor != candidate) {
			return ""
		}
		vendor = candidate
	}
	return vendor
}

func ResolveChannelVendor(channel *Channel) string {
	switch channel.GetChannelVendorSetting() {
	case ChannelVendorProtocol:
		return ""
	case ChannelVendorAuto:
		return InferChannelVendor(channel)
	default:
		return channel.GetChannelVendorSetting()
	}
}

// ChannelMatchesVendor uses the explicit channel vendor first and falls back
// to conservative model inference. Channel.Type remains the relay protocol.
func ChannelMatchesVendor(channel *Channel, vendor string) bool {
	if channel == nil {
		return false
	}

	switch NormalizeChannelVendorFilter(vendor) {
	case ChannelVendorAll:
		return true
	default:
		return ResolveChannelVendor(channel) == NormalizeChannelVendorFilter(vendor)
	}
}

func FilterChannelsByVendor(channels []*Channel, vendor string) []*Channel {
	vendor = NormalizeChannelVendorFilter(vendor)
	if vendor == ChannelVendorAll {
		return channels
	}

	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if ChannelMatchesVendor(channel, vendor) {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

// CountChannelVendors returns independent facet counts. "all" is the number
// of distinct channels, while a multi-model channel is counted at most once
// for each supported vendor.
func CountChannelVendors(channels []*Channel) map[string]int64 {
	counts := map[string]int64{
		ChannelVendorAll:  0,
		ChannelVendorMiMo: 0,
	}
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		counts[ChannelVendorAll]++
		if vendor := ResolveChannelVendor(channel); vendor != "" {
			counts[vendor]++
		}
	}
	return counts
}

func CountChannelTypes(channels []*Channel) map[int64]int64 {
	counts := make(map[int64]int64)
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		counts[int64(channel.Type)]++
	}
	return counts
}

func CountChannelCategories(channels []*Channel) map[string]int64 {
	counts := map[string]int64{ChannelCategoryAll: 0}
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		counts[ChannelCategoryAll]++
		if vendor := ResolveChannelVendor(channel); vendor != "" {
			counts[ChannelCategoryVendorPrefix+vendor]++
			continue
		}
		counts[ChannelCategoryTypePrefix+strconv.Itoa(channel.Type)]++
	}
	return counts
}

func FilterChannelsByType(channels []*Channel, typeFilter int) []*Channel {
	if typeFilter < 0 {
		return channels
	}
	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if channel != nil && channel.Type == typeFilter {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

func FilterChannelsByCategory(channels []*Channel, category ChannelCategory) []*Channel {
	if category.IsAll() {
		return channels
	}

	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if category.Vendor != "" {
			if ChannelMatchesVendor(channel, category.Vendor) {
				filtered = append(filtered, channel)
			}
			continue
		}
		if channel.Type == category.Type && ResolveChannelVendor(channel) == "" {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}
