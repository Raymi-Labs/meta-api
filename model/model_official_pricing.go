package model

import (
	"database/sql/driver"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// OfficialPricing is display-only metadata, never a source for billing ratios.
// Prices are in the declared currency and unit; nil means unknown, zero is free.
type OfficialPricing struct {
	Currency  string                `json:"currency,omitempty"`
	SourceURL string                `json:"source_url,omitempty"`
	Notes     string                `json:"notes,omitempty"`
	Tiers     []OfficialPricingTier `json:"tiers,omitempty"`
}

type OfficialPricingTier struct {
	Label      string   `json:"label"`
	Unit       string   `json:"unit"`
	Input      *float64 `json:"input,omitempty"`
	Output     *float64 `json:"output,omitempty"`
	CacheRead  *float64 `json:"cache_read,omitempty"`
	CacheWrite *float64 `json:"cache_write,omitempty"`
	Price      *float64 `json:"price,omitempty"`
}

func (p OfficialPricing) Value() (driver.Value, error) {
	data, err := common.Marshal(p)
	return string(data), err
}

func (p *OfficialPricing) Scan(value interface{}) error {
	*p = OfficialPricing{}
	var data []byte
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return fmt.Errorf("unsupported official pricing database value %T", value)
	}
	if len(data) == 0 {
		return nil
	}
	return common.Unmarshal(data, p)
}

func (p *OfficialPricing) Normalize(nameRule int) error {
	if len(p.Tiers) == 0 {
		*p = OfficialPricing{}
		return nil
	}
	if nameRule != NameRuleExact {
		return fmt.Errorf("官方参考价仅支持精确匹配模型")
	}
	if len(p.Tiers) > 20 {
		return fmt.Errorf("官方参考价最多 20 档")
	}
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	if p.Currency != "USD" && p.Currency != "CNY" && p.Currency != "EUR" {
		return fmt.Errorf("官方参考价币种仅支持 USD、CNY、EUR")
	}
	p.SourceURL = strings.TrimSpace(p.SourceURL)
	if len(p.SourceURL) > 2048 {
		return fmt.Errorf("官方价格来源链接过长")
	}
	if p.SourceURL != "" {
		u, err := url.Parse(p.SourceURL)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return fmt.Errorf("官方价格来源必须为 HTTP(S) 链接")
		}
	}
	p.Notes = strings.TrimSpace(p.Notes)
	if utf8.RuneCountInString(p.Notes) > 2000 {
		return fmt.Errorf("官方价格说明不能超过 2000 个字符")
	}
	for i := range p.Tiers {
		tier := &p.Tiers[i]
		tier.Label = strings.TrimSpace(tier.Label)
		if tier.Label == "" || utf8.RuneCountInString(tier.Label) > 128 {
			return fmt.Errorf("官方价格档位名称须为 1 至 128 个字符")
		}
		switch tier.Unit {
		case "million_tokens", "million_characters", "million_audio_tokens", "million_image_tokens", "million_video_tokens", "request", "image", "second", "minute", "hour":
		default:
			return fmt.Errorf("官方参考价计价单位无效")
		}
		hasPrice := false
		for _, price := range []*float64{tier.Input, tier.Output, tier.CacheRead, tier.CacheWrite, tier.Price} {
			if price == nil {
				continue
			}
			hasPrice = true
			if math.IsNaN(*price) || math.IsInf(*price, 0) || *price < 0 || *price > 1e9 {
				return fmt.Errorf("官方参考价必须为 0 至 1000000000 的有效数值")
			}
		}
		if !hasPrice {
			return fmt.Errorf("每档官方参考价至少填写一个价格")
		}
	}
	return nil
}

// Compare values rather than pointer identity or legacy JSON whitespace.
func (p OfficialPricing) Equal(other OfficialPricing) bool { return reflect.DeepEqual(p, other) }
