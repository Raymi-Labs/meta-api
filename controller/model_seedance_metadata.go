package controller

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

const seedance25ID = "doubao-seedance-2-5-260628"
const seedance25URL = "https://docs.volcengine.com/docs/ark/seedance-2-5?lang=zh"
const seedancePricingURL = "https://docs.volcengine.com/docs/ark/model-pricing?lang=zh"

const seedanceFamilyURL = "https://docs.volcengine.com/docs/ark/video-generation-tutorial?lang=zh"

var seedanceModels = map[string]string{
	seedance25ID:                          "doubao-seedance-2.5",
	"doubao-seedance-2-0-260128":          "doubao-seedance-2.0",
	"doubao-seedance-2-0-fast-260128":     "doubao-seedance-2.0-fast",
	"doubao-seedance-2-0-mini-260615":     "doubao-seedance-2.0-mini",
	"doubao-seedance-1-0-pro-250528":      "doubao-seedance-1.0-pro",
	"doubao-seedance-1-0-pro-fast-251015": "doubao-seedance-1.0-pro-fast",
}

func parseSeedanceAttributes(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	empty := model.ModelCapabilityMetadata{}
	if seedanceModels[id] == "" {
		return empty, errors.New("unknown Seedance version")
	}
	for _, table := range documentTables(data) {
		if len(table) < 2 {
			continue
		}
		ids := []string{}
		for _, v := range table[1] {
			if v != "" {
				ids = append(ids, v)
			}
		}
		if len(ids) < 2 || ids[0] != "Model ID" {
			continue
		}
		col := -1
		for i, v := range ids[1:] {
			if v == id {
				col = i
			}
		}
		if col < 0 {
			continue
		}
		values := map[string]string{}
		for _, row := range table[2:] {
			cells := []string{}
			for _, v := range row {
				if v != "" {
					cells = append(cells, v)
				}
			}
			n := len(ids) - 1
			if len(cells) < n+1 {
				continue
			}
			labels := cells[:len(cells)-n]
			key := strings.Join(labels, " ")
			values[key] = cells[len(cells)-n+col]
		}
		first := values["图生视频-首帧"]
		if first == "" {
			first = values["首帧生视频"]
		}
		if values["文生视频"] != "✓" || first != "✓" {
			continue
		}
		e := model.ModelCapabilityMetadata{Provider: "bytedance", Tags: "video", InputModalities: model.ModelModalities{"text", "image"}, OutputModalities: model.ModelModalities{"video"}, Capabilities: model.ModelCapabilities{}}
		// Conditional audio reference still establishes audio input, not standalone audio support.
		if v := values["全模态参考 音频参考"]; v == "✓" || v == "✗（需搭配图片/视频）" {
			e.InputModalities = append(e.InputModalities, "audio")
		}
		if values["全模态参考 视频参考"] == "✓" {
			e.InputModalities = append(e.InputModalities, "video")
		}
		if values["生成有声视频"] == "✓" {
			e.OutputModalities = append(e.OutputModalities, "audio")
		}
		if v, ok := documentBool(values["联网搜索工具"]); ok {
			e.Capabilities["web_search"] = v
		}
		return markOfficialAttributes(e, address), nil
	}
	return empty, errors.New("Seedance exact capability table missing or changed")
}

func parseSeedancePricing(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	if seedanceModels[id] == "" {
		return model.ModelCapabilityMetadata{}, errors.New("unknown Seedance version")
	}
	for _, table := range documentTables(data) {
		if len(table) < 2 || len(table[0]) != 3 || table[0][0] != "模型" || table[0][1] != "在线推理 元/M token" || table[0][2] != "离线推理 元/M token" {
			continue
		}
		for _, row := range table[1:] {
			if len(row) != 3 || len(strings.Fields(row[0])) == 0 || strings.Fields(row[0])[0] != seedanceModels[id] {
				continue
			}
			p := model.OfficialPricing{Currency: "CNY", SourceURL: strings.Split(address, "?")[0], Notes: "按实际视频 token 用量计费；输入视频可能影响用量及最低用量限制。官方 token 单价不能直接与本站按秒价格比较。"}
			if strings.HasPrefix(seedanceModels[id], "doubao-seedance-1.0-") {
				for i, label := range []string{"在线推理", "离线推理"} {
					if !regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`).MatchString(row[i+1]) {
						return model.ModelCapabilityMetadata{}, errors.New("invalid Seedance price")
					}
					v, err := strconv.ParseFloat(row[i+1], 64)
					if err != nil {
						return model.ModelCapabilityMetadata{}, err
					}
					p.Tiers = append(p.Tiers, model.OfficialPricingTier{Label: label, Unit: "million_tokens", Price: &v})
				}
			} else {
				pattern := regexp.MustCompile(`输出视频分辨率为 (480p，720p|1080p|4K) 输入不含视频：([0-9]+(?:\.[0-9]+)?) 输入包含视频：([0-9]+(?:\.[0-9]+)?)`)
				matches := pattern.FindAllStringSubmatch(row[1], -1)
				expected := []string{"480p，720p"}
				if id == seedance25ID {
					expected = append(expected, "1080p")
				}
				if seedanceModels[id] == "doubao-seedance-2.0" {
					expected = append(expected, "1080p", "4K")
				}
				if len(matches) != len(expected) || strings.TrimSpace(pattern.ReplaceAllString(row[1], "")) != "" {
					continue
				}
				for i, m := range matches {
					if m[1] != expected[i] {
						return model.ModelCapabilityMetadata{}, errors.New("Seedance resolution changed")
					}
					for j, label := range []string{"输入不含视频", "输入包含视频"} {
						v, err := strconv.ParseFloat(m[j+2], 64)
						if err != nil {
							return model.ModelCapabilityMetadata{}, err
						}
						p.Tiers = append(p.Tiers, model.OfficialPricingTier{Label: strings.ReplaceAll(m[1], "，", " / ") + " · " + label, Unit: "million_tokens", Price: &v})
					}
				}
			}
			if err := p.Normalize(model.NameRuleExact); err != nil {
				return model.ModelCapabilityMetadata{}, err
			}
			return officialPriceEntry("bytedance", p.SourceURL, p), nil
		}
	}
	return model.ModelCapabilityMetadata{}, errors.New("Seedance CNY price table missing or changed")
}
