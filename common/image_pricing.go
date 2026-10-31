package common

import "strings"

const ImagePricingField = "_image_billing"

func IsQwen3ImageModel(name string) bool {
	return name == "qwen-image-3.0" || name == "qwen-image-3.0-pro"
}

func IsImagePriceExpression(expr string) bool {
	return strings.Contains(expr, ImagePricingField) || strings.Contains(expr, `tier("image_config_preview"`)
}
