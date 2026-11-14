package common

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

// Preview input is synthetic. Never populate it from an authenticated request's
// headers or a stored channel: previewing a draft needs no real credentials.
type ParamOverridePreviewContext struct {
	OriginalModel  string                 `json:"original_model"`
	UpstreamModel  string                 `json:"upstream_model"`
	RequestPath    string                 `json:"request_path"`
	IsChannelTest  bool                   `json:"is_channel_test"`
	RetryIndex     int                    `json:"retry_index"`
	RequestHeaders map[string]string      `json:"request_headers"`
	HeaderOverride map[string]interface{} `json:"header_override"`
	LastError      *struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Code       string `json:"code"`
		Type       string `json:"type"`
	} `json:"last_error"`
}

type ParamOverridePreviewStep struct {
	Index             int    `json:"index"`
	Mode              string `json:"mode"`
	ConditionsMatched bool   `json:"conditions_matched"`
}

type ParamOverridePreviewResult struct {
	Body           string                     `json:"body"`
	HeaderOverride map[string]interface{}     `json:"header_override"`
	Steps          []ParamOverridePreviewStep `json:"steps"`
	Error          string                     `json:"error,omitempty"`
	Rejection      *ParamOverrideReturnError  `json:"rejection,omitempty"`
}

// PreviewParamOverride uses the same engine and context builder as relay. It
// neither constructs an HTTP relay nor reads/writes models, quotas or logs.
func PreviewParamOverride(body string, override map[string]interface{}, sample ParamOverridePreviewContext) (*ParamOverridePreviewResult, error) {
	if !common.Valid([]byte(body)) || common.GetJsonType([]byte(body)) != "object" {
		return nil, fmt.Errorf("sample body must be a JSON object")
	}
	if _, exists := override["operations"]; exists {
		ops, ok := tryParseOperations(override)
		if !ok {
			return nil, fmt.Errorf("invalid operations")
		}
		if len(ops) > 100 {
			return nil, fmt.Errorf("preview supports at most 100 operations")
		}
	}
	info := &RelayInfo{
		ChannelMeta:     &ChannelMeta{UpstreamModelName: sample.UpstreamModel, HeadersOverride: sample.HeaderOverride},
		OriginModelName: sample.OriginalModel, RequestURLPath: sample.RequestPath,
		IsChannelTest: sample.IsChannelTest, RetryIndex: sample.RetryIndex,
		RequestHeaders: sample.RequestHeaders,
	}
	if sample.LastError != nil {
		info.LastError = types.WithOpenAIError(types.OpenAIError{Message: sample.LastError.Message, Code: sample.LastError.Code, Type: sample.LastError.Type}, sample.LastError.StatusCode)
	}
	ctx := BuildParamOverrideContext(info)
	recorder := &paramOverrideAuditRecorder{preview: true}
	ctx[paramOverrideContextAuditRecorder] = recorder
	output, err := ApplyParamOverride([]byte(body), override, ctx)
	result := &ParamOverridePreviewResult{Body: string(output), Steps: recorder.steps, HeaderOverride: ctx[paramOverrideContextHeaderOverride].(map[string]interface{})}
	if result.Steps == nil {
		result.Steps = []ParamOverridePreviewStep{}
	}
	if err != nil {
		result.Error = err.Error()
		result.Rejection, _ = AsParamOverrideReturnError(err)
	}
	return result, nil
}
