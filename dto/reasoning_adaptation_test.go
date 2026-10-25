package dto

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReasoningAdaptationValidation(t *testing.T) {
	for _, mode := range []string{"", "inherit", "auto", ReasoningTemplateZhipuGLM53} {
		require.NoError(t, (ChannelSettings{ReasoningAdaptation: ReasoningAdaptation{Mode: mode}}).Validate())
	}
	rule := ReasoningAdaptationRule{Model: "glm-5.3-flash", Protocol: "chat", Mapping: map[string]string{"high": "high", "max": "max"}}
	require.NoError(t, (ReasoningAdaptation{Mode: "auto", Rules: []ReasoningAdaptationRule{rule}}).Validate())
	require.Error(t, (ReasoningAdaptation{Mode: "bad"}).Validate())
	require.Error(t, (ReasoningAdaptation{Rules: []ReasoningAdaptationRule{rule, rule}}).Validate())
	for _, bad := range []ReasoningAdaptationRule{
		{Model: "glm-*", Protocol: "chat", Mapping: rule.Mapping},
		{Model: " glm-5.3", Protocol: "chat", Mapping: rule.Mapping},
		{Model: "glm-5.3", Protocol: "responses", Mapping: rule.Mapping},
		{Model: "glm-5.3", Protocol: "chat", Mapping: map[string]string{"high": "none"}},
		{Model: "glm-5.3", Protocol: "chat", Mapping: map[string]string{}},
	} {
		require.Error(t, (ReasoningAdaptation{Rules: []ReasoningAdaptationRule{bad}}).Validate())
	}
}
