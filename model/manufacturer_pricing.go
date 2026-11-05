package model

import (
	"net/url"
	"strings"
)

// Explicit manufacturer namespaces and first-party origins. Unknown providers
// remain unknown; reseller hosting does not establish a manufacturer's price.
var manufacturerPriceDomains = map[string][]string{
	"amazon": {"amazon.com", "amazonaws.com"}, "thinkingmachines": {"thinkingmachines.ai"},
	"openai": {"openai.com"}, "anthropic": {"anthropic.com", "claude.com"},
	"google": {"ai.google.dev", "cloud.google.com"}, "cohere": {"cohere.com"},
	"deepseek": {"deepseek.com"}, "x-ai": {"x.ai"},
	"moonshotai": {"moonshot.ai", "moonshot.cn", "kimi.ai", "kimi.com"},
	"xiaomi":     {"mimo.mi.com"}, "minimax": {"minimax.io", "minimaxi.com"},
	"z-ai": {"z.ai", "bigmodel.cn"}, "qwen": {"aliyun.com", "alibabacloud.com", "qwen.ai"},
	"mistralai": {"mistral.ai"}, "inception": {"inceptionlabs.ai"},
	"typesafe": {"typesafe.ai"},
	"meta":     {"meta.ai"}, "liquid": {"liquid.ai"}, "moondream": {"moondream.ai"}, "rekaai": {"reka.ai"}, "meituan": {"longcat.chat"}, "fireworks": {"fireworks.ai"}, "perceptron": {"perceptron.inc"}, "particle-ai": {"particle.ai"}, "inference-net": {"inference.net"}, "unbiased": {"unbiased.ai"},
	"writer": {"writer.com"}, "upstage": {"upstage.ai"}, "morph": {"morphllm.com"}, "aion-labs": {"aionlabs.ai"},
	"perplexity": {"perplexity.ai"}, "meta-llama": {"llama.com", "meta.com"},
	"ai21": {"ai21.com"}, "baidu": {"baidu.com", "baidubce.com"},
	"tencent": {"tencent.com", "tencentcloud.com"}, "bytedance": {"volcengine.com", "byteplus.com"},
}

func manufacturerForPrice(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if namespace, _, ok := strings.Cut(name, "/"); ok {
		return namespace // Never infer a different maker from a reseller's model suffix.
	}
	for _, rule := range []struct{ prefix, manufacturer string }{
		{"jev-", "typesafe"}, {"nova-", "amazon"}, {"inkling", "thinkingmachines"}, {"gpt-", "openai"}, {"chatgpt-", "openai"}, {"o1", "openai"}, {"o3", "openai"}, {"o4", "openai"},
		{"claude-", "anthropic"}, {"gemini-", "google"}, {"gemma-", "google"}, {"veo-", "google"}, {"imagen-", "google"},
		{"command-", "cohere"}, {"deepseek-", "deepseek"}, {"grok-", "x-ai"},
		{"kimi-", "moonshotai"}, {"moonshot-", "moonshotai"}, {"mimo-", "xiaomi"},
		{"wan", "qwen"}, {"happyhorse-", "qwen"}, {"happyoyster-", "qwen"}, {"z-image-", "qwen"}, {"minimax-", "minimax"}, {"glm-", "z-ai"}, {"qwen", "qwen"},
		{"mistral", "mistralai"}, {"ministral", "mistralai"}, {"codestral", "mistralai"}, {"devstral", "mistralai"}, {"pixtral", "mistralai"},
		{"muse-", "meta"}, {"lfm-", "liquid"}, {"moondream", "moondream"}, {"reka-", "rekaai"}, {"longcat-", "meituan"}, {"ember-", "fireworks"}, {"perceptron-", "perceptron"}, {"particle-", "particle-ai"}, {"schematron-", "inference-net"}, {"pareto-", "unbiased"}, {"aion-", "aion-labs"}, {"palmyra-", "writer"}, {"solar-", "upstage"}, {"morph-v3-", "morph"}, {"mercury", "inception"}, {"sonar", "perplexity"}, {"llama", "meta-llama"},
		{"ernie", "baidu"}, {"hunyuan", "tencent"}, {"doubao", "bytedance"},
	} {
		if strings.HasPrefix(name, rule.prefix) {
			return rule.manufacturer
		}
	}
	return ""
}

func priceSourceHost(address string) string {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func hostWithin(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func IsManufacturerPriceSource(name, address string) bool {
	if strings.Contains(name, ":") {
		return false
	}
	host := priceSourceHost(address)
	if host == "" {
		return false
	}
	for _, domain := range manufacturerPriceDomains[manufacturerForPrice(name)] {
		if hostWithin(host, domain) {
			return true
		}
	}
	return false
}

// Legacy imported reseller sheets can be replaced after successful official
// lookup, even without an automatic snapshot. No other manual sheet is claimed.
func (p OfficialPricing) IsLegacyResellerPrice() bool {
	u, err := url.Parse(p.SourceURL)
	return err == nil && hostWithin(strings.ToLower(u.Hostname()), "openrouter.ai")
}

func (m Model) ManufacturerOfficialPricing() OfficialPricing {
	if m.NameRule == NameRuleExact && (IsManufacturerPriceSource(m.ModelName, m.OfficialPricing.SourceURL) || isVerifiedFreeVariantPricing(m.ModelName, m.OfficialPricing)) {
		return m.OfficialPricing
	}
	return OfficialPricing{}
}

// Accept namespaces only when the native ID belongs to the same manufacturer.
// Arbitrary reseller renames and platform variants remain unknown.
func ManufacturerNativeModel(name string) string {
	if _, native, ok := strings.Cut(name, "/"); ok && manufacturerForPrice(native) != "" && manufacturerForPrice(name) == manufacturerForPrice(native) {
		return native
	}
	return name
}

// A verified platform free variant is a reference for that exact service only,
// not a manufacturer's general inference price. Entries were checked against
// the exact OpenRouter catalog and endpoint quotes on 2026-11-06.
// See .agents/notes/implemented/feature/2026-11-06-openrouter-native-names-free-variants.md.
func isVerifiedFreeVariantPricing(name string, price OfficialPricing) bool {
	native := name
	if _, suffix, ok := strings.Cut(name, "/"); ok {
		native = suffix
	}
	upstream, ok := verifiedOpenRouterFreeVariants[native]
	if !ok || (name != native && name != upstream) {
		return false
	}
	u, err := url.Parse(price.SourceURL)
	if err != nil || priceSourceHost(price.SourceURL) != "openrouter.ai" ||
		u.Path != "/"+upstream || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if price.Currency != "USD" || len(price.Tiers) != 1 {
		return false
	}
	tier := price.Tiers[0]
	if tier.Unit != "million_tokens" || tier.Input == nil || tier.Output == nil || *tier.Input != 0 || *tier.Output != 0 {
		return false
	}
	for _, extra := range []*float64{tier.CacheRead, tier.CacheWrite, tier.Price} {
		if extra != nil && *extra != 0 {
			return false
		}
	}
	return true
}

// Exact service identities; an open license or :free suffix alone is insufficient.
var verifiedOpenRouterFreeVariants = map[string]string{
	"flint-v1.0:free":                             "recursive/flint-v1.0:free",
	"xing4.0-29b-a4b:free":                        "xingchen-agi/xing4.0-29b-a4b:free",
	"nemotron-3.5-super-vl:free":                  "nvidia/nemotron-3.5-super-vl:free",
	"dots-3-note-preview:free":                    "dots-studio/dots-3-note-preview:free",
	"lfm-2.5-2.6b:free":                           "liquid/lfm-2.5-2.6b:free",
	"nemotron-3.5-lightning:free":                 "nvidia/nemotron-3.5-lightning:free",
	"north-mini-code:free":                        "cohere/north-mini-code:free",
	"nemotron-3.5-content-safety:free":            "nvidia/nemotron-3.5-content-safety:free",
	"nemotron-3-ultra-550b-a55b:free":             "nvidia/nemotron-3-ultra-550b-a55b:free",
	"nemotron-3-nano-omni-30b-a3b-reasoning:free": "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free",
}
