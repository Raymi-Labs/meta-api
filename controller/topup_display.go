package controller

import (
	"strconv"
	"strings"
)

// Presentation never changes the gateway type or the stored PayMethods maps.
func stripeTopUpPayMethods(methods []map[string]string, enabled bool, name, icon string, minTopUp int) []map[string]string {
	result := make([]map[string]string, 0, len(methods)+1)
	name = strings.TrimSpace(name)
	found := false
	decorate := func(method map[string]string) {
		if name != "" {
			method["name"] = name
		} else if strings.TrimSpace(method["name"]) == "" {
			method["name"] = "Stripe"
		}
		if icon != "" {
			method["display_icon"] = icon
		}
		if method["min_topup"] == "" {
			method["min_topup"] = strconv.Itoa(minTopUp)
		}
	}
	for _, method := range methods {
		copy := make(map[string]string, len(method)+2)
		for key, value := range method {
			copy[key] = value
		}
		if copy["type"] == "stripe" {
			found = true
			decorate(copy)
		}
		result = append(result, copy)
	}
	if enabled && !found {
		method := map[string]string{
			"type": "stripe", "color": "rgba(var(--semi-purple-5), 1)",
		}
		decorate(method)
		result = append(result, method)
	}
	return result
}
