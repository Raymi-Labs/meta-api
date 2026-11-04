package service

import (
	"fmt"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

// Match only structured pre-execution refusal codes, never generic 429/503 text.
func IsVMChannelCapacityError(ch *model.Channel, err *types.NewAPIError) bool {
	if !VMCapacityEnabled(ch) || err == nil {
		return false
	}
	status := err.UpstreamStatusCode
	if status == 0 {
		status = err.StatusCode
	}
	if status != 429 && status != 503 {
		return false
	}
	code := fmt.Sprint(err.ToOpenAIError().Code)
	switch code {
	case "channel_capacity_busy", "account_pool_exhausted", "channel_capacity_unavailable", "api_key_concurrency_limit":
		return true
	}
	return false
}
