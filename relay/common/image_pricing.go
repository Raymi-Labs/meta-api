package common

// ImagePricingDimensions is a server-owned view, never forwarded upstream.
type ImagePricingDimensions struct {
	InputCount  int    `json:"input_count"`
	OutputCount int    `json:"output_count"`
	Resolution  string `json:"resolution"`
}
