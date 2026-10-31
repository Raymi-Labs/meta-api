package controller

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RootAuth protects this endpoint. Never serialize Token: even masked keys are
// unnecessary here; the configuration uses IDs only.
func GetSystemTranslationSettings(c *gin.Context) {
	type choice struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Group string `json:"group"`
	}
	var tokens []model.Token
	if err := model.DB.WithContext(c.Request.Context()).Where("user_id = ? AND model_limits_enabled = ? AND model_limits = ?", c.GetInt("id"), true, model.SystemTranslationModel).Order("id DESC").Find(&tokens).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	candidates := make([]choice, 0, len(tokens))
	for _, token := range tokens {
		if _, err := model.ValidateSystemTranslationToken(c.Request.Context(), token.Id, c.GetInt("id")); err == nil {
			candidates = append(candidates, choice{token.Id, token.Name, token.Group})
		}
	}
	id := model.SystemTranslationTokenID()
	state := "unconfigured"
	reason := ""
	if id > 0 {
		if _, err := model.ValidateSystemTranslationToken(c.Request.Context(), id, 0); err != nil {
			state = "invalid_token"
			reason = err.Error()
		} else {
			state = "ready"
		}
	}
	var worker model.ModelDescriptionWorker
	err := model.DB.WithContext(c.Request.Context()).First(&worker, model.ModelDescriptionWorkerID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return
	}
	if state == "ready" && worker.Failures > 0 && worker.NextRunAt > time.Now().Unix() {
		state = "cooldown"
	}
	if !common.GetEnvOrDefaultBool("MODEL_DESCRIPTION_TRANSLATION_ENABLED", true) {
		state = "environment_disabled"
	}
	common.ApiSuccess(c, gin.H{"model": model.SystemTranslationModel, "token_id": id, "tokens": candidates, "state": state, "reason": reason, "next_attempt_at": worker.NextRunAt, "failures": worker.Failures})
}
