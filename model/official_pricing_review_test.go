package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOfficialPricingReviewPublicProjection(t *testing.T) {
	require.Len(t, officialPricingReviews, 40)
	for name, r := range officialPricingReviews {
		require.Contains(t, []string{"unverified", "source_available", "source_unavailable", "platform_variant", "legacy_review", "identity_review"}, r.Code, name)
		_, err := time.Parse("2006-01-02", r.CheckedAt)
		require.NoError(t, err, name)
	}
	zero := 0.0
	inputs := []Pricing{
		{ModelName: "gemini-4-carbon"},
		{ModelName: "unknown"},
		{ModelName: "flint-v1.0:free", OfficialPricing: OfficialPricing{SourceURL: "https://openrouter.ai/recursive/flint-v1.0:free", Tiers: []OfficialPricingTier{{Input: &zero, Output: &zero}}}},
		{ModelName: "gemini-4-carbon", OfficialPricing: OfficialPricing{Currency: "USD", SourceURL: "https://aistudio.google.com/docs/pricing", Tiers: []OfficialPricingTier{{Input: &zero, Output: &zero}}}, OfficialPricingReview: &OfficialPricingReview{Code: "unverified"}},
	}
	out, _ := PublicPricingMetadata(inputs, nil)
	require.Equal(t, "source_available", out[0].OfficialPricingReview.Code)
	require.Equal(t, "unverified", out[1].OfficialPricingReview.Code)
	require.Empty(t, out[1].OfficialPricingReview.CheckedAt)
	require.Empty(t, out[2].OfficialPricing.Tiers)
	require.Equal(t, "platform_variant", out[2].OfficialPricingReview.Code)
	require.Nil(t, out[3].OfficialPricingReview)
	require.Len(t, out[3].OfficialPricing.Tiers, 1)
	north := Pricing{ModelName: "north-mini-code:free", ModelRatio: 0, OfficialPricing: OfficialPricing{Currency: "USD", SourceURL: "https://docs.cohere.com/docs/north-mini-code-1.0", Tiers: []OfficialPricingTier{{Label: "Within rate limits", Unit: "million_tokens", Input: &zero, Output: &zero}}}}
	published, _ := PublicPricingMetadata([]Pricing{north}, nil)
	require.Len(t, published[0].OfficialPricing.Tiers, 1)
	require.Nil(t, published[0].OfficialPricingReview)
	require.Zero(t, published[0].ModelRatio)
	require.Nil(t, inputs[0].OfficialPricingReview)
	out[0].OfficialPricingReview.Code = "changed"
	require.Equal(t, "source_available", publicOfficialPricingReview("gemini-4-carbon", OfficialPricing{}).Code)
}
