package controller

import (
	"bytes"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
)

const aliyunPricingURL = "https://help.aliyun.com/zh/model-studio/model-pricing"

var aliyunCNYPrice = regexp.MustCompile(`^(原价 )?([0-9]+(?:\.[0-9]+)?)\s*元(?:[/／](百万 Token|百万Token|张|秒|分钟|小时|万字符))?(?:\s*（限时 ([0-9]+(?:\.[0-9]+)?) 折）)?$`)

func aliyunRate(cell, header string) (*float64, string) {
	if regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`).MatchString(cell) && strings.Contains(header, "元/百万 Token") {
		cell += " 元"
	}
	m := aliyunCNYPrice.FindStringSubmatch(cell)
	if m == nil {
		return nil, ""
	}
	unit := ""
	hint := header + " " + m[3]
	switch {
	case strings.Contains(hint, "每万字符") || m[3] == "万字符":
		unit = "million_characters"
	case strings.Contains(hint, "每百万") || strings.Contains(hint, "百万 Token") || strings.Contains(hint, "百万Token"):
		unit = "million_tokens"
	case m[3] == "张" || strings.Contains(hint, "每张"):
		unit = "image"
	case m[3] == "秒":
		unit = "second"
	case m[3] == "分钟":
		unit = "minute"
	case m[3] == "小时":
		unit = "hour"
	}
	if unit == "" {
		return nil, ""
	}
	v, _ := strconv.ParseFloat(m[2], 64)
	if m[4] != "" {
		d, _ := strconv.ParseFloat(m[4], 64)
		if d <= 0 || d > 10 {
			return nil, ""
		}
		v *= d / 10
	} else if m[1] != "" {
		return nil, ""
	}
	if unit == "million_characters" {
		v *= 100
	}
	return &v, unit
}

func aliyunRegion(t *html.Node) string {
	for n := t.Parent; n != nil; n = n.Parent {
		for _, a := range n.Attr {
			if a.Key != "id" {
				continue
			}
			for _, name := range []string{"华北2-北京", "中国内地", "全球", "国际", "美国", "新加坡", "中国香港", "德国"} {
				if a.Val == name || strings.HasPrefix(a.Val, name+"-") {
					return name
				}
			}
		}
	}
	return ""
}

func parseAliyunPricing(data []byte, names map[string]bool, address string) map[string]model.ModelCapabilityMetadata {
	prices := map[string]map[string][]model.OfficialPricingTier{}
	invalid := map[string]bool{}
	doc, _ := html.Parse(bytes.NewReader(data))
	if doc == nil {
		return nil
	}
	for _, node := range documentNodes(doc, "table") {
		var b bytes.Buffer
		_ = html.Render(&b, node)
		tables := documentTables(b.Bytes())
		if len(tables) != 1 {
			continue
		}
		t := tables[0]
		h := t[0]
		if len(h) < 2 || (!strings.Contains(h[0], "模型") && h[0] != "Model") {
			continue
		}
		sub := make([]string, len(h))
		start := 1
		if len(t) > 1 && t[1][0] == h[0] && len(t[1]) == len(h) {
			copy(sub, t[1])
			start = 2
		}
		for _, row := range t[start:] {
			if len(row) != len(h) {
				continue
			}
			fields := strings.Fields(row[0])
			if len(fields) == 0 {
				continue
			}
			id := fields[0]
			if !names[id] || !model.IsManufacturerPriceSource(id, address) {
				continue
			}
			region := aliyunRegion(node)
			var conditions []string
			for c, heading := range h {
				if c == 0 {
					continue
				}
				if strings.Contains(heading, "部署范围") || strings.Contains(heading, "服务范围") {
					region = row[c]
				}
				if strings.Contains(heading, "模式") || strings.Contains(heading, "范围") && strings.Contains(heading, "Token") || strings.Contains(strings.ReplaceAll(heading, " ", ""), "输入Token数") || strings.Contains(heading, "分辨率") {
					conditions = append(conditions, row[c])
				}
			}
			if region == "" {
				region = "来源页面默认地域"
			}
			base := region
			if len(conditions) > 0 {
				base += " · " + strings.Join(conditions, " · ")
			}
			variants := map[string]model.OfficialPricingTier{}
			failed := false
			for c, heading := range h {
				if c == 0 || !strings.Contains(heading, "单价") {
					continue
				}
				values := aliyunCellRates(row[c], heading)
				if len(values) == 0 {
					failed = true
					break
				}
				for _, cellRate := range values {
					value, unit := cellRate.value, cellRate.unit
					mode := sub[c]
					if mode == heading {
						mode = ""
					}
					mode = strings.TrimPrefix(strings.TrimPrefix(mode, "输入："), "输出：")
					if cellRate.mode != "" {
						mode = cellRate.mode
					}
					if unit == "million_tokens" {
						switch mode {
						case "图片/视频", "图片":
							unit = "million_image_tokens"
						case "音频":
							unit = "million_audio_tokens"
						case "视频":
							unit = "million_video_tokens"
						}
					}
					key := unit + "|" + mode
					tier := variants[key]
					tier.Unit = unit
					tier.Label = base
					if mode != "" {
						tier.Label += " · " + mode
					}
					switch {
					case strings.Contains(heading, "输入和输出"):
						tier.Input = value
						tier.Output = value
					case strings.Contains(heading, "缓存"):
						tier.CacheRead = value
					case strings.Contains(heading, "输入"):
						if tier.Input != nil {
							failed = true
						}
						tier.Input = value
					case strings.Contains(heading, "输出"):
						if tier.Output != nil {
							failed = true
						}
						tier.Output = value
					default:
						tier.Price = value
					}
					variants[key] = tier
				}
			}
			if failed || len(variants) == 0 {
				invalid[id+"|"+region] = true
				continue
			}
			// A shared input rate applies to both thinking/non-thinking output modes.
			generic, hasGeneric := variants["million_tokens|"]
			if hasGeneric && generic.Input != nil && generic.Output == nil && len(variants) > 1 {
				used := false
				for k, v := range variants {
					if v.Output != nil && v.Unit == "million_tokens" && strings.Contains(k, "思考") {
						v.Input = generic.Input
						v.CacheRead = generic.CacheRead
						variants[k] = v
						used = true
					}
				}
				if used {
					delete(variants, "million_tokens|")
				}
			}
			if prices[id] == nil {
				prices[id] = map[string][]model.OfficialPricingTier{}
			}
			keys := make([]string, 0, len(variants))
			for k := range variants {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				tier := variants[k]
				duplicate := false
				for _, old := range prices[id][region] {
					if old.Label == tier.Label && old.Unit == tier.Unit {
						duplicate = true
						if !reflect.DeepEqual(old, tier) {
							invalid[id+"|"+region] = true
						}
					}
				}
				if !duplicate {
					prices[id][region] = append(prices[id][region], tier)
				}
			}
		}
	}
	out := map[string]model.ModelCapabilityMetadata{}
	for id, regions := range prices {
		keys := make([]string, 0, len(regions))
		for k := range regions {
			if invalid[id+"|"+k] {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// Deterministic mainland reference; never combine prices across regions.
		for _, preferred := range []string{"中国内地", "华北2-北京"} {
			if _, ok := regions[preferred]; ok && !invalid[id+"|"+preferred] {
				keys = []string{preferred}
				break
			}
		}
		if len(keys) == 0 {
			continue
		}
		tiers := regions[keys[0]]
		p := model.OfficialPricing{Currency: "CNY", SourceURL: address, Notes: "按档位标注的地域、模式、模态与上下文范围展示官方按量价格；显式限时折扣按来源页面当前折扣计算。Batch、免费额度、缓存与额外工具费用请参阅来源页面，不从其他模型或地域推算。", Tiers: tiers}
		e := officialPriceEntry("alibaba", address, p)
		if len(e.OfficialPricing.Tiers) > 0 {
			out[id] = e
		}
	}
	return out
}

type aliyunCellRate struct {
	mode, unit string
	value      *float64
}

var aliyunModalRate = regexp.MustCompile(`(文本|图片)输入：([0-9]+(?:\.[0-9]+)?) 元`)
var aliyunPromptRate = regexp.MustCompile(`(关闭|开启)提示词改写（ prompt_extend=(false|true) ）：([0-9]+(?:\.[0-9]+)?) 元/张`)

func aliyunCellRates(cell, header string) []aliyunCellRate {
	if v, u := aliyunRate(cell, header); v != nil {
		return []aliyunCellRate{{unit: u, value: v}}
	}
	matches := aliyunModalRate.FindAllStringSubmatch(cell, -1)
	if len(matches) > 0 && strings.TrimSpace(aliyunModalRate.ReplaceAllString(cell, "")) == "" {
		var out []aliyunCellRate
		for _, m := range matches {
			v, u := aliyunRate(m[2]+" 元", header)
			if v == nil {
				return nil
			}
			out = append(out, aliyunCellRate{mode: m[1], unit: u, value: v})
		}
		return out
	}
	matches = aliyunPromptRate.FindAllStringSubmatch(cell, -1)
	if len(matches) == 2 && strings.TrimSpace(aliyunPromptRate.ReplaceAllString(cell, "")) == "" {
		out := []aliyunCellRate{}
		for _, m := range matches {
			if m[1] == "关闭" && m[2] != "false" || m[1] == "开启" && m[2] != "true" {
				return nil
			}
			v, u := aliyunRate(m[3]+" 元/张", header)
			out = append(out, aliyunCellRate{mode: "prompt_extend=" + m[2], unit: u, value: v})
		}
		return out
	}
	return nil
}
