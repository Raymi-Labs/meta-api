package model

import "strings"

const (
	defaultInceptionVendorName        = "Inception"
	defaultInceptionVendorDescription = "Inception 的 Mercury 扩散式语言模型系列"
)

func GetDefaultVendorDescription(name string) string {
	if isInceptionVendorAlias(name) {
		return defaultInceptionVendorDescription
	}
	return ""
}

func isInceptionVendorAlias(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "inception", "inception labs", "inceptionlabs":
		return true
	default:
		return false
	}
}

func isInceptionModel(modelName string) bool {
	return inferModelDisplayVendor(modelName) == "inception"
}

// Fill only missing display metadata; administrator-supplied fields win.
func fillInceptionVendorDefaults(vendor *Vendor) {
	if vendor == nil || !isInceptionVendorAlias(vendor.Name) {
		return
	}
	if strings.TrimSpace(vendor.Icon) == "" {
		vendor.Icon = getDefaultVendorIcon(defaultInceptionVendorName)
	}
	if strings.TrimSpace(vendor.Description) == "" {
		vendor.Description = defaultInceptionVendorDescription
	}
}
