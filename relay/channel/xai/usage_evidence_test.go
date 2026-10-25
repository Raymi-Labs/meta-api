package xai

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestXAIUsageEstimatesRemainMarkedForMeasuredMetrics(t *testing.T) {
	for _, tc := range []struct {
		name          string
		usage         dto.Usage
		text          string
		wantEstimated bool
	}{
		{name: "actual upstream counts", usage: dto.Usage{PromptTokens: 100, CompletionTokens: 10}},
		{name: "estimated prompt", usage: dto.Usage{CompletionTokens: 10}, wantEstimated: true},
		{name: "estimated completion", usage: dto.Usage{PromptTokens: 100}, text: "hello", wantEstimated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			usage := ensureXAIUsage(c, &tc.usage, tc.text, 0, "grok-test", 100)
			require.Equal(t, tc.wantEstimated, usage.InputTokensEstimated)
			require.Equal(t, 100, usage.PromptTokens)
		})
	}
}
