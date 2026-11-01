package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func priceHTML(rows ...[]string) string {
	var b strings.Builder
	b.WriteString("<table>")
	for i, r := range rows {
		b.WriteString("<tr>")
		tag := "td"
		if i == 0 {
			tag = "th"
		}
		for _, c := range r {
			fmt.Fprintf(&b, "<%s>%s</%s>", tag, c, tag)
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}
func haikuConditionalPriceRows() [][]string {
	cells := []string{"$0.10 / MTok for prompts up to 100,000 tokens; $0.50 / MTok for prompts over 100,000 tokens", "$0.50 / MTok for prompts up to 100,000 tokens; $2.50 / MTok for prompts over 100,000 tokens", "$0.01 / MTok for prompts up to 100,000 tokens; $0.05 / MTok for prompts over 100,000 tokens", "$0.125 / MTok for prompts up to 100,000 tokens; $0.625 / MTok for prompts over 100,000 tokens", "$0.20 / MTok for prompts up to 100,000 tokens; $1 / MTok for prompts over 100,000 tokens"}
	fields := []string{"Input", "Output", "Cache read", "5m cache write", "1h cache write"}
	rows := [][]string{{"Feature", "Haiku 5.5"}}
	for i, k := range fields {
		rows = append(rows, []string{k, cells[i]})
	}
	return rows
}
func TestClaudeConditionalOfficialPrices(t *testing.T) {
	rows := haikuConditionalPriceRows()
	price, err := parseClaudePricing([][][]string{rows}, "https://platform.claude.com/docs/en/models/haiku-5-5/overview")
	require.NoError(t, err)
	require.Len(t, price.Tiers, 4)
	require.Contains(t, price.Tiers[0].Label, "≤ 100000")
	require.Contains(t, price.Tiers[0].Label, "cache 5m")
	require.Equal(t, .1, *price.Tiers[0].Input)
	require.Equal(t, .625, *price.Tiers[2].CacheWrite)
	require.Equal(t, 1.0, *price.Tiers[3].CacheWrite)
	rows[2][1] = "$0.5 / MTok"
	_, err = parseClaudePricing([][][]string{rows}, "https://platform.claude.com")
	require.Error(t, err)
	for _, cell := range []string{"From $0.1 / MTok", "$1 / MTok for prompts over 100,000 tokens", "$1 / MTok for prompts up to 100,000 tokens; $2 / MTok for prompts over 200,000 tokens", "$1 / MTok; $2 / MTok", "$1 / MTok for prompts up to 0 tokens; $2 / MTok for prompts over 0 tokens"} {
		_, err = claudePriceRanges(cell)
		require.Error(t, err, cell)
	}
}
func TestAliyunOfficialPriceConditionsAndUnits(t *testing.T) {
	h := []string{"模型 ID", "模式", "单次请求的输入Token数", "输入单价（每百万Token）", "输出单价（每百万Token）"}
	body := "<section id='中国内地'>" + priceHTML(h, []string{"qwen-example", "思考", "0&lt;Token≤32K", "2 元", "6 元"}, []string{"qwen-example", "思考", "32K&lt;Token≤128K", "4 元", "12 元"}) + "</section>"
	got := parseAliyunPricing([]byte(body), map[string]bool{"qwen-example": true}, aliyunPricingURL)
	require.Len(t, got["qwen-example"].OfficialPricing.Tiers, 2)
	require.Contains(t, got["qwen-example"].OfficialPricing.Tiers[0].Label, "0<Token≤32K")
	broken := strings.Replace(body, "12 元", "From 12 元", 1)
	require.Empty(t, parseAliyunPricing([]byte(broken), map[string]bool{"qwen-example": true}, aliyunPricingURL))
	conflict := body + "<section id='中国内地'>" + priceHTML(h, []string{"qwen-example", "思考", "0&lt;Token≤32K", "999 元", "6 元"}) + "</section>"
	require.Empty(t, parseAliyunPricing([]byte(conflict), map[string]bool{"qwen-example": true}, aliyunPricingURL))
	for _, tc := range []struct {
		cell, header, unit string
		v                  float64
	}{{"1.4 元/万字符", "单价", "million_characters", 140}, {"6", "输入单价（元/百万 Token）", "million_tokens", 6}, {"原价 10 元/张 （限时 8 折）", "单价", "image", 8}, {"0.5 元/小时", "单价", "hour", .5}} {
		v, u := aliyunRate(tc.cell, tc.header)
		require.NotNil(t, v, tc)
		require.Equal(t, tc.unit, u)
		require.Equal(t, tc.v, *v)
	}
	for _, c := range []string{"6", "原价 6 元/张", "1-3 元/张", "3 元起/张", "3 美元/张"} {
		v, _ := aliyunRate(c, "单价")
		require.Nil(t, v, c)
	}
	modes := aliyunCellRates("文本输入：0.5 元 图片输入：0.5 元", "输入单价（每百万Token）")
	require.Len(t, modes, 2)
	require.Equal(t, "图片", modes[1].mode)
	require.Len(t, aliyunCellRates("关闭提示词改写（ prompt_extend=false ）：0.1 元/张 开启提示词改写（ prompt_extend=true ）：0.2 元/张", "输出单价"), 2)
	require.Empty(t, aliyunCellRates("文本输入：0.5 元 额外费用另计", "输入单价（每百万Token）"))
	audio := "<section id='中国内地'>" + priceHTML([]string{"模型", "输入单价（每百万Token）", "输出单价（每百万Token）"}, []string{"模型", "音频", "音频"}, []string{"qwen-audio", "1 元", "2 元"}) + "</section>"
	require.Equal(t, "million_audio_tokens", parseAliyunPricing([]byte(audio), map[string]bool{"qwen-audio": true}, aliyunPricingURL)["qwen-audio"].OfficialPricing.Tiers[0].Unit)
}
func TestAdditionalManufacturerPriceTables(t *testing.T) {
	cases := []struct {
		name, id, body, url string
		parse               func([]byte, map[string]bool, string) map[string]model.ModelCapabilityMetadata
		input, output       float64
	}{
		{"writer", "palmyra-x5", priceHTML([]string{"Model name", "Model ID", "Input / 1M", "Output / 1M"}, []string{"Palmyra X5", "palmyra-x5", "$0.6", "$6"}), writerPricingURL, parseWriterPricing, .6, 6},
		{"inception", "mercury-2", `"mercury-2"` + priceHTML([]string{"Model", "Input Price (1M Tokens)", "Cached Input Price (1M Tokens)", "Output Price (1M Tokens)"}, []string{"Mercury 2", "~~$1~~ → $0.25", "$0.05", "$1"}), inceptionPricingURL, parseInceptionPricing, .25, 1},
		{"mistral", "codestral-2508", "<div><div><div>" + priceHTML([]string{"Model", "Input", "Cached input", "Output"}, []string{`<a href="/models/codestral-25-08">Codestral ↗</a>`, "$0.3", "$0.03", "$0.9"}) + "</div></div><p>Prices /M Tokens</p></div>", mistralPricingURL, parseMistralPricing, .3, .9},
		{"reka", "reka-flash-3", priceHTML([]string{"Model ID", "Input $/M", "Output $/M", "Cache read $/M"}, []string{"reka-flash-3", "$0.1", "$0.4", "$0.05"}), rekaPricingURL, parseRekaPricing, .1, .4},
		{"longcat", "longcat-2.0", "<h1>LongCat-2.0</h1>" + priceHTML([]string{"Item", "Price $/1M Tokens", "Discounted Price $/1M Tokens (limited-time)"}, []string{"Uncached Input", "$1", "$0.5"}, []string{"Cached Input", "$0.1", "$0.05"}, []string{"Output", "$2", "$1"}), longcatPricingURL, parseLongcatPricing, .5, 1},
		{"meta", "muse-wonder-1.0", "<h2>Muse Wonder</h2><p>muse-wonder-1.0</p>" + priceHTML([]string{"Usage", "Standard mode (per 1M tokens)", "Contributor mode (per 1M tokens)"}, []string{"Input", "$4", "$1"}, []string{"Output", "$20", "$5"}, []string{"Cached input", "$0.2", "$0.05"}), metaPricingURL, parseMetaPricing, 4, 20},
		{"moondream", "moondream3-preview", "<h2>Moondream 3 Preview — Real-time</h2><p>$0.1</p><p>/ 1M input tokens</p><p>$0.4</p><p>/ 1M output tokens</p><h2>Moondream 3 Preview — Batch</h2><p>$0.05</p><p>/ 1M input tokens</p><p>$0.2</p><p>/ 1M output tokens</p>", moondreamPricingURL, parseMoondreamPricing, .1, .4},
		{"liquid", "lfm-2.5-24b-a2b", "<h2>LFM2.5-24B-A2B</h2><p>Input</p><p>$0.05</p><p>/ MTok</p><p>Cached input</p><p>$0.01</p><p>/ MTok</p><p>Output</p><p>$0.2</p><p>/ MTok</p><p>Other</p><p>details</p>", liquidPricingURL, parseLiquidPricing, .05, .2},
		{"fireworks", "ember-1", "<p>accounts/fireworks/models/ember-1</p><p>Available Serverless</p><p>$1 / $0.1 / $4</p><p>USD</p><p>Per 1M Tokens (input/cached input/output)</p>", fireworksPricingURL, parseFireworksPricing, 1, 4},
		{"particle", "particle-1", "<p>Prices per million tokens for uncached input, cached input, and output (including reasoning) are $1, $0.1, and $3 for both Particle-1 and DeepSeek V4.1 Flash, with standard terms.</p>", particlePricingURL, parseParticlePricing, 1, 3},
		{"unbiased", "pareto-26.10", "<h2>Pareto 26.10 is generally available</h2><p>Pricing is unchanged from the preview: $0.8 input, $0.03 cached input, $3.2 output per 1M tokens, taxes excluded.</p>", unbiasedPricingURL, parseUnbiasedPricing, .8, 3.2},
		{"inference", "schematron-v2-turbo", "<p>two new models: Schematron V2 Small and Schematron V2 Turbo</p><code>inference-net/schematron-v2-small</code><h2>Schematron V2 Turbo:</h2><p>79.42</p><p>Schematron V2 is priced to make internet-scale extraction accessible:</p><h3>Schematron V2 Turbo:</h3><p>$0.06/1M input tokens – $0.15/1M output tokens</p>", inferencePricingURL, parseInferencePricing, .06, .15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.parse([]byte(tc.body), map[string]bool{tc.id: true}, tc.url)[tc.id].OfficialPricing
			require.NotEmpty(t, p.Tiers)
			require.NoError(t, p.Normalize(model.NameRuleExact))
			require.Equal(t, tc.input, *p.Tiers[0].Input)
			require.Equal(t, tc.output, *p.Tiers[0].Output)
			require.True(t, model.IsManufacturerPriceSource(tc.id, tc.url))
			require.Empty(t, tc.parse([]byte(tc.body), map[string]bool{"unrelated-model": true}, tc.url))
			require.Empty(t, tc.parse([]byte("<p>unavailable</p>"), map[string]bool{tc.id: true}, tc.url))
		})
	}
}
func TestManufacturerStructuredPrices(t *testing.T) {
	aion := `{"data":[{"id":"aion-labs/aion-2.1","pricing":{"prompt":"0.000002","completion":"0.000008","input_cache_read":"0.0000002"}},{"id":"openai/gpt-6","pricing":{"prompt":"1","completion":"2"}}]}`
	p := parseAionPricing([]byte(aion), map[string]bool{"aion-2.1": true, "gpt-6": true}, aionPricingURL)
	require.Len(t, p, 1)
	require.Equal(t, 2.0, *p["aion-2.1"].OfficialPricing.Tiers[0].Input)
	for _, x := range []string{"NaN", "Inf", "-1", "", "1e99"} {
		got := parseAionPricing([]byte(strings.Replace(aion, "0.000002", x, 1)), map[string]bool{"aion-2.1": true}, aionPricingURL)
		require.Empty(t, got["aion-2.1"].OfficialPricing.Tiers)
	}
	morph := `{"data":[{"id":"morph-v3-fast","input_modalities":[{"type":"text","pricing":[{"type":"prompt","unit":"token","cost_usd":"0.0000008"}]}],"output_modalities":[{"type":"text","pricing":[{"type":"completion","unit":"token","cost_usd":"0.0000012"}]}]}]}`
	got := parseMorphPricing([]byte(morph), map[string]bool{"morph-v3-fast": true}, morphPricingURL)
	require.InDelta(t, .8, *got["morph-v3-fast"].OfficialPricing.Tiers[0].Input, 1e-12)
	require.Empty(t, parseMorphPricing([]byte(strings.ReplaceAll(morph, `"unit":"token"`, `"unit":"request"`)), map[string]bool{"morph-v3-fast": true}, morphPricingURL))
}
func TestOpenAIImagePriceCardsSeparateModalities(t *testing.T) {
	lines := []string{"Pricing", "Text tokens", "Per 1M tokens", "Input", "$5", "Cached input", "$1.25", "Image tokens", "Per 1M tokens", "Input", "$8", "Output", "$30", "Cached input", "$2", "Modalities"}
	p, err := parseOpenAIPriceLines(lines, "https://developers.openai.com/api/docs/models/gpt-image-2.5-flare")
	require.NoError(t, err)
	require.Len(t, p.Tiers, 2)
	require.Nil(t, p.Tiers[0].Output)
	require.Equal(t, "million_image_tokens", p.Tiers[1].Unit)
	require.Equal(t, 30.0, *p.Tiers[1].Output)
	lines[10] = "$8 per image"
	_, err = parseOpenAIPriceLines(lines, "https://developers.openai.com")
	require.Error(t, err)
}
func TestAWSOfficialRateJoin(t *testing.T) {
	markup := "<h4>On Demand Inference</h4>" + priceHTML([]string{"Amazon Nova Models", "Price per 1M input tokens", "Price per 1M input tokens (cache read)", "Price per 1M output tokens"}, []string{"Amazon Nova Lite", "{priceOf!bedrock/bedrock!input!*!1000}", "{priceOf!bedrock/bedrock!cache!*!1000!opt}", "{priceOf!bedrock/bedrock!output!*!1000}"})
	page := "<div><div data-pricing-markup='" + markup + "'></div></div>"
	rates := `{"manifest":{"currencyCode":"USD","serviceId":"bedrock"},"regions":{"US East (N. Virginia)":{"input":{"price":"0.00006"},"cache":{"price":"0.000015"},"output":{"price":"0.00024"}},"US West (Oregon)":{"input":{"price":"99"}}}}`
	got, err := parseAWSNovaPricing([]byte(page), []byte(rates), map[string]bool{"nova-lite-v1": true})
	require.NoError(t, err)
	p := got["nova-lite-v1"].OfficialPricing
	require.Len(t, p.Tiers, 1)
	require.InDelta(t, .06, *p.Tiers[0].Input, 1e-12)
	require.Contains(t, p.Tiers[0].Label, "Virginia")
	_, err = parseAWSNovaPricing([]byte(page), []byte(strings.Replace(rates, "USD", "EUR", 1)), map[string]bool{"nova-lite-v1": true})
	require.Error(t, err)
	got, err = parseAWSNovaPricing([]byte(strings.Replace(page, "!input!", "!unknown!", 1)), []byte(rates), map[string]bool{"nova-lite-v1": true})
	require.NoError(t, err)
	require.Empty(t, got)
}

// Captured component props are a compact first-party price fixture, without
// navigation, analytics scripts or private data.
func TestOpenAIHiddenOfficialImagePrice(t *testing.T) {
	body, err := os.ReadFile("testdata/official-pricing/openai-image-fragment.html")
	require.NoError(t, err)
	got := parseOpenAIImagePricing(body, map[string]bool{"gpt-image-2": true}, openAIImagePricingURL)
	p := got["gpt-image-2"].OfficialPricing
	require.Len(t, p.Tiers, 2)
	require.Equal(t, 8.0, *p.Tiers[0].Input)
	require.Equal(t, 30.0, *p.Tiers[0].Output)
	require.Nil(t, p.Tiers[1].Output)
	require.Empty(t, parseOpenAIImagePricing([]byte(strings.ReplaceAll(string(body), `data-value="standard"`, `data-value="batch"`)), map[string]bool{"gpt-image-2": true}, openAIImagePricingURL))
	var invalid interface{}
	require.NoError(t, common.Unmarshal([]byte(`[99,"value"]`), &invalid))
	_, ok := astroPricingValue(invalid, 0)
	require.False(t, ok)
}

func TestMiniMaxServiceTiersAndVideoUnits(t *testing.T) {
	body := "> [tab] Standard\n> | Model | Input $ / M tokens | Output $ / M tokens | Prompt caching Read $ / M tokens |\n> | --- | --- | --- | --- |\n> | MiniMax-M3 ≤ 512k input tokens | $0.3 | $1.2 | $0.06 |\n> | MiniMax-M3 > 512k input tokens | $0.6 | $2.4 | $0.12 |\n> [tab] Priority\n> | Model | Input $ / M tokens | Output $ / M tokens | Prompt caching Read $ / M tokens |\n> | --- | --- | --- | --- |\n> | MiniMax-M3 ≤ 512k input tokens | $0.45 | $1.8 | $0.09 |\n"
	p := parseMiniMaxPricing([]byte(body), map[string]bool{"MiniMax-M3": true}, minimaxPricingURL)["MiniMax-M3"].OfficialPricing
	require.Len(t, p.Tiers, 3)
	require.Contains(t, p.Tiers[1].Label, "> 512k")
	require.Contains(t, p.Tiers[2].Label, "Priority")
	require.Empty(t, parseMiniMaxPricing([]byte(strings.Replace(body, "$2.4", "from $2.4", 1)), map[string]bool{"MiniMax-M3": true}, minimaxPricingURL))
	h := []string{"Model", "Output Resolution", "Output Video per second", "Input Video per second", "Input Image per image beyond free quota"}
	body = priceHTML(h, []string{"MiniMax-H3\\ Quality first", "2K", "$0.13", "$0.13", "$0.04"}, []string{"MiniMax-H3\\ Quality first", "768P", "$0.08", "$0.08", "$0.04"}, []string{"MiniMax-H3-Max", "2K", "$1", "$1", "$1"})
	got := parseMiniMaxPricing([]byte(body), map[string]bool{"MiniMax-H3": true, "MiniMax-H3-Max": true}, minimaxPricingURL)
	require.Len(t, got, 1)
	p = got["MiniMax-H3"].OfficialPricing
	require.Len(t, p.Tiers, 3)
	require.Equal(t, "second", p.Tiers[0].Unit)
	require.Equal(t, "image", p.Tiers[1].Unit)
	require.Contains(t, p.Tiers[1].Label, "first 5 free")
}

func TestMetaPriceTableFollowsModelSection(t *testing.T) {
	header := []string{"Usage", "Standard mode (per 1M tokens)", "Contributor mode (per 1M tokens)"}
	spark := "<h3>Muse Spark</h3><code>muse-spark-1.3</code>" + priceHTML(header, []string{"Input", "$1.25", "$0.10"}, []string{"Output", "$4.25", "$0.20"})
	wonder := "<h3>Muse Wonder</h3><code>muse-wonder-1.0</code>" + priceHTML(header, []string{"Input", "$4", "$1"}, []string{"Output", "$20", "$5"})
	names := map[string]bool{"muse-spark-1.3": true, "muse-wonder-1.0": true}
	for _, body := range []string{spark + wonder, wonder + spark, priceHTML([]string{"Unrelated"}, []string{"price table"}) + spark + wonder} {
		got := parseMetaPricing([]byte(body), names, metaPricingURL)
		require.Len(t, got, 2)
		require.Equal(t, 1.25, *got["muse-spark-1.3"].OfficialPricing.Tiers[0].Input)
		require.Equal(t, 4.0, *got["muse-wonder-1.0"].OfficialPricing.Tiers[0].Input)
	}
}

func TestDownloadedAdditionalPrices(t *testing.T) {
	dir := os.Getenv("TEST_ADDITIONAL_PRICE_DIR")
	if dir == "" {
		t.Skip("opt-in downloaded manufacturer pages")
	}
	for _, spec := range []struct {
		file, id string
		parse    func([]byte, map[string]bool, string) map[string]model.ModelCapabilityMetadata
		url      string
	}{
		{"meta.txt", "muse-spark-1.3", parseMetaPricing, metaPricingURL},
		{"minimaxprice.txt", "MiniMax-M3", parseMiniMaxPricing, minimaxPricingURL},
		{"mistral.txt", "codestral-2508", parseMistralPricing, mistralPricingURL},
	} {
		data, err := os.ReadFile(dir + "/" + spec.file)
		require.NoError(t, err)
		got := spec.parse(data, map[string]bool{spec.id: true}, spec.url)
		require.NotEmpty(t, got[spec.id].OfficialPricing.Tiers, spec.id)
	}
}

func TestUpstageAndPerceptronIdentity(t *testing.T) {
	body := "### solar-pro4-260806 (Alias: solar-pro4)\n- **Price**: $0.6/1M tokens (Input Token)\n- **Price**: $3.6/1M tokens (Output Token)\n- **Price**: $0.06/1M tokens (Cached Token)\n"
	e, err := parseUpstagePricing([]byte(body), "solar-pro4", "https://console.upstage.ai/docs/models/solar-pro-4.md")
	require.NoError(t, err)
	require.Equal(t, .6, *e.OfficialPricing.Tiers[0].Input)
	_, err = parseUpstagePricing([]byte(body), "solar-mini4", "https://console.upstage.ai")
	require.Error(t, err)
	_, err = parseUpstagePricing([]byte(body+"- **Price**: $9/1M tokens (Input Token)\n"), "solar-pro4", "https://console.upstage.ai")
	require.Error(t, err)
	body = "<code>perceptron-mk1.5</code><p>Pricing:</p><p>$0.12/M input tokens | $0.6/M output tokens</p>"
	e, err = parsePerceptronPricing([]byte(body), "perceptron-mk1.5", "https://www.perceptron.inc/blog/introducing-perceptron-mk1-5")
	require.NoError(t, err)
	require.Equal(t, .12, *e.OfficialPricing.Tiers[0].Input)
	_, err = parsePerceptronPricing([]byte(body), "perceptron-mk1", "https://www.perceptron.inc")
	require.Error(t, err)
}

func TestThinkingMachinesInferenceNotTraining(t *testing.T) {
	h := []string{"Model", "Tinker ID", "Context", "Prefill (Input)", "Sample (Output)"}
	body := "<p>All prices are per million tokens. Checkpoint storage is charged at $0.10 per GB per month.</p>" + priceHTML(h, []string{"Inkling", "thinkingmachines/Inkling:peft:262144:sampling-nvfp4", "256K", "$1 $0.17 (cached)", "$4.05"})
	p := parseThinkingMachinesPricing([]byte(body), map[string]bool{"inkling": true}, thinkingMachinesPricingURL)["inkling"].OfficialPricing
	require.Len(t, p.Tiers, 1)
	require.Equal(t, 1.0, *p.Tiers[0].Input)
	require.Contains(t, p.Tiers[0].Label, "NVFP4")
	require.Empty(t, parseThinkingMachinesPricing([]byte(strings.Replace(body, "Prefill (Input)", "Training", 1)), map[string]bool{"inkling": true}, thinkingMachinesPricingURL))
}

func TestDeepSeekExplicitBillingAliases(t *testing.T) {
	body := priceHTML([]string{"MODEL", "MODEL", "deepseek-flash"}, []string{"PEAK", "1M INPUT TOKENS (CACHE MISS)", "$1"}, []string{"PEAK", "1M OUTPUT TOKENS", "$2"}, []string{"PEAK", "1M INPUT TOKENS (CACHE HIT)", "$0.1"}, []string{"OFF-PEAK", "1M INPUT TOKENS (CACHE MISS)", "$0.5"}, []string{"OFF-PEAK", "1M OUTPUT TOKENS", "$1"}, []string{"OFF-PEAK", "1M INPUT TOKENS (CACHE HIT)", "$0.05"})
	alias := "<p>Use deepseek-flash as the model name. The legacy names deepseek-v4-flash and deepseek-v4-flash-vision-exp are still accepted, but the corresponding models have been retired, their requests are served by the DeepSeek-V4.1-Flash-1017 model and billed at the Flash price.</p>"
	body += "<p>Peak hours are 00:00–12:00 UTC; official schedule applies.</p>"
	names := map[string]bool{"deepseek-v4-flash": true, "deepseek-v4-flash-vision-exp": true, "deepseek-v4.1-flash": true}
	got := parseDeepSeekDocumentWithPrices([]byte(body+alias), names, "https://api-docs.deepseek.com/quick_start/pricing")
	require.Len(t, got, 2)
	require.Len(t, got["deepseek-v4-flash"].OfficialPricing.Tiers, 2)
	require.Empty(t, parseDeepSeekDocumentWithPrices([]byte(body), names, "https://api-docs.deepseek.com/quick_start/pricing"))
}

// Exercises the existing sync -> persistence -> public catalog path, including
// malformed upstream prices and the administrator's explicit field ownership.
func TestHaikuConditionalPriceSyncAndPublicCatalog(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Vendor{}, &model.Channel{}, &model.ModelDescriptionTranslation{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		_ = db.Where("1=1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = originalDB
		d, _ := db.DB()
		_ = d.Close()
	})
	m := model.Model{ModelName: "claude-haiku-5-5", SyncOfficial: 1, Status: 1}
	require.NoError(t, m.Insert())
	require.NoError(t, db.Create(&model.Ability{Model: m.ModelName, Group: "default", ChannelId: 1, Enabled: true}).Error)
	original := getHTTPClient()
	client := *original
	httpClient = &client
	t.Cleanup(func() { httpClient = original })
	t.Setenv("MODEL_METADATA_ANTHROPIC_API_KEY", "")
	t.Setenv("MODEL_METADATA_GEMINI_API_KEY", "")
	rows := haikuConditionalPriceRows()
	client.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "https://platform.claude.com/docs/en/models/haiku-5-5/overview", r.URL.String())
		body := priceHTML([]string{"Provider", "ID"}, []string{"Claude API", m.ModelName}) + priceHTML(rows...)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	sync := func() []string {
		entries, warnings, err := fetchOfficialModelAttributesFor(context.Background(), map[string]bool{m.ModelName: true})
		require.NoError(t, err)
		_, err = model.ApplyModelCapabilityMetadata(context.Background(), entries, "https://community.example")
		require.NoError(t, err)
		model.RefreshPricing()
		return warnings
	}
	require.Empty(t, sync())
	got := model.GetPricing()[0]
	require.Len(t, got.OfficialPricing.Tiers, 4)
	require.Equal(t, "USD", got.OfficialPricing.Currency)
	require.Equal(t, .1, *got.OfficialPricing.Tiers[0].Input)
	require.Equal(t, 1.0, *got.OfficialPricing.Tiers[3].CacheWrite)
	require.Equal(t, "official", got.AttributeSources["official_pricing"].Kind)
	rows[2][1] = "$0.50 / MTok" // incompatible output context tiers
	require.NotEmpty(t, sync())
	require.Equal(t, got.OfficialPricing, model.GetPricing()[0].OfficialPricing)
	manual := got.OfficialPricing
	manual.Tiers = append([]model.OfficialPricingTier(nil), manual.Tiers...)
	manual.Tiers[0].Input = new(float64) // explicit administrator zero is valid
	require.NoError(t, db.Model(&model.Model{}).Where("id = ?", m.Id).Update("official_pricing", manual).Error)
	rows = haikuConditionalPriceRows()
	require.Empty(t, sync())
	require.Equal(t, 0.0, *model.GetPricing()[0].OfficialPricing.Tiers[0].Input)
}
