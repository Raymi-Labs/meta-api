package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// TaskPayerAllowed preserves same-account access while separating personal and
// enterprise API tokens. Browser history remains a separate authenticated route.
func TaskPayerAllowed(c *gin.Context, task *model.Task) bool {
	if task == nil || task.UserId != c.GetInt("id") {
		return false
	}
	ent := common.GetContextKeyInt(c, constant.ContextKeyEnterpriseId)
	member := common.GetContextKeyInt(c, constant.ContextKeyEnterpriseMemberId)
	if ent == 0 {
		return task.EnterpriseId == 0
	}
	return task.EnterpriseId == ent && task.EnterpriseMemberId == member
}

// EnterpriseTaskChannel pins a stored credential to the endpoint that accepted
// the task. Editing a channel must not send the old secret to a new destination.
func EnterpriseTaskChannel(channel *model.Channel, task *model.Task) *model.Channel {
	if channel == nil || task == nil || task.EnterpriseId <= 0 || task.PrivateData.EnterpriseBaseURL == "" {
		return channel
	}
	copied := *channel
	endpoint := task.PrivateData.EnterpriseBaseURL
	copied.BaseURL = &endpoint
	return &copied
}
