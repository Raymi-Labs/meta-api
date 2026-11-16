package controller

import (
	"context"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestListPriceRejectsAmbiguousOffers(t *testing.T) {
	for _, text := range []string{
		"$2 through November 30, 2026.",
		"$4 starting December 1, 2026.",
		"$2 through November 30, 2026. $4 starting December 2, 2026.",
		"$2 through November 30, 2026. $4 starting November 30, 2026.",
		"$4 through November 30, 2026. $2 starting December 1, 2026.",
		"$2 through November 31, 2026. $4 starting December 1, 2026.",
		"$2 through November 30, 2026. $4 starting December 1, 2026. $8 starting January 1, 2027.",
	} {
		_, ok := geminiPriceParts(text, time.Now())
		require.False(t, ok, text)
	}
	p, ok := geminiPriceParts("$4 starting December 1, 2026. $2 through November 30, 2026.", time.Now())
	require.True(t, ok)
	require.Equal(t, 4.0, *p[""])
	for _, text := range []string{"from $1", "~~$1~~ → $2", "~~$1~~", "$1 / $2"} {
		require.Nil(t, listDocumentUSD(text), text)
	}
	require.Equal(t, 0.0, *listDocumentUSD("$0"))
	require.Equal(t, 1.0, *listDocumentUSD(`~~\$1~~ → \$0.5`))
	for _, text := range []string{"10 元/张 （限时 8 折）", "原价 10 元/张 （限时 11 折）"} {
		rate, _ := aliyunRate(text, "单价")
		require.Nil(t, rate, text)
	}
	rate, unit := aliyunRate("原价 10 元/张", "单价")
	require.Equal(t, "image", unit)
	require.Equal(t, 10.0, *rate)
}

func TestMiniMaxListPriceRetainsServiceAndContext(t *testing.T) {
	body := "> [tab] Standard\n> | Model | Input $ / M tokens | Output $ / M tokens | Prompt caching Read $ / M tokens |\n> | --- | --- | --- | --- |\n> | MiniMax-M3 ≤ 512k input tokens Permanent 50% off | ~~$0.60~~ $0.30 | ~~$2.40~~ $1.20 | ~~$0.12~~ $0.06 |\n> [tab] Priority\n> | Model | Input $ / M tokens | Output $ / M tokens | Prompt caching Read $ / M tokens |\n> | --- | --- | --- | --- |\n> | MiniMax-M3 ≤ 512k input tokens Permanent 50% off | ~~$0.90~~ $0.45 | ~~$3.60~~ $1.80 | ~~$0.18~~ $0.09 |\n"
	p := parseMiniMaxPricing([]byte(body), map[string]bool{"MiniMax-M3": true}, minimaxPricingURL)["MiniMax-M3"].OfficialPricing
	require.Len(t, p.Tiers, 2)
	require.Equal(t, .6, *p.Tiers[0].Input)
	require.Equal(t, 2.4, *p.Tiers[0].Output)
	require.Equal(t, .12, *p.Tiers[0].CacheRead)
	require.Equal(t, "Standard ≤ 512k input tokens", p.Tiers[0].Label)
	require.Equal(t, "Priority ≤ 512k input tokens", p.Tiers[1].Label)
	require.Equal(t, .9, *p.Tiers[1].Input)
}

func TestDownloadedListPrices(t *testing.T) {
	dir := os.Getenv("TEST_LIST_PRICE_DIR")
	if dir == "" {
		t.Skip("opt-in manufacturer documents")
	}
	for _, tc := range []struct {
		file, id, url        string
		parse                func([]byte, map[string]bool, string) map[string]model.ModelCapabilityMetadata
		input, output, cache float64
	}{
		{"gemini-list-prices.md", "gemini-4-carbon", geminiPricingURL, parseGeminiPricing, 4, 20, .2},
		{"gemini-list-prices.md", "gemini-4-argon", geminiPricingURL, parseGeminiPricing, 4, 20, .2},
		{"inception-list-prices.md", "mercury-3", inceptionPricingURL, parseInceptionPricing, .2, .7, .02},
		{"longcat-list-prices.html", "longcat-2.0", longcatPricingURL, parseLongcatPricing, .75, 2.95, .015},
		{"minimax-list-prices.md", "MiniMax-M3", minimaxPricingURL, parseMiniMaxPricing, .6, 2.4, .12},
	} {
		t.Run(tc.id, func(t *testing.T) {
			b, err := os.ReadFile(dir + "/" + tc.file)
			require.NoError(t, err)
			p := tc.parse(b, map[string]bool{tc.id: true}, tc.url)[tc.id].OfficialPricing
			require.NotEmpty(t, p.Tiers)
			require.Equal(t, tc.input, *p.Tiers[0].Input)
			require.Equal(t, tc.output, *p.Tiers[0].Output)
			require.Equal(t, tc.cache, *p.Tiers[0].CacheRead)
			require.False(t, strings.Contains(p.Tiers[0].Label, "discount"))
		})
	}
}

func TestListPriceSyncUpdatesAutomaticAndProtectsManual(t *testing.T) {
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
	m := model.Model{ModelName: "gemini-4-carbon", SyncOfficial: 1, Status: 1}
	require.NoError(t, m.Insert())
	require.NoError(t, db.Create(&model.Ability{Model: m.ModelName, Group: "default", ChannelId: 1, Enabled: true}).Error)
	data, err := os.ReadFile("testdata/gemini-carbon-pricing.markdoc")
	require.NoError(t, err)
	apply := func(data []byte) model.Pricing {
		entries := parseGeminiPricing(data, map[string]bool{m.ModelName: true}, geminiPricingURL)
		_, err := model.ApplyModelCapabilityMetadata(context.Background(), entries, "https://community.example")
		require.NoError(t, err)
		model.RefreshPricing()
		return model.GetPricing()[0]
	}
	old := strings.ReplaceAll(string(data), "$4.00 starting December 1, 2026.", "")
	old = strings.ReplaceAll(old, "$20.00 starting December 1, 2026.", "")
	old = strings.ReplaceAll(old, "$0.20 starting December 1, 2026.", "")
	old = strings.ReplaceAll(old, " through November 30, 2026.", "")
	before := apply([]byte(old))
	require.Equal(t, 2.0, *before.OfficialPricing.Tiers[0].Input)
	after := apply(data)
	require.Equal(t, 4.0, *after.OfficialPricing.Tiers[0].Input)
	require.Equal(t, 20.0, *after.OfficialPricing.Tiers[0].Output)
	require.Equal(t, .2, *after.OfficialPricing.Tiers[0].CacheRead)
	require.Equal(t, before.ModelRatio, after.ModelRatio)
	require.Equal(t, before.CompletionRatio, after.CompletionRatio)
	public, _ := model.PublicPricingMetadata([]model.Pricing{after}, nil)
	require.Equal(t, after.OfficialPricing, public[0].OfficialPricing)
	manual := after.OfficialPricing
	manual.Tiers = append([]model.OfficialPricingTier(nil), manual.Tiers...)
	manual.Tiers[0].Input = new(float64)
	require.NoError(t, db.Model(&model.Model{}).Where("id = ?", m.Id).Update("official_pricing", manual).Error)
	require.Equal(t, 0.0, *apply(data).OfficialPricing.Tiers[0].Input)
}
