package types

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestClassifyFailure(t *testing.T) {
	cases := []struct {
		name string
		err  *NewAPIError
		want FailureClass
	}{
		{"quota", NewErrorWithStatusCode(&QuotaRejection{Source: "wallet", Message: "quota"}, ErrorCodeInsufficientUserQuota, 403), FailureUserRequest},
		{"bad body", NewErrorWithStatusCode(errors.New("bad json"), ErrorCodeBadRequestBody, 400), FailureUserRequest},
		{"adapter bad body", NewErrorWithStatusCode(errors.New("websocket write failed"), ErrorCodeBadRequestBody, 500), FailureService},
		{"internal validation", NewErrorWithStatusCode(errors.New("invalid request type"), ErrorCodeInvalidRequest, 400), FailureUnknown},
		{"model not found", WithOpenAIError(OpenAIError{Code: "model_not_found"}, 404), FailureUnknown},
		{"context too long", WithOpenAIError(OpenAIError{Code: "context_length_exceeded"}, 400), FailureUserRequest},
		{"rate limit", WithOpenAIError(OpenAIError{Code: "rate_limit_exceeded"}, 429), FailureService},
		{"cancel", NewErrorWithStatusCode(context.Canceled, ErrorCodeDoRequestFailed, 500), FailureClientCanceled},
		{"stream cancel", NewErrorWithStatusCode(errors.New("client canceled while receiving responses stream"), ErrorCodeDoRequestFailed, 500), FailureClientCanceled},
		{"timeout", NewErrorWithStatusCode(context.DeadlineExceeded, ErrorCodeDoRequestFailed, 504), FailureService},
		{"no route", NewErrorWithStatusCode(errors.New("no route"), ErrorCodeGetChannelFailed, 503), FailureService},
		{"upstream quota code", WithOpenAIError(OpenAIError{Code: "insufficient_user_quota"}, 403), FailureUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyFailure(tc.err); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestClientCancellationClassificationSurvivesMessageReplacement(t *testing.T) {
	err := NewClientCanceledError(ErrOptionWithHideErrMsg("client request canceled"))
	err.SetMessage(err.Error() + " (request id: req-canceled)")
	if ClassifyFailure(err) != FailureClientCanceled || err.StatusCode != StatusClientClosedRequest || !IsSkipRetryError(err) {
		t.Fatalf("lost cancellation classification after message replacement: %+v", err)
	}
	upstream := WithOpenAIError(OpenAIError{Code: "client_canceled", Message: "provider canceled"}, 500)
	if ClassifyFailure(upstream) != FailureService {
		t.Fatal("upstream cancellation must not be attributed to the client")
	}
}

func TestQuotaFailureRequiresLocalStructuredEvidence(t *testing.T) {
	for _, code := range []ErrorCode{ErrorCodeInsufficientUserQuota, ErrorCodePreConsumeTokenQuotaFailed} {
		for _, source := range []string{"wallet", "token", "token_package", "subscription"} {
			t.Run(string(code)+"/"+source, func(t *testing.T) {
				cause := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", &QuotaRejection{Source: source, Message: "fixture"}))
				e := NewErrorWithStatusCode(cause, code, 403)
				e.SetMessage("translated text (request id: fixture)")
				if got := ClassifyFailure(e); got != FailureUserRequest {
					t.Fatalf("wrapped quota: %s", got)
				}
				e.UpstreamStatusCode = 403
				if ClassifyFailure(e) == FailureUserRequest {
					t.Fatal("upstream evidence must prevent exclusion")
				}
				system := NewErrorWithStatusCode(cause, ErrorCodeUpdateDataError, 500)
				if ClassifyFailure(system) != FailureService {
					t.Fatal("system failure must dominate nested quota")
				}
			})
		}
		for _, cause := range []error{errors.New("quota insufficient"), errors.New("database timeout"), errors.New("subscription quota insufficient")} {
			e := NewErrorWithStatusCode(cause, code, 403)
			if ClassifyFailure(e) == FailureUserRequest {
				t.Fatal("untyped cause must stay in denominator")
			}
		}
		upstream := WithOpenAIError(OpenAIError{Code: string(code), Message: "quota insufficient"}, 403)
		if ClassifyFailure(upstream) == FailureUserRequest {
			t.Fatal("provider cannot spoof local rejection")
		}
	}
}
