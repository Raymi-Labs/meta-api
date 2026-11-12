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

// Only the verified version is mapped to the manufacturer's family price row.
// Future versions and flash variants require their own identity verification.
func parseSeedanceAttributes(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	empty := model.ModelCapabilityMetadata{}
	if id != seedance25ID {
		return empty, errors.New("unknown Seedance version")
	}
	for _, table := range documentTables(data) {
		if len(table) < 2 {
			continue
		}
		ids := table[1]
		if len(ids) < 3 || ids[0] != "Model ID" {
			continue
		}
		col := -1
		for i, v := range ids {
			if v == id {
				col = i
			}
		}
		if col < 2 || len(table[0]) != len(ids) || table[0][col] != "Seedance 2.5" {
			continue
		}
		values := map[string]string{}
		for _, row := range table[2:] {
			// Do not consume irregular rowspan rows: a shifted column can describe
			// a neighboring model. Simple capability rows have the identity width.
			if len(row) == len(ids) && row[1] == "" {
				values[row[0]] = row[col]
			}
		}
		if values["文生视频"] != "✓" || values["首帧生视频"] != "✓" || values["生成有声视频"] != "✓" {
			continue
		}
		// Reference counts explicitly establish all three reference modalities.
		if values["参考素材数量上限"] != "50（30张图+10个视频+10个音频）" {
			continue
		}
		e := model.ModelCapabilityMetadata{Provider: "bytedance", Tags: "video", InputModalities: model.ModelModalities{"text", "image", "audio", "video"}, OutputModalities: model.ModelModalities{"video", "audio"}, Capabilities: model.ModelCapabilities{}}
		if v := values["联网搜索工具"]; v == "✓" {
			e.Capabilities["web_search"] = true
		} else if v == "✗" {
			e.Capabilities["web_search"] = false
		}
		return markOfficialAttributes(e, address), nil
	}
	return empty, errors.New("Seedance exact capability table missing or changed")
}

var seedance25PriceRow = regexp.MustCompile(`^输出视频分辨率为 480p，720p 输入不含视频：([0-9]+(?:\.[0-9]+)?) 输入包含视频：([0-9]+(?:\.[0-9]+)?) 输出视频分辨率为 1080p 输入不含视频：([0-9]+(?:\.[0-9]+)?) 输入包含视频：([0-9]+(?:\.[0-9]+)?)$`)

func parseSeedancePricing(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	if id != seedance25ID {
		return model.ModelCapabilityMetadata{}, errors.New("unknown Seedance version")
	}
	for _, table := range documentTables(data) {
		if len(table) < 2 || len(table[0]) != 3 || table[0][0] != "模型" || table[0][1] != "在线推理 元/M token" || table[0][2] != "离线推理 元/M token" {
			continue
		}
		for _, row := range table[1:] {
			if len(row) != 3 || row[0] != "doubao-seedance-2.5 按输出视频分辨率和输入是否包含视频区分定价" {
				continue
			}
			m := seedance25PriceRow.FindStringSubmatch(row[1])
			if m == nil {
				continue
			}
			p := model.OfficialPricing{Currency: "CNY", SourceURL: strings.Split(address, "?")[0], Notes: "在线推理；按实际视频 token 用量计费，包含输入视频时还计入输入视频时长并适用最低用量限制。官方 token 单价不能直接与本站按秒价格比较。"}
			for i, label := range []string{"480p / 720p · 输入不含视频", "480p / 720p · 输入包含视频", "1080p · 输入不含视频", "1080p · 输入包含视频"} {
				v, err := strconv.ParseFloat(m[i+1], 64)
				if err != nil {
					return model.ModelCapabilityMetadata{}, err
				}
				p.Tiers = append(p.Tiers, model.OfficialPricingTier{Label: label, Unit: "million_tokens", Price: &v})
			}
			if err := p.Normalize(model.NameRuleExact); err != nil {
				return model.ModelCapabilityMetadata{}, err
			}
			return officialPriceEntry("bytedance", strings.Split(address, "?")[0], p), nil
		}
	}
	return model.ModelCapabilityMetadata{}, errors.New("Seedance CNY online price table missing or changed")
}
