package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var unknownProviderModelPattern = regexp.MustCompile(`^unknown provider for model [^\s]+$`)
var responsesToolCapabilityPattern = regexp.MustCompile(`^[^\s]+ requires responses for tool calls; this account only supports chat completions\.?$`)

// IsUpstreamCapabilityError recognizes explicit pre-execution rejections, not
// generic model_not_found or invalid_request errors. Require an actual upstream
// 400 so local validation and errors inside an already-started 200 stream do not
// acquire new retry behavior.
func IsUpstreamCapabilityError(err *types.NewAPIError) bool {
	if err == nil || err.UpstreamStatusCode != http.StatusBadRequest || err.GetErrorType() != types.ErrorTypeOpenAIError {
		return false
	}
	oai := err.ToOpenAIError()
	if oai.Type != "invalid_request_error" {
		return false
	}
	message := strings.ToLower(strings.Join(strings.Fields(oai.Message), " "))
	return (oai.Code == "model_not_found" && unknownProviderModelPattern.MatchString(message)) ||
		((oai.Code == nil || oai.Code == "" || oai.Code == "invalid_request_error") && responsesToolCapabilityPattern.MatchString(message))
}

// GuardUpstreamCapabilityRetry leaves provider-owned conversation references on
// their original channel. Self-contained tool history may use normal failover.
// Even a flushed heartbeat is conservatively treated as a committed response.
func GuardUpstreamCapabilityRetry(c *gin.Context, request dto.Request, err *types.NewAPIError) *types.NewAPIError {
	if !IsUpstreamCapabilityError(err) {
		return err
	}
	unsafe := c == nil || c.Writer == nil || c.Writer.Written() || request == nil
	if c != nil {
		// Auto-group selection can reset its priority retry counter. Do not let
		// that grant these new failures more than the configured total budget.
		unsafe = unsafe || len(c.GetStringSlice("use_channel")) > common.RetryTimes
	}
	switch req := request.(type) {
	case *dto.OpenAIResponsesRequest:
		conversation := bytes.TrimSpace(req.Conversation)
		unsafe = unsafe || req.PreviousResponseID != "" || (len(conversation) > 0 && !bytes.Equal(conversation, []byte("null")))
	case *dto.OpenAIResponsesCompactionRequest:
		unsafe = unsafe || req.PreviousResponseID != ""
	}
	if unsafe {
		return types.NewError(err, err.GetErrorCode(), types.ErrOptionWithSkipRetry())
	}
	return err
}

// recoverUpstreamCapabilityError handles a known malformed error envelope:
// one JSON error immediately followed by one empty response.failed SSE event.
// Never accept arbitrary trailing JSON, output events, or a successful response.
func recoverUpstreamCapabilityError(body []byte, status int) *types.NewAPIError {
	if status != http.StatusBadRequest || len(body) > 64*1024 {
		return nil
	}
	body = bytes.TrimSpace(body)
	var first json.RawMessage
	if common.DecodeJson(bytes.NewReader(body), &first) != nil {
		return nil
	}
	var envelope dto.GeneralErrorResponse
	if common.Unmarshal(first, &envelope) != nil || common.GetJsonType(envelope.Error) != "object" {
		return nil
	}
	oai := envelope.TryToOpenAIError()
	if oai == nil {
		return nil
	}
	apiErr := types.WithOpenAIError(*oai, status)
	apiErr.UpstreamStatusCode = status
	if !IsUpstreamCapabilityError(apiErr) {
		return nil
	}
	tail := strings.TrimSpace(string(body[len(first):]))
	event, data, ok := strings.Cut(tail, "\n")
	if !ok || strings.TrimSpace(event) != "event: response.failed" {
		return nil
	}
	data = strings.TrimSpace(data)
	if !strings.HasPrefix(data, "data:") {
		return nil
	}
	var failure struct {
		Type     string `json:"type"`
		Response struct {
			Status string            `json:"status"`
			Output []json.RawMessage `json:"output"`
		} `json:"response"`
	}
	if common.UnmarshalJsonStr(strings.TrimSpace(strings.TrimPrefix(data, "data:")), &failure) != nil ||
		failure.Type != "response.failed" || failure.Response.Status != "failed" || len(failure.Response.Output) != 0 {
		return nil
	}
	return apiErr
}
