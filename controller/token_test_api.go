package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type tokenTestRequest struct {
	Model string `json:"model"`
}

func GetTokenModels(c *gin.Context) {
	userId := c.GetInt("id")
	tokenID := strings.TrimSpace(c.Query("token_id"))
	requestedGroup := strings.TrimSpace(c.Query("group"))
	detail, _ := strconv.ParseBool(c.Query("detail"))
	fallback, _ := strconv.ParseBool(c.Query("auto_group_fallback"))

	var (
		models []string
		token  *model.Token
		err    error
	)

	if tokenID != "" {
		id := common.String2Int(tokenID)
		if id <= 0 {
			common.ApiError(c, fmt.Errorf("token_id 无效"))
			return
		}
		var tokenErr error
		token, tokenErr = model.GetTokenByIds(id, userId)
		if tokenErr != nil {
			common.ApiError(c, tokenErr)
			return
		}
		models, err = resolveTokenAllowedModels(userId, token)
	} else {
		models, err = resolveRequestedTokenModels(userId, requestedGroup, fallback)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if detail {
		modelOptions, optionErr := buildTokenModelOptions(userId, token, requestedGroup, fallback)
		if optionErr != nil {
			common.ApiError(c, optionErr)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    modelOptions,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    models,
	})
}

func TestToken(c *gin.Context) {
	userId := c.GetInt("id")
	id := common.String2Int(c.Param("id"))
	if id <= 0 {
		common.ApiError(c, fmt.Errorf("id 无效"))
		return
	}

	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if token.Status != common.TokenStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌已被禁用或已耗尽，无法测试",
			"time":    0.0,
		})
		return
	}
	if token.ExpiredTime != -1 && token.ExpiredTime <= common.GetTimestamp() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌已过期，无法测试",
			"time":    0.0,
		})
		return
	}

	req := tokenTestRequest{}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		common.ApiError(c, fmt.Errorf("model 不能为空"))
		return
	}
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !model.IsModelCallableByRole(req.Model, userCache.Role) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权调用此模型",
			"time":    0.0,
		})
		return
	}
	isAllowed := false
	matchName := ratio_setting.FormatMatchingModelName(req.Model)
	models, err := resolveTokenAllowedModelsRaw(userId, token)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, modelName := range models {
		if modelName == req.Model || ratio_setting.FormatMatchingModelName(modelName) == matchName {
			isAllowed = true
			break
		}
	}
	if !isAllowed {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该令牌当前分组不可测试此模型",
			"time":    0.0,
		})
		return
	}
	consumedTime, message, runErr := runTokenRelayTest(c, token, req.Model, "")
	if value, exists := c.Get(videoPreflightKind); exists {
		c.JSON(http.StatusOK, videoPreflightResponse(value.(*videoPreflight)))
		return
	}
	if runErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": message,
			"time":    consumedTime,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"time":    consumedTime,
	})
}
