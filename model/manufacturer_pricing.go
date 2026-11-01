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
	"meta": {"meta.ai"}, "liquid": {"liquid.ai"}, "moondream": {"moondream.ai"}, "rekaai": {"reka.ai"}, "meituan": {"longcat.chat"}, "fireworks": {"fireworks.ai"}, "perceptron": {"perceptron.inc"}, "particle-ai": {"particle.ai"}, "inference-net": {"inference.net"}, "unbiased": {"unbiased.ai"},
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
		{"nova-", "amazon"}, {"inkling", "thinkingmachines"}, {"gpt-", "openai"}, {"chatgpt-", "openai"}, {"o1", "openai"}, {"o3", "openai"}, {"o4", "openai"},
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
	if m.NameRule == NameRuleExact && IsManufacturerPriceSource(m.ModelName, m.OfficialPricing.SourceURL) {
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
