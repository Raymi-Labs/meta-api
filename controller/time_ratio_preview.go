package controller

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func GetTimeRatioPreview(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	usingGroup := strings.TrimSpace(c.Query("group"))
	userGroup := strings.TrimSpace(c.Query("user_group"))
	now := time.Now()
	info := ratio_setting.ResolveTimeRatio(modelName, usingGroup, userGroup, now)
	noDiscount := ratio_setting.IsNoDiscountModel(modelName)
	if noDiscount {
		info = types.TimeRatioInfo{Ratio: 1}
	}

	c.JSON(200, gin.H{
		"success": true,
		"data": gin.H{
			"model":       modelName,
			"no_discount": noDiscount,
			"group":       usingGroup,
			"user_group":  userGroup,
			"ratio":       info.EffectiveRatio(),
			"matched":     info.Matched(),
			"rule_id":     info.RuleID,
			"timezone":    info.Timezone,
			"matched_at":  info.MatchedAt,
			"checked_at":  now.Format(time.RFC3339),
		},
	})
}
