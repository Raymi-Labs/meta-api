package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"
)

func MidjourneyErrorWrapper(code int, desc string) *dto.MidjourneyResponse {
	return &dto.MidjourneyResponse{
		Code:        code,
		Description: desc,
	}
}

func MidjourneyErrorWithStatusCodeWrapper(code int, desc string, statusCode int) *dto.MidjourneyResponseWithStatusCode {
	return &dto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   *MidjourneyErrorWrapper(code, desc),
	}
}

//// OpenAIErrorWrapper wraps an error into an OpenAIErrorWithStatusCode
//func OpenAIErrorWrapper(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	text := err.Error()
//	lowerText := strings.ToLower(text)
//	if !strings.HasPrefix(lowerText, "get file base64 from url") && !strings.HasPrefix(lowerText, "mime type is not supported") {
//		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
//			common.SysLog(fmt.Sprintf("error: %s", text))
//			text = "请求上游地址失败"
//		}
//	}
//	openAIError := dto.OpenAIError{
//		Message: text,
//		Type:    "new_api_error",
//		Code:    code,
//	}
//	return &dto.OpenAIErrorWithStatusCode{
//		Error:      openAIError,
//		StatusCode: statusCode,
//	}
//}
//
//func OpenAIErrorWrapperLocal(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	openaiErr := OpenAIErrorWrapper(err, code, statusCode)
//	openaiErr.LocalError = true
//	return openaiErr
//}

func ClaudeErrorWrapper(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if !strings.HasPrefix(lowerText, "get file base64 from url") {
		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
			common.SysLog(fmt.Sprintf("error: %s", text))
			text = "请求上游地址失败"
		}
	}
	claudeError := types.ClaudeError{
		Message: text,
		Type:    "new_api_error",
	}
	return &dto.ClaudeErrorWithStatusCode{
		Error:      claudeError,
		StatusCode: statusCode,
	}
}

func ClaudeErrorWrapperLocal(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	claudeErr := ClaudeErrorWrapper(err, code, statusCode)
	claudeErr.LocalError = true
	return claudeErr
}

func RelayErrorHandler(ctx context.Context, resp *http.Response, showBodyWhenFail bool) (newApiErr *types.NewAPIError) {
	newApiErr = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	upstreamStatusCode := resp.StatusCode
	retryAfter := ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	upstreamDiagnostics := buildUpstreamDiagnostics(resp, 0)
	defer func() {
		if newApiErr == nil {
			return
		}
		newApiErr.UpstreamStatusCode = upstreamStatusCode
		newApiErr.RetryAfter = retryAfter
		if !upstreamDiagnostics.IsZero() {
			newApiErr.Upstream = upstreamDiagnostics
		}
	}()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	upstreamDiagnostics.ResponseLength = int64(len(responseBody))
	CloseResponseBodyGracefully(resp)
	var errResponse dto.GeneralErrorResponse
	buildErrWithBody := func(message string) error {
		if message == "" {
			return fmt.Errorf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody))
		}
		return fmt.Errorf("bad response status code %d, message: %s, body: %s", resp.StatusCode, message, string(responseBody))
	}

	err = common.Unmarshal(responseBody, &errResponse)
	if err != nil {
		if capabilityErr := recoverUpstreamCapabilityError(responseBody, upstreamStatusCode); capabilityErr != nil {
			newApiErr = capabilityErr
			if showBodyWhenFail {
				newApiErr.Err = buildErrWithBody(newApiErr.Error())
			}
			return
		}
		if showBodyWhenFail {
			newApiErr.Err = buildErrWithBody("")
		} else {
			logMessage := fmt.Sprintf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody))
			if diagnostics := upstreamDiagnostics.LogString(); diagnostics != "" {
				logMessage = fmt.Sprintf("%s, upstream={%s}", logMessage, diagnostics)
			}
			logger.LogError(ctx, logMessage)
			newApiErr.Err = fmt.Errorf("bad response status code %d", resp.StatusCode)
		}
		return
	}

	if common.GetJsonType(errResponse.Error) == "object" {
		// General format error (OpenAI, Anthropic, Gemini, etc.)
		oaiError := errResponse.TryToOpenAIError()
		if oaiError != nil {
			newApiErr = types.WithOpenAIError(*oaiError, resp.StatusCode)
			if showBodyWhenFail {
				newApiErr.Err = buildErrWithBody(newApiErr.Error())
			}
			return
		}
	}
	newApiErr = types.NewOpenAIError(errors.New(errResponse.ToMessage()), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	if showBodyWhenFail {
		newApiErr.Err = buildErrWithBody(newApiErr.Error())
	}
	return
}

// ParseRetryAfter 解析上游 Retry-After，支持秒数和 HTTP-date。
func ParseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	retryAfter := retryAt.Sub(now)
	if retryAfter <= 0 {
		return 0
	}
	return retryAfter
}

func buildUpstreamDiagnostics(resp *http.Response, responseLength int64) *types.UpstreamDiagnostics {
	if resp == nil {
		return nil
	}
	diagnostics := &types.UpstreamDiagnostics{
		ResponseLength: responseLength,
	}
	if requestID, header := getUpstreamRequestID(resp.Header); requestID != "" {
		diagnostics.RequestID = requestID
		diagnostics.RequestIDHeader = header
	}
	if resp.Request != nil {
		diagnostics.RequestLength = resp.Request.ContentLength
		if resp.Request.URL != nil {
			diagnostics.URL = sanitizeUpstreamURL(resp.Request.URL)
		}
	}
	return diagnostics
}

func getUpstreamRequestID(header http.Header) (string, string) {
	for _, key := range []string{
		"X-Request-ID",
		"X-Request-Id",
		"OpenAI-Request-ID",
		"Openai-Request-Id",
		"Request-Id",
		"X-Correlation-ID",
	} {
		if value := strings.TrimSpace(header.Get(key)); value != "" {
			return value, key
		}
	}
	return "", ""
}

func sanitizeUpstreamURL(rawURL *url.URL) string {
	if rawURL == nil {
		return ""
	}
	copied := *rawURL
	copied.RawQuery = ""
	copied.Fragment = ""
	return copied.String()
}

func ResetStatusCode(newApiErr *types.NewAPIError, statusCodeMappingStr string) {
	if newApiErr == nil {
		return
	}
	if statusCodeMappingStr == "" || statusCodeMappingStr == "{}" {
		return
	}
	statusCodeMapping := make(map[string]any)
	err := common.Unmarshal([]byte(statusCodeMappingStr), &statusCodeMapping)
	if err != nil {
		return
	}
	if newApiErr.StatusCode == http.StatusOK {
		return
	}
	codeStr := strconv.Itoa(newApiErr.StatusCode)
	if value, ok := statusCodeMapping[codeStr]; ok {
		intCode, ok := parseStatusCodeMappingValue(value)
		if !ok {
			return
		}
		newApiErr.StatusCode = intCode
	}
}

func parseStatusCodeMappingValue(value any) (int, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return 0, false
		}
		statusCode, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return statusCode, true
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	case json.Number:
		statusCode, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, false
		}
		return statusCode, true
	default:
		return 0, false
	}
}

func TaskErrorWrapperLocal(err error, code string, statusCode int) *dto.TaskError {
	openaiErr := TaskErrorWrapper(err, code, statusCode)
	openaiErr.LocalError = true
	return openaiErr
}

func TaskErrorWrapper(err error, code string, statusCode int) *dto.TaskError {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
		common.SysLog(fmt.Sprintf("error: %s", text))
		//text = "请求上游地址失败"
		text = common.MaskSensitiveInfo(text)
	}
	//避免暴露内部错误
	taskError := &dto.TaskError{
		Code:       code,
		Message:    text,
		StatusCode: statusCode,
		Error:      err,
	}

	return taskError
}

// TaskErrorFromAPIError 将 PreConsumeBilling 返回的 NewAPIError 转换为 TaskError。
func TaskErrorFromAPIError(apiErr *types.NewAPIError) *dto.TaskError {
	if apiErr == nil {
		return nil
	}
	return &dto.TaskError{
		Code:               string(apiErr.GetErrorCode()),
		Message:            apiErr.Err.Error(),
		StatusCode:         apiErr.StatusCode,
		UpstreamStatusCode: apiErr.UpstreamStatusCode,
		RetryAfter:         apiErr.RetryAfter,
		Error:              apiErr.Err,
	}
}
