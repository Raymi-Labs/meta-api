package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRejectedSamplingParameter(t *testing.T) {
	for _, tc := range []struct{ message, want string }{
		{"claude-haiku-5-5 does not support non-default temperature", "temperature"},
		{"vendor-model-20261019 does not support non-default `temperature`.", "temperature"},
		{"`temperature` is deprecated for this model.", "temperature"},
		{"`top_k` is not supported with thinking.", "top_k"},
		{"Unsupported parameter: top_p", "top_p"},
		{"Unsupported parameter: temperature_extra", ""},
		{"Unsupported parameter: max_tokens", ""},
		{"temperature must be between 0 and 1", ""},
		{"`temperature` is not supported with top_p", ""},
		{"Example says `temperature` is deprecated for this model", ""},
		{"temperature supports non-default values", ""},
		{"unknown model", ""},
	} {
		t.Run(tc.message, func(t *testing.T) { require.Equal(t, tc.want, RejectedSamplingParameter(tc.message)) })
	}
}

func TestDescribeModelAndSamplingFailure(t *testing.T) {
	local := types.NewErrorWithStatusCode(fmt.Errorf("model unavailable"), types.ErrorCodeModelNotFound, 404)
	category, _, hint := describeRequestFailure(local, false)
	require.Equal(t, "model_unavailable", category)
	require.Contains(t, hint, "/v1/models")
	category, _, _ = describeRequestFailure(local, true)
	require.Equal(t, "invalid_request", category, "upstream 404 must not be blamed on local grouping")
	upstream := types.WithOpenAIError(types.OpenAIError{Type: "invalid_request_error", Message: "claude-haiku-5-5 does not support non-default temperature"}, 400)
	category, _, hint = describeRequestFailure(upstream, true)
	require.Equal(t, "sampling_parameter", category)
	require.Contains(t, hint, "temperature")
	category, _, _ = describeRequestFailure(upstream, false)
	require.Equal(t, "invalid_request", category)
}
