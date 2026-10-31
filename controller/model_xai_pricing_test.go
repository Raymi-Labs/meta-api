package controller

import (
	"context"
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

const xaiTestModel = `{"name":"grok-test","cluster":"us-east-1","promptTextTokenPrice":"20000","completionTextTokenPrice":"60000","cachedPromptTokenPrice":"5000","promptTextTokenPriceLongContext":"40000","completionTokenPriceLongContext":"120000","cachedPromptTokenPriceLongContext":"10000","longContextThreshold":"200000"}`
const xaiTestCard = `<h2>Pricing</h2><h4>Input</h4><div>Tokens</div><div>$2.00</div><span>/ 1M tokens</span><div>Cached tokens</div><div>$0.50</div><span>/ 1M tokens</span><h4>Output</h4><div>Tokens</div><div>$6.00</div><span>/ 1M tokens</span><div>Higher context pricing</div><p>We charge different rates for requests which exceed the <span>200K</span> context window</p>`

func xaiTestPage(record, kind, card string) string {
	return `<main>` + card + `<h2>Details</h2><div>Model name</div><div>grok-test</div><div>Region</div><div>us-east-1</div></main><script>globalThis.__XAI_PUBLIC_MODELS__={"clusterConfigs":[{"clusterName":"us-east-1","` + kind + `":[` + record + `]}]};</script>`
}

func TestXAIOfficialPricing(t *testing.T) {
	page := xaiTestPage(xaiTestModel, "languageModels", xaiTestCard)
	entry, err := parseXAIModelDocument([]byte(page), "grok-test", "https://docs.x.ai/developers/models/grok-test")
	require.NoError(t, err)
	p := entry.OfficialPricing
	require.Equal(t, "USD", p.Currency)
	require.Len(t, p.Tiers, 2)
	require.Equal(t, 2.0, *p.Tiers[0].Input)
	require.Equal(t, 6.0, *p.Tiers[0].Output)
	require.Equal(t, .5, *p.Tiers[0].CacheRead)
	require.Nil(t, p.Tiers[0].CacheWrite)
	require.Equal(t, 4.0, *p.Tiers[1].Input)
	require.Equal(t, 12.0, *p.Tiers[1].Output)
	require.Contains(t, p.Tiers[1].Label, "> 200000 input tokens")
	require.Equal(t, "official", entry.FieldSources["official_pricing"].Kind)
	for name, bad := range map[string]string{
		"identity":               strings.ReplaceAll(page, "<div>grok-test</div>", "<div>grok-other</div>"),
		"catalog identity":       strings.ReplaceAll(page, `"name":"grok-test"`, `"name":"grok-other"`),
		"unit":                   strings.ReplaceAll(page, "1M tokens", "1K tokens"),
		"currency":               strings.ReplaceAll(page, "$", "€"),
		"scale":                  strings.ReplaceAll(page, `"20000"`, `"200000"`),
		"negative":               strings.ReplaceAll(page, `"40000"`, `"-40000"`),
		"unknown":                strings.ReplaceAll(page, `"40000"`, `"N/A"`),
		"missing long output":    strings.ReplaceAll(page, `,"completionTokenPriceLongContext":"120000"`, ``),
		"missing threshold":      strings.ReplaceAll(page, `,"longContextThreshold":"200000"`, ``),
		"threshold disagreement": strings.ReplaceAll(page, "200K", "100K"),
		"visible region":         strings.Replace(page, "<div>us-east-1</div>", "<div>us-west-2</div>", 1),
		"region disagreement":    strings.Replace(page, `"cluster":"us-east-1"`, `"cluster":"eu-west-1"`, 1),
		"executable tail":        strings.ReplaceAll(page, "};</script>", "};doSomething();</script>"),
		"duplicate assignment":   page + `<script>globalThis.__XAI_PUBLIC_MODELS__={};</script>`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseXAIModelDocument([]byte(bad), "grok-test", "https://docs.x.ai/test")
			require.Error(t, err)
		})
	}
	zero := strings.ReplaceAll(strings.ReplaceAll(page, `"20000"`, `"0"`), "$2.00", "$0.00")
	e, err := parseXAIModelDocument([]byte(zero), "grok-test", "https://docs.x.ai/test")
	require.NoError(t, err)
	require.Zero(t, *e.OfficialPricing.Tiers[0].Input)
	// Same model in multiple regions is not deduplicated or treated as one price.
	regional := strings.Replace(page, `}]};</script>`, `},{"clusterName":"us-central-1","languageModels":[`+strings.ReplaceAll(strings.ReplaceAll(xaiTestModel, "us-east-1", "us-central-1"), `"20000"`, `"22000"`)+`]}]};</script>`, 1)
	e, err = parseXAIModelDocument([]byte(regional), "grok-test", "https://docs.x.ai/test")
	require.NoError(t, err)
	require.Len(t, e.OfficialPricing.Tiers, 4)
	require.Equal(t, 2.2, *e.OfficialPricing.Tiers[2].Input)
}

func TestXAIMediaPricing(t *testing.T) {
	image := `{"name":"grok-test","cluster":"us-east-1","imagePrice":"600000000","pricePerInputImage":"100000000","resolutionPricing":[{"resolution":"IMAGE_RESOLUTION_1K","quality":"low","pricePerImage":"400000000"},{"resolution":"IMAGE_RESOLUTION_2K","quality":"medium","pricePerImage":"800000000"}]}`
	imageCard := `<h2>Pricing</h2><div>Image input</div><div>$0.01 per image</div><div>Output</div><div>$0.04 per image</div>`
	e, err := parseXAIModelDocument([]byte(xaiTestPage(image, "imageGenerationModels", imageCard)), "grok-test", "https://docs.x.ai/test")
	require.NoError(t, err)
	require.Len(t, e.OfficialPricing.Tiers, 2)
	require.Equal(t, .01, *e.OfficialPricing.Tiers[0].Input)
	require.Equal(t, .04, *e.OfficialPricing.Tiers[0].Output)
	require.Equal(t, "image", e.OfficialPricing.Tiers[0].Unit)
	require.Contains(t, e.OfficialPricing.Tiers[1].Label, "2K · medium")
	video := `{"name":"grok-test","cluster":"us-east-1","pricePerInputImage":"100000000","pricePerInputVideoSecond":"1000000000","resolutionPricing":[{"resolution":"VIDEO_RESOLUTION_480P","pricePerSecond":"800000000"},{"resolution":"VIDEO_RESOLUTION_1080P","pricePerSecond":"2500000000"}]}`
	videoCard := `<h2>Pricing</h2><div>Output</div><div>$0.08 per second</div>`
	e, err = parseXAIModelDocument([]byte(xaiTestPage(video, "videoGenerationModels", videoCard)), "grok-test", "https://docs.x.ai/test")
	require.NoError(t, err)
	require.Len(t, e.OfficialPricing.Tiers, 3)
	require.Equal(t, .08, *e.OfficialPricing.Tiers[0].Output)
	require.Equal(t, .1, *e.OfficialPricing.Tiers[0].Input)
	require.Equal(t, "second", e.OfficialPricing.Tiers[0].Unit)
	require.Equal(t, .25, *e.OfficialPricing.Tiers[1].Output)
	require.Equal(t, "image", e.OfficialPricing.Tiers[2].Unit)
	require.Nil(t, e.OfficialPricing.Tiers[2].Output)
	for _, bad := range []string{strings.ReplaceAll(video, "480P", "UNKNOWN"), strings.ReplaceAll(video, "pricePerSecond", "pricePerImage"), strings.ReplaceAll(video, `"800000000"`, `"0.08"`)} {
		_, err := parseXAIModelDocument([]byte(xaiTestPage(bad, "videoGenerationModels", videoCard)), "grok-test", "https://docs.x.ai/test")
		require.Error(t, err)
	}
}

func TestXAISyncPublicPriceAndRetention(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Vendor{}, &model.Channel{}, &model.ModelDescriptionTranslation{}))
	old := model.DB
	model.DB = db
	t.Cleanup(func() {
		_ = db.Where("1=1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = old
		sql, _ := db.DB()
		_ = sql.Close()
	})
	m := model.Model{ModelName: "x-ai/grok-test", SyncOfficial: 1, Status: 1}
	require.NoError(t, m.Insert())
	require.NoError(t, db.Create(&model.Ability{Model: m.ModelName, Group: "default", ChannelId: 1, Enabled: true}).Error)
	original := getHTTPClient()
	client := *original
	httpClient = &client
	t.Cleanup(func() { httpClient = original })
	t.Setenv("MODEL_METADATA_ANTHROPIC_API_KEY", "")
	t.Setenv("MODEL_METADATA_GEMINI_API_KEY", "")
	page := xaiTestPage(xaiTestModel, "languageModels", xaiTestCard)
	client.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "https://docs.x.ai/developers/models/grok-test", r.URL.String())
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(page)), Request: r}, nil
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
	p := model.GetPricing()[0]
	require.Len(t, p.OfficialPricing.Tiers, 2)
	require.Equal(t, "official", p.AttributeSources["official_pricing"].Kind)
	page = strings.ReplaceAll(page, "1M tokens", "1K tokens")
	page = strings.Replace(page, "<h2>Details</h2>", "<h2>Details</h2><div>Context window</div><div>256K</div>", 1)
	require.NotEmpty(t, sync())
	require.Equal(t, p.OfficialPricing, model.GetPricing()[0].OfficialPricing)
	require.EqualValues(t, 256000, model.GetPricing()[0].ContextLength)
	// A deliberate manual manufacturer price is never overwritten by later sync.
	manual := p.OfficialPricing
	manual.Tiers[0].Input = new(float64)
	*manual.Tiers[0].Input = 9
	require.NoError(t, db.Model(&model.Model{}).Where("id = ?", m.Id).Update("official_pricing", manual).Error)
	page = xaiTestPage(xaiTestModel, "languageModels", xaiTestCard)
	require.Empty(t, sync())
	require.Equal(t, 9.0, *model.GetPricing()[0].OfficialPricing.Tiers[0].Input)
}

func TestDownloadedXAIPrices(t *testing.T) {
	dir := os.Getenv("TEST_XAI_PRICE_DIR")
	if dir == "" {
		t.Skip("opt-in downloaded official pages")
	}
	for _, id := range []string{"grok-4.5", "grok-4.6", "grok-4.7", "grok-imagine-image", "grok-imagine-image-2.0", "grok-imagine-video", "grok-imagine-video-1.5"} {
		t.Run(id, func(t *testing.T) {
			data, err := os.ReadFile(dir + "/" + id + ".html")
			require.NoError(t, err)
			e, err := parseXAIModelDocument(data, id, "https://docs.x.ai/developers/models/"+id)
			require.NoError(t, err)
			require.NotEmpty(t, e.OfficialPricing.Tiers)
			b, err := common.Marshal(e.OfficialPricing)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(dir+"/"+id+"-pricing.json", b, 0600))
			t.Logf("%s %d tiers", id, len(e.OfficialPricing.Tiers))
		})
	}
}
