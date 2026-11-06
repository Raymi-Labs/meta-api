package types

// RoutePlanSnapshot is the routing plan resolved once when a request is
// authenticated. Every retry, fallback group and settlement of that request
// uses this same snapshot, so a switch to another upstream group never changes
// the customer's unit price mid-request.
type RoutePlanSnapshot struct {
	ID          int                `json:"id"`
	Version     int64              `json:"version"`
	Name        string             `json:"name"`
	Groups      []string           `json:"groups"`
	SaleRatio   float64            `json:"sale_ratio"`
	ModelRatios map[string]float64 `json:"model_ratios,omitempty"`
}

// RatioFor returns the contract multiplier for a model. normalized is the
// matching-name fallback (e.g. a dated variant mapped to its family name).
func (p *RoutePlanSnapshot) RatioFor(modelName, normalized string) float64 {
	if p == nil {
		return 1
	}
	if ratio, ok := p.ModelRatios[modelName]; ok && ratio >= 0 {
		return ratio
	}
	if normalized != "" {
		if ratio, ok := p.ModelRatios[normalized]; ok && ratio >= 0 {
			return ratio
		}
	}
	return p.SaleRatio
}
