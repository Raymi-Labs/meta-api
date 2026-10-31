package service

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const tokenQuotaOutcomeTokenKey = "token_quota_outcome_token"

// IsTokenQuotaInvocation excludes catalog, task polling, token counting and
// management routes, including POST-based queries.
func IsTokenQuotaInvocation(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	path := c.Request.URL.Path
	if c.Request.Method == http.MethodGet {
		return path == "/v1/realtime" || path == "/v1/audio/speech/stream"
	}
	if c.Request.Method != http.MethodPost {
		return false
	}
	switch path {
	case "/v1/messages", "/v1/completions", "/v1/chat/completions", "/v1/responses", "/v1/responses/compact", "/v1/edits", "/v1/images/generations", "/v1/images/edits", "/v1/embeddings", "/v1/audio/transcriptions", "/v1/audio/translations", "/v1/audio/speech", "/v1/rerank", "/v1/moderations", "/v1/videos", "/v1/video/generations":
		return true
	}
	if strings.HasPrefix(path, "/v1/engines/") && strings.HasSuffix(path, "/embeddings") {
		return true
	}
	if strings.HasPrefix(path, "/v1/models/") || strings.HasPrefix(path, "/v1beta/models/") {
		for _, action := range []string{":generateContent", ":streamGenerateContent", ":embedContent", ":batchEmbedContents", ":predict", ":predictLongRunning"} {
			if strings.HasSuffix(path, action) {
				return true
			}
		}
	}
	if strings.HasPrefix(path, "/v1/videos/") && strings.HasSuffix(path, "/remix") {
		return true
	}
	if strings.HasPrefix(path, "/kling/v1/videos/") {
		return true
	}
	if strings.HasPrefix(path, "/jimeng/") {
		return c.Query("Action") == "CVSync2AsyncSubmitTask"
	}
	if strings.HasPrefix(path, "/suno/") || strings.HasPrefix(path, "/mj/") || strings.Contains(c.FullPath(), "/mj/") {
		return (strings.Contains(path, "/submit/") && !strings.HasSuffix(path, "/upload-discord-images")) || strings.HasSuffix(path, "/insight-face/swap")
	}
	return false
}

func MarkTokenQuotaIdentity(c *gin.Context, token *model.Token) {
	if token != nil && IsTokenQuotaInvocation(c) {
		// Copy only the identity/generation; never retain credentials for the observer.
		c.Set(tokenQuotaOutcomeTokenKey, model.Token{Id: token.Id, QuotaFailureVersion: token.QuotaFailureVersion})
	}
}

func ObserveTokenQuotaOutcome(c *gin.Context) {
	value, exists := c.Get(tokenQuotaOutcomeTokenKey)
	if !exists || !IsTokenQuotaInvocation(c) || c.GetBool("token_quota_outcome_observed") {
		return
	}
	c.Set("token_quota_outcome_observed", true)
	token, ok := value.(model.Token)
	if !ok {
		return
	}
	var rejected *types.QuotaRejection
	if !RequestSucceeded(c) {
		value, _ = c.Get(requestFailureKey)
		if apiErr, ok := value.(*types.NewAPIError); ok && apiErr.GetErrorType() == types.ErrorTypeNewAPIError {
			errors.As(apiErr, &rejected)
		}
		if rejected == nil {
			value, _ = c.Get("token_quota_auth_rejection")
			rejected, _ = value.(*types.QuotaRejection)
		}
	}
	insufficient := false
	if rejected != nil {
		switch rejected.Source {
		case "token", "token_package", BillingSourceWallet, BillingSourceSubscription:
			insufficient = true
		}
	}
	if err := model.RecordTokenQuotaOutcome(token.Id, token.QuotaFailureVersion, insufficient); err != nil {
		common.SysError("failed to record token quota outcome: " + err.Error())
	}
}
