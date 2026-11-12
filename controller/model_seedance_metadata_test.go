package controller

import (
	"context"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestSeedanceOfficialMetadata(t *testing.T) {
	read := func(name string) []byte {
		b, e := os.ReadFile("testdata/seedance-" + name + ".html")
		require.NoError(t, e)
		if os.Getenv("SEEDANCE_LIVE_FIXTURE") != "" {
			b, e = os.ReadFile("/tmp/seedance-" + name + ".html")
			require.NoError(t, e)
		}
		return b
	}
	attrs := read("doc")
	prices := read("price")
	a, err := parseSeedanceAttributes(attrs, seedance25ID, seedance25URL)
	require.NoError(t, err)
	require.Equal(t, model.ModelModalities{"text", "image", "audio", "video"}, a.InputModalities)
	require.Equal(t, model.ModelModalities{"video", "audio"}, a.OutputModalities)
	require.True(t, a.Capabilities["web_search"])
	require.NotContains(t, a.Capabilities, "function_calling")
	require.Zero(t, a.ContextLength)
	require.Zero(t, a.MaxOutputTokens)
	p, err := parseSeedancePricing(prices, seedance25ID, seedancePricingURL)
	require.NoError(t, err)
	require.Equal(t, "CNY", p.OfficialPricing.Currency)
	require.Len(t, p.OfficialPricing.Tiers, 4)
	for i, want := range []float64{70, 42, 77, 46} {
		require.Equal(t, want, *p.OfficialPricing.Tiers[i].Price)
		require.Equal(t, "million_tokens", p.OfficialPricing.Tiers[i].Unit)
	}
	require.True(t, model.IsManufacturerPriceSource(seedance25ID, p.OfficialPricing.SourceURL))
	for _, id := range []string{"doubao-seedance-2-0-260128", "doubao-seedance-2-5-flash", "doubao-seedance-2-5-999999"} {
		_, err = parseSeedanceAttributes(attrs, id, seedance25URL)
		require.Error(t, err)
		_, err = parseSeedancePricing(prices, id, seedancePricingURL)
		require.Error(t, err)
	}
	for _, change := range [][2]string{{"元/M token", "USD/M token"}, {"70.00", "unknown"}, {"doubao-seedance-2.5", "doubao-seedance-2.5-flash"}, {"输入包含视频", "输入其他"}} {
		_, err = parseSeedancePricing([]byte(strings.ReplaceAll(string(prices), change[0], change[1])), seedance25ID, seedancePricingURL)
		require.Error(t, err)
	}
	_, err = parseSeedanceAttributes([]byte(strings.ReplaceAll(string(attrs), seedance25ID, "different-model")), seedance25ID, seedance25URL)
	require.Error(t, err)
}

func TestSeedanceSyncAndPublicProjection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Model{}, &model.Ability{}, &model.Vendor{}, &model.Channel{}, &model.ModelDescriptionTranslation{}))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		_ = db.Where("1=1").Delete(&model.Ability{}).Error
		model.RefreshPricing()
		model.DB = oldDB
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	m := model.Model{ModelName: seedance25ID, SyncOfficial: 1, Status: 1}
	require.NoError(t, m.Insert())
	require.NoError(t, db.Create(&model.Ability{Model: m.ModelName, Group: "default", ChannelId: 1, Enabled: true}).Error)
	oldClient := getHTTPClient()
	client := *oldClient
	httpClient = &client
	t.Cleanup(func() { httpClient = oldClient })
	t.Setenv("SYNC_UPSTREAM_BASE", "https://community.example")
	t.Setenv("MODEL_METADATA_ANTHROPIC_API_KEY", "")
	t.Setenv("MODEL_METADATA_GEMINI_API_KEY", "")
	attrs, err := os.ReadFile("testdata/seedance-doc.html")
	require.NoError(t, err)
	prices, err := os.ReadFile("testdata/seedance-price.html")
	require.NoError(t, err)
	fail := false
	client.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		body, status := `{"test":{"id":"test","models":{"unrelated":{"id":"unrelated","limit":{"context":1}}}}}`, 200
		switch r.URL.String() {
		case seedance25URL:
			body = string(attrs)
		case seedancePricingURL:
			body = string(prices)
		}
		if fail && r.URL.Host == "docs.volcengine.com" {
			status = 503
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	result, err := runModelCapabilitySync(context.Background())
	require.NoError(t, err)
	require.Zero(t, result.SourceFailures)
	require.Equal(t, 1, result.PricingUpdated)
	out, _ := model.PublicPricingMetadata(model.GetPricing(), nil)
	require.Len(t, out, 1)
	require.Len(t, out[0].OfficialPricing.Tiers, 4)
	require.Len(t, out[0].InputModalities, 4)
	require.True(t, out[0].Capabilities["web_search"])
	require.Contains(t, out[0].OfficialPricing.Tiers[1].Label, "输入包含视频")
	saved := out[0].OfficialPricing
	fail = true
	result, err = runModelCapabilitySync(context.Background())
	require.NoError(t, err)
	require.Positive(t, result.SourceFailures)
	out, _ = model.PublicPricingMetadata(model.GetPricing(), nil)
	require.Equal(t, saved, out[0].OfficialPricing)
	// Manual modality edits remain owned by the administrator across syncs.
	require.NoError(t, db.Model(&model.Model{}).Where("id = ?", m.Id).Update("input_modalities", model.ModelModalities{"text"}).Error)
	fail = false
	_, err = runModelCapabilitySync(context.Background())
	require.NoError(t, err)
	model.RefreshPricing()
	out, _ = model.PublicPricingMetadata(model.GetPricing(), nil)
	require.Equal(t, model.ModelModalities{"text"}, out[0].InputModalities)
}
