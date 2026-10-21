package grouphealth

import (
	"context"
	"errors"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRequestFailureClassification(t *testing.T) {
	now := time.Now()
	info := &relaycommon.RelayInfo{UsingGroup: "group-a"}
	userErr := types.NewErrorWithStatusCode(errors.New("bad body"), types.ErrorCodeBadRequestBody, 400)
	r := NewRequest("model", true)
	r.ObserveResult(info, userErr, true, now)
	require.Equal(t, OutcomeUserError, r.Samples(now)[0].Outcome, "excluded request errors remain observable")
	r.Failure("group-a")
	r.ObserveResult(info, userErr, true, now)
	require.Equal(t, OutcomeServiceFailure, r.Samples(now)[0].Outcome, "user error cannot erase service failure")
	r.ObserveResult(info, nil, true, now)
	require.Equal(t, OutcomeSuccess, r.Samples(now)[0].Outcome, "successful same-group retry folds")
	info.UsingGroup = "group-b"
	r.ObserveResult(info, types.NewErrorWithStatusCode(context.Canceled, types.ErrorCodeDoRequestFailed, 500), false, now)
	require.Len(t, r.Samples(now), 2, "cancellation must not silently inflate availability")
	require.Equal(t, OutcomeClientCanceled, r.groups["group-b"].Outcome)
	r.ObserveResult(info, types.WithOpenAIError(types.OpenAIError{Code: "model_not_found"}, 404), true, now)
	require.Equal(t, OutcomeUnknownFailure, r.groups["group-b"].Outcome, "unsupported upstream model is not success")
}

func TestRequestSuccessfulTerminalCannotBeDowngraded(t *testing.T) {
	now := time.Now()
	info := &relaycommon.RelayInfo{UsingGroup: "group", StreamStatus: relaycommon.NewStreamStatus()}
	info.StreamStatus.RequireTerminal()
	info.StreamStatus.MarkOutcome(relaycommon.ResponseOutcomeCompleted)
	r := NewRequest("model", true)
	r.ObserveResult(info, nil, true, now)
	require.Equal(t, OutcomeSuccess, r.groups["group"].Outcome)
	// Only after the delivered terminal was sampled do late teardown events occur.
	info.StreamStatus.RecordError("late teardown event")
	r.ObserveResult(info, types.NewClientCanceledError(), false, now)
	r.ObserveResult(info, nil, false, now)
	r.ObserveUnrecordedFailure("group", types.NewClientCanceledError())
	require.Equal(t, OutcomeSuccess, r.groups["group"].Outcome)

	fresh := NewRequest("model", true)
	fresh.ObserveResult(info, nil, true, now)
	require.Equal(t, OutcomeUnknownFailure, fresh.groups["group"].Outcome, "an error before initial sampling still fails")
}

func TestRequestRequiresActualDeliveryAndStreamTerminal(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name      string
		delivered bool
		status    *relaycommon.StreamStatus
		want      Outcome
	}{
		{name: "nil error but no write", want: OutcomeUnknownFailure},
		{name: "written body", delivered: true, want: OutcomeSuccess},
		{name: "missing stream terminal", delivered: true, status: relaycommon.NewStreamStatus(), want: OutcomeUnknownFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRequest("model", false)
			r.ObserveResult(&relaycommon.RelayInfo{UsingGroup: "group", StreamStatus: tc.status}, nil, tc.delivered, now)
			require.Equal(t, tc.want, r.groups["group"].Outcome)
		})
	}
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonClientGone, context.Canceled)
	r := NewRequest("model", true)
	r.ObserveResult(&relaycommon.RelayInfo{UsingGroup: "group", StreamStatus: status}, nil, false, now)
	require.Equal(t, OutcomeClientCanceled, r.groups["group"].Outcome)
}

func TestRequestUnobservedFailuresKeepGroupAttribution(t *testing.T) {
	r := NewRequest("model", false)
	r.ObserveFailure("first", types.NewErrorWithStatusCode(errors.New("upstream"), types.ErrorCodeDoRequestFailed, 502))
	r.ObserveUnrecordedFailure("first", types.NewClientCanceledError())
	r.ObserveUnrecordedFailure("fallback", types.NewClientCanceledError())
	r.ObserveUnrecordedFailure("auto", types.NewClientCanceledError())
	r.ObserveUnrecordedFailure("", types.NewClientCanceledError())
	r.ObserveUnrecordedFailure("nil-error", nil)
	require.Len(t, r.groups, 2)
	require.Equal(t, OutcomeServiceFailure, r.groups["first"].Outcome)
	require.Equal(t, OutcomeClientCanceled, r.groups["fallback"].Outcome)
}

func TestOutcomeForErrorDoesNotInferIntentFromStatusOrUpstreamCode(t *testing.T) {
	require.Equal(t, OutcomeUnknownFailure, OutcomeForError(types.WithOpenAIError(types.OpenAIError{Code: "client_canceled"}, 499)))
	require.Equal(t, OutcomeClientCanceled, OutcomeForError(types.NewClientCanceledError()))
	require.Equal(t, OutcomeUnknownFailure, OutcomeForError(nil))
	for _, tc := range []struct {
		code   types.ErrorCode
		status int
	}{
		{types.ErrorCodeInvalidRequest, 400},
		{types.ErrorCodeAccessDenied, 403},
	} {
		local := types.NewErrorWithStatusCode(errors.New("local request rejection"), tc.code, tc.status)
		require.Equal(t, OutcomeUserError, OutcomeForError(local))
		upstream := types.WithOpenAIError(types.OpenAIError{Code: string(tc.code)}, tc.status)
		require.NotEqual(t, OutcomeUserError, OutcomeForError(upstream))
		local.UpstreamStatusCode = tc.status
		require.NotEqual(t, OutcomeUserError, OutcomeForError(local), "upstream evidence prevents local-only exclusion")
	}
}

func TestRequestLocalPolicyExclusionDoesNotExcludeUpstreamRateLimit(t *testing.T) {
	r := NewRequest("model", false)
	r.ExcludeUserError("local-policy")
	r.ObserveUnrecordedFailure("local-policy", types.WithOpenAIError(types.OpenAIError{Code: "content_safety_cooldown"}, 429))
	require.Equal(t, OutcomeUserError, r.groups["local-policy"].Outcome)
	r.ObserveFailure("upstream-policy", types.WithOpenAIError(types.OpenAIError{Code: "content_safety_cooldown"}, 429))
	require.Equal(t, OutcomeServiceFailure, r.groups["upstream-policy"].Outcome)
	r.ExcludeUserError("upstream-policy")
	require.Equal(t, OutcomeServiceFailure, r.groups["upstream-policy"].Outcome)
}

func TestRequestKnownServiceFailureSurvivesCancellationAtEveryStage(t *testing.T) {
	for _, stage := range []string{"before attempt", "capacity wait", "during attempt"} {
		t.Run(stage, func(t *testing.T) {
			r := NewRequest("model", false)
			r.Failure("group")
			switch stage {
			case "before attempt":
				r.ObserveUnrecordedFailure("group", types.NewClientCanceledError())
			case "capacity wait":
				r.ObserveFailure("group", types.NewClientCanceledError())
			case "during attempt":
				r.ObserveResult(&relaycommon.RelayInfo{UsingGroup: "group"}, types.NewClientCanceledError(), false, time.Now())
			}
			require.Equal(t, OutcomeServiceFailure, r.groups["group"].Outcome)
			r.ObserveResult(&relaycommon.RelayInfo{UsingGroup: "group"}, nil, true, time.Now())
			require.Equal(t, OutcomeSuccess, r.groups["group"].Outcome, "a recovered request still succeeds")
		})
	}
}
