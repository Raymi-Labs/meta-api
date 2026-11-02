package types

import (
	"context"
	"errors"
	"strings"
)

// FailureClass is conservative: only positively identified request faults are
// excluded from availability metrics. HTTP status alone cannot assign blame.
type FailureClass string

const (
	FailureUnknown        FailureClass = "unknown"
	FailureUserRequest    FailureClass = "user_request"
	FailureClientCanceled FailureClass = "client_canceled"
	FailureService        FailureClass = "service"
)

// StatusClientClosedRequest matches the status used by the reverse proxy when
// the downstream connection closes before a response can be delivered.
const StatusClientClosedRequest = 499

func NewClientCanceledError(ops ...NewAPIErrorOptions) *NewAPIError {
	return NewErrorWithStatusCode(context.Canceled, ErrorCodeClientCanceled, StatusClientClosedRequest,
		append([]NewAPIErrorOptions{ErrOptionWithSkipRetry()}, ops...)...)
}

func ClassifyFailure(err *NewAPIError) FailureClass {
	if err == nil {
		return FailureUnknown
	}
	code := err.GetErrorCode()
	local := err.GetErrorType() == ErrorTypeNewAPIError && err.UpstreamStatusCode == 0
	if local {
		if code == ErrorCodeClientCanceled || errors.Is(err.Err, context.Canceled) || (code == ErrorCodeDoRequestFailed && strings.HasPrefix(err.Error(), "client canceled while receiving responses stream")) {
			return FailureClientCanceled
		}
		switch code {
		case ErrorCodeInsufficientUserQuota, ErrorCodePreConsumeTokenQuotaFailed:
			// Codes/status/text can also describe failed database operations. Only a
			// typed rejection from a local authoritative check can leave the denominator.
			var rejection *QuotaRejection
			if errors.As(err.Err, &rejection) && rejection != nil {
				return FailureUserRequest
			}
		case ErrorCodeBadRequestBody, ErrorCodeReadRequestBodyFailed:
			if err.StatusCode == 400 || err.StatusCode == 413 {
				return FailureUserRequest
			}
		case ErrorCodeGetChannelFailed, ErrorCodeChannelNoAvailableKey, ErrorCodeChannelInvalidKey,
			ErrorCodeChannelModelMappedError, ErrorCodeChannelParamOverrideInvalid, ErrorCodeChannelHeaderOverrideInvalid:
			return FailureService
		}
	}
	// Structured upstream parameter errors are narrower than generic 400/404.
	// Model-not-found and permission failures may be gateway configuration faults.
	if err.StatusCode == 400 || err.StatusCode == 422 {
		switch string(code) {
		case "context_length_exceeded", "max_tokens_exceeded", "invalid_image_format", "invalid_base64":
			return FailureUserRequest
		}
	}
	if err.StatusCode >= 500 || err.StatusCode == 429 || err.UpstreamStatusCode == 401 || err.UpstreamStatusCode == 403 {
		return FailureService
	}
	return FailureUnknown
}
