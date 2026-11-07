package gemini

import (
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func buildGeminiEmbeddingUsage(c *gin.Context, info *relaycommon.RelayInfo, metadata *dto.GeminiEmbeddingUsageMetadata) *dto.Usage {
	if metadata == nil || metadata.PromptTokenCount <= 0 {
		return service.ResponseText2Usage(c, "", info.UpstreamModelName, info.GetEstimatePromptTokens())
	}
	usage := buildUsageFromGeminiMetadata(dto.GeminiUsageMetadata{
		PromptTokenCount:    metadata.PromptTokenCount,
		TotalTokenCount:     metadata.PromptTokenCount,
		PromptTokensDetails: metadata.PromptTokenDetails,
	}, 0)
	return &usage
}
