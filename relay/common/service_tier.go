package common

import (
	"bytes"
	"fmt"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"strings"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

func (info *RelayInfo) ServiceTierMultiplier() float64 {
	if info == nil || !info.ManagedServiceTier {
		return 1
	}
	switch info.RequestedServiceTier {
	case "priority":
		return 2
	case "ultrafast":
		return 6
	default:
		return 1
	}
}

// Check the final serialized body, after filtering, model mapping and overrides.
// It also covers channel retries and internal Chat-to-Responses reissues.
func ValidatePaidServiceTierBody(info *RelayInfo, body io.Reader) (io.Reader, error) {
	if info == nil || !info.ManagedServiceTier {
		return body, nil
	}
	if body == nil {
		return nil, fmt.Errorf("missing service tier request body")
	}
	raw, err := io.ReadAll(io.LimitReader(body, appcommon.GetRequestBodyLimitBytes(nil)+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > appcommon.GetRequestBodyLimitBytes(nil) {
		return nil, fmt.Errorf("service tier request exceeds size limit")
	}
	tierKeys := 0
	canonical := true
	gjson.ParseBytes(raw).ForEach(func(key, value gjson.Result) bool {
		if strings.EqualFold(key.String(), "service_tier") {
			tierKeys++
			canonical = canonical && key.String() == "service_tier"
		}
		return true
	})
	if tierKeys > 1 || !canonical {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("service_tier must appear once with its canonical spelling"), types.ErrorCode("service_tier_invalid"), http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	var request struct {
		ServiceTier string `json:"service_tier"`
	}
	if err := appcommon.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	expected := info.RequestedServiceTier
	if expected == "" {
		expected = "default"
	}
	actual := request.ServiceTier
	if actual == "" {
		actual = "default"
	}
	if actual != expected {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("outbound service tier differs from selected tier; request was not sent"), types.ErrorCode("service_tier_mismatch"), http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	return bytes.NewReader(raw), nil
}
