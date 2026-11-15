package model

import (
	_ "embed"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// OfficialPricingReview is a dated review, not a live provider availability
// claim or a price. Only public empty states consume it; billing never does.
type OfficialPricingReview struct {
	Code      string `json:"code"`
	CheckedAt string `json:"checked_at,omitempty"`
}

//go:embed data/official_pricing_reviews.json
var officialPricingReviewsJSON []byte
var officialPricingReviews = func() map[string]OfficialPricingReview {
	var reviews map[string]OfficialPricingReview
	if err := common.Unmarshal(officialPricingReviewsJSON, &reviews); err != nil {
		panic(err)
	}
	return reviews
}()

func publicOfficialPricingReview(name string, price OfficialPricing) *OfficialPricingReview {
	if len(price.Tiers) > 0 {
		return nil
	}
	review, ok := officialPricingReviews[name]
	if !ok {
		review.Code = "unverified"
	}
	// This follows the existing public manufacturer-source policy, not an
	// inference that a suffix means free inference at the manufacturer.
	if strings.HasSuffix(name, ":free") {
		review.Code = "platform_variant"
	}
	return &review
}
