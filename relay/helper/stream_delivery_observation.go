package helper

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Inspect only the already serialized frame after the write/flush result is
// known. No payload is retained. A role, ID, usage or heartbeat is not output.
func observeDeliveredStreamFrame(c *gin.Context, data string, heartbeat bool, err error) {
	if c.Request == nil {
		return
	}
	switch c.Request.URL.Path {
	case "/v1/chat/completions", "/pg/chat/completions", "/v1/completions", "/v1/responses", "/v1/messages":
	default:
		return // Other protocols must remain unknown, not zero-content streams.
	}
	if _, exists := c.Get(common.StreamDeliveryObservationKey); !exists {
		return
	}
	common.ObserveStreamDelivery(c, heartbeat, !heartbeat && err == nil && effectiveStreamFrame(data), err)
}

func effectiveStreamFrame(data string) bool {
	v := gjson.Parse(data)
	switch v.Get("type").String() {
	case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.refusal.delta", "response.function_call_arguments.delta":
		return v.Get("delta").Type == gjson.String && v.Get("delta").String() != ""
	case "content_block_delta":
		for _, key := range []string{"delta.text", "delta.thinking", "delta.partial_json"} {
			if v.Get(key).String() != "" {
				return true
			}
		}
	case "content_block_start":
		return v.Get("content_block.text").String() != "" || v.Get("content_block.thinking").String() != ""
	}
	for _, choice := range v.Get("choices").Array() {
		for _, key := range []string{"text", "delta.content", "delta.reasoning_content", "delta.reasoning", "delta.function_call.name", "delta.function_call.arguments"} {
			field := choice.Get(key)
			if field.Type == gjson.String && field.String() != "" {
				return true
			}
		}
		for _, call := range choice.Get("delta.tool_calls").Array() {
			if call.Get("function.name").String() != "" || call.Get("function.arguments").String() != "" {
				return true
			}
		}
	}
	return false
}
